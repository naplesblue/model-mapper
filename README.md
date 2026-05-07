**中文** | [English](./README_EN.md)

# Model Mapper

轻量级 LLM API 反向代理，拦截请求并重新映射模型名称后转发至上游。适用于客户端（如 Claude Code、Claude Desktop）与上游使用不同模型名称，甚至不同协议的场景。

## 功能特性

- **模型名映射** — 将 `claude-opus-4-6` → `deepseek-v4-pro`、`claude-sonnet-4-6` → `deepseek-v4-flash` 等，每个模型独立映射。
- **双工作模式** — `passthrough` 透传模式适配 Anthropic 兼容上游；`anthropic-to-openai` 转换模式适配 OpenAI 兼容上游（完整协议转换，支持流式 SSE、Tool Use）。
- **Web 管理界面** — 在浏览器中配置一切，地址 `http://localhost:9483`。
- **安全防护** — 默认绑定 `127.0.0.1`，Origin/Referer CSRF 校验，`config.json` 以 `0600` 权限保存。
- **单文件部署** — 零依赖，支持交叉编译至 macOS arm64 和 Linux amd64。

## 快速开始

从 [Releases](https://github.com/naplesblue/model-mapper/releases) 下载预编译二进制，或从源码构建：

```bash
# 构建
make build

# 首次运行 — 自动生成默认 config.json
./model-mapper

# 打开 http://127.0.0.1:9483 进行配置
```

编辑 `config.json`（或通过 Web UI）设置上游 Token：

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

在客户端中指向代理：

```bash
# Claude Code
ANTHROPIC_BASE_URL=http://127.0.0.1:9483 ANTHROPIC_API_KEY=any claude

# Claude Desktop — 将 Base URL 设为 http://127.0.0.1:9483
```

## 工作模式

| 模式 | `upstream_url` | 适用场景 |
|---|---|---|
| `passthrough` | Anthropic 兼容端点（如 `api.deepseek.com/anthropic`） | 上游原生支持 Anthropic 协议，仅做模型名替换 |
| `anthropic-to-openai` | OpenAI 兼容端点（如 `api.deepseek.com`） | 完整协议转换：Anthropic 请求 → OpenAI，OpenAI 响应 → Anthropic。支持流式、Tool Use |

## 部署到服务器（Linux）

```bash
# 交叉编译（或从 Releases 下载）
make build-linux

# 传输到服务器
scp dist/model-mapper-linux-amd64 deploy/* user@server:/tmp/

# 在服务器上
sudo mkdir -p /opt/model-mapper
sudo cp /tmp/model-mapper-linux-amd64 /opt/model-mapper/model-mapper
sudo cp /tmp/config.example.json /opt/model-mapper/config.json
sudo chmod +x /opt/model-mapper/model-mapper

# 编辑 config.json 填入 API Key
sudo vi /opt/model-mapper/config.json

# 安装 systemd 服务
sudo cp /tmp/model-mapper.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now model-mapper

# 可选：nginx 反代供局域网访问
sudo cp /tmp/nginx.conf.example /etc/nginx/sites-available/model-mapper
sudo ln -s /etc/nginx/sites-available/model-mapper /etc/nginx/sites-enabled/
sudo nginx -t && sudo systemctl reload nginx
```

## 从源码构建

```bash
make build          # 本机二进制 → ./model-mapper
make build-darwin   # macOS arm64  → dist/model-mapper-darwin-arm64
make build-linux    # Linux amd64  → dist/model-mapper-linux-amd64
make build-all      # 两个平台
make clean          # 清理构建产物
```

## 项目结构

```
├── main.go                 # 入口、路由、embed
├── config.go               # 配置结构、加载/保存、CSRF 防护
├── proxy.go                # 透传代理、模型名替换
├── convert.go              # Anthropic ↔ OpenAI 协议转换引擎
├── web/
│   └── index.html          # Web 管理界面（编译进二进制）
├── deploy/
│   ├── config.example.json # 配置模板
│   ├── nginx.conf.example  # Nginx 反代配置
│   └── model-mapper.service # Systemd unit 文件
├── Makefile
└── go.mod
```

## 许可证

MIT
