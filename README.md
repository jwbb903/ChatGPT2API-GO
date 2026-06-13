<h1 align="center">
  <img src="assets/logo.png" alt="ChatGPT2API" width="72" height="72" />
  <br />
  ChatGPT2API Go
</h1>

<p align="center">ChatGPT2API Go 是一个自托管的 ChatGPT Web API 转发服务。它通过 ChatGPT 网页端账号调用上游能力，并向外提供 OpenAI 兼容接口、Anthropic 兼容接口、在线画图工作台、账号池管理、图片管理、日志审计和 Web 管理面板。</p>

<p align="center">
  <img src="assets/hero.png" alt="ChatGPT2API" width="100%" />
</p>

> [!WARNING]
> 免责声明：
>
> 本项目基于对 ChatGPT 网页端相关接口的技术研究实现，仅供个人学习、技术研究与非商业性技术交流使用。
>
> - 不得将本项目用于商业服务、倒卖、批量调用、自动化滥用或任何盈利性用途。
> - 不得将本项目用于破坏市场秩序、恶意竞争、套利、刷量、账号滥用或绕过平台限制。
> - 不得将本项目用于生成、传播或协助生成违法、暴力、色情、未成年人相关内容，或用于诈骗、欺诈、骚扰等非法或不当用途。
> - 使用本项目可能导致账号被限制、临时封禁或永久封禁。
> - 使用者应自行承担全部风险和法律责任。
> - 使用本项目即表示你已理解并同意本免责声明。

> [!IMPORTANT]
> 请不要使用重要账号、常用账号或高价值账号进行测试。上游接口、风控策略、模型权限和额度规则可能随时变化，本项目不保证长期可用。

## 项目定位

ChatGPT2API Go 的目标是把 ChatGPT 网页端能力封装成可自托管的 API 服务，适合下列场景：

- 需要把 ChatGPT 网页端文本能力接入 OpenAI 兼容客户端。
- 需要通过 API 调用 ChatGPT 网页端图片生成和图片编辑能力。
- 需要集中管理多个 ChatGPT 账号、额度、限流状态和失败状态。
- 需要一个带权限控制的在线画图工作台。
- 需要在服务器端缓存、管理、下载和清理生成图片。
- 需要查看每次请求的完整处理链路和上游请求过程。

本项目是 Go 后端实现，后端入口为 `cmd/server/main.go`，前端静态产物由 Go 服务直接托管。

## 核心能力

### OpenAI 兼容接口

- `GET /v1/models`
- `POST /v1/chat/completions`
- `POST /v1/responses`
- `POST /v1/images/generations`
- `POST /v1/images/edits`

### Anthropic 兼容接口

- `POST /v1/messages`

### ChatGPT Web 上游能力

- 文本对话 SSE 流式转发。
- ChatGPT 网页端图片生成。
- ChatGPT 网页端图片编辑。
- 普通 1K 图片生成路线。
- Codex 高清图片生成路线，支持 2K 和 4K 规格。
- 参考图上传到 ChatGPT 文件系统后再发起图生图请求。
- 对话触发图片工具后解析并下载最终图片。
- Sentinel proof token、Turnstile token、SO token、Conduit token 和 trace id 请求头处理。
- 账号指纹复用，包括 `user-agent`、`oai-device-id`、`oai-session-id`、`sec-ch-ua` 等字段。
- 403 HTML 拦截识别和未输出前换号重试。
- 无输出错误时自动换号重试，避免拼接半截响应。

### 工具调用支持

- `/v1/chat/completions` 支持 OpenAI tools 格式。
- 支持流式工具调用增量输出。
- 会剥离上游工具 XML 标记，避免把 `<tool_calls>` 等内部标记直接显示给客户端。
- 支持非流式工具调用解析和 `finish_reason=tool_calls`。

### 多模态输入

- 支持 OpenAI 风格图片输入。
- 支持 ChatGPT 文件上传链路中的图片上传、确认上传、图片指针引用。
- 支持 `input_file` / `file` 内容块的本地文本提取。
- 支持 TXT、Markdown 和轻量 PDF 文本提取。
- 含图片的 PDF 不做 OCR，会返回友好错误。

### 在线画图工作台

- 内置 Web 管理面板和在线画图页面。
- 支持文生图。
- 支持图生图。
- 支持多图参考编辑。
- 支持 `gpt-image-2` 和 `codex-gpt-image-2`。
- 支持 `1:1`、`16:9`、`9:16`、`4:3`、`3:4` 画幅。
- 支持 1K、2K、4K 清晰度。
- 支持图片任务、历史记录、图片归属和下载。

### 账号池管理

