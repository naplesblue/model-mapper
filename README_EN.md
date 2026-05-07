[中文](./README.md) | **English**

# Model Mapper

A lightweight reverse proxy that intercepts LLM API requests, remaps model names, and forwards them to any upstream provider. Built for scenarios where your client (e.g. Claude Code, Claude Desktop) speaks one API format but your upstream uses different model names — or even a different protocol entirely.

## Features

- **Model name mapping** — Route `claude-opus-4-6` → `deepseek-v4-pro`, `claude-sonnet-4-6` → `deepseek-v4-flash`, etc. Each model maps independently.
- **Dual mode** — `passthrough` for Anthropic-compatible upstreams, `anthropic-to-openai` for OpenAI-compatible upstreams (full protocol conversion with streaming SSE, tool use, and tool results).
- **Web UI** — Configure everything from the browser at `http://localhost:9483`.
- **Security** — Binds to `127.0.0.1` by default, Origin/Referer CSRF protection, `config.json` saved with `0600` permissions.
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
  "upstream_url": "https://api.deepseek.com/anthropic",
  "upstream_token": "sk-your-key-here",
  "protocol": "anthropic",
  "mode": "passthrough",
  "model_map": {
    "claude-opus-4-6": "deepseek-v4-pro",
    "claude-sonnet-4-6": "deepseek-v4-flash",
    "claude-3-5-sonnet-20241022": "deepseek-v4-flash"
  }
}
```

Then point your client at the proxy:

```bash
# Claude Code
ANTHROPIC_BASE_URL=http://127.0.0.1:9483 ANTHROPIC_API_KEY=any claude

# Claude Desktop — set Base URL to http://127.0.0.1:9483
```

## Modes

| Mode | `upstream_url` | Use case |
|---|---|---|
| `passthrough` | Anthropic-compatible endpoint (e.g. `api.deepseek.com/anthropic`) | Upstream natively speaks Anthropic protocol. Only model names are remapped. |
| `anthropic-to-openai` | OpenAI-compatible endpoint (e.g. `api.deepseek.com`) | Full protocol conversion: Anthropic requests → OpenAI, OpenAI responses → Anthropic. Supports streaming, tool use, and tool results. |

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
