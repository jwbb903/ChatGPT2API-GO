package app

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha3"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"math/rand"
	"mime"
	stdhttp "net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	http "github.com/bogdanfinn/fhttp"
	tlsclient "github.com/bogdanfinn/tls-client"
)

const chatGPTBaseURL = "https://chatgpt.com"
const defaultClientVersion = "prod-be885abbfcfe7b1f511e88b3003d9ee44757fbad"
const defaultClientBuild = "5955942"
const defaultPowScript = "https://chatgpt.com/backend-api/sentinel/sdk.js"
const bootstrapMaxAttempts = 3

type UpstreamClient struct {
	token           string
	client          upstreamHTTPDoer
	userAgent       string
	deviceID        string
	sessionID       string
	secCHUA         string
	secCHUAMobile   string
	secCHUAPlatform string
	scriptSources   []string
	dataBuild       string
}

type chatRequirements struct{ Token, ProofToken, TurnstileToken, SOToken string }

type upstreamImageResult struct {
	Bytes         []byte
	RevisedPrompt string
}

type UpstreamTextEvent struct {
	Delta          string
	ConversationID string
	MessageID      string
	CurrentNode    string
	FileIDs        []string
	SedimentIDs    []string
	Blocked        bool
	ToolInvoked    any
	TurnUseCase    string
	AccountToken   string
}

func NewUpstreamClient(token, proxyURL string, binGetter func() (string, error)) (*UpstreamClient, error) {
	return NewUpstreamClientForAccount(Account{AccessToken: token}, proxyURL, binGetter)
}

func NewUpstreamClientForAccount(account Account, proxyURL string, binGetter func() (string, error)) (*UpstreamClient, error) {
	token := account.AccessToken
	fp := accountFingerprint(account)
	transport := strings.ToLower(strings.TrimSpace(firstNonEmpty(os.Getenv("CHATGPT2API_UPSTREAM_TRANSPORT"), fp["impersonate"])))
	if transport == "curl" || transport == "curl-impersonate" || transport == "impersonate" {
		client, err := newCurlImpersonateClient(proxyURL, binGetter)
		if err != nil {
			return nil, err
		}
		return &UpstreamClient{token: token, client: client, userAgent: fp["user-agent"], deviceID: fp["oai-device-id"], sessionID: fp["oai-session-id"], secCHUA: fp["sec-ch-ua"], secCHUAMobile: fp["sec-ch-ua-mobile"], secCHUAPlatform: fp["sec-ch-ua-platform"], scriptSources: []string{defaultPowScript}}, nil
	}
	options := []tlsclient.HttpClientOption{
		tlsclient.WithClientProfile(upstreamTLSProfile()),
		tlsclient.WithTimeoutSeconds(0),
		// ChatGPT Web 当前主要走 H2；禁用 H3 可避免部分代理/移动网络下 QUIC 行为不一致。
		tlsclient.WithDisableHttp3(),
	}
	if strings.TrimSpace(proxyURL) != "" {
		if _, err := url.Parse(proxyURL); err != nil {
			return nil, err
		}
		options = append(options, tlsclient.WithProxyUrl(proxyURL))
	}
	client, err := tlsclient.NewHttpClient(tlsclient.NewNoopLogger(), options...)
	if err != nil {
		return nil, err
	}
	return &UpstreamClient{token: token, client: client, userAgent: fp["user-agent"], deviceID: fp["oai-device-id"], sessionID: fp["oai-session-id"], secCHUA: fp["sec-ch-ua"], secCHUAMobile: fp["sec-ch-ua-mobile"], secCHUAPlatform: fp["sec-ch-ua-platform"], scriptSources: []string{defaultPowScript}}, nil
}

func accountFingerprint(account Account) map[string]string {
	fp := map[string]string{}
	for k, v := range account.FP {
		if strings.TrimSpace(v) != "" {
			fp[strings.ToLower(strings.TrimSpace(k))] = strings.TrimSpace(v)
		}
	}
	defaults := map[string]string{
		"user-agent":         upstreamUserAgent(),
		"impersonate":        "edge_101",
		"oai-device-id":      uuid4(),
		"oai-session-id":     uuid4(),
		"sec-ch-ua":          upstreamSecCHUA(),
		"sec-ch-ua-mobile":   "?0",
		"sec-ch-ua-platform": `"Windows"`,
	}
	for k, v := range defaults {
		if strings.TrimSpace(fp[k]) == "" {
			fp[k] = v
		}
	}
	return fp
}

func (c *UpstreamClient) headers(path string, extra map[string]string) http.Header {
	h := http.Header{}
	h.Set("User-Agent", c.userAgent)
	h.Set("Origin", chatGPTBaseURL)
	h.Set("Referer", chatGPTBaseURL+"/")
	h.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8,en-US;q=0.7")
	h.Set("Cache-Control", "no-cache")
	h.Set("Pragma", "no-cache")
	h.Set("Priority", "u=1, i")
	h.Set("Sec-Ch-Ua", c.secCHUA)
	h.Set("Sec-Ch-Ua-Arch", `"x86"`)
	h.Set("Sec-Ch-Ua-Bitness", `"64"`)
	h.Set("Sec-Ch-Ua-Full-Version", `"143.0.3650.96"`)
	h.Set("Sec-Ch-Ua-Full-Version-List", `"Microsoft Edge";v="143.0.3650.96", "Chromium";v="143.0.7499.147", "Not A(Brand";v="24.0.0.0"`)
	h.Set("Sec-Ch-Ua-Mobile", c.secCHUAMobile)
	h.Set("Sec-Ch-Ua-Model", `""`)
	h.Set("Sec-Ch-Ua-Platform", c.secCHUAPlatform)
	h.Set("Sec-Ch-Ua-Platform-Version", `"19.0.0"`)
	h.Set("Sec-Fetch-Dest", "empty")
	h.Set("Sec-Fetch-Mode", "cors")
	h.Set("Sec-Fetch-Site", "same-origin")
	h.Set("OAI-Device-Id", c.deviceID)
	h.Set("OAI-Session-Id", c.sessionID)
	h.Set("OAI-Language", "zh-CN")
	h.Set("OAI-Client-Version", defaultClientVersion)
	h.Set("OAI-Client-Build-Number", defaultClientBuild)
	h.Set("X-OpenAI-Target-Path", path)
	h.Set("X-OpenAI-Target-Route", path)
	if c.token != "" {
		h.Set("Authorization", "Bearer "+c.token)
	}
	for k, v := range extra {
		h.Set(k, v)
	}
	h[http.HeaderOrderKey] = []string{
		"user-agent", "accept", "content-type", "authorization", "origin", "referer", "accept-language",
		"cache-control", "pragma", "priority", "sec-ch-ua", "sec-ch-ua-arch", "sec-ch-ua-bitness",
		"sec-ch-ua-full-version", "sec-ch-ua-full-version-list", "sec-ch-ua-mobile", "sec-ch-ua-model",
		"sec-ch-ua-platform", "sec-ch-ua-platform-version", "sec-fetch-dest", "sec-fetch-mode", "sec-fetch-site",
		"oai-device-id", "oai-session-id",
		"oai-language", "oai-client-version", "oai-client-build-number", "x-openai-target-path",
		"x-openai-target-route", "openai-sentinel-chat-requirements-token", "openai-sentinel-proof-token",
		"openai-sentinel-turnstile-token", "openai-sentinel-so-token", "x-conduit-token", "x-oai-turn-trace-id",
	}
	return h
}
func (c *UpstreamClient) do(ctx context.Context, method, path string, headers map[string]string, body any, stream bool) (*http.Response, error) {
	var rdr io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rdr = bytes.NewReader(b)
	}
	traceLogf(ctx, "│  ├─ upstream request %s %s stream=%v", method, path, stream)
	traceLogf(ctx, "│  │  headers: %s", traceHeaderPreview(headers))
	traceLogf(ctx, "│  │  body: %s", traceBodyPreview(body))
	start := time.Now()
	req, err := http.NewRequestWithContext(ctx, method, chatGPTBaseURL+path, rdr)
	if err != nil {
		traceLogf(ctx, "│  └─ upstream request build failed duration=%s error=%v", traceHTTPDuration(start), err)
		return nil, err
	}
	req.Header = c.headers(strings.Split(path, "?")[0], headers)
	if body != nil && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.client.Do(req)
	if err != nil {
		traceLogf(ctx, "│  └─ upstream network failed duration=%s error=%v", traceHTTPDuration(start), err)
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		defer resp.Body.Close()
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		traceLogf(ctx, "│  └─ upstream response status=%d duration=%s error_body=%s", resp.StatusCode, traceHTTPDuration(start), truncateText(string(b), 800))
		return nil, fmt.Errorf("%s %s failed: status=%d body=%s", method, path, resp.StatusCode, string(b))
	}
	traceLogf(ctx, "│  └─ upstream response status=%d duration=%s", resp.StatusCode, traceHTTPDuration(start))
	if !stream {
		return resp, nil
	}
	return resp, nil
}
func (c *UpstreamClient) bootstrapHeaders() http.Header {
	h := http.Header{}
	h.Set("User-Agent", c.userAgent)
	h.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8")
	h.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")
	h.Set("Sec-Ch-Ua", c.secCHUA)
	h.Set("Sec-Ch-Ua-Mobile", c.secCHUAMobile)
	h.Set("Sec-Ch-Ua-Platform", c.secCHUAPlatform)
	h.Set("Sec-Fetch-Dest", "document")
	h.Set("Sec-Fetch-Mode", "navigate")
	h.Set("Sec-Fetch-Site", "none")
	h.Set("Sec-Fetch-User", "?1")
	h.Set("Upgrade-Insecure-Requests", "1")
	h[http.HeaderOrderKey] = []string{
		"user-agent", "accept", "accept-language", "sec-ch-ua", "sec-ch-ua-mobile",
		"sec-ch-ua-platform", "sec-fetch-dest", "sec-fetch-mode", "sec-fetch-site",
		"sec-fetch-user", "upgrade-insecure-requests",
	}
	return h
}
func (c *UpstreamClient) bootstrap(ctx context.Context) error {
	var lastErr error
	for attempt := 1; attempt <= bootstrapMaxAttempts; attempt++ {
		if attempt > 1 {
			delay := time.Duration(attempt-1) * 300 * time.Millisecond
			traceLogf(ctx, "│  ├─ bootstrap retry attempt=%d delay=%s previous_error=%v", attempt, delay, lastErr)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
			}
		}
		err := c.bootstrapOnce(ctx)
		if err == nil {
			return nil
		}
		lastErr = err
		if !isRetryableBootstrapError(err) {
			return err
		}
	}
	return lastErr
}

