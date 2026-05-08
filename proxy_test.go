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

	up, target := findUpstream("claude-opus-4-6")
	if up == nil || up.Name != "mimo" || target != "mimo-opus" {
		t.Fatalf("opus route = (%v, %q), want (mimo, mimo-opus)", upstreamName(up), target)
	}

	up, target = findUpstream("claude-sonnet-4-6")
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
