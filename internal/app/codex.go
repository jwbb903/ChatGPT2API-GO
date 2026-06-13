package app

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	http "github.com/bogdanfinn/fhttp"
)

const codexResponsesModel = "gpt-5.5"
const codexImageInstruction = "You are an image generation assistant."

func isCodexImageRequest(model, resolution string) bool {
	m := strings.ToLower(strings.TrimSpace(model))
	return strings.Contains(m, "codex-gpt-image-2") || normalizeResolution(resolution) == "2k" || normalizeResolution(resolution) == "4k"
}

func normalizeResolution(v string) string {
	x := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(strings.TrimSpace(v), " ", ""), "-", ""))
	switch x {
	case "2", "2k", "2048", "2048px", "2048x2048":
		return "2k"
	case "4", "4k", "3840", "3840px", "3840x2160", "2160x3840", "4096", "4096px", "4096x4096":
		return "4k"
	case "1", "1k", "1024", "1024px", "1024x1024":
		return "1k"
	default:
		return ""
	}
}

func codexImageSize(size, resolution string) string {
	res := normalizeResolution(resolution)
	aspect := strings.TrimSpace(size)
	if res == "2k" {
		switch aspect {
		case "16:9":
			return "2048x1152"
		case "9:16":
			return "1152x2048"
		case "4:3":
			return "2048x1536"
		case "3:4":
			return "1536x2048"
		default:
			return "2048x2048"
		}
	}
	if res == "4k" {
		switch aspect {
		case "1:1":
			return "2048x2048"
		case "9:16":
			return "2160x3840"
		case "4:3":
			return "3072x2304"
		case "3:4":
			return "2304x3072"
		default:
			return "3840x2160"
		}
	}
	return "1024x1024"
}

func (c *UpstreamClient) codexHeaders(path, installationID string) map[string]string {
	h := map[string]string{
		"Authorization":                     "Bearer " + c.token,
		"originator":                        "Codex Desktop",
		"x-openai-internal-codex-residency": "us",
		"x-client-request-id":               randID(16),
		"x-codex-installation-id":           installationID,
		"OpenAI-Beta":                       "responses_websockets=2026-02-06",
		"User-Agent":                        "Codex Desktop/26.519.81530 (win32; x64)",
		"sec-ch-ua":                         `"Chromium";v="146", "Not:A-Brand";v="24"`,
		"sec-ch-ua-mobile":                  "?0",
		"sec-ch-ua-platform":                `"Windows"`,
		"Accept-Encoding":                   "gzip, deflate, br, zstd",
		"Accept-Language":                   "en-US,en;q=0.9",
		"sec-fetch-site":                    "same-origin",
		"sec-fetch-mode":                    "cors",
		"sec-fetch-dest":                    "empty",
		"Content-Type":                      "application/json",
		"Accept":                            "text/event-stream",
		"Origin":                            chatGPTBaseURL,
		"Referer":                           chatGPTBaseURL + "/",
		"X-OpenAI-Target-Path":              path,
		"X-OpenAI-Target-Route":             path,
	}
	if accountID := chatGPTAccountID(c.token); accountID != "" {
		h["ChatGPT-Account-Id"] = accountID
	}
	return h
}

func (c *UpstreamClient) GenerateCodexImage(ctx context.Context, prompt, model, size string, refs [][]byte) ([]upstreamImageResult, error) {
	if strings.TrimSpace(c.token) == "" {
		return nil, errors.New("access token is required for codex image generation")
	}
	path := "/backend-api/codex/responses"
	installationID := randID(16)
	content := []map[string]any{{"type": "input_text", "text": prompt}}
	for _, img := range refs {
		if len(img) == 0 {
			continue
		}
		content = append(content, map[string]any{
			"type":      "input_image",
			"image_url": "data:image/png;base64," + base64.StdEncoding.EncodeToString(img),
			"detail":    "auto",
		})
	}
	body := map[string]any{
		"model":        codexResponsesModel,
		"input":        []map[string]any{{"role": "user", "content": content}},
		"instructions": codexImageInstruction,
		"tools": []map[string]any{{
			"type":  "image_generation",
			"model": imageGenerationModelForTool(model),
			"size":  size,
		}},
		"tool_choice":     map[string]any{"type": "image_generation"},
		"stream":          true,
		"store":           false,
		"client_metadata": map[string]any{"x-codex-installation-id": installationID},
	}
	resp, err := c.do(ctx, http.MethodPost, path, c.codexHeaders(path, installationID), body, true)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	items := []map[string]any{}
	completed := []map[string]any{}
	for payload := range parseSSE(resp.Body) {
		if payload == "[DONE]" {
			break
		}
		var event map[string]any
		if json.Unmarshal([]byte(payload), &event) != nil {
			continue
		}
		typ := strAny(event["type"], "")
		if typ == "response.failed" || typ == "error" {
			errObj, _ := event["error"].(map[string]any)
			return nil, fmt.Errorf("codex image generation failed: %s", firstNonEmpty(strAny(errObj["message"], ""), strAny(event["message"], "")))
		}
		if typ == "response.output_item.done" {
			if item, ok := event["item"].(map[string]any); ok && strAny(item["type"], "") == "image_generation_call" && strAny(item["result"], "") != "" {
				items = append(items, item)
			}
		}
		if typ == "response.completed" {
			if respObj, ok := event["response"].(map[string]any); ok {
				if out, ok := respObj["output"].([]any); ok {
					for _, raw := range out {
						if item, ok := raw.(map[string]any); ok {
							completed = append(completed, item)
						}
					}
				}
			}
		}
	}
	if len(items) == 0 {
		for _, item := range completed {
			if strAny(item["type"], "") == "image_generation_call" && strAny(item["result"], "") != "" {
				items = append(items, item)
			}
		}
	}
	results := []upstreamImageResult{}
	for _, item := range items {
		b64 := strAny(item["result"], "")
		if b64 == "" {
			continue
		}
		data, err := base64.StdEncoding.DecodeString(b64)
		if err != nil {
			continue
		}
		results = append(results, upstreamImageResult{Bytes: data, RevisedPrompt: strAny(item["revised_prompt"], prompt)})
	}
	if len(results) == 0 {
		return nil, errors.New("Codex image generation returned no image")
	}
	return results, nil
}

func imageGenerationModelForTool(model string) string {
	m := strings.ToLower(strings.TrimSpace(model))
	if strings.Contains(m, "codex-gpt-image-2") || strings.Contains(m, "gpt-image-2") {
		return "gpt-image-2"
	}
	return "gpt-image-2"
}

func chatGPTAccountID(token string) string {
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return ""
	}
	payload := parts[1] + strings.Repeat("=", (4-len(parts[1])%4)%4)
	decoded, err := base64.URLEncoding.DecodeString(payload)
	if err != nil {
		return ""
	}
	var data map[string]any
	if json.Unmarshal(decoded, &data) != nil {
		return ""
	}
	if auth, ok := data["https://api.openai.com/auth"].(map[string]any); ok {
		return strings.TrimSpace(strAny(auth["chatgpt_account_id"], ""))
	}
	return ""
}

func markAccountRateLimited(a *Account, errText string) {
	now := nowISO()
	a.Status = "限流"
	a.Fail++
	a.LastUsedAt = &now
	reset := time.Now().UTC().Add(5 * time.Minute).Format(time.RFC3339)
	a.RestoreAt = &reset
}

func markAccountInvalid(a *Account) {
	now := nowISO()
	a.Status = "异常"
	a.Quota = 0
	a.Fail++
	a.LastUsedAt = &now
}