func (c *UpstreamClient) bootstrapOnce(ctx context.Context) error {
	traceLogf(ctx, "│  ├─ step bootstrap ChatGPT home page")
	start := time.Now()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, chatGPTBaseURL+"/", nil)
	req.Header = c.bootstrapHeaders()
	traceLogf(ctx, "│  │  request GET / headers: %s", traceHeaderPreview(req.Header))
	resp, err := c.client.Do(req)
	if err != nil {
		traceLogf(ctx, "│  └─ bootstrap failed duration=%s error=%v", traceHTTPDuration(start), err)
		return fmt.Errorf("bootstrap failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		location := strings.TrimSpace(resp.Header.Get("Location"))
		traceLogf(ctx, "│  └─ bootstrap response status=%d location=%q duration=%s body=%s", resp.StatusCode, location, traceHTTPDuration(start), truncateText(string(b), 800))
		if resp.StatusCode >= 300 && resp.StatusCode < 400 {
			return fmt.Errorf("bootstrap redirect: status=%d location=%s body=%s", resp.StatusCode, location, string(b))
		}
		return fmt.Errorf("bootstrap failed: status=%d body=%s", resp.StatusCode, string(b))
	}
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	c.scriptSources, c.dataBuild = parsePowResources(string(b))
	if len(c.scriptSources) == 0 {
		c.scriptSources = []string{defaultPowScript}
	}
	traceLogf(ctx, "│  └─ bootstrap ok status=%d duration=%s pow_scripts=%d data_build=%q", resp.StatusCode, traceHTTPDuration(start), len(c.scriptSources), c.dataBuild)
	return nil
}

func isRetryableBootstrapError(err error) bool {
	if err == nil {
		return false
	}
	text := strings.ToLower(err.Error())
	if !strings.Contains(text, "bootstrap") {
		return false
	}
	if strings.Contains(text, "status=401") || strings.Contains(text, "status=403") {
		return false
	}
	if strings.Contains(text, "redirect") || strings.Contains(text, "status=3") || strings.Contains(text, "status=429") {
		return true
	}
	for _, code := range []string{"status=500", "status=502", "status=503", "status=504"} {
		if strings.Contains(text, code) {
			return true
		}
	}
	return !strings.Contains(text, "status=")
}
func (c *UpstreamClient) chatRequirements(ctx context.Context) (chatRequirements, error) {
	traceLogf(ctx, "│  ├─ step build sentinel chat requirements")
	path := "/backend-api/sentinel/chat-requirements"
	if c.token == "" {
		path = "/backend-anon/sentinel/chat-requirements"
	}
	traceLogf(ctx, "│  │  calculating legacy proof input")
	p := buildLegacyRequirementsToken(c.userAgent, c.scriptSources, c.dataBuild)
	resp, err := c.do(ctx, http.MethodPost, path, map[string]string{"Content-Type": "application/json"}, map[string]any{"p": p}, false)
	if err != nil {
		return chatRequirements{}, err
	}
	defer resp.Body.Close()
	var data map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return chatRequirements{}, err
	}
	if ark, ok := data["arkose"].(map[string]any); ok && boolAny(ark["required"], false) {
		return chatRequirements{}, errors.New("upstream requires arkose token; not implemented")
	}
	cr := chatRequirements{Token: strAny(data["token"], ""), SOToken: strAny(data["so_token"], "")}
	if pow, ok := data["proofofwork"].(map[string]any); ok && boolAny(pow["required"], false) {
		proof, err := buildProofToken(strAny(pow["seed"], ""), strAny(pow["difficulty"], ""), c.userAgent, c.scriptSources, c.dataBuild)
		if err != nil {
			return cr, err
		}
		cr.ProofToken = proof
	}
	if turnstile, ok := data["turnstile"].(map[string]any); ok && boolAny(turnstile["required"], false) {
		turnstileSourceP := p
		if c.token != "" {
			turnstileSourceP = ""
		}
		if dx := strAny(turnstile["dx"], ""); dx != "" {
			cr.TurnstileToken = solveTurnstileToken(dx, turnstileSourceP)
		}
	}
	if cr.Token == "" {
		traceLogf(ctx, "│  └─ sentinel failed missing requirements token")
		return cr, errors.New("missing chat requirements token")
	}
	traceLogf(ctx, "│  └─ sentinel ok token=%v proof=%v turnstile=%v so=%v", cr.Token != "", cr.ProofToken != "", cr.TurnstileToken != "", cr.SOToken != "")
	return cr, nil
}
func (c *UpstreamClient) conversationHeaders(path string, cr chatRequirements) map[string]string {
	h := map[string]string{"Accept": "text/event-stream", "Content-Type": "application/json", "OpenAI-Sentinel-Chat-Requirements-Token": cr.Token}
	if cr.ProofToken != "" {
		h["OpenAI-Sentinel-Proof-Token"] = cr.ProofToken
	}
	if cr.TurnstileToken != "" {
		h["OpenAI-Sentinel-Turnstile-Token"] = cr.TurnstileToken
	}
	if cr.SOToken != "" {
		h["OpenAI-Sentinel-SO-Token"] = cr.SOToken
	}
	return h
}
func (c *UpstreamClient) imageHeaders(path string, cr chatRequirements, conduit, accept string) map[string]string {
	h := map[string]string{"Content-Type": "application/json", "Accept": accept, "OpenAI-Sentinel-Chat-Requirements-Token": cr.Token}
	if cr.ProofToken != "" {
		h["OpenAI-Sentinel-Proof-Token"] = cr.ProofToken
	}
	if cr.TurnstileToken != "" {
		h["OpenAI-Sentinel-Turnstile-Token"] = cr.TurnstileToken
	}
	if cr.SOToken != "" {
		h["OpenAI-Sentinel-SO-Token"] = cr.SOToken
	}
	if conduit != "" {
		h["X-Conduit-Token"] = conduit
	}
	if accept == "text/event-stream" {
		h["X-Oai-Turn-Trace-Id"] = uuid4()
	}
	return h
}

func (c *UpstreamClient) StreamText(ctx context.Context, messages []map[string]any, model string) (<-chan string, <-chan error) {
	return c.StreamTextConversation(ctx, messages, model, "")
}

func (c *UpstreamClient) StreamTextConversation(ctx context.Context, messages []map[string]any, model, conversationID string) (<-chan string, <-chan error) {
	events, errs := c.StreamTextConversationEvents(ctx, messages, model, conversationID)
	out := make(chan string)
	go func() {
		defer close(out)
		for ev := range events {
			if ev.Delta != "" {
				out <- ev.Delta
			}
		}
	}()
	return out, errs
}

