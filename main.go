package main

import (
	_ "embed"
	"fmt"
	"log"
	"net/http"
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

	// Anthropic Messages API — route by mode
	mux.HandleFunc("POST /v1/messages", func(w http.ResponseWriter, r *http.Request) {
		cfgMu.RLock()
		mode := cfg.Mode
		cfgMu.RUnlock()

		if mode == "anthropic-to-openai" {
			handleMessagesConvert(w, r)
		} else {
			proxyHandler(w, r)
		}
	})

	// Token counting stub
	mux.HandleFunc("POST /v1/messages/count_tokens", handleCountTokensStub)

	// Fallback: passthrough proxy for all other paths
	mux.HandleFunc("/", proxyHandler)

	cfgMu.RLock()
	addr := fmt.Sprintf("%s:%d", cfg.BindHost, cfg.Port)
	cfgMu.RUnlock()

	log.Printf("model-mapper listening on %s  upstream=%s  mode=%s  model_map=%v",
		addr, cfg.UpstreamURL, cfg.Mode, cfg.ModelMap)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
