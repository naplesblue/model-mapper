[中文](./README.md) | **English**

# Model Mapper

A lightweight reverse proxy that intercepts LLM API requests, remaps model names, and forwards them to any upstream provider. Built for scenarios where your client (e.g. Claude Code, Claude Desktop) speaks one API format but your upstream uses different model names — or even a different protocol entirely.

## Features

- **Model routing table** — Route Opus / Sonnet / Haiku to different upstreams, so providers like DeepSeek and MiMo can be mixed per model tier.
- **Per-upstream model catalog** — Each upstream keeps its own model mappings; the routing table only selects which upstream handles the request.
- **Protocol-aware dispatch** — Anthropic-compatible upstreams are passed through directly; OpenAI-compatible upstreams use automatic Anthropic ↔ OpenAI conversion with streaming SSE and tool use.
- **Web UI** — Configure everything from the browser at `http://localhost:9483`.
- **Security** — Binds to `127.0.0.1` by default, Origin/Referer CSRF protection, `config.json` saved with `0600` permissions. Set `MODEL_MAPPER_CONFIG` to override the config path.
- **Single binary** — No dependencies. Cross-compiles to macOS arm64 and Linux amd64.

## Quick Start

Download the latest binary from [Releases](https://github.com/naplesblue/model-mapper/releases), or build from source:

```bash
# Build
make build

# First run — generates config.json with defaults
./model-mapper

# Open http://127.0.0.1:9483 in your browser to configure
```

Edit `config.json` (or use the Web UI) to set your upstream token:

```json
{
  "bind_host": "127.0.0.1",
  "port": 9483,
  "default_upstream": 0,
  "model_routes": [
    {"client_model": "claude-opus-4-6", "upstream": 0},
    {"client_model": "claude-sonnet-4-6", "upstream": 0},
    {"client_model": "claude-haiku-4-5", "upstream": 1}
  ],
  "upstreams": [
    {
      "name": "deepseek-anthropic",
      "url": "https://api.deepseek.com/anthropic",
      "token": "sk-your-deepseek-key",
      "auth_type": "anthropic",
      "protocol": "anthropic",
      "mappings": [
        {"client_model": "claude-opus-4-6", "upstream_model": "deepseek-v4-pro"},
        {"client_model": "claude-sonnet-4-6", "upstream_model": "deepseek-v4-flash"},
        {"client_model": "claude-haiku-4-5", "upstream_model": "deepseek-v4-flash"}
      ]
    },
    {
      "name": "xiaomi-mimo",
      "url": "https://api.xiaomimimo.com/anthropic",
      "token": "tp-your-mimo-key",
      "auth_type": "anthropic",
      "protocol": "anthropic",
      "mappings": [
        {"client_model": "claude-opus-4-6", "upstream_model": "mimo-2.5-pro"},
        {"client_model": "claude-sonnet-4-6", "upstream_model": "mimo-2.5"},
        {"client_model": "claude-haiku-4-5", "upstream_model": "mimo-2.5-flash"}
      ]
    }
  ]
}
```

Then point your client at the proxy:

```bash
# Claude Code
ANTHROPIC_BASE_URL=http://127.0.0.1:9483 ANTHROPIC_API_KEY=any claude

# Claude Desktop — set Base URL to http://127.0.0.1:9483
```

## Configuration Model

| Field | Purpose |
|---|---|
| `model_routes` | Client model → upstream index. Use this for mixed routing, such as Opus on DeepSeek and Haiku on MiMo. |
| `default_upstream` | Fallback upstream when no `model_routes` entry matches. |
| `upstreams[].mappings` | The model catalog for that upstream. Multiple upstreams can keep the same client model names and map them to different upstream model names. |
| `upstreams[].protocol` | `anthropic` is passed through directly; `openai` enables Anthropic ↔ OpenAI conversion. |

## Deploy to Server (Linux)

```bash
# Cross-compile (or download from Releases)
make build-linux

# Copy to server
scp dist/model-mapper-linux-amd64 deploy/* user@server:/tmp/

# On the server
sudo useradd -r -s /usr/sbin/nologin model-mapper   # create service user
sudo mkdir -p /opt/model-mapper
sudo cp /tmp/model-mapper-linux-amd64 /opt/model-mapper/model-mapper
sudo cp /tmp/config.example.json /opt/model-mapper/config.json
sudo chmod +x /opt/model-mapper/model-mapper
sudo chown -R model-mapper:model-mapper /opt/model-mapper

# Edit config.json with your API key
sudo vi /opt/model-mapper/config.json

# Install systemd service
sudo cp /tmp/model-mapper.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now model-mapper

# Optional: nginx reverse proxy for LAN access
sudo cp /tmp/nginx.conf.example /etc/nginx/sites-available/model-mapper
sudo ln -s /etc/nginx/sites-available/model-mapper /etc/nginx/sites-enabled/
sudo nginx -t && sudo systemctl reload nginx
```

## Build from Source

```bash
make build          # native binary → ./model-mapper
make build-darwin   # macOS arm64   → dist/model-mapper-darwin-arm64
make build-linux    # Linux amd64   → dist/model-mapper-linux-amd64
make build-all      # both platforms
make clean          # remove build artifacts
```

## Project Structure

```
├── main.go                 # Entry point, routing, embed
├── config.go               # Config struct, load/save, CSRF protection
├── proxy.go                # Passthrough proxy, model name replacement
├── convert.go              # Anthropic ↔ OpenAI protocol conversion engine
├── web/
│   └── index.html          # Web UI (embedded into binary)
├── deploy/
│   ├── config.example.json # Config template
│   ├── nginx.conf.example  # Nginx reverse proxy config
│   └── model-mapper.service # Systemd unit file
├── Makefile
└── go.mod
```

## License

MIT