func (c *UpstreamClient) StreamTextConversationEvents(ctx context.Context, messages []map[string]any, model, conversationID string) (<-chan UpstreamTextEvent, <-chan error) {
	return c.streamTextConversationEvents(ctx, messages, model, conversationID, true)
}

func (c *UpstreamClient) StreamChatConversationEvents(ctx context.Context, messages []map[string]any, model, conversationID string) (<-chan UpstreamTextEvent, <-chan error) {
	return c.streamTextConversationEvents(ctx, messages, model, conversationID, false)
}

func (c *UpstreamClient) streamTextConversationEvents(ctx context.Context, messages []map[string]any, model, conversationID string, historyDisabled bool) (<-chan UpstreamTextEvent, <-chan error) {
	out := make(chan UpstreamTextEvent)
	errs := make(chan error, 1)
	go func() {
		defer close(out)
		defer close(errs)
		traceLogf(ctx, "├─ text stream start model=%s history_disabled=%v conversation_id=%s messages=%d", model, historyDisabled, maskedValue(conversationID), len(messages))
		if err := c.bootstrap(ctx); err != nil {
			errs <- err
			return
		}
		cr, err := c.chatRequirements(ctx)
		if err != nil {
			errs <- err
			return
		}
		if model == "" {
			model = "auto"
		}
		parentID := uuid4()
		conversationID = strings.TrimSpace(conversationID)
		if conversationID != "" {
			if conv, err := c.getConversation(ctx, conversationID); err == nil {
				if cur := strings.TrimSpace(strAny(conv["current_node"], "")); cur != "" {
					parentID = cur
				}
			}
		}
		traceLogf(ctx, "│  ├─ step convert messages to ChatGPT conversation format")
		conversationMessages, err := c.toConversationMessages(ctx, messages)
		if err != nil {
			errs <- err
			return
		}
		payload := map[string]any{"action": "next", "messages": conversationMessages, "model": model, "parent_message_id": parentID, "conversation_mode": map[string]any{"kind": "primary_assistant"}, "conversation_origin": nil, "force_paragen": false, "force_paragen_model_slug": "", "force_rate_limit": false, "force_use_sse": true, "history_and_training_disabled": historyDisabled, "reset_rate_limits": false, "suggestions": []any{}, "supported_encodings": []string{}, "system_hints": []string{}, "timezone": "Asia/Shanghai", "timezone_offset_min": -480, "variant_purpose": "comparison_implicit", "websocket_request_id": uuid4(), "client_contextual_info": clientContext()}
		if conversationID != "" {
			payload["conversation_id"] = conversationID
			payload["history_and_training_disabled"] = false
		}
		path := "/backend-api/conversation"
		if c.token == "" {
			path = "/backend-anon/conversation"
		}
		traceLogf(ctx, "│  ├─ step open upstream SSE conversation")
		resp, err := c.do(ctx, http.MethodPost, path, c.conversationHeaders(path, cr), payload, true)
		if err != nil {
			errs <- err
			return
		}
		defer resp.Body.Close()
		historyMessages := assistantHistoryMessages(messages)
		historyText := strings.Join(historyMessages, "")
		state := newUpstreamConversationState(historyText, historyMessages)
		eventCount := 0
		for ev := range parseSSE(resp.Body) {
			if ev == "[DONE]" {
				traceLogf(ctx, "└─ text stream done events=%d", eventCount)
				return
			}
			eventCount++
			if meta, ok := state.Apply(ev); ok {
				meta.AccountToken = c.token
				out <- meta
			}
		}
	}()
	return out, errs
}

