package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"time"
)

// findUpstream returns the configured upstream and target upstream model name
// for the given client model. ModelRoutes decide which upstream handles a
// client model; the selected upstream's own Mappings decide the target model.
// If no explicit route exists, default_upstream is used as fallback, then the
// remaining upstreams are searched for backward compatibility.
func findUpstream(model string) (*UpstreamConfig, string) {
	cfgMu.RLock()
	defer cfgMu.RUnlock()

	for _, route := range cfg.ModelRoutes {
		if !strings.EqualFold(model, route.ClientModel) {
			continue
		}
		if route.Upstream < 0 || route.Upstream >= len(cfg.Upstreams) {
			return nil, ""
		}
		up := &cfg.Upstreams[route.Upstream]
		if targetModel, ok := findMapping(up, model); ok {
			return up, targetModel
		}
		return up, model
	}

	if cfg.DefaultUpstream >= 0 && cfg.DefaultUpstream < len(cfg.Upstreams) {
		up := &cfg.Upstreams[cfg.DefaultUpstream]
		if targetModel, ok := findMapping(up, model); ok {
			return up, targetModel
		}
	}

	for i := range cfg.Upstreams {
		if i == cfg.DefaultUpstream {
			continue
		}
		if targetModel, ok := findMapping(&cfg.Upstreams[i], model); ok {
			return &cfg.Upstreams[i], targetModel
		}
	}
	return nil, ""
}

func findMapping(up *UpstreamConfig, model string) (string, bool) {
	for _, m := range up.Mappings {
		if strings.EqualFold(model, m.ClientModel) {
			return m.UpstreamModel, true
		}
	}
	return "", false
}

// setAuthHeaders sets authentication headers on the outgoing request based on
// the upstream's AuthType.
func setAuthHeaders(req *http.Request, up *UpstreamConfig) {
	switch strings.ToLower(up.AuthType) {
	case "openai":
		req.Header.Set("Authorization", "Bearer "+up.Token)
	default: // anthropic
		req.Header.Set("x-api-key", up.Token)
		req.Header.Set("anthropic-version", "2023-06-01")
	}
}

// replaceModelInBody replaces the "model" field in the JSON body and returns
// the modified body. If the body cannot be parsed, it is returned as-is.
func replaceModelInBody(body []byte, newModel string) []byte {
	if len(body) == 0 || newModel == "" {
		return body
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return body
	}
	modelVal, ok := raw["model"]
	if !ok {
		return body
	}
	var modelStr string
	if err := json.Unmarshal(modelVal, &modelStr); err != nil {
		return body
	}
	// Only replace if model changed
	if strings.EqualFold(modelStr, newModel) {
		return body
	}
	raw["model"], _ = json.Marshal(newModel)
	newBody, err := json.Marshal(raw)
	if err != nil {
		return body
	}
	return newBody
}

var hopHeaders = map[string]bool{
	"Connection":          true,
	"Keep-Alive":          true,
	"Proxy-Authenticate":  true,
	"Proxy-Authorization": true,
	"Te":                  true,
	"Trailers":            true,
	"Transfer-Encoding":   true,
	"Upgrade":             true,
}

func copyHeaders(dst, src http.Header, skip ...string) {
	skipSet := make(map[string]bool, len(skip))
	for _, s := range skip {
		skipSet[http.CanonicalHeaderKey(s)] = true
	}
	for k, vv := range src {
		if hopHeaders[k] || skipSet[k] {
			continue
		}
		for _, v := range vv {
			dst.Add(k, v)
		}
	}
}

var httpClient = &http.Client{
	Transport: &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           (&net.Dialer{Timeout: 10 * time.Second}).DialContext,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 60 * time.Second,
		IdleConnTimeout:       90 * time.Second,
	},
	// 不设置 Timeout，让流式响应自由传输
}

// readBody 从 r.Body 读取全部内容并关闭。
func readBody(r *http.Request) ([]byte, error) {
	defer r.Body.Close()
	return io.ReadAll(r.Body)
}

// handlePassthrough forwards the request body to the upstream without protocol
// conversion. It performs model name replacement and auth header injection.
func handlePassthrough(w http.ResponseWriter, r *http.Request, body []byte, up *UpstreamConfig, clientModel, targetModel string) {
	// Replace model in body if target differs
	if targetModel != clientModel {
		newBody := replaceModelInBody(body, targetModel)
		if !bytes.Equal(newBody, body) {
			log.Printf("[passthrough] model replaced: %s -> %s  upstream=%s", clientModel, targetModel, up.Name)
			body = newBody
		}
	}

	upstreamURL := strings.TrimRight(up.URL, "/") + r.URL.RequestURI()

	req, err := http.NewRequestWithContext(r.Context(), r.Method, upstreamURL, bytes.NewReader(body))
	if err != nil {
		http.Error(w, "build upstream request failed", http.StatusInternalServerError)
		return
	}

	// Copy client headers (strip Host and auth-related headers)
	copyHeaders(req.Header, r.Header, "Host", "Authorization", "X-Api-Key", "Anthropic-Version")

	// Set upstream auth
	setAuthHeaders(req, up)

	if req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept-Encoding", "identity")

	resp, err := httpClient.Do(req)
	if err != nil {
		log.Printf("[passthrough] upstream=%s error: %v", up.Name, err)
		http.Error(w, fmt.Sprintf("upstream error: %v", err), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// Copy response headers
	copyHeaders(w.Header(), resp.Header)

	isSSE := strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream")
	if isSSE {
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no")
	}

	w.WriteHeader(resp.StatusCode)

	if isSSE {
		flusher, ok := w.(http.Flusher)
		buf := make([]byte, 32*1024)
		for {
			n, readErr := resp.Body.Read(buf)
			if n > 0 {
				if _, writeErr := w.Write(buf[:n]); writeErr != nil {
					break
				}
				if ok {
					flusher.Flush()
				}
			}
			if readErr != nil {
				break
			}
		}
		return
	}

	if _, err := io.Copy(w, resp.Body); err != nil {
		log.Printf("[passthrough] copy response failed: %v", err)
	}
}

// proxyHandler is the catch-all fallback for non-/v1/messages paths.
// It uses the first configured upstream as default.
func proxyHandler(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 64<<20)
	body, err := readBody(r)
	if err != nil {
		http.Error(w, "read body failed", http.StatusBadRequest)
		return
	}

	// Try to route by model; fall back to first upstream
	var up *UpstreamConfig
	var clientModel, targetModel string

	if len(body) > 0 {
		var peek struct {
			Model string `json:"model"`
		}
		if json.Unmarshal(body, &peek) == nil && peek.Model != "" {
			clientModel = peek.Model
			up, targetModel = findUpstream(clientModel)
		}
	}

	if up == nil {
		// Default: use default_upstream if valid, otherwise first upstream
		cfgMu.RLock()
		if cfg.DefaultUpstream >= 0 && cfg.DefaultUpstream < len(cfg.Upstreams) {
			up = &cfg.Upstreams[cfg.DefaultUpstream]
		} else if len(cfg.Upstreams) > 0 {
			up = &cfg.Upstreams[0]
		}
		cfgMu.RUnlock()
	}

	if up == nil {
		http.Error(w, "no upstream configured", http.StatusBadGateway)
		return
	}

	if targetModel == "" {
		targetModel = clientModel
	}

	handlePassthrough(w, r, body, up, clientModel, targetModel)
}
