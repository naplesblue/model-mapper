package main

import (
	"encoding/json"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sync"
)

// ModelMapping pairs a client-facing model name with the upstream model name.
type ModelMapping struct {
	ClientModel   string `json:"client_model"`
	UpstreamModel string `json:"upstream_model"`
}

// UpstreamConfig defines a single upstream LLM provider.
type UpstreamConfig struct {
	Name     string         `json:"name"`      // "deepseek", "anthropic-direct"
	URL      string         `json:"url"`       // base URL, e.g. "https://api.deepseek.com"
	Token    string         `json:"token"`     // API key
	AuthType string         `json:"auth_type"` // "anthropic" (x-api-key) or "openai" (Bearer)
	Protocol string         `json:"protocol"`  // upstream's native protocol: "anthropic" | "openai"
	Mappings []ModelMapping `json:"mappings"`  // client model → upstream model (first match wins)
}

// Config 是代理服务的全部配置，持久化到 config.json。
type Config struct {
	BindHost  string           `json:"bind_host"`
	Port      int              `json:"port"`
	Upstreams []UpstreamConfig `json:"upstreams"`

	// Legacy fields kept for backward-compat parsing during migration.
	UpstreamURL   string `json:"upstream_url,omitempty"`
	UpstreamToken string `json:"upstream_token,omitempty"`
	Protocol      string `json:"protocol,omitempty"`
	Mode          string `json:"mode,omitempty"`
}

func defaultConfig() Config {
	return Config{
		BindHost: "127.0.0.1",
		Port:     9483,
		Upstreams: []UpstreamConfig{
			{
				Name:     "deepseek-anthropic",
				URL:      "https://api.deepseek.com/anthropic",
				Token:    "",
				AuthType: "anthropic",
				Protocol: "anthropic",
				Mappings: []ModelMapping{
					{ClientModel: "claude-opus-4-6", UpstreamModel: "deepseek-v4-pro"},
					{ClientModel: "claude-sonnet-4-6", UpstreamModel: "deepseek-v4-flash"},
					{ClientModel: "claude-3-5-sonnet-20241022", UpstreamModel: "deepseek-v4-flash"},
					{ClientModel: "claude-3-5-haiku-20241022", UpstreamModel: "deepseek-v4-flash"},
					{ClientModel: "claude-3-haiku-20240307", UpstreamModel: "deepseek-v4-flash"},
				},
			},
		},
	}
}

var (
	cfg     Config
	cfgMu   sync.RWMutex
	cfgPath string
)

func configFilePath() string {
	exe, err := os.Executable()
	if err != nil {
		return "config.json"
	}
	return filepath.Join(filepath.Dir(exe), "config.json")
}

func loadConfig() {
	cfgPath = configFilePath()
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		cfg = defaultConfig()
		saveConfig()
		return
	}

	// Try unmarshal into current format
	c := defaultConfig()
	if err := json.Unmarshal(data, &c); err != nil {
		log.Printf("config parse error, using defaults: %v", err)
		cfg = defaultConfig()
		return
	}

	// Migration 1: original single-upstream format (upstream_url + model_map)
	if len(c.Upstreams) == 0 && c.UpstreamURL != "" {
		var old struct {
			UpstreamURL   string            `json:"upstream_url"`
			UpstreamToken string            `json:"upstream_token"`
			Protocol      string            `json:"protocol"`
			ModelMap      map[string]string `json:"model_map"`
		}
		json.Unmarshal(data, &old)
		mappings := make([]ModelMapping, 0, len(old.ModelMap))
		for clientModel, upstreamModel := range old.ModelMap {
			mappings = append(mappings, ModelMapping{ClientModel: clientModel, UpstreamModel: upstreamModel})
		}
		c.Upstreams = []UpstreamConfig{{
			Name:     "default",
			URL:      old.UpstreamURL,
			Token:    old.UpstreamToken,
			AuthType: old.Protocol,
			Protocol: old.Protocol,
			Mappings: mappings,
		}}
		c.UpstreamURL = ""
		c.UpstreamToken = ""
		c.Protocol = ""
		c.Mode = ""
		log.Printf("[config] migrated original single-upstream config")
		cfg = c
		saveConfig()
		return
	}

	// Migration 2: intermediate format (Models[] + global model_map) → Mappings[]
	if needsModelMigration(data) {
		c = migrateModelsToMappings(data, c)
		log.Printf("[config] migrated Models[]+model_map to per-upstream Mappings[]")
		cfg = c
		saveConfig()
		return
	}

	cfg = c
}

// needsModelMigration checks if the raw JSON contains old-style Models[] or model_map fields.
func needsModelMigration(data []byte) bool {
	var raw struct {
		ModelMap  map[string]string `json:"model_map"`
		Upstreams []struct {
			Models []string `json:"models"`
		} `json:"upstreams"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return false
	}
	if len(raw.ModelMap) > 0 {
		return true
	}
	for _, u := range raw.Upstreams {
		if len(u.Models) > 0 {
			return true
		}
	}
	return false
}

// migrateModelsToMappings converts upstreams with Models[]+global model_map into Mappings[].
func migrateModelsToMappings(data []byte, c Config) Config {
	var raw struct {
		ModelMap  map[string]string `json:"model_map"`
		Upstreams []struct {
			Models   []string       `json:"models"`
			Mappings []ModelMapping `json:"mappings"`
		} `json:"upstreams"`
	}
	json.Unmarshal(data, &raw)

	for i := range c.Upstreams {
		if len(c.Upstreams[i].Mappings) > 0 {
			continue // already has mappings
		}
		models := []string{}
		if i < len(raw.Upstreams) {
			models = raw.Upstreams[i].Models
		}
		mappings := make([]ModelMapping, 0, len(models))
		for _, clientModel := range models {
			upstreamModel := clientModel
			if mapped, ok := raw.ModelMap[clientModel]; ok {
				upstreamModel = mapped
			}
			mappings = append(mappings, ModelMapping{ClientModel: clientModel, UpstreamModel: upstreamModel})
		}
		c.Upstreams[i].Mappings = mappings
	}
	return c
}

func saveConfig() {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		log.Printf("save config failed: marshal error: %v", err)
		return
	}
	if err := os.WriteFile(cfgPath, data, 0600); err != nil {
		log.Printf("save config failed: %v", err)
	}
}

// sameOrigin 校验 Origin/Referer 是否与 Host 一致，防止 CSRF。
// 本机 curl 无 Origin/Referer 头时放行。
func sameOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		origin = r.Header.Get("Referer")
	}
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	return u.Host == r.Host
}

func handleGetConfig(w http.ResponseWriter, _ *http.Request) {
	cfgMu.RLock()
	data, err := json.Marshal(cfg)
	cfgMu.RUnlock()
	if err != nil {
		http.Error(w, "marshal config failed", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(data)
}

func handlePostConfig(w http.ResponseWriter, r *http.Request) {
	if !sameOrigin(r) {
		http.Error(w, "origin mismatch", http.StatusForbidden)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 32<<10) // 32KB limit

	body, err := readBody(r)
	if err != nil {
		http.Error(w, "read body failed", http.StatusBadRequest)
		return
	}

	cfgMu.Lock()
	defer cfgMu.Unlock()

	// Patch 语义：先复制现有配置，再 unmarshal 覆盖，避免漏字段被清零
	patched := cfg
	if err := json.Unmarshal(body, &patched); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	cfg = patched
	saveConfig()

	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"ok":true}`))
}