func (c *UpstreamClient) GenerateImage(ctx context.Context, prompt, model, size, resolution string, refs [][]byte) ([]upstreamImageResult, error) {
	traceLogf(ctx, "├─ image generation start model=%s size=%s resolution=%s refs=%d", model, size, resolution, len(refs))
	if c.token == "" {
		return nil, errors.New("access token is required for image generation")
	}
	if isCodexImageRequest(model, resolution) {
		traceLogf(ctx, "│  ├─ route to Codex image path")
		return c.GenerateCodexImage(ctx, buildImagePrompt(prompt, size), model, codexImageSize(size, resolution), refs)
	}
	finalPrompt := buildImagePromptWithOptions(prompt, size, resolution)
	if err := c.bootstrap(ctx); err != nil {
		return nil, err
	}
	cr, err := c.chatRequirements(ctx)
	if err != nil {
		return nil, err
	}
	uploads := []map[string]any{}
	for i, b := range refs {
		traceLogf(ctx, "│  ├─ step upload reference image %d/%d bytes=%d", i+1, len(refs), len(b))
		up, err := c.uploadImage(ctx, b, fmt.Sprintf("image_%d.png", i+1))
		if err != nil {
			return nil, err
		}
		uploads = append(uploads, up)
	}
	traceLogf(ctx, "│  ├─ step prepare image conversation")
	conduit, err := c.prepareImage(ctx, finalPrompt, model, cr)
	if err != nil {
		return nil, err
	}
	traceLogf(ctx, "│  ├─ step start image SSE conversation uploads=%d", len(uploads))
	resp, err := c.startImage(ctx, finalPrompt, model, cr, conduit, uploads)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	cid := ""
	fileIDs := []string{}
	sedimentIDs := []string{}
	message := ""
	state := newUpstreamConversationState("", nil)
	generated := false
	// 用 120s 超时读取 SSE，防止无限挂起
	sseCtx, sseCancel := context.WithTimeout(ctx, 120*time.Second)
	defer sseCancel()
	// 从 resp.Body 读取 SSE，受 sseCtx 控制
	done := make(chan struct{}, 1)
	go func() {
		defer func() { done <- struct{}{} }()
		for ev := range parseSSE(resp.Body) {
			if ev == "[DONE]" {
				generated = true
				return
			}
			updateImageState(ev, &cid, &message)
			if meta, ok := state.Apply(ev); ok {
				if meta.ConversationID != "" {
					cid = meta.ConversationID
				}
				if meta.Delta != "" || state.Text != "" {
					message = state.Text
				}
			}
		}
		generated = true
	}()
	select {
	case <-done:
	case <-sseCtx.Done():
		if !generated {
			return nil, errors.New("image generation SSE timed out (120s)")
		}
	}
	if message != "" && len(fileIDs) == 0 && len(sedimentIDs) == 0 && (state.Blocked || state.ToolInvoked == false || state.TurnUseCase == "text") {
		return nil, errors.New(cleanImageMessage(message))
	}
	if cid != "" && !isImageQuotaMessage(message) {
		traceLogf(ctx, "│  ├─ step poll final image tool records conversation=%s", maskedValue(cid))
		f, s := c.pollImageIDs(ctx, cid, 120*time.Second)
		fileIDs = append(fileIDs, f...)
		sedimentIDs = append(sedimentIDs, s...)
	}
	traceLogf(ctx, "│  ├─ step resolve image download URLs file_ids=%d sediment_ids=%d", len(fileIDs), len(sedimentIDs))
	urls, err := c.resolveImageURLs(ctx, cid, fileIDs, sedimentIDs)
	if err != nil {
		return nil, err
	}
	if len(urls) == 0 {
		if message != "" {
			return nil, errors.New(cleanImageMessage(message))
		}
		return nil, errors.New("upstream returned no image")
	}
	results := []upstreamImageResult{}
	for i, u := range urls {
		traceLogf(ctx, "│  ├─ step download generated image %d/%d", i+1, len(urls))
		b, err := c.download(ctx, u)
		if err != nil {
			return nil, err
		}
		results = append(results, upstreamImageResult{Bytes: b, RevisedPrompt: prompt})
	}
	traceLogf(ctx, "└─ image generation done images=%d", len(results))
	return results, nil
}
func (c *UpstreamClient) prepareImage(ctx context.Context, prompt, model string, cr chatRequirements) (string, error) {
	path := "/backend-api/f/conversation/prepare"
	payload := map[string]any{"action": "next", "fork_from_shared_post": false, "parent_message_id": uuid4(), "model": imageModelSlug(model), "client_prepare_state": "success", "timezone_offset_min": -480, "timezone": "Asia/Shanghai", "conversation_mode": map[string]any{"kind": "primary_assistant"}, "system_hints": []string{"picture_v2"}, "partial_query": map[string]any{"id": uuid4(), "author": map[string]any{"role": "user"}, "content": map[string]any{"content_type": "text", "parts": []string{prompt}}}, "supports_buffering": true, "supported_encodings": []string{"v1"}, "client_contextual_info": map[string]any{"app_name": "chatgpt.com"}}
	resp, err := c.do(ctx, http.MethodPost, path, c.imageHeaders(path, cr, "", "*/*"), payload, false)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var data map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&data)
	token := strAny(data["conduit_token"], "")
	if token == "" {
		return "", errors.New("missing conduit_token")
	}
	return token, nil
}
func (c *UpstreamClient) startImage(ctx context.Context, prompt, model string, cr chatRequirements, conduit string, refs []map[string]any) (*http.Response, error) {
	path := "/backend-api/f/conversation"
	parts := []any{}
	attachments := []map[string]any{}
	for _, r := range refs {
		parts = append(parts, map[string]any{"content_type": "image_asset_pointer", "asset_pointer": "file-service://" + strAny(r["file_id"], ""), "width": r["width"], "height": r["height"], "size_bytes": r["file_size"]})
		attachments = append(attachments, map[string]any{"id": r["file_id"], "mimeType": r["mime_type"], "name": r["file_name"], "size": r["file_size"], "width": r["width"], "height": r["height"]})
	}
	parts = append(parts, prompt)
	content := map[string]any{"content_type": "text", "parts": []string{prompt}}
	if len(refs) > 0 {
		content = map[string]any{"content_type": "multimodal_text", "parts": parts}
	}
	metadata := map[string]any{"developer_mode_connector_ids": []any{}, "selected_github_repos": []any{}, "selected_all_github_repos": false, "system_hints": []string{"picture_v2"}, "serialization_metadata": map[string]any{"custom_symbol_offsets": []any{}}}
	if len(attachments) > 0 {
		metadata["attachments"] = attachments
	}
	payload := map[string]any{"action": "next", "messages": []any{map[string]any{"id": uuid4(), "author": map[string]any{"role": "user"}, "create_time": float64(time.Now().UnixNano()) / 1e9, "content": content, "metadata": metadata}}, "parent_message_id": uuid4(), "model": imageModelSlug(model), "client_prepare_state": "sent", "timezone_offset_min": -480, "timezone": "Asia/Shanghai", "conversation_mode": map[string]any{"kind": "primary_assistant"}, "enable_message_followups": true, "system_hints": []string{"picture_v2"}, "supports_buffering": true, "supported_encodings": []string{"v1"}, "client_contextual_info": clientContext(), "paragen_cot_summary_display_override": "allow", "force_parallel_switch": "auto"}
	return c.do(ctx, http.MethodPost, path, c.imageHeaders(path, cr, conduit, "text/event-stream"), payload, true)
}
func (c *UpstreamClient) uploadImage(ctx context.Context, data []byte, name string) (map[string]any, error) {
	traceLogf(ctx, "│  ├─ upload image begin name=%s bytes=%d", name, len(data))
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		cfg.Width = 1024
		cfg.Height = 1024
	}
	mimeType := stdhttp.DetectContentType(data)
	if ext := mime.TypeByExtension(filepath.Ext(name)); ext != "" {
		mimeType = ext
	}
	path := "/backend-api/files"
	meta := map[string]any{"file_name": name, "file_size": len(data), "use_case": "multimodal", "width": cfg.Width, "height": cfg.Height}
	traceLogf(ctx, "│  │  image metadata width=%d height=%d mime=%s", cfg.Width, cfg.Height, mimeType)
	resp, err := c.do(ctx, http.MethodPost, path, map[string]string{"Content-Type": "application/json", "Accept": "application/json"}, meta, false)
	if err != nil {
		return nil, err
	}
	var up map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&up)
	resp.Body.Close()
	uploadURL := strAny(up["upload_url"], "")
	fileID := strAny(up["file_id"], "")
	if uploadURL == "" || fileID == "" {
		return nil, errors.New("file upload metadata missing")
	}
	putReq, err := stdhttp.NewRequestWithContext(ctx, stdhttp.MethodPut, uploadURL, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	putReq.ContentLength = int64(len(data))
	putReq.Header.Set("Content-Type", mimeType)
	putReq.Header.Set("x-ms-blob-type", "BlockBlob")
	putReq.Header.Set("x-ms-version", "2020-04-08")
	putReq.Header.Set("Origin", chatGPTBaseURL)
	putReq.Header.Set("Referer", chatGPTBaseURL+"/")
	putReq.Header.Set("User-Agent", c.userAgent)
	putReq.Header.Set("Accept", "application/json, text/plain, */*")
	putReq.Header.Set("Accept-Language", "en-US,en;q=0.8")
	traceLogf(ctx, "│  ├─ object storage PUT %s bytes=%d headers: %s", safeURLForLog(uploadURL), len(data), traceHeaderPreview(putReq.Header))
	putStart := time.Now()
	put, err := (&stdhttp.Client{Timeout: 120 * time.Second}).Do(putReq)
	if err != nil {
		traceLogf(ctx, "│  └─ object storage PUT failed duration=%s error=%v", traceHTTPDuration(putStart), err)
		return nil, err
	}
	putBody, _ := io.ReadAll(io.LimitReader(put.Body, 2048))
	put.Body.Close()
	if put.StatusCode < 200 || put.StatusCode >= 300 {
		traceLogf(ctx, "│  └─ object storage PUT response status=%d duration=%s body=%s", put.StatusCode, traceHTTPDuration(putStart), truncateText(strings.TrimSpace(string(putBody)), 500))
		return nil, fmt.Errorf("image upload failed: status=%d body=%s", put.StatusCode, strings.TrimSpace(string(putBody)))
	}
	traceLogf(ctx, "│  └─ object storage PUT ok status=%d duration=%s", put.StatusCode, traceHTTPDuration(putStart))
	done := fmt.Sprintf("/backend-api/files/%s/uploaded", fileID)
	resp, err = c.do(ctx, http.MethodPost, done, map[string]string{"Content-Type": "application/json", "Accept": "application/json"}, map[string]any{}, false)
	if err != nil {
		return nil, err
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	traceLogf(ctx, "│  └─ upload image done file_id=%s", maskedValue(fileID))
	return map[string]any{"file_id": fileID, "file_name": name, "file_size": len(data), "mime_type": mimeType, "width": cfg.Width, "height": cfg.Height}, nil
}
func (c *UpstreamClient) pollImageIDs(ctx context.Context, cid string, timeout time.Duration) ([]string, []string) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conv, err := c.getConversation(ctx, cid)
		if err == nil {
			f, s := extractToolIDs(conv)
			if len(f) > 0 || len(s) > 0 {
				return f, s
			}
		}
		time.Sleep(4 * time.Second)
	}
	return nil, nil
}
func (c *UpstreamClient) getConversation(ctx context.Context, cid string) (map[string]any, error) {
	path := "/backend-api/conversation/" + cid
	resp, err := c.do(ctx, http.MethodGet, path, map[string]string{"Accept": "application/json"}, nil, false)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var data map[string]any
	err = json.NewDecoder(resp.Body).Decode(&data)
	return data, err
}
func (c *UpstreamClient) resolveImageURLs(ctx context.Context, cid string, fileIDs, sedimentIDs []string) ([]string, error) {
	urls := []string{}
	seen := map[string]bool{}
	for _, id := range unique(fileIDs) {
		if id == "" || id == "file_upload" {
			continue
		}
		path := "/backend-api/files/" + id + "/download"
		resp, err := c.do(ctx, http.MethodGet, path, map[string]string{"Accept": "application/json"}, nil, false)
		if err != nil {
			continue
		}
		var data map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&data)
		resp.Body.Close()
		u := strAny(data["download_url"], strAny(data["url"], ""))
		if u != "" && !seen[u] {
			seen[u] = true
			urls = append(urls, u)
		}
	}
	if len(urls) > 0 || cid == "" {
		return urls, nil
	}
	for _, id := range unique(sedimentIDs) {
		path := "/backend-api/conversation/" + cid + "/attachment/" + id + "/download"
		resp, err := c.do(ctx, http.MethodGet, path, map[string]string{"Accept": "application/json"}, nil, false)
		if err != nil {
			continue
		}
		var data map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&data)
		resp.Body.Close()
		u := strAny(data["download_url"], strAny(data["url"], ""))
		if u != "" && !seen[u] {
			seen[u] = true
			urls = append(urls, u)
		}
	}
	return urls, nil
}
func (c *UpstreamClient) download(ctx context.Context, u string) ([]byte, error) {
	traceLogf(ctx, "│  ├─ download request %s", safeURLForLog(u))
	start := time.Now()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("Accept", "image/avif,image/webp,image/apng,image/*,*/*;q=0.8")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")
	req.Header.Set("Referer", chatGPTBaseURL+"/")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	req.Header[http.HeaderOrderKey] = []string{"user-agent", "accept", "accept-language", "referer", "authorization"}
	resp, err := c.client.Do(req)
	if err != nil {
		traceLogf(ctx, "│  └─ download failed duration=%s error=%v", traceHTTPDuration(start), err)
		return nil, fmt.Errorf("download failed: %v (url: %.100s)", err, u)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 200))
		traceLogf(ctx, "│  └─ download response status=%d duration=%s body=%s", resp.StatusCode, traceHTTPDuration(start), truncateText(string(b), 200))
		return nil, fmt.Errorf("download failed: status=%d body=%.100s", resp.StatusCode, string(b))
	}
	data, err := io.ReadAll(resp.Body)
	traceLogf(ctx, "│  └─ download ok status=%d bytes=%d duration=%s", resp.StatusCode, len(data), traceHTTPDuration(start))
	return data, err
}
func (c *UpstreamClient) DeleteConversation(ctx context.Context, cid string) {
	cid = strings.TrimSpace(cid)
	if cid == "" || c.token == "" {
		return
	}
	path := "/backend-api/conversation/" + cid
	resp, err := c.do(ctx, http.MethodPatch, path, map[string]string{"Content-Type": "application/json", "Accept": "application/json"}, map[string]any{"is_visible": false}, false)
	if err == nil && resp != nil {
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}
}

