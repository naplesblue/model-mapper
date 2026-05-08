package main

import "testing"

func TestFindUpstreamUsesModelRoutesBeforeDefault(t *testing.T) {
	cfgMu.Lock()
	original := cfg
	cfg = Config{
		DefaultUpstream: 0,
		ModelRoutes: []ModelRoute{
			{ClientModel: "claude-opus-4-6", Upstream: 1},
			{ClientModel: "claude-sonnet-4-6", Upstream: 0},
		},
		Upstreams: []UpstreamConfig{
			{
				Name: "deepseek",
				Mappings: []ModelMapping{
					{ClientModel: "claude-opus-4-6", UpstreamModel: "deepseek-opus"},
					{ClientModel: "claude-sonnet-4-6", UpstreamModel: "deepseek-sonnet"},
				},
			},
			{
				Name: "mimo",
				Mappings: []ModelMapping{
					{ClientModel: "claude-opus-4-6", UpstreamModel: "mimo-opus"},
					{ClientModel: "claude-sonnet-4-6", UpstreamModel: "mimo-sonnet"},
				},
			},
		},
	}
	cfgMu.Unlock()
	defer func() {
		cfgMu.Lock()
		cfg = original
		cfgMu.Unlock()
	}()

	up, target, routeErr := findUpstream("claude-opus-4-6", false)
	if routeErr != "" {
		t.Fatalf("unexpected route error: %s", routeErr)
	}
	if up == nil || up.Name != "mimo" || target != "mimo-opus" {
		t.Fatalf("opus route = (%v, %q), want (mimo, mimo-opus)", upstreamName(up), target)
	}

	up, target, routeErr = findUpstream("claude-sonnet-4-6", false)
	if routeErr != "" {
		t.Fatalf("unexpected route error: %s", routeErr)
	}
	if up == nil || up.Name != "deepseek" || target != "deepseek-sonnet" {
		t.Fatalf("sonnet route = (%v, %q), want (deepseek, deepseek-sonnet)", upstreamName(up), target)
	}
}

func upstreamName(up *UpstreamConfig) string {
	if up == nil {
		return "<nil>"
	}
	return up.Name
}

func TestFindUpstreamUsesVisionMappingForImageRequests(t *testing.T) {
	cfgMu.Lock()
	original := cfg
	cfg = Config{
		DefaultUpstream: 0,
		ModelRoutes:     []ModelRoute{{ClientModel: "claude-opus-4-6", Upstream: 0}},
		Upstreams: []UpstreamConfig{{
			Name: "mimo",
			Mappings: []ModelMapping{
				{ClientModel: "claude-opus-4-6", UpstreamModel: "mimo-v2.5-pro"},
			},
			VisionMappings: []ModelMapping{
				{ClientModel: "claude-opus-4-6", UpstreamModel: "mimo-v2-omni"},
			},
		}},
	}
	cfgMu.Unlock()
	defer func() {
		cfgMu.Lock()
		cfg = original
		cfgMu.Unlock()
	}()

	up, target, routeErr := findUpstream("claude-opus-4-6", true)
	if routeErr != "" {
		t.Fatalf("unexpected route error: %s", routeErr)
	}
	if up == nil || up.Name != "mimo" || target != "mimo-v2-omni" {
		t.Fatalf("vision route = (%v, %q), want (mimo, mimo-v2-omni)", upstreamName(up), target)
	}
}

func TestFindUpstreamErrorsWhenVisionMappingMissing(t *testing.T) {
	cfgMu.Lock()
	original := cfg
	cfg = Config{
		DefaultUpstream: 0,
		ModelRoutes:     []ModelRoute{{ClientModel: "claude-opus-4-6", Upstream: 0}},
		Upstreams: []UpstreamConfig{{
			Name: "deepseek",
			Mappings: []ModelMapping{
				{ClientModel: "claude-opus-4-6", UpstreamModel: "deepseek-v4-pro"},
			},
		}},
	}
	cfgMu.Unlock()
	defer func() {
		cfgMu.Lock()
		cfg = original
		cfgMu.Unlock()
	}()

	up, target, routeErr := findUpstream("claude-opus-4-6", true)
	if up == nil || up.Name != "deepseek" || target != "" {
		t.Fatalf("vision route without mapping = (%v, %q), want (deepseek, empty)", upstreamName(up), target)
	}
	if routeErr != "upstream deepseek has no vision model configured for claude-opus-4-6" {
		t.Fatalf("routeErr = %q", routeErr)
	}
}

func TestRequestHasImage(t *testing.T) {
	body := []byte(`{"messages":[{"role":"user","content":[{"type":"image","source":{"type":"base64","media_type":"image/png","data":"abc"}},{"type":"text","text":"describe"}]}]}`)
	if !requestHasImage(body) {
		t.Fatal("requestHasImage = false, want true")
	}
}
