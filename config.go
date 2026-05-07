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

// Config 是代理服务的全部配置，持久化到 config.json。
type Config struct {
	BindHost      string            `json:"bind_host"`
	Port          int               `json:"port"`
	UpstreamURL   string            `json:"upstream_url"`
	UpstreamToken string            `json:"upstream_token"`
	Protocol      string            `json:"protocol"`       // "anthropic" | "openai"
	Mode          string            `json:"mode"`            // "anthropic-to-openai" | "passthrough"
	ModelMap      map[string]string `json:"model_map"`       // 客户端模型名 → 上游模型名
}

func defaultConfig() Config {
	return Config{
		BindHost:      "127.0.0.1",
		Port:          9483,
		UpstreamURL:   "https://api.deepseek.com/anthropic",
		UpstreamToken: "",
		Protocol:      "anthropic",
		Mode:          "passthrough",
		ModelMap: map[string]string{
			"claude-opus-4-6":           "deepseek-v4-pro",
			"claude-sonnet-4-6":         "deepseek-v4-flash",
			"claude-3-5-sonnet-20241022": "deepseek-v4-flash",
			"claude-3-5-haiku-20241022":  "deepseek-v4-flash",
			"claude-3-haiku-20240307":    "deepseek-v4-flash",
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
	c := defaultConfig()
	if err := json.Unmarshal(data, &c); err != nil {
		log.Printf("config parse error, using defaults: %v", err)
		cfg = defaultConfig()
		return
	}
	cfg = c
}

func saveConfig() {
	data, _ := json.MarshalIndent(cfg, "", "  ")
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
	data, _ := json.Marshal(cfg)
	cfgMu.RUnlock()
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
