# Android 客户端对接文档

面向 [Draw](https://github.com/RemotePinee) 安卓客户端的后端 API 集成指南。所有 `/v1/*` 接口与 OpenAI 官方协议兼容，可直接复用 OpenAI 安卓 SDK 的请求结构。

> 本文档跟随后端代码同步演进。如果发现接口与代码不一致，以代码为准并提 issue。

> [!NOTE]
> **官方 APK 闭源 / 后端 API 开源。**
> - 官方维护的 Draw 安卓客户端只在 [Releases](https://github.com/RemotePinee/ChatGPT2API/releases) 以 APK 形式发布，本仓库不包含其源码。
> - 后端 API（含 `/v1/*`、`/api/gallery/*`、`/api/me/images` 等）完全开源，本文档就是用来支持你**基于该 API 自行实现安卓 / iOS / 桌面客户端**的。
> - 如果你只是终端用户，直接装 Releases 里的 APK 即可，不需要读这篇文档。

---

## 一、环境基线

下表是写文档时验证过的版本组合，安卓项目按这套来不会踩坑。

| 项 | 版本 |
|---|---|
| Android Studio | Ladybug 2024.2.1 及以上 |
| JDK | 21 |
| Kotlin | 2.0+ |
| Compose Compiler | 由 `org.jetbrains.kotlin.plugin.compose` 插件管理 |
| Compose BOM | `2026.03.01` |
| compileSdk / targetSdk | 36 |
| minSdk | 26 (Android 8.0) |
| Gradle | 8.7+ |
| AGP | 8.5+ |

后端环境：

| 项 | 版本 |
|---|---|
| Python | 3.13 |
| 包管理 | uv |
| Web 框架 | FastAPI |
| 后端 repo | https://github.com/RemotePinee/ChatGPT2API |
| API 版本 | 见 `VERSION` 文件 |

---

## 二、推荐安卓依赖

```kotlin
dependencies {
    // Compose
    implementation(platform("androidx.compose:compose-bom:2026.03.01"))
    implementation("androidx.compose.ui:ui")
    implementation("androidx.compose.material3:material3")
    implementation("androidx.compose.material:material-icons-extended")
    implementation("androidx.activity:activity-compose:1.13.0")
    implementation("androidx.navigation:navigation-compose:2.9.7")

    // 网络层（OpenAI 兼容协议直接用 Retrofit + OkHttp 即可）
    implementation("com.squareup.retrofit2:retrofit:3.0.0")
    implementation("com.squareup.retrofit2:converter-gson:3.0.0")
    implementation("com.squareup.okhttp3:okhttp:5.3.2")
    implementation("com.squareup.okhttp3:logging-interceptor:5.3.2")

    // 异步
    implementation("org.jetbrains.kotlinx:kotlinx-coroutines-android:1.10.2")

    // 图片加载
    implementation("io.coil-kt:coil-compose:2.7.0")

    // 本地存储
    val roomVersion = "2.8.4"
    implementation("androidx.room:room-runtime:$roomVersion")
    implementation("androidx.room:room-ktx:$roomVersion")
    ksp("androidx.room:room-compiler:$roomVersion")
    implementation("androidx.datastore:datastore-preferences:1.1.1")
}
```

---

## 三、鉴权

所有 `/v1/*` 接口都要带 `Authorization: Bearer <token>`。token 有两种来源：

| 来源 | 说明 | 推荐场景 |
|---|---|---|
| 管理员 auth_key | 部署后端时在 `config.json` 里设置的 `auth-key`，全权限 | 自部署、个人用 |
| 用户密钥 | 管理员在后台 `/api/auth/users` 创建的 `sk-...` 形式密钥，可设额度 | 给别人发用 |

后端识别这两种来自同一个 header，AP 端无需区分。

### 配置入口

AP 首启引导页 / 设置页让用户输入两项：

```
后端地址 (Base URL)：例如 https://api.example.com  或 http://192.168.1.10:8000
访问密钥 (Auth Key)：admin auth_key 或 sk-... 用户密钥
```

存储建议：

- Base URL → DataStore Preferences（明文）
- Auth Key → DataStore Preferences（**建议过 EncryptedSharedPreferences 或 Tink 加密**）

### 401 处理

任意接口返回 401 都意味着密钥无效或被吊销，AP 端要：

1. 清掉本地保存的 auth_key（base url 保留）
2. 跳回登录/设置页

### 启动体检 + 连不上后端的处理

冷启动时 AP 端应该主动调一次 `GET /api/auth/me` 做"登录态体检"，避免用户携带过期密钥进入 home 页才被各个业务接口零散踢回登录页。推荐策略（Draw 当前实现）：

| 启动 me() 结果 | 处理 |
|---|---|
| 200 | 登录态有效，进 home，顺手把 identity 缓存进 ViewModel |
| 401 | 同步清密钥跳登录页，文案"登录已失效" |
| IOException / connect failed / TLS 失败 | 视为"后端连不到"，同样清密钥跳登录页，文案"无法连接到后端，请检查地址或稍后重试" |
| 5xx / 其它错误 | 不踢，让用户进 home 后正常使用，真要有问题后续请求再触发拦截器 |

**3 秒超时封顶**：体检要给一个上限（OkHttp connectTimeout 15s 太长会让 splash underlay 卡很久），否则用户以为应用死掉了。超时按"连不上"处理。

### 鉴权钩子的两个开口

普通业务请求收 401 / IOException 时由 AP 端的 `OkHttp Interceptor` 全局拦截 → 触发 forceLogout。但有两条路径**不应该**触发全局踢出：

1. **登录页校验密钥**（用户输错正常的事）：调一个特殊版本的 `GET /api/auth/me`，带自定义 header `X-Draw-Skip-Unauth-Hook: 1`。拦截器看到这个 header 就跳过踢出钩子，只让 LoginViewModel 把错误反馈到表单上。
2. **生图请求**（上游限速 / 内容拒绝 / 5xx 太常见）：`/v1/images/generations` 和 `/v1/images/edits` 也都带 `X-Draw-Skip-Unauth-Hook: 1`，**生图链路任何错误都不踢用户**。错误反馈走 ViewModel 的 errorMessage / Toast。

> 后端不识别 `X-Draw-Skip-Unauth-Hook` 这个 header，它只是 AP 端拦截器内部用的标记，看到就跳过钩子。其他 client（OpenAI 官方 SDK 等）不带这个 header，行为不变。

---

## 四、AP 端用得到的接口清单

### 1. 拉模型列表

```http
GET /v1/models
Authorization: Bearer <token>
```

**响应（200，节选）**：

```json
{
  "object": "list",
  "data": [
    { "id": "gpt-image-2", "object": "model", "owned_by": "chatgpt2api" },
    { "id": "codex-gpt-image-2", "object": "model", "owned_by": "chatgpt2api" },
    { "id": "auto", "object": "model", "owned_by": "chatgpt2api" },
    { "id": "gpt-5", "object": "model", "owned_by": "chatgpt2api" },
    { "id": "gpt-5-mini", "object": "model", "owned_by": "chatgpt2api" }
  ]
}
```

**实现要点**：

- `/v1/models` 返回的是后端**全部**模型，覆盖文本对话和画图两类。
- AP 端只关心**画图模型**，过滤 `id.contains("image")` 即可。命中规则的目前固定是：
  - `gpt-image-2`：默认画图通道（上游 `gpt-5-3` slug）
  - `codex-gpt-image-2`：Codex 画图通道，仅 Plus / Team / Pro 订阅可用，与官网画图共用账号但额度独立
- 其它模型（`gpt-5` / `gpt-5-mini` / `auto` 等）属于文本类，传给 `/v1/images/*` 会被后端**回落到 `auto`**——既不可控也违反"用户选什么用什么"的预期，AP 端 dropdown 不要展示。
- 列表会随上游 ChatGPT Web 变化，**不要在 AP 端硬编码**画图模型清单，靠"id 含 `image`"过滤即可适配后续新增画图模型。

---

### 2. 文生图

```http
POST /v1/images/generations
Authorization: Bearer <token>
Content-Type: application/json
```

**请求体**：

```json
{
  "prompt": "一只穿宇航服的猫，蹲在月球表面",
  "model": "gpt-image-2",
  "n": 1,
  "size": "1:1",
  "response_format": "url",
  "stream": false
}
```

| 字段 | 类型 | 必需 | 说明 |
|---|---|---|---|
| `prompt` | string | ✓ | 至少 1 字符 |
| `model` | string |   | 默认 `gpt-image-2` |
| `n` | int |   | 1-4，默认 1。**会按 n 整体预扣额度**，详见下方"额度与失败退额度"|
| `size` | string |   | 支持 `1:1` `16:9` `9:16` `4:3` `3:4`，其它字符串会原样注入 prompt |
| `resolution` | string |   | 支持 `1k` `2k` `4k`。`2k` / `4k` 需要高级用户密钥，后端会强制校验 |
| `response_format` | string |   | `url` 或 `b64_json`，默认 `b64_json`。**安卓端强烈建议 `url`**（解 b64 慢且耗内存） |
| `stream` | bool |   | 见下文流式说明 |

**清晰度与账号池调度**：

- `resolution` 为空或 `1k`：普通画图路线，按用户权限选择对应账号池。
- `resolution=2k/4k`：后端优先走 Codex 高清画图路线，并按 `Pro` → `Plus` → `Team` 从账号池挑选可用账号。
- `4k + size=9:16` 会映射到 `2160x3840`；`4k + size=16:9` 会映射到 `3840x2160`；其它画幅按同等长边规则映射。
- 高清路线如果上游失败，后端会返回错误，不会静默降级成 1K；AP 端应该把错误提示给用户。

**非流式响应（200）**：

```json
{
  "created": 1779256269,
  "data": [
    {
      "url": "https://your-backend/images/2026/05/20/1779256269_abc.png",
      "revised_prompt": "..."
    }
  ]
}
```

如果上游拒绝（内容策略命中等），返回 OpenAI 标准错误：

```json
{
  "error": {
    "message": "Image generation was rejected by upstream policy.",
    "type": "invalid_request_error",
    "code": "content_policy_violation"
  }
}
```

**额度与失败退额度**：

- 入口扣费：`POST /v1/images/generations` 进入处理流程时按 `n` 整体预扣额度（admin / unlimited 用户跳过扣费）。
- 失败退额度：上游真实失败（content_policy / 5xx / 上游超时 / SSE 中途断流）后端会自动把预扣的 `n` 张额度退还。
- 用户错误不退：参数错误（400）、内容审查不过（敏感词命中）走 fail-fast 路径，扣费前就 raise，**不会扣额度也不需要退**。
- AP 端不需要做任何退额度逻辑——纯后端语义，但生图后建议主动调一次 `GET /api/auth/me` 刷新本地缓存的 `remaining`，让"可用额度"数字立刻更新。

---

### 3. 图生图（编辑）

```http
POST /v1/images/edits
Authorization: Bearer <token>
Content-Type: multipart/form-data
```

**multipart 字段**：

| 字段 | 类型 | 必需 | 说明 |
|---|---|---|---|
| `image` | file | ✓ | 单图传 `image`；多图重复传 `image[]` 字段 |
| `prompt` | string | ✓ | |
| `model` | string |   | 默认 `gpt-image-2` |
| `n` | int |   | 1-4 |
| `size` | string |   | 同上 |
| `resolution` | string |   | 同上，multipart 表单字段名就是 `resolution` |
| `response_format` | string |   | 同上 |
| `stream` | string |   | `true`/`false` |

响应结构跟 `/v1/images/generations` 一致。

**额度与失败退额度**：与文生图一致——按 `n` 预扣，上游真失败自动退还，参数错误 fail-fast 不扣。

**OkHttp + Retrofit 写法**：

```kotlin
interface DrawApi {
    @Multipart
    @POST("v1/images/edits")
    suspend fun editImages(
        @Part image: MultipartBody.Part,
        @Part("prompt") prompt: RequestBody,
        @Part("model") model: RequestBody,
        @Part("n") n: RequestBody,
        @Part("size") size: RequestBody?,
        @Part("resolution") resolution: RequestBody?,
        @Part("response_format") responseFormat: RequestBody,
    ): ImageGenerationResponse
}
```

---

### 4. 流式生成（SSE）

请求里带 `"stream": true` 后，响应类型变为 `text/event-stream`。每一帧 `data:` 后跟一段 JSON。

**事件类型有三种**，靠 `object` 字段区分：

#### 4.1 进度事件

```json
{
  "object": "image.generation.chunk",
  "created": 1779256269,
  "model": "gpt-image-2",
  "index": 1,
  "total": 1,
  "progress_text": "正在生成...",
  "upstream_event_type": "conversation.delta",
  "data": []
}
```

UI 展示：进度条/进度文案。`index` / `total` 表示"第几张/共几张"，多图模式下连续推送。

#### 4.2 文本事件（含上游拒绝信息）

```json
{
  "object": "image.generation.message",
  "created": 1779256269,
  "model": "gpt-image-2",
  "index": 1,
  "total": 1,
  "message": "I cannot generate this image because..."
}
```

收到后等同于"上游拒绝/失败"，AP 端把 `message` 直接展示给用户。

#### 4.3 结果事件

```json
{
  "object": "image.generation.result",
  "created": 1779256269,
  "model": "gpt-image-2",
  "index": 1,
  "total": 1,
  "data": [
    {
      "url": "https://your-backend/images/...",
      "revised_prompt": "..."
    }
  ]
}
```

收到这个事件就把图渲染出来。流可能会推多个 result（n>1 的情况），按 `index` 累积。

#### 4.4 终结

最后会有一帧：

```
data: [DONE]
```

或直接关闭连接。AP 端收到 `[DONE]` 字面值或连接关闭都算正常结束。

#### 4.5 OkHttp + Flow 解析样板

```kotlin
fun streamImageGeneration(request: ImageGenerationRequest): Flow<ImageEvent> = flow {
    val body = json.encodeToString(request).toRequestBody("application/json".toMediaType())
    val req = Request.Builder()
        .url("$baseUrl/v1/images/generations")
        .header("Authorization", "Bearer $token")
        .header("Accept", "text/event-stream")
        .post(body)
        .build()

    okHttpClient.newCall(req).execute().use { response ->
        if (!response.isSuccessful) throw HttpException(response)
        val source = response.body!!.source()
        while (!source.exhausted()) {
            val line = source.readUtf8Line() ?: break
            if (!line.startsWith("data:")) continue
            val payload = line.removePrefix("data:").trim()
            if (payload == "[DONE]") break
            val obj = json.parseToJsonElement(payload).jsonObject
            when (obj["object"]?.jsonPrimitive?.content) {
                "image.generation.chunk" -> emit(ImageEvent.Progress(...))
                "image.generation.message" -> emit(ImageEvent.Message(...))
                "image.generation.result" -> emit(ImageEvent.Result(...))
            }
        }
    }
}.flowOn(Dispatchers.IO)
```

---

### 5. 拉自己的额度

```http
GET /api/auth/me
Authorization: Bearer <token>
Cache-Control: no-store
```

> 注意路径是 `/api/auth/me`，不是 `/v1/...`。这是后端自己的接口，不是 OpenAI 兼容。

**响应**：

```json
{
  "identity": {
    "id": "abc123",
    "name": "用户A",
    "role": "user",
    "account_tier": "premium",
    "can_use_paid_image_accounts": true,
    "can_use_high_resolution": true,
    "image_daily_quota": 20,
    "image_daily_used": 3,
    "image_daily_unlimited": false,
    "image_daily_remaining": 17,
    "image_monthly_quota": 200,
    "image_monthly_used": 31,
    "image_monthly_unlimited": false,
    "image_monthly_remaining": 169,
    "image_total_quota": 1000,
    "image_total_used": 122,
    "image_total_unlimited": false,
    "image_total_remaining": 878
  }
}
```

`role` = `admin` 或 token 是 admin 的 auth_key 时，六档额度都会返回 `*_unlimited: true`、`*_remaining: null`，AP 端按"无限额度"展示。

**用户等级**：

| 字段 | 含义 |
|---|---|
| `account_tier=free` | 普通用户。只能使用 free 账号池，AP 端应禁用 2K / 4K |
| `account_tier=premium` | 高级用户。可使用 Plus / Team / Pro 账号池和 2K / 4K |
| `can_use_high_resolution=true` | 直接作为 AP 端是否允许点 2K / 4K 的判断字段 |

> UI 禁用只是体验。后端也会在 `/v1/images/*` 入口强制校验：普通用户即使用抓包或 F12 改出 `resolution=4k`，也会返回 403。

**调用时机**：

- AP 启动后调一次，存到 ViewModel
- 每次画图成功后再调一次刷新
- 每次画图失败后也建议静默刷新一次，因为后端可能已经扣费后又退回，或者账号池限流状态发生变化

---

### 6. 公共画廊

画廊是后端原生功能：用户可以把自己生成的图发布到公共池，其它用户能浏览、复用 prompt、把这张图当参考图二创。所有接口都走 `require_identity`，admin / user key 都能调，部分写操作只对发布者本人或 admin 开放。

#### 6.1 GalleryItem 字段

所有画廊接口返回的统一结构：

```json
{
  "id": "abc123hex",
  "url": "https://your-backend/images/2026/05/21/foo.png",
  "image_rel": "2026/05/21/foo.png",
  "prompt": "雪夜的莫斯科红场，雪花纷飞",
  "model": "gpt-image-2",
  "size": "9:16",
  "width": 1024,
  "height": 1820,
  "publisher_name": "用户A",
  "created_at": 1779256269,
  "status": "visible",
  "is_edit": false,
  "is_mine": true
}
```

| 字段 | 说明 |
|---|---|
| `id` | 服务端 uuid hex，作为 detail / unpublish 的主键 |
| `url` | 完整 http(s) 图片地址，可直接喂给 Coil |
| `image_rel` | `image_owners` 的 rel 路径，发布者本人的"撤回 / 重新覆盖"靠这个匹配 |
| `width` / `height` | 用于瀑布流 aspectRatio；后端没存就给 0，AP 端按 1:1 兜底 |
| `status` | `visible` / `hidden`。普通用户只能看见 visible；admin 加 `?include_hidden=true` 能看到 hidden |
| `created_at` | epoch **秒**（不是毫秒），AP 端展示"几分钟前"时注意 |
| `is_edit` | true = 图生图产出。后端 publish 时会强制把 `prompt` 落空——离开参考图后那段"换个浅色版"指令对其他用户无复用价值。AP 端据此把 prompt 区换成提示卡 + 复制按钮置灰 |
| `is_mine` | 当前请求者 == 发布者。仅 viewer_id 与 publisher_id 一致时为 true，避免 publisher_id 直接外泄。AP 端据此给本人显示"撤回发布"按钮 |
| ⚠️ 没有 `publisher_id` 字段 | 后端故意不返，避免身份信息泄露给其他用户 |

#### 6.2 GET /api/gallery/feed — 画廊主流

游标分页。第一次调用 `cursor=null`；后续把上次响应里的 `next_cursor` 传回来。`next_cursor=""` 表示已到底。

```http
GET /api/gallery/feed?cursor=&limit=20
Authorization: Bearer <token>
```

| Query | 类型 | 说明 |
|---|---|---|
| `cursor` | string |   首次调用传空；后续传 `next_cursor` |
| `limit` | int | 1-100，默认 20 |
| `include_hidden` | bool | **仅 admin 有效**。其他用户带这个 query 也被无视 |

**响应**：

```json
{
  "items": [ { "id": "...", "url": "...", "..." } ],
  "next_cursor": "eyJ0Ijo..."
}
```

#### 6.3 GET /api/gallery/items/{id} — 单条详情

```json
{ "item": { "id": "...", "url": "...", "..." } }
```

404 = 条目不存在或被下架。admin 看 hidden 也返 200。

#### 6.4 POST /api/gallery/publish — 发布到画廊

```http
POST /api/gallery/publish
Authorization: Bearer <token>
Content-Type: application/json

{
  "image_rel": "2026/05/21/foo.png",
  "prompt": "雪夜的莫斯科红场",
  "model": "gpt-image-2",
  "size": "9:16",
  "width": 1024,
  "height": 1820
}
```

| 字段 | 必需 | 说明 |
|---|---|---|
| `image_rel` | ✓ | 后端 `image_owners` 里挂在自己名下的图。403 = 不是自己的图 |
| `prompt` / `model` / `size` / `width` / `height` |   | 全部可选，但**强烈建议都带**——画廊条目跟生成时元数据一致，"用此 prompt 生成"才能完整还原 |

**幂等**：同一 `(publisher_id, image_rel)` 重复发布会返回旧记录，不会重复生成。

**返回**：跟 `/items/{id}` 一致的 `{ "item": { ... } }`。

#### 6.5 DELETE /api/gallery/items/{id} — 撤回 / 删除

发布者本人撤回 / admin 删除任意条目共用此接口。

```http
DELETE /api/gallery/items/abc123hex
Authorization: Bearer <token>
```

后端通过 `publisher_id == requester_id` 校验本人撤回权限；admin 全权。原图（`image_owners`）不会动，作品在"我的作品"里仍保留——撤回 = 从画廊里把这条记录删了。

#### 6.6 POST /api/gallery/items/{id}/hide / unhide — admin 软下架

```http
POST /api/gallery/items/abc123hex/hide
POST /api/gallery/items/abc123hex/unhide
Authorization: Bearer <admin-token>
```

软下架：不删原图、不丢数据，只把 `status` 改成 `hidden`。前台 feed 看不到，admin 后台带 `include_hidden=true` 仍能看到。发布者本人若再 publish 同一张图，service 会自动恢复成 visible。

#### 6.7 GET /api/gallery/published?image_rel=... — 单条查询

给"我的作品"页判定"这张图我发了没"，让卡片菜单切换"发布到画廊 / 已发布·撤回"两态。

```json
{
  "published": true,
  "item": { "id": "abc123hex", "status": "visible" }
}
```

未发布时 `{ "published": false, "item": null }`。

#### 6.8 POST /api/gallery/published/batch — 批量查询

"我的作品"页 reload 时一次播种 `publishStates`，避免逐张发单条请求被浏览器并发数撑满。

```http
POST /api/gallery/published/batch
Authorization: Bearer <token>
Content-Type: application/json

{ "image_rels": ["2026/05/21/a.png", "2026/05/21/b.png"] }
```

**响应**只包含查到记录的 rel——未发布的 rel 不在 key 里：

```json
{
  "items": {
    "2026/05/21/a.png": { "published": true, "id": "...", "status": "visible" }
  }
}
```

> admin 视角自动跨用户查询：admin 在图片管理页要管理任何用户的图，只关心"这张图被任何人发过没"，不区分发布者；普通 user 仍按自己 publisher_id 过滤。

---

### 7. 我的作品（云端聚合）

```http
GET /api/me/images?start_date=2026-05-01&end_date=2026-05-31
Authorization: Bearer <token>
```

返回当前 identity 名下的所有云端图。**用途**：AP 启动 / 进入"我的作品"页时拉一次，与本地 Room 历史合并去重，让重装应用 / 换设备时云端图也能恢复出来。

| Query | 必需 | 说明 |
|---|---|---|
| `start_date` |   | `YYYY-MM-DD`，留空表示不限下界 |
| `end_date` |   | 同上，留空表示不限上界 |

**身份过滤逻辑**：

- 普通 user 密钥：只返回自己生成的图（按 identity.id 过滤 `image_owners.json`）
- admin 密钥：自动过滤为 `__admin__`，把所有 admin 角色生成的图聚合返回（语义上"我"= 管理员这个角色）
- 不开放 `owner` 参数：避免用户冒名查别人的图

**响应**：

```json
{
  "items": [
    {
      "rel": "2026/05/21/foo.png",
      "url": "https://your-backend/images/2026/05/21/foo.png",
      "thumb_url": "https://your-backend/images/2026/05/21/foo.thumb.webp",
      "date": "2026-05-21",
      "size_bytes": 1234567,
      "mtime": 1779256269,
      "owner": "<user_key_id>"
    }
  ],
  "groups": [
    { "date": "2026-05-21", "items": [ ... ] }
  ]
}
```

`groups` 是按日期分组的视图，AP 端如果要"按日期分段"渲染可以直接用；不需要的话只看 `items` 即可。

---

## 五、错误码与重试

| HTTP | 含义 | 退额度？ | AP 端建议 |
|---|---|---|---|
| 200 | 成功 | n/a | |
| 400 | 参数错误（prompt 为空、size 不合法、image 缺失、image_rel 不存在等） | 不扣不退 | 提示具体 `error` 字段，不重试 |
| 401 | 密钥无效 | n/a | 清密钥跳登录页（生图链路除外，见鉴权钩子说明） |
| 402 | **额度不足** | n/a | 弹窗提示"额度不足，请联系管理员"，不要自动重试 |
| 403 | 权限不足（发布别人的图、user key 调 admin 接口、普通用户请求 2K / 4K 等） | n/a | 提示具体 `error`，不重试 |
| 404 | 画廊条目不存在 / 已下架 | n/a | 把对应卡片从列表中移除 |
| 429 | 号池没有可用配额（所有上游账户都被限流） | 已退还 | 提示"服务繁忙，稍后重试"，可定时重试 |
| 502 | 上游 ChatGPT Web 异常或网络错误 | 已退还 | 自动重试 1 次，仍失败弹错 |
| IO 异常 | 后端连不到 / DNS 失败 / TLS 失败 | n/a | 启动体检场景：清密钥跳登录；普通业务请求：错误条提示后保留登录态等下次重试（生图链路也不踢） |

**退额度规则总结**：

- 入口扣费：`/v1/images/*` 进入处理流程时按 `n` 整体预扣（admin / unlimited 跳过）
- 上游真实失败（5xx / 502 / 内容策略 / SE 中途断流 / 取消任务）→ 后端**自动退还**
- 用户错误（400 / 内容审查命中 / 鉴权失败）→ 走 fail-fast，**根本没扣**所以也不用退
- AP 端不需要做退额度逻辑，但生图链路 finally 里建议主动调一次 `/api/auth/me` 刷新本地 `remaining`，让"可用额度"数字立刻更新

错误响应统一格式：

```json
{ "detail": { "error": "具体错误信息" } }
```

或（部分接口）：

```json
{ "error": { "message": "...", "type": "...", "code": "..." } }
```

AP 端解析时两种都要兼容。

---

## 六、图片 URL 生命周期

后端把图存在 `data/images/` 下，按 `YYYY/MM/DD/` 分目录。**图片有过期时间**，由 `config.json` 里 `image_retention_days` 控制（默认 30 天）。

但单纯按 mtime 清会把"还在用的图"也删掉（画廊瓦片瞬间变裂图、用户作品凭空消失），所以从 v1.2.2 开始引入**两个保护开关**，默认开启：

| 配置项 | 默认 | 作用 |
|---|---|---|
| `cleanup_protect_gallery` | true | 已发布到画廊的图永不被自动清理 |
| `cleanup_protect_user_images` | true | 所有挂在 user key 名下的图永不被自动清理。匿名 / admin 生成的无归属图仍按 mtime 清 |

管理员可以在设置页关掉这两个开关回到一刀切清理。

**AP 端策略**（与服务端清理是否开启都建议遵守）：

1. **生成成功后立刻把图下载到本地缓存**（用户相册或 AP 私有目录）
2. **历史记录里只存本地路径**，不依赖远程 URL 长期存活
3. **远程 URL 仅作为生成后短期回显**，不要把它写进 Room 当唯一来源

为什么不能完全靠服务端保护开关？两个原因：

- 管理员可能关掉保护开关释放存储，AP 端不能假定一定开启
- 用户可能在多端使用（换设备 / 重装 AP）：本地缓存丢了，靠 `/api/me/images` 拉云端能恢复，但前提是云端 PNG 还在——保护开关让这个前提更稳

---

## 七、AP 端推荐架构

```
app/
├── data/
│   ├── api/
│   │   ├── DrawApi.kt              # Retrofit 接口
│   │   ├── SseClient.kt            # OkHttp + Flow 流式
│   │   └── dto/                    # ImageGenerationRequest / Response 等
│   ├── repository/
│   │   ├── DrawRepository.kt       # 业务封装
│   │   ├── HistoryRepository.kt    # 历史记录
│   │   └── AuthRepository.kt       # base url + token
│   ├── db/                          # Room
│   └── prefs/                       # DataStore
├── domain/                          # 业务模型（与 dto 解耦）
├── ui/
│   ├── compose/
│   │   ├── login/                   # 输入 base url + auth key
│   │   ├── generate/                # 主生成页
│   │   ├── history/                 # 历史记录
│   │   └── settings/                # 设置（base url、登出）
│   ├── components/                   # 可复用 Composable
│   └── theme/
└── di/                                # 简单的 manual DI 即可，不必 Hilt
```

---

## 八、需要后端配合的优化点

| 优先级 | 内容 | 现状 |
|---|---|---|
| P0 | `/v1/images/*` 接受 `response_format=url` | ✅ 已实现，AP 端显式传 `url` 即可拿到完整 http(s) URL |
| P1 | `/api/auth/me` 已加 `Cache-Control: no-store` | ✅ 已实现 |
| P1 | 上游真失败 / 取消任务自动退还预扣额度 | ✅ v1.2.2 实现，AP 端不需要手动退 |
| P1 | 用户密钥普通 / 高级等级 | ✅ 已实现。普通用户限制 free 账号池和 1K，高级用户可用 Plus / Team / Pro 与 2K / 4K |
| P1 | `/v1/images/*` 接受 `resolution=1k/2k/4k` | ✅ 已实现。2K / 4K 会走 Codex 高清路线，并保持后端强制权限校验 |
| P1 | 图片 429 限流账号自动标记与恢复 | ✅ 已实现。命中 429 / `rate_limit_exceeded` / `usage_limit_reached` 时跳过当前账号并按 reset header 恢复 |
| P1 | 画廊已发布 / 用户作品不被自动清理 | ✅ v1.2.2 实现，两个保护开关默认开启 |
| P1 | `/api/gallery/published/batch` 批量查询 | ✅ v1.2.2 实现，避免逐张 N+1 请求 |
| P2 | 流式事件加上 `error` 单独事件类型 | 暂未实现，目前用 `image.generation.message` 复用 |
| P3 | 提供 `/v1/images/sizes` 查询模型支持的尺寸 | 暂未实现，AP 端硬编码 `1:1` `16:9` `9:16` `4:3` `3:4` 即可 |
| P3 | 画廊 feed 支持 tag / 关键词筛选 | 暂未实现，画廊条目数大了再加 |

---

## 九、常用调试命令

后端起服务（开发）：

```bash
uv run main.py
# 默认监听 http://127.0.0.1:8000
```

curl 自测：

```bash
# 拉模型列表
curl -H "Authorization: Bearer YOUR_KEY" http://127.0.0.1:8000/v1/models

# 文生图（非流式）
curl -X POST http://127.0.0.1:8000/v1/images/generations \
  -H "Authorization: Bearer YOUR_KEY" \
  -H "Content-Type: application/json" \
  -d '{"prompt":"a cat","n":1,"size":"9:16","resolution":"1k","response_format":"url"}'

# 4K 竖图。需要高级用户密钥，后端会选 Pro / Plus / Team Codex 账号池
curl -X POST http://127.0.0.1:8000/v1/images/generations \
  -H "Authorization: Bearer YOUR_KEY" \
  -H "Content-Type: application/json" \
  -d '{"prompt":"一张极简高级的产品海报","n":1,"size":"9:16","resolution":"4k","response_format":"url"}'

# 文生图（流式）
curl -N -X POST http://127.0.0.1:8000/v1/images/generations \
  -H "Authorization: Bearer YOUR_KEY" \
  -H "Content-Type: application/json" \
  -d '{"prompt":"a cat","n":1,"response_format":"url","stream":true}'

# 拉额度
curl -H "Authorization: Bearer YOUR_KEY" http://127.0.0.1:8000/api/auth/me
```

---

## 十、常见问题

**Q：AP 端选了模型 `gpt-5` 但回复始终是 mini？**
A：`/v1/chat/completions` 这条线被上游 ChatGPT Web 的反爬策略限制，免费账号会被强制路由到轻量模型。**Draw AP 只关心画图，不需要文本对话**，无需关注此问题。

**Q：图片生成中途没有进度，结果直接出来了？**
A：上游 SSE 事件分布不固定，有时全程只推一个 result。AP 端不要假定一定会有 progress 事件。

**Q：n>1 时 result 一次返回还是分多次？**
A：流式模式下每张图触发一个 result（`index` 不同）；非流式模式整体一次返回 `data: [...]`。

**Q：上传图片大小限制？**
A：后端没硬限制，但建议 AP 端压到 4MB 以内，否则上行慢且容易触发上游限流。

---

## 修订记录

| 日期 | 版本 | 改动 |
|---|---|---|
| 2026-05-22 | v1.2.2 | 新增画廊接口（feed/publish/unpublish/published/batch）+ `/api/me/images` 章节；鉴权节加 IOException / 启动 me() 体检策略与 skip-unauth-hook 头使用；图片清理引入 `cleanup_protect_gallery` / `cleanup_protect_user_images` 两个保护开关；生图链路统一上游真失败 / 取消自动退还预扣额度；错误码表加退额度列；澄清画图模型只有 `gpt-image-2` / `codex-gpt-image-2` |
| 2026-05-20 | v1.2.1 | 初版 |
