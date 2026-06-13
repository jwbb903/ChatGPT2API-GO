# Go 迁移说明

本仓库已加入 Go 后端实现，入口为 `cmd/server/main.go`，默认监听 `:3000`，Docker 内监听 `:80`。

## 本次迁移范围

- 保留 `web/` Next.js 前端，生产环境由 Go 后端直接服务 `web_dist/`。
- 存储仅保留 JSON 文件，数据目录为 `data/`。
- 移除数据库 / Git 存储后端。
- 移除注册机真实逻辑，相关接口返回 disabled 状态，避免前端页面崩溃。
- 移除 Cloudflare R2 备份真实逻辑，备份接口返回 disabled/空列表。
- 保留管理面板核心端点：账号、用户密钥、设置、日志、图片管理、画廊、图片任务、聊天会话。
- 保留 OpenAI/Anthropic 兼容端点：`/v1/models`、`/v1/images/*`、`/v1/chat/completions`、`/v1/responses`、`/v1/messages`。

## 重要说明

当前 Go 版本已经接入真实 ChatGPT Web 上游链路：

- `/v1/images/generations` 通过账号池 access_token 调用 ChatGPT Web 图片生成；
- `/v1/images/edits` 会先上传参考图，再调用 ChatGPT Web 图片链路；
- `/v1/chat/completions`、`/v1/responses`、`/v1/messages` 会通过 ChatGPT Web SSE 文本链路返回；
- `/api/chat/stream` 使用同一条真实文本 SSE 链路；
- `/api/accounts/refresh` 会调用 `/backend-api/me` 刷新账号基础信息。

上游 HTTP 客户端支持两种模式：

- 默认：纯 Go `github.com/bogdanfinn/tls-client`，默认 `Chrome_146` profile，不使用 C/C++、curl 或 CGO。
- 完全一致模式：外部 `curl-impersonate` 子进程，由 Go 通过 `exec.Command` 启动，不使用 CGO。

环境变量：

- `CHATGPT2API_UPSTREAM_TRANSPORT=tls-client` 默认纯 Go 模式
- `CHATGPT2API_UPSTREAM_TRANSPORT=curl-impersonate` 使用外部 curl-impersonate
- `CHATGPT2API_CURL_IMPERSONATE_BIN=/path/to/curl_edge101` 指定二进制
- `CHATGPT2API_TLS_PROFILE=chrome_146|chrome_133|chrome_120|...` 仅 tls-client 模式有效
- `CHATGPT2API_USER_AGENT=<自定义 UA>`

如果上游触发 Arkose，Go 版会返回真实上游错误；Arkose token 求解仍未内置。

## 本地运行

```bash
go test ./...
make run
```

然后访问：

- Web：`http://localhost:3000`
- API：`http://localhost:3000/v1`

## 构建前端

```bash
make web
make run
```

## Docker

```bash
docker build -t chatgpt2api-go .
docker run --rm -p 3000:80 -v "$PWD/data:/app/data" -e CHATGPT2API_AUTH_KEY=chatgpt2api chatgpt2api-go
```

## JSON 数据文件

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