- 支持直接导入 ChatGPT `access_token`。
- 支持导入带账号元数据的 JSON 记录。
- 支持账号类型、状态、额度、恢复时间和失败次数管理。
- 支持账号轮询选择。
- 支持图片账号并发控制。
- 支持普通账号和 Plus / Team / Pro 账号分层使用。
- 支持 Token 失效识别。
- 支持限流识别和恢复时间记录。
- 支持失败账号标记。
- 支持按账号类型筛选文本请求。

### 权限和配额

- 支持全局 `auth-key`。
- 支持用户级密钥。
- 支持 admin / user 权限区分。
- 支持普通用户和高级用户等级。
- 支持图片日额度、月额度、总额度。
- 支持聊天日额度、月额度、总额度。
- 普通用户可限制为 1K 图片和 free 账号池。
- 高级用户可使用 Plus / Team / Pro 账号池和高清图片能力。

### 日志和可视化链路追踪

终端会打印同一个 trace id 下的完整处理链路，便于排查请求失败原因。

可看到的步骤包括：

- 客户端请求进入。
- 选择文本账号或图片账号。
- bootstrap ChatGPT 首页。
- 获取 Sentinel chat requirements。
- 计算 legacy proof input。
- 是否携带 proof、turnstile、SO token。
- 上传图片 metadata 请求。
- object storage PUT 上传。
- 标记文件 uploaded。
- 转换消息格式。
- 请求 `/backend-api/conversation`。
- SSE 开始、事件数量和结束。
- 图片任务 prepare。
- 图片任务 start。
- 轮询图片 tool records。
- 解析下载 URL。
- 下载生成图片。
- 失败后换号重试。
- 最终成功或失败。

敏感信息会脱敏，包括：

- `Authorization`
- access token
- refresh token
- id token
- cookie
- password
- secret
- `OAI-Device-Id`
- `OAI-Session-Id`
- 上传 URL query

关闭终端链路追踪：

```bash
CHATGPT2API_TRACE=0 ./start.sh --port 3000
```

也可以使用：

```bash
CHATGPT2API_NETWORK_TRACE=0 ./start.sh --port 3000
```

### 图片管理和画廊

- 服务端缓存生成图片。
- 支持图片浏览和下载。
- 支持图片归属记录。
- 支持提示词记录。
- 支持标签管理。
- 支持按日期清理旧图片。
- 支持保护画廊图片和用户图片，避免被自动清理。
- 支持公共画廊发布、撤回、查看和批量查询发布状态。

### 存储

当前后端使用本地 JSON 文件存储。

主要数据文件：

- `data/accounts.json`
- `data/auth_keys.json`
- `data/gallery.json`
- `data/image_tasks.json`
- `data/logs.json`
- `data/image_owners.json`
- `data/image_prompts.json`
- `data/image_tags.json`
- `data/chat_conversations.json`
- `data/cpa_pools.json`
- `data/sub2api_servers.json`

## 模型说明

文本接口的 `model` 字段会尽量透传给 ChatGPT 上游。实际可用模型取决于账号权限和上游当前开放状态。

图片接口推荐使用以下模型名：

| 模型 | 用途 |
|---|---|
| `gpt-image-2` | 普通图片生成和图片编辑 |
| `codex-gpt-image-2` | Codex 高清图片生成路线，适合 2K / 4K |

图片参数：

| 参数 | 支持值 |
|---|---|
| `size` | `1:1`、`16:9`、`9:16`、`4:3`、`3:4` |
| `resolution` | `1k`、`2k`、`4k` |

`resolution=2k` 或 `resolution=4k` 会优先走 Codex 高清路线，需要可用的 Plus / Team / Pro 账号。

## 快速开始

### 使用发布包运行

```bash
tar -xzf chatgpt2api-go-linux-amd64.tar.gz
cd chatgpt2api-go-linux-amd64
cp config.example.json config.json
./start.sh --port 3000
```

启动前请编辑 `config.json`，设置 `auth-key`。

访问地址：

- Web 面板：`http://localhost:3000`
- API 地址：`http://localhost:3000/v1`

### 使用 Docker 运行

```bash
docker build -t chatgpt2api-go .
docker run --rm \
  -p 3000:80 \
  -v "$PWD/data:/app/data" \
  -e CHATGPT2API_AUTH_KEY=chatgpt2api \
  chatgpt2api-go
```

### 本地开发运行

要求 Go 版本与 `go.mod` 一致。

```bash
go test ./...
make run
```

构建前端静态产物：

```bash
make web
make run
```

### 打包发布

```bash
scripts/package_release.sh
```

或：

```bash
make package
```