func (c *UpstreamClient) ListModels(ctx context.Context) (map[string]any, error) {
	if err := c.bootstrap(ctx); err != nil {
		return nil, err
	}
	path := "/backend-api/models?history_and_training_disabled=false"
	route := "/backend-api/models"
	if c.token == "" {
		path = "/backend-anon/models?iim=false&is_gizmo=false"
		route = "/backend-anon/models"
	}
	resp, err := c.do(ctx, http.MethodGet, path, map[string]string{"Accept": "application/json", "X-OpenAI-Target-Path": route, "X-OpenAI-Target-Route": route}, nil, false)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var payload map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	data := []map[string]any{}
	if models, ok := payload["models"].([]any); ok {
		for _, raw := range models {
			if m, ok := raw.(map[string]any); ok {
				slug := strings.TrimSpace(strAny(m["slug"], ""))
				if slug != "" && !seen[slug] {
					seen[slug] = true
					data = append(data, map[string]any{"id": slug, "object": "model", "created": intAny(m["created"], 0), "owned_by": strAny(m["owned_by"], "chatgpt"), "permission": []any{}, "root": slug, "parent": nil})
				}
			}
		}
	}
	for _, slug := range []string{"gpt-image-2", "codex-gpt-image-2"} {
		if !seen[slug] {
			data = append(data, map[string]any{"id": slug, "object": "model", "created": time.Now().Unix(), "owned_by": "chatgpt2api-go", "permission": []any{}, "root": slug, "parent": nil})
		}
	}
	sort.Slice(data, func(i, j int) bool { return strAny(data[i]["id"], "") < strAny(data[j]["id"], "") })
	return map[string]any{"object": "list", "data": data}, nil
}

func (c *UpstreamClient) GetUserInfo(ctx context.Context) (Account, error) {
	if c.token == "" {
		return Account{}, errors.New("access_token is required")
	}
	me, err := c.getJSON(ctx, http.MethodGet, "/backend-api/me", map[string]string{"Accept": "application/json"}, nil)
	if err != nil {
		return Account{}, err
	}
	initPayload, initErr := c.getJSON(ctx, http.MethodPost, "/backend-api/conversation/init", map[string]string{"Content-Type": "application/json", "Accept": "application/json"}, map[string]any{"gizmo_id": nil, "requested_default_model": nil, "conversation_id": nil, "timezone_offset_min": -480})
	accountPayload, accountErr := c.getJSON(ctx, http.MethodGet, "/backend-api/accounts/check/v4-2023-04-27?timezone_offset_min=-480", map[string]string{"Accept": "application/json", "X-OpenAI-Target-Path": "/backend-api/accounts/check/v4-2023-04-27", "X-OpenAI-Target-Route": "/backend-api/accounts/check/v4-2023-04-27"}, nil)
	planType := "free"
	if accountErr == nil {
		if accounts, ok := accountPayload["accounts"].(map[string]any); ok {
			if def, ok := accounts["default"].(map[string]any); ok {
				if acct, ok := def["account"].(map[string]any); ok {
					planType = normalizeAccountType(strAny(acct["plan_type"], planType))
				}
			}
		}
	}
	quota, restoreAt, quotaUnknown, limits := 0, "", true, []map[string]any{}
	defaultModel := ""
	if initErr == nil {
		if arr, ok := initPayload["limits_progress"].([]any); ok {
			for _, item := range arr {
				m, ok := item.(map[string]any)
				if !ok {
					continue
				}
				limits = append(limits, m)
				if strAny(m["feature_name"], "") == "image_gen" {
					quota = intAny(m["remaining"], 0)
					restoreAt = strAny(m["reset_after"], strAny(m["resets_at"], ""))
					quotaUnknown = false
				}
			}
		}
		defaultModel = strAny(initPayload["default_model_slug"], "")
	}
	status := "正常"
	if !quotaUnknown && quota <= 0 {
		status = "限流"
	}
	acc := Account{AccessToken: c.token, Type: planType, Status: status, SourceType: "web", Quota: quota, ImageQuotaUnknown: quotaUnknown, LimitsProgress: limits}
	if e := strAny(me["email"], ""); e != "" {
		acc.Email = &e
	}
	if uid := strAny(me["id"], ""); uid != "" {
		acc.UserID = &uid
	}
	if defaultModel != "" {
		acc.DefaultModelSlug = &defaultModel
	}
	if restoreAt != "" {
		acc.RestoreAt = &restoreAt
		acc.RateLimitResetAt = &restoreAt
	}
	if acc.InitialQuota < acc.Quota {
		acc.InitialQuota = acc.Quota
	}
	return acc, nil
}

func (c *UpstreamClient) getJSON(ctx context.Context, method, path string, headers map[string]string, body any) (map[string]any, error) {
	resp, err := c.do(ctx, method, path, headers, body, false)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var data map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}
	return data, nil
}

func parseSSE(r io.Reader) <-chan string {
	ch := make(chan string)
	go func() {
		defer close(ch)
		scanner := bufio.NewScanner(r)
		scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
		var lines []string
		flush := func() {
			if len(lines) == 0 {
				return
			}
			parts := []string{}
			for _, l := range lines {
				if strings.HasPrefix(l, "data:") {
					parts = append(parts, strings.TrimSpace(strings.TrimPrefix(l, "data:")))
				}
			}
			if len(parts) > 0 {
				ch <- strings.Join(parts, "\n")
			}
			lines = nil
		}
		for scanner.Scan() {
			line := scanner.Text()
			if strings.TrimSpace(line) == "" {
				flush()
				continue
			}
			lines = append(lines, line)
		}
		flush()
	}()
	return ch
}
func assistantDelta(payload string, current *string) string {
	state := newUpstreamConversationState("", nil)
	state.Text = *current
	state.CleanText = *current
	meta, ok := state.Apply(payload)
	*current = state.Text
	if !ok {
		return ""
	}
	return meta.Delta
}

type upstreamConversationState struct {
	Text            string
	CleanText       string
	HistoryText     string
	HistoryMessages []string
	HistoryIndex    int
	ConversationID  string
	MessageID       string
	CurrentNode     string
	Blocked         bool
	ToolInvoked     any
	TurnUseCase     string
	FileIDs         []string
	SedimentIDs     []string
}

func newUpstreamConversationState(historyText string, historyMessages []string) *upstreamConversationState {
	return &upstreamConversationState{HistoryText: historyText, HistoryMessages: historyMessages}
}

func (s *upstreamConversationState) Apply(payload string) (UpstreamTextEvent, bool) {
	meta := upstreamTextMeta(payload)
	s.mergeMeta(meta)
	var ev map[string]any
	if json.Unmarshal([]byte(payload), &ev) != nil {
		meta = s.snapshotMeta()
		return meta, meta.ConversationID != "" || meta.MessageID != "" || meta.CurrentNode != ""
	}
	s.updateState(ev)
	if s.HistoryIndex < len(s.HistoryMessages) && eventAssistantText(ev, s.HistoryText) == s.HistoryMessages[s.HistoryIndex] {
		s.HistoryIndex++
		s.Text = ""
		s.CleanText = ""
		return s.snapshotMeta(), false
	}
	next := assistantTextWithHistory(ev, s.Text, s.HistoryText)
	if next == s.Text {
		meta = s.snapshotMeta()
		return meta, meta.ConversationID != "" || meta.MessageID != "" || meta.CurrentNode != "" || len(meta.FileIDs) > 0 || len(meta.SedimentIDs) > 0 || meta.Blocked || meta.ToolInvoked != nil || meta.TurnUseCase != ""
	}
	s.Text = next
	clean := next
	delta := ""
	if strings.HasPrefix(clean, s.CleanText) {
		delta = clean[len(s.CleanText):]
	} else {
		delta = clean
	}
	s.CleanText = clean
	meta = s.snapshotMeta()
	meta.Delta = delta
	return meta, delta != "" || meta.ConversationID != "" || meta.MessageID != "" || meta.CurrentNode != ""
}

