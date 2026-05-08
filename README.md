**中文** | [English](./README_EN.md)

# Model Mapper

轻量级 LLM API 反向代理，拦截请求并重新映射模型名称后转发至上游。适用于客户端（如 Claude Code、Claude Desktop）与上游使用不同模型名称，甚至不同协议的场景。

## 功能特性

- **模型路由表** — Opus / Sonnet / Haiku 可分别选择不同上游，支持 DeepSeek、MiMo 等模型混搭。
- **上游模型目录** — 每个上游保留自己的模型名映射；路由表只决定请求走哪个上游，不会破坏上游映射关系。
- **协议自适配** — Anthropic 兼容上游直接透传；OpenAI 兼容上游自动做 Anthropic ↔ OpenAI 协议转换（支持流式 SSE、Tool Use）。
- **Web 管理界面** — 在浏览器中配置一切，地址 `http://localhost:9483`。
- **安全防护** — 默认绑定 `127.0.0.1`，Origin/Referer CSRF 校验，`config.json` 以 `0600` 权限保存。可用 `MODEL_MAPPER_CONFIG` 指定配置路径。
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

在客户端中指向代理：

```bash
# Claude Code
ANTHROPIC_BASE_URL=http://127.0.0.1:9483 ANTHROPIC_API_KEY=any claude

# Claude Desktop — 将 Base URL 设为 http://127.0.0.1:9483
```

## 配置模型

| 字段 | 作用 |
|---|---|
| `model_routes` | 客户端模型 → 上游索引。用于模型混搭，例如 Opus 走 DeepSeek、Haiku 走 MiMo。 |
| `default_upstream` | 没有命中 `model_routes` 时使用的默认上游。 |
| `upstreams[].mappings` | 该上游自己的模型名目录。多个上游可以保留同一个客户端模型名，各自映射到不同上游模型。 |
| `upstreams[].protocol` | `anthropic` 直接透传；`openai` 自动做 Anthropic ↔ OpenAI 协议转换。 |

## 部署到服务器（Linux）

```bash
# 交叉编译（或从 Releases 下载）
make build-linux

# 传输到服务器
scp dist/model-mapper-linux-amd64 deploy/* user@server:/tmp/

# 在服务器上
sudo useradd -r -s /usr/sbin/nologin model-mapper   # 创建服务用户
sudo mkdir -p /opt/model-mapper
sudo cp /tmp/model-mapper-linux-amd64 /opt/model-mapper/model-mapper
sudo cp /tmp/config.example.json /opt/model-mapper/config.json
sudo chmod +x /opt/model-mapper/model-mapper
sudo chown -R model-mapper:model-mapper /opt/model-mapper

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
