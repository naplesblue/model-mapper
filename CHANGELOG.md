# Changelog

## [1.3] — 2026-05-08

### Added
- `vision_mappings` per upstream for routing image requests to image-capable models.
- Xiaomi MiMo vision model support via `mimo-v2-omni` on the Anthropic-compatible endpoint.
- Web UI fields for configuring text and image models separately per tier.
- Regression tests for image request detection, vision routing, and missing vision-model errors.

### Changed
- Requests containing Anthropic image blocks now use `vision_mappings` instead of text `mappings`.
- Sample config and README now document text versus image model mappings.

### Fixed
- Image requests no longer get sent to text-only upstream models such as `mimo-v2.5-pro`.
- Upstreams without vision mappings now return a clear configuration error for image requests instead of forwarding to an unsupported model.

---

## [1.2] — 2026-05-08

### Added
- Explicit `model_routes` config for choosing which upstream handles each client-facing model.
- Web UI model routing table now preserves each upstream's full model catalog while allowing per-tier mixed routing.
- Regression test covering mixed routing when multiple upstreams define the same client model names.
- `MODEL_MAPPER_CONFIG` environment variable for selecting a config file path.

### Changed
- Request dispatch now resolves upstream selection from `model_routes` first, then falls back to `default_upstream`.
- Documentation and sample config now describe the `model_routes` + `upstreams[].mappings[]` configuration model.

### Fixed
- New processes started from the project directory now read the local `config.json` instead of only checking the executable directory.
- Web UI add-upstream flow now works after fixing a JavaScript parse error.
- Mixed upstream routing no longer collapses all Opus / Sonnet / Haiku requests onto the global default upstream.

---

## [1.11] — 2026-05-08

### Added
- Multi-upstream routing: per-model dispatch to different providers (DeepSeek, XiaoMi MiMo, etc.)
- 3-tier routing table (Opus / Sonnet / Haiku) with upstream selector dropdown
- Per-upstream model catalog: each upstream defines its own tier model names
- Auto protocol detection: passthrough or Anthropic→OpenAI conversion based on upstream type
- `/healthz` endpoint for health checks
- Graceful shutdown on SIGTERM (30s drain timeout)
- HTTP client connection timeouts (Dial + TLS handshake)

### Changed
- Config format: single `upstream_url`/`upstream_token` → `upstreams[]` array with per-upstream `mappings[]`
- Web UI: full redesign with routing table + upstream config cards
- Old config.json auto-migrated on startup (backward compatible)
- `Mode` field removed; conversion auto-detected from upstream protocol
- Nginx example: `$host` → `$http_host` for correct port handling

### Fixed
- `sameOrigin` check now ignores port mismatch from reverse proxies
- Upstream model definitions persisted even when no route points to them
- `saveConfig()` no longer silently drops marshal errors

---

## [1.0.0] — 2026-05-07

### Added
- Initial release
- Single-upstream proxy with model name remapping
- Dual mode: passthrough and Anthropic→OpenAI protocol conversion
- Embedded Web UI for config management
- SSE streaming support
- systemd service + nginx config for x86 NUC deployment