func (s *upstreamConversationState) mergeMeta(meta UpstreamTextEvent) {
	if meta.ConversationID != "" {
		s.ConversationID = meta.ConversationID
	}
	if meta.MessageID != "" {
		s.MessageID = meta.MessageID
	}
	if meta.CurrentNode != "" {
		s.CurrentNode = meta.CurrentNode
	}
}

func (s *upstreamConversationState) snapshotMeta() UpstreamTextEvent {
	return UpstreamTextEvent{ConversationID: s.ConversationID, MessageID: s.MessageID, CurrentNode: s.CurrentNode, FileIDs: append([]string{}, s.FileIDs...), SedimentIDs: append([]string{}, s.SedimentIDs...), Blocked: s.Blocked, ToolInvoked: s.ToolInvoked, TurnUseCase: s.TurnUseCase}
}

func (s *upstreamConversationState) updateState(ev map[string]any) {
	meta := upstreamTextMetaFromMap(ev)
	s.mergeMeta(meta)
	if isImageToolEvent(ev) {
		f, sed := imageIDsFromPayload(ev)
		s.FileIDs = unique(append(s.FileIDs, f...))
		s.SedimentIDs = unique(append(s.SedimentIDs, sed...))
	}
	if strAny(ev["type"], "") == "moderation" {
		if moderation, ok := ev["moderation_response"].(map[string]any); ok && boolAny(moderation["blocked"], false) {
			s.Blocked = true
		}
	}
	if strAny(ev["type"], "") == "server_ste_metadata" {
		if metadata, ok := ev["metadata"].(map[string]any); ok {
			if v, exists := metadata["tool_invoked"]; exists {
				if _, ok := v.(bool); ok {
					s.ToolInvoked = v
				}
			}
			if value := strings.TrimSpace(strAny(metadata["turn_use_case"], "")); value != "" {
				s.TurnUseCase = value
			}
		}
	}
}
func upstreamTextMeta(payload string) UpstreamTextEvent {
	meta := UpstreamTextEvent{}
	var ev map[string]any
	if json.Unmarshal([]byte(payload), &ev) != nil {
		if m := regexp.MustCompile(`"conversation_id"\s*:\s*"([^"]+)"`).FindStringSubmatch(payload); len(m) > 1 {
			meta.ConversationID = m[1]
		}
		if m := regexp.MustCompile(`"message_id"\s*:\s*"([^"]+)"`).FindStringSubmatch(payload); len(m) > 1 {
			meta.MessageID = m[1]
		}
		if m := regexp.MustCompile(`"current_node"\s*:\s*"([^"]+)"`).FindStringSubmatch(payload); len(m) > 1 {
			meta.CurrentNode = m[1]
		}
		return meta
	}
	return upstreamTextMetaFromMap(ev)
}

func upstreamTextMetaFromMap(ev map[string]any) UpstreamTextEvent {
	meta := UpstreamTextEvent{}
	for _, cand := range []any{ev, ev["v"]} {
		m, ok := cand.(map[string]any)
		if !ok {
			continue
		}
		if v := strings.TrimSpace(strAny(m["conversation_id"], "")); v != "" {
			meta.ConversationID = v
		}
		if v := strings.TrimSpace(strAny(m["current_node"], "")); v != "" {
			meta.CurrentNode = v
		}
		if msg, ok := m["message"].(map[string]any); ok {
			if v := strings.TrimSpace(strAny(msg["id"], "")); v != "" {
				meta.MessageID = v
			}
		}
	}
	return meta
}

func assistantTextFrom(v any) string {
	m, ok := v.(map[string]any)
	if !ok {
		return ""
	}
	return assistantTextFromMap(m)
}

func assistantTextFromMap(m map[string]any) string {
	for _, cand := range []any{m, m["v"]} {
		cm, ok := cand.(map[string]any)
		if !ok {
			continue
		}
		msg, ok := cm["message"].(map[string]any)
		if !ok {
			continue
		}
		author, _ := msg["author"].(map[string]any)
		if strAny(author["role"], "") != "assistant" {
			continue
		}
		content, _ := msg["content"].(map[string]any)
		parts, ok := content["parts"].([]any)
		if !ok {
			continue
		}
		b := strings.Builder{}
		for _, p := range parts {
			if s, ok := p.(string); ok {
				b.WriteString(s)
			}
		}
		if b.Len() > 0 {
			return b.String()
		}
	}
	return ""
}

func assistantTextWithHistory(event map[string]any, currentText, historyText string) string {
	if text := assistantTextFromMap(event); text != "" {
		return stripHistoryText(text, historyText)
	}
	if next, ok := applyTextPatchWithHistory(event, currentText, historyText); ok {
		return next
	}
	return currentText
}

func eventAssistantText(event map[string]any, historyText string) string {
	return stripHistoryText(assistantTextFromMap(event), historyText)
}

func stripHistoryText(text, historyText string) string {
	for historyText != "" && strings.HasPrefix(text, historyText) {
		text = text[len(historyText):]
	}
	return text
}

func applyTextPatchWithHistory(event map[string]any, current, historyText string) (string, bool) {
	if strAny(event["p"], "") == "/message/content/parts/0" {
		op := strAny(event["o"], "")
		value := strAny(event["v"], "")
		switch op {
		case "append":
			return current + value, true
		case "replace":
			return stripHistoryText(value, historyText), true
		}
	}
	if strAny(event["o"], "") == "patch" {
		ops, ok := event["v"].([]any)
		if !ok {
			return current, false
		}
		changed := false
		text := current
		for _, item := range ops {
			m, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if next, ok := applyTextPatchWithHistory(m, text, historyText); ok {
				text = next
				changed = true
			}
		}
		return text, changed
	}
	if ops, ok := event["v"].([]any); ok {
		changed := false
		text := current
		for _, item := range ops {
			m, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if next, ok := applyTextPatchWithHistory(m, text, historyText); ok {
				text = next
				changed = true
			}
		}
		return text, changed
	}
	if value, ok := event["v"].(string); ok && value != "" && current != "" && event["p"] == nil && event["o"] == nil {
		return current + value, true
	}
	return current, false
}

func assistantHistoryMessages(messages []map[string]any) []string {
	out := []string{}
	for _, message := range messages {
		if strings.TrimSpace(strAny(message["role"], "")) == "assistant" {
			if text := strings.TrimSpace(messageTextAny(message["content"])); text != "" {
				out = append(out, text)
			}
		}
	}
	return out
}
func isImageToolEvent(ev map[string]any) bool {
	for _, cand := range []any{ev, ev["v"]} {
		m, ok := cand.(map[string]any)
		if !ok {
			continue
		}
		msg, ok := m["message"].(map[string]any)
		if !ok {
			continue
		}
		if isPythonImageToolMessage(msg) {
			return true
		}
	}
	return false
}

func imageIDsFromPayload(v any) ([]string, []string) {
	b, _ := json.Marshal(v)
	return imageIDsFromText(string(b))
}

func imageIDsFromText(text string) ([]string, []string) {
	fileIDs := []string{}
	sedimentIDs := []string{}
	for _, m := range regexp.MustCompile(`file-service://([A-Za-z0-9_-]+)`).FindAllStringSubmatch(text, -1) {
		if len(m) > 1 {
			fileIDs = append(fileIDs, m[1])
		}
	}
	for _, m := range regexp.MustCompile(`\bfile[_-][A-Za-z0-9][A-Za-z0-9_-]*`).FindAllStringSubmatch(text, -1) {
		if len(m) == 0 {
			continue
		}
		candidate := m[0]
		lower := strings.ToLower(candidate)
		if lower == "file_upload" || strings.HasPrefix(lower, "file-service") {
			continue
		}
		fileIDs = append(fileIDs, candidate)
	}
	for _, m := range regexp.MustCompile(`sediment://([A-Za-z0-9_-]+)`).FindAllStringSubmatch(text, -1) {
		if len(m) > 1 {
			sedimentIDs = append(sedimentIDs, m[1])
		}
	}
	return unique(fileIDs), unique(sedimentIDs)
}