如需重新构建前端后再打包：

```bash
make package-web
```

发布包会生成到：

```text
release/chatgpt2api-go-linux-amd64.tar.gz
```

## 配置

配置文件路径：

```text
config.json
```

常用字段：

```json
{
  "auth-key": "change-me",
  "proxy": "",
  "base_url": "",
  "refresh_account_interval_minute": 60,
  "image_retention_days": 15,
  "image_poll_timeout_secs": 120,
  "image_account_concurrency": 3,
  "auto_remove_rate_limited_accounts": false,
  "auto_remove_invalid_accounts": false,
  "cleanup_protect_gallery": true,
  "cleanup_protect_user_images": true,
  "global_system_prompt": ""
}
```

### 环境变量

| 变量 | 说明 |
|---|---|
| `CHATGPT2API_AUTH_KEY` | 覆盖 `config.json` 中的 `auth-key` |
| `CHATGPT2API_ADDR` | 服务监听地址，例如 `:3000` |
| `CHATGPT2API_UPSTREAM_TRANSPORT` | 上游传输方式，支持 `tls-client`、`curl-impersonate` |
| `CHATGPT2API_CURL_IMPERSONATE_BIN` | 指定 `curl-impersonate` 二进制路径 |
| `CHATGPT2API_CURL_IMPERSONATE_URL` | 指定 `curl-impersonate` 下载地址 |
| `CHATGPT2API_TLS_PROFILE` | 指定 tls-client 指纹 profile |
| `CHATGPT2API_USER_AGENT` | 覆盖默认 User-Agent |
| `CHATGPT2API_SEC_CH_UA` | 覆盖默认 `Sec-Ch-Ua` |
| `CHATGPT2API_TRACE` | 设置为 `0`、`false`、`off`、`no`、`quiet` 可关闭终端链路追踪 |
| `CHATGPT2API_NETWORK_TRACE` | 同样用于关闭终端链路追踪 |

## 上游传输模式

默认使用 Go 内置 tls-client 模式。

也可以使用外部 `curl-impersonate`：

```bash
CHATGPT2API_UPSTREAM_TRANSPORT=curl-impersonate \
CHATGPT2API_CURL_IMPERSONATE_BIN=/path/to/curl_edge101 \
./start.sh --port 3000
```

发布包会包含：

```text
data/bin/curl-impersonate/curl_edge101
```

## API 使用

所有 AI 接口都需要鉴权：

```http
Authorization: Bearer <auth-key>
```

### GET /v1/models

```bash
curl http://localhost:3000/v1/models \
  -H "Authorization: Bearer <auth-key>"
```

### POST /v1/chat/completions

非流式文本请求：

```bash
curl http://localhost:3000/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <auth-key>" \
  -d '{
    "model": "gpt-5",
    "messages": [
      {"role": "user", "content": "写一段简短的项目介绍"}
    ]
  }'
```

流式文本请求：

```bash
curl http://localhost:3000/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <auth-key>" \
  -d '{
    "model": "gpt-5",
    "stream": true,
    "messages": [
      {"role": "user", "content": "用三点说明这个项目"}
    ]
  }'
```

带工具请求：

```bash
curl http://localhost:3000/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <auth-key>" \
  -d '{
    "model": "gpt-5",
    "messages": [
      {"role": "user", "content": "查询天气并返回摘要"}
    ],
    "tools": [
      {
        "type": "function",
        "function": {
          "name": "get_weather",
          "description": "查询城市天气",
          "parameters": {
            "type": "object",
            "properties": {
              "city": {"type": "string"}
            },
            "required": ["city"]
          }
        }
      }
    ]
  }'
```

### POST /v1/responses

```bash
curl http://localhost:3000/v1/responses \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <auth-key>" \
  -d '{
    "model": "gpt-5",
    "input": "用一句话介绍 ChatGPT2API Go"
  }'
```

### POST /v1/messages

```bash
curl http://localhost:3000/v1/messages \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <auth-key>" \
  -H "anthropic-version: 2023-06-01" \
  -d '{
    "model": "gpt-5",
    "max_tokens": 512,
    "messages": [
      {"role": "user", "content": "解释这个项目的作用"}
    ]
  }'
```

### POST /v1/images/generations

```bash
curl http://localhost:3000/v1/images/generations \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <auth-key>" \
  -d '{
    "model": "gpt-image-2",
    "prompt": "一只漂浮在太空里的猫，电影感，细节丰富",
    "n": 1,
    "size": "1:1",
    "resolution": "1k",
    "response_format": "b64_json"
  }'
```

### POST /v1/images/edits

