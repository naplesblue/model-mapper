package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

// replaceModel 检查请求体中的 model 字段，若命中映射表则替换。
// 返回 (新 body, 客户端原始 model 名, 替换后的目标 model 名)。
func replaceModel(body []byte, c *Config) ([]byte, string, string) {
	if len(body) == 0 {
		return body, "", ""
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return body, "", ""
	}
	modelVal, ok := raw["model"]
	if !ok {
		return body, "", ""
	}
	var modelStr string
	if err := json.Unmarshal(modelVal, &modelStr); err != nil {
		return body, "", ""
	}
	for src, dst := range c.ModelMap {
		if strings.EqualFold(modelStr, src) {
			raw["model"], _ = json.Marshal(dst)
			newBody, _ := json.Marshal(raw)
			return newBody, modelStr, dst
		}
	}
	return body, "", ""
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

var client = &http.Client{
	Transport: &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
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

func proxyHandler(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 64<<20) // 64MB limit
	body, err := readBody(r)
	if err != nil {
		http.Error(w, "read body failed", http.StatusBadRequest)
		return
	}

	cfgMu.RLock()
	c := cfg
	cfgMu.RUnlock()

	newBody, originalModel, targetModel := replaceModel(body, &c)
	if originalModel != "" {
		log.Printf("[proxy] model replaced: %s -> %s  path=%s", originalModel, targetModel, r.URL.Path)
	}

	upstreamBase := strings.TrimRight(c.UpstreamURL, "/")
	upstreamURL := upstreamBase + r.URL.RequestURI()

	req, err := http.NewRequestWithContext(r.Context(), r.Method, upstreamURL, bytes.NewReader(newBody))
	if err != nil {
		http.Error(w, "build upstream request failed", http.StatusInternalServerError)
		return
	}

	// 透传客户端请求头（过滤 Host 和逐跳头）
	copyHeaders(req.Header, r.Header, "Host", "Authorization", "X-Api-Key", "Anthropic-Version")

	// 按协议设置鉴权头
	switch strings.ToLower(c.Protocol) {
	case "openai":
		req.Header.Set("Authorization", "Bearer "+c.UpstreamToken)
	default: // anthropic
		req.Header.Set("x-api-key", c.UpstreamToken)
		req.Header.Set("anthropic-version", "2023-06-01")
	}

	if req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept-Encoding", "identity")

	resp, err := client.Do(req)
	if err != nil {
		log.Printf("[proxy] upstream error: %v", err)
		http.Error(w, fmt.Sprintf("upstream error: %v", err), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// 透传响应头
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
		log.Printf("[proxy] copy response failed: %v", err)
	}
}