func updateImageState(payload string, cid *string, message *string) {
	if *cid == "" {
		if m := regexp.MustCompile(`"conversation_id"\s*:\s*"([^"]+)"`).FindStringSubmatch(payload); len(m) > 1 {
			*cid = m[1]
		}
	}
	var ev map[string]any
	if json.Unmarshal([]byte(payload), &ev) != nil {
		return
	}
	if t := assistantTextFrom(ev); t != "" {
		*message = t
	}
	if t, ok := applyTextPatch(ev, *message); ok {
		*message = t
	}
}

func applyTextPatch(event map[string]any, current string) (string, bool) {
	if strAny(event["p"], "") == "/message/content/parts/0" {
		op := strAny(event["o"], "")
		value := strAny(event["v"], "")
		switch op {
		case "append":
			return current + value, true
		case "replace":
			return value, true
		}
	}
	if strAny(event["o"], "") == "patch" {
		ops, ok := event["v"].([]any)
		if !ok {
			return current, false
		}
		changed := false
		text := current
		for _, item := range ops {
			m, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if next, ok := applyTextPatch(m, text); ok {
				text = next
				changed = true
			}
		}
		return text, changed
	}
	if value, ok := event["v"].(string); ok && value != "" && current != "" && event["p"] == nil && event["o"] == nil {
		return current + value, true
	}
	return current, false
}

func isImageQuotaMessage(s string) bool {
	text := strings.ToLower(s)
	return strings.Contains(text, "free plan limit") ||
		strings.Contains(text, "limit for image generation") ||
		strings.Contains(text, "image generations requests") ||
		strings.Contains(text, "when the limit resets") ||
		strings.Contains(text, "usage limit")
}

func cleanImageMessage(s string) string {
	s = strings.TrimSpace(s)
	lower := strings.ToLower(s)
	for _, needle := range []string{"you've hit", "you have hit", "free plan limit", "usage limit"} {
		if i := strings.Index(lower, needle); i >= 0 {
			return strings.TrimSpace(s[i:])
		}
	}
	return s
}
func extractToolIDs(conv map[string]any) ([]string, []string) {
	fileIDs, sedimentIDs := extractPythonImageToolIDs(conv)
	fallbackFileIDs, fallbackSedimentIDs := extractFallbackImageIDs(conv)
	fileIDs = append(fileIDs, fallbackFileIDs...)
	sedimentIDs = append(sedimentIDs, fallbackSedimentIDs...)
	return unique(fileIDs), unique(sedimentIDs)
}

func extractPythonImageToolIDs(conv map[string]any) ([]string, []string) {
	fileIDs := []string{}
	sedimentIDs := []string{}
	filePat := regexp.MustCompile(`file-service://([A-Za-z0-9_-]+)`)
	sedPat := regexp.MustCompile(`sediment://([A-Za-z0-9_-]+)`)
	if mapping, ok := conv["mapping"].(map[string]any); ok {
		for _, rawNode := range mapping {
			node, ok := rawNode.(map[string]any)
			if !ok {
				continue
			}
			msg, ok := node["message"].(map[string]any)
			if !ok || !isPythonImageToolMessage(msg) {
				continue
			}
			content, _ := msg["content"].(map[string]any)
			if parts, ok := content["parts"].([]any); ok {
				for _, part := range parts {
					text := ""
					if m, ok := part.(map[string]any); ok {
						text = strAny(m["asset_pointer"], "")
					} else if s, ok := part.(string); ok {
						text = s
					}
					for _, m := range filePat.FindAllStringSubmatch(text, -1) {
						if len(m) > 1 {
							fileIDs = append(fileIDs, m[1])
						}
					}
					for _, m := range sedPat.FindAllStringSubmatch(text, -1) {
						if len(m) > 1 {
							sedimentIDs = append(sedimentIDs, m[1])
						}
					}
				}
			}
		}
	}
	return unique(fileIDs), unique(sedimentIDs)
}

func extractFallbackImageIDs(conv map[string]any) ([]string, []string) {
	fileIDs := []string{}
	sedimentIDs := []string{}
	if mapping, ok := conv["mapping"].(map[string]any); ok {
		for _, rawNode := range mapping {
			node, ok := rawNode.(map[string]any)
			if !ok {
				continue
			}
			msg, ok := node["message"].(map[string]any)
			if !ok || !isFallbackImageResultMessage(msg) {
				continue
			}
			f, s := imageIDsFromPayload(msg)
			fileIDs = append(fileIDs, f...)
			sedimentIDs = append(sedimentIDs, s...)
		}
	}
	return unique(fileIDs), unique(sedimentIDs)
}

func isPythonImageToolMessage(msg map[string]any) bool {
	author, _ := msg["author"].(map[string]any)
	metadata, _ := msg["metadata"].(map[string]any)
	content, _ := msg["content"].(map[string]any)
	return strAny(author["role"], "") == "tool" &&
		strAny(metadata["async_task_type"], "") == "image_gen" &&
		strAny(content["content_type"], "") == "multimodal_text"
}