```bash
curl http://localhost:3000/v1/images/edits \
  -H "Authorization: Bearer <auth-key>" \
  -F "model=gpt-image-2" \
  -F "prompt=把这张图改成赛博朋克夜景风格" \
  -F "n=1" \
  -F "size=9:16" \
  -F "resolution=1k" \
  -F "image=@./input.png"
```

### 聊天接口上传图片

`/v1/chat/completions` 支持 OpenAI 风格图片输入：

```bash
curl http://localhost:3000/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <auth-key>" \
  -d '{
    "model": "gpt-5",
    "messages": [
      {
        "role": "user",
        "content": [
          {"type": "text", "text": "描述这张图片"},
          {
            "type": "image_url",
            "image_url": {
              "url": "data:image/png;base64,<base64>"
            }
          }
        ]
      }
    ]
  }'
```

## Web 管理面板

Web 面板包含以下页面和能力：

- 登录和用户鉴权。
- 账号池管理。
- 用户密钥管理。
- 在线画图。
- 图片任务。
- 图片管理。
- 图片标签。
- 公共画廊。
- 日志查询。
- 系统设置。
- 代理测试。
- 存储信息。

## 安卓客户端

项目可配合安卓客户端 Draw 使用。安卓客户端通过本项目后端完成文生图、图生图、画廊、作品管理等操作。

安卓客户端为独立 APK 发布，本仓库不包含其源码。后端 API 完全开放，可参考 `docs/android-integration.md` 自行实现客户端。

兼容性：

| 项 | 要求 |
|---|---|
| Android 最低版本 | 8.0，API 26 |
| 后端接口 | 需要支持 `/v1/images/*`、`/api/gallery/*`、`/api/me/images` |
| 网络 | 建议生产环境使用 HTTPS 反向代理 |

## 截图

号池管理：

![accounts](assets/accounts.png)

在线画图：

![image-studio](assets/image-studio.png)

日志管理：

![logs](assets/logs.png)

图片管理：

![image-manager](assets/image-manager.png)

## 已禁用或未包含的能力

当前 Go 后端不包含以下能力：

- 注册机真实逻辑。
- Cloudflare R2 自动备份真实逻辑。
- SQLite、PostgreSQL、Git 存储后端。
- CPA 远程导入真实同步。
- sub2api 远程导入真实同步。
- 非图片文件上传到 ChatGPT 网页端的真实文件协议。
- PDF 图片 OCR。
- Arkose token 自动求解。

相关接口会尽量返回 disabled 状态或保留兼容结构，避免前端页面崩溃。

## 常见问题

### 返回 401 或 Token 失效

说明账号 access token 可能已失效。请刷新或重新导入账号。

### 返回 429 或额度限制

说明上游账号触发限流或额度不足。系统会标记账号状态，并在恢复时间后重新尝试。

### 返回 403 HTML

说明请求可能触发上游风控。系统会识别该类错误，并在没有输出内容前尝试换号重试。仍然失败时，请检查账号质量、代理质量和账号指纹配置。

### 返回 upstream requires arkose token

说明上游要求 Arkose 校验。本项目当前不内置 Arkose token 求解。

### 上传图片后返回 422

新版已修复普通聊天图片消息格式。`multimodal_text.parts` 中的文字部分会作为字符串发送，图片部分作为 `image_asset_pointer` 对象发送。

## 安全建议

- 不要暴露未加鉴权的服务到公网。
- 生产环境建议使用 HTTPS 反向代理。
- 不要使用重要账号。
- 不要在日志、Issue、截图中公开 token、cookie、账号邮箱和代理地址。
- 建议为不同用户创建不同用户密钥，并配置额度。

## 开发命令

```bash
go test ./...
gofmt -w internal/app cmd/server
make run
make web
make package
make package-web
```

## 目录结构

```text
cmd/server/main.go                 服务入口
internal/app/server.go             HTTP 服务和路由
internal/app/v1.go                 OpenAI / Anthropic 兼容接口
internal/app/upstream.go           ChatGPT Web 上游协议
internal/app/codex.go              Codex 图片路线
internal/app/text_stream_retry.go  文本流换号重试
internal/app/image_pool.go         图片账号池和重试
internal/app/turnstile.go          Turnstile token 本地求解
internal/app/trace_log.go          终端链路追踪日志
internal/app/accounts.go           账号导入和刷新
internal/app/account_pool.go       账号选择和并发控制
internal/app/store.go              JSON 文件存储
web/                               前端源码
web_dist/                          前端构建产物
scripts/package_release.sh         发布打包脚本
data/                              运行数据目录
```

## 友情链接

- [LINUX DO - 新的理想型社区](https://linux.do/)
