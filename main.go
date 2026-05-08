package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

//go:embed web/index.html
var indexHTML []byte

func main() {
	loadConfig()

	mux := http.NewServeMux()

	// Web UI
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(indexHTML)
	})

	// Config API
	mux.HandleFunc("GET /api/config", handleGetConfig)
	mux.HandleFunc("POST /api/config", handlePostConfig)

	// Health check
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("ok"))
	})

	// Anthropic Messages API — unified model-based dispatch
	mux.HandleFunc("POST /v1/messages", handleMessages)

	// Token counting stub
	mux.HandleFunc("POST /v1/messages/count_tokens", handleCountTokensStub)

	// Fallback: passthrough proxy for all other paths
	mux.HandleFunc("/", proxyHandler)

	cfgMu.RLock()
	addr := fmt.Sprintf("%s:%d", cfg.BindHost, cfg.Port)
	upNames := make([]string, len(cfg.Upstreams))
	for i, u := range cfg.Upstreams {
		upNames[i] = u.Name
	}
	cfgMu.RUnlock()

	log.Printf("model-mapper listening on %s  upstreams=%s", addr, strings.Join(upNames, ", "))

	srv := &http.Server{Addr: addr, Handler: mux}

	// Graceful shutdown on SIGINT / SIGTERM
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		sig := <-sigCh
		log.Printf("received %v, shutting down gracefully...", sig)
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil {
			log.Printf("shutdown error: %v", err)
		}
	}()

	if err := srv.ListenAndServe(); err != http.ErrServerClosed {
		log.Fatalf("server error: %v", err)
	}
	log.Printf("server stopped")
}

// handleMessages is the unified dispatch handler for POST /v1/messages.
// It extracts the model from the request, finds the matching upstream,
// and dispatches to passthrough or protocol conversion as needed.
func handleMessages(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 64<<20)
	body, err := readBody(r)
	if err != nil {
		http.Error(w, "read body failed", http.StatusBadRequest)
		return
	}

	// Extract model for routing
	var peek struct {
		Model string `json:"model"`
	}
	if err := json.Unmarshal(body, &peek); err != nil || peek.Model == "" {
		http.Error(w, "model field required", http.StatusBadRequest)
		return
	}
	clientModel := peek.Model

	up, targetModel := findUpstream(clientModel)
	if up == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"type": "error",
			"error": map[string]interface{}{
				"type":    "not_found",
				"message": fmt.Sprintf("no upstream configured for model: %s", clientModel),
			},
		})
		return
	}

	if targetModel != clientModel {
		log.Printf("[dispatch] model=%s -> %s  upstream=%s protocol=%s", clientModel, targetModel, up.Name, up.Protocol)
	} else {
		log.Printf("[dispatch] model=%s  upstream=%s protocol=%s", clientModel, up.Name, up.Protocol)
	}

	// Client always sends Anthropic format (this is /v1/messages)
	if up.Protocol == "anthropic" {
		handlePassthrough(w, r, body, up, clientModel, targetModel)
	} else {
		handleAnthropicToOpenAI(w, r, body, up, targetModel, clientModel)
	}
}