func isFallbackImageResultMessage(msg map[string]any) bool {
	author, _ := msg["author"].(map[string]any)
	metadata, _ := msg["metadata"].(map[string]any)
	content, _ := msg["content"].(map[string]any)
	role := strAny(author["role"], "")
	asyncTaskType := strAny(metadata["async_task_type"], "")
	contentType := strAny(content["content_type"], "")
	if role == "user" {
		return false
	}
	if role == "tool" && asyncTaskType == "image_gen" {
		return true
	}
	if role == "assistant" && (asyncTaskType == "image_gen" || contentType == "multimodal_text") {
		f, s := imageIDsFromPayload(msg)
		return len(f) > 0 || len(s) > 0
	}
	return false
}
func unique(in []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, v := range in {
		v = strings.TrimPrefix(v, "sediment://")
		v = strings.TrimPrefix(v, "file-service://")
		if v != "" && !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}
func buildImagePrompt(prompt, size string) string {
	p := strings.TrimSpace(prompt)
	if strings.TrimSpace(size) == "" {
		return p
	}
	switch strings.TrimSpace(size) {
	case "1:1":
		return p + "\n\n输出为 1:1 正方形构图，主体居中，适合正方形画幅。"
	case "16:9":
		return p + "\n\n输出为 16:9 横屏构图，适合宽画幅展示。"
	case "9:16":
		return p + "\n\n输出为 9:16 竖屏构图，适合竖版画幅展示。"
	case "4:3":
		return p + "\n\n输出为 4:3 比例，兼顾宽度与高度，适合展示画面细节。"
	case "3:4":
		return p + "\n\n输出为 3:4 比例，纵向构图，适合人物肖像或竖向场景。"
	default:
		return p + "\n\n输出图片，宽高比为 " + strings.TrimSpace(size) + "。"
	}
}

func normalizeImageResolution(value string) string {
	n := strings.ToLower(strings.NewReplacer(" ", "", "-", "").Replace(strings.TrimSpace(value)))
	switch n {
	case "auto", "default", "":
		return ""
	case "1", "1024", "1024px", "1024x1024", "1k":
		return "1k"
	case "2", "2048", "2048px", "2048x2048", "2k":
		return "2k"
	case "4", "3840", "3840px", "3840x2160", "2160x3840", "4096", "4096px", "4096x4096", "4k":
		return "4k"
	default:
		return ""
	}
}

func buildImagePromptWithOptions(prompt, size, resolution string) string {
	final := strings.TrimSpace(buildImagePrompt(prompt, size))
	switch normalizeImageResolution(resolution) {
	case "1k":
		return final + "\n\n目标输出分辨率为 1K 级别，优先保证构图稳定和主体清晰。"
	case "2k":
		return final + "\n\n目标输出分辨率为 2K 级别，尽可能输出长边约 2048px 的高清图片，保留细节纹理。"
	case "4k":
		return final + "\n\n目标输出分辨率为 4K 级别，尽可能输出接近 3840px 长边的超清图片，保留丰富细节和干净边缘。"
	default:
		return final
	}
}

func normalizeAccountType(value string) string {
	raw := strings.TrimSpace(value)
	if raw == "" {
		return "free"
	}
	key := strings.ToLower(strings.NewReplacer("-", "", "_", "", " ", "").Replace(raw))
	switch key {
	case "free":
		return "free"
	case "plus":
		return "Plus"
	case "pro":
		return "Pro"
	case "prolite":
		return "ProLite"
	case "team", "business":
		return "Team"
	case "enterprise":
		return "Enterprise"
	default:
		return raw
	}
}

func imageModelSlug(model string) string {
	switch strings.TrimSpace(strings.ToLower(model)) {
	case "gpt-image-2":
		return "gpt-5-3"
	case "codex-gpt-image-2":
		return "codex-gpt-image-2"
	default:
		return "auto"
	}
}
func clientContext() map[string]any {
	return map[string]any{"is_dark_mode": false, "time_since_loaded": 120, "page_height": 900, "page_width": 1400, "pixel_ratio": 2, "screen_height": 1440, "screen_width": 2560, "app_name": "chatgpt.com"}
}
func (c *UpstreamClient) toConversationMessages(ctx context.Context, messages []map[string]any) ([]map[string]any, error) {
	if len(messages) == 0 {
		messages = []map[string]any{{"role": "user", "content": "Hello"}}
	}
	out := []map[string]any{}
	for _, m := range messages {
		role, rawContent := upstreamConversationRoleAndContent(m)
		content, metadata, err := c.conversationContent(ctx, rawContent)
		if err != nil {
			return nil, err
		}
		msg := map[string]any{"id": uuid4(), "author": map[string]any{"role": role}, "content": content}
		if len(metadata) > 0 {
			msg["metadata"] = metadata
		}
		out = append(out, msg)
	}
	return out, nil
}

func upstreamConversationRoleAndContent(m map[string]any) (string, any) {
	role := strings.ToLower(strings.TrimSpace(strAny(m["role"], "user")))
	switch role {
	case "assistant", "user":
		return role, m["content"]
	case "system", "developer":
		return "user", "System instructions:\n" + messageTextAny(m["content"])
	case "tool", "function":
		name := firstNonEmpty(strAny(m["name"], ""), strAny(m["tool_call_id"], ""), strAny(m["tool_use_id"], ""), "tool")
		return "user", "Tool result from " + name + ":\n" + messageTextAny(m["content"])
	default:
		return "user", messageTextAny(m["content"])
	}
}

func (c *UpstreamClient) conversationContent(ctx context.Context, raw any) (map[string]any, map[string]any, error) {
	text := messageTextAny(raw)
	images := extractImagesFromContent(raw)
	if len(images) == 0 {
		return map[string]any{"content_type": "text", "parts": []string{text}}, nil, nil
	}
	parts := []any{}
	attachments := []map[string]any{}
	if strings.TrimSpace(text) != "" {
		parts = append(parts, text)
	}
	for i, imageBytes := range images {
		uploaded, err := c.uploadImage(ctx, imageBytes, fmt.Sprintf("chat_image_%d.png", i+1))
		if err != nil {
			return nil, nil, err
		}
		fileID := strAny(uploaded["file_id"], "")
		parts = append(parts, map[string]any{"content_type": "image_asset_pointer", "asset_pointer": "file-service://" + fileID, "width": uploaded["width"], "height": uploaded["height"], "size_bytes": uploaded["file_size"]})
		attachments = append(attachments, map[string]any{"id": fileID, "mimeType": uploaded["mime_type"], "name": uploaded["file_name"], "size": uploaded["file_size"], "width": uploaded["width"], "height": uploaded["height"]})
	}
	metadata := map[string]any{"attachments": attachments}
	return map[string]any{"content_type": "multimodal_text", "parts": parts}, metadata, nil
}

func parsePowResources(html string) ([]string, string) {
	re := regexp.MustCompile(`<script[^>]+src=["']([^"']+)["']`)
	matches := re.FindAllStringSubmatch(html, -1)
	src := []string{}
	build := ""
	for _, m := range matches {
		src = append(src, m[1])
		if build == "" {
			if mm := regexp.MustCompile(`c/[^/]*/_`).FindStringSubmatch(m[1]); len(mm) > 0 {
				build = mm[0]
			}
		}
	}
	if build == "" {
		if m := regexp.MustCompile(`<html[^>]*data-build=["']([^"']*)`).FindStringSubmatch(html); len(m) > 1 {
			build = m[1]
		}
	}
	return src, build
}
func buildPowConfig(ua string, scripts []string, dataBuild string) []any {
	nav := []string{"webdriver−false", "vendor−Google Inc.", "hardwareConcurrency−32", "language−zh-CN", "cookieEnabled−true"}
	win := []string{"window", "document", "location", "navigator", "crypto", "fetch", "screen"}
	script := defaultPowScript
	if len(scripts) > 0 {
		script = scripts[rand.Intn(len(scripts))]
	}
	return []any{[]int{3000, 4000, 5000}[rand.Intn(3)], time.Now().In(time.FixedZone("EST", -5*3600)).Format("Mon Jan 02 2006 15:04:05") + " GMT-0500 (Eastern Standard Time)", 4294705152, 0, ua, script, dataBuild, "en-US", "en-US,es-US,en,es", 0, nav[rand.Intn(len(nav))], "location", win[rand.Intn(len(win))], float64(time.Now().UnixNano()) / 1e6, randID(16), "", []int{8, 16, 24, 32}[rand.Intn(4)], float64(time.Now().UnixNano())/1e6 - 1000}
}
func powGenerate(seed, diff string, cfg []any, limit int) (string, bool) {
	target, err := hexDecode(diff)
	if err != nil || len(target) == 0 {
		return "", false
	}
	diffLen := len(diff) / 2
	s1 := jsonCompact(cfg[:3])
	s1 = s1[:len(s1)-1] + ","
	s2 := jsonCompact(cfg[4:9])
	s2 = "," + s2[1:len(s2)-1] + ","
	s3 := jsonCompact(cfg[10:])
	s3 = "," + s3[1:]
	for i := 0; i < limit; i++ {
		final := []byte(s1 + fmt.Sprint(i) + s2 + fmt.Sprint(i>>1) + s3)
		enc := base64.StdEncoding.EncodeToString(final)
		h := sha3.Sum512(append([]byte(seed), []byte(enc)...))
		if bytes.Compare(h[:diffLen], target) <= 0 {
			return enc, true
		}
	}
	return "wQ8Lk5FbGpA2NcR9dShT6gYjU7VxZ4D" + base64.StdEncoding.EncodeToString([]byte("\""+seed+"\"")), false
}
func buildLegacyRequirementsToken(ua string, scripts []string, dataBuild string) string {
	seed := fmt.Sprintf("%f", rand.Float64())
	ans, _ := powGenerate(seed, "0fffff", buildPowConfig(ua, scripts, dataBuild), 500000)
	return "gAAAAAC" + ans
}
func buildProofToken(seed, diff, ua string, scripts []string, dataBuild string) (string, error) {
	ans, ok := powGenerate(seed, diff, buildPowConfig(ua, scripts, dataBuild), 500000)
	if !ok {
		return "", fmt.Errorf("failed to solve proof token: difficulty=%s", diff)
	}
	return "gAAAAAB" + ans, nil
}
func jsonCompact(v any) string { b, _ := json.Marshal(v); return string(b) }
func hexDecode(s string) ([]byte, error) {
	if len(s)%2 == 1 {
		s = "0" + s
	}
	out := make([]byte, len(s)/2)
	for i := 0; i < len(out); i++ {
		var x byte
		for _, c := range []byte{s[2*i], s[2*i+1]} {
			x <<= 4
			switch {
			case c >= '0' && c <= '9':
				x += c - '0'
			case c >= 'a' && c <= 'f':
				x += c - 'a' + 10
			case c >= 'A' && c <= 'F':
				x += c - 'A' + 10
			default:
				return nil, fmt.Errorf("bad hex")
			}
		}
		out[i] = x
	}
	return out, nil
}

func (s *Server) upstreamToken() (string, error) {
	return s.pickTextToken()
}

func (s *Server) upstreamClient() (*UpstreamClient, error) {
	account, err := s.pickTextAccountExcluding(nil, "")
	if err != nil {
		return nil, err
	}
	return NewUpstreamClientForAccount(account, s.cfg.Proxy, s.ensureCurlImpersonateBinary)
}
func (s *Server) upstreamClientForImage(model, resolution string) (*UpstreamClient, error) {
	return s.upstreamClientForImageExcluding(model, resolution, nil)
}

func (s *Server) upstreamClientForImageExcluding(model, resolution string, excluded map[string]bool) (*UpstreamClient, error) {
	account, err := s.pickAccountExcluding(model, resolution, excluded)
	if err != nil {
		return nil, err
	}
	client, _, err := s.upstreamClientForImageAccount(model, resolution, account)
	return client, err
}

func (s *Server) upstreamClientForImageAccount(model, resolution string, account Account) (*UpstreamClient, Account, error) {
	if isCodexImageRequest(model, resolution) {
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()
		if refreshed, err := s.refreshOAuthAccount(ctx, account.AccessToken); err == nil && refreshed.AccessToken != "" {
			account = refreshed
		}
	}
	client, err := NewUpstreamClientForAccount(account, s.cfg.Proxy, s.ensureCurlImpersonateBinary)
	return client, account, err
}

func init() { rand.Seed(time.Now().UnixNano()) }
