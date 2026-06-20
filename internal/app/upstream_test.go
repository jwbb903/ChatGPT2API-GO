package app

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	stdhttp "net/http"
	"net/http/httptest"
	"strings"
	"testing"

	http "github.com/bogdanfinn/fhttp"
)

type upstreamDoerFunc func(req *http.Request) (*http.Response, error)

func (f upstreamDoerFunc) Do(req *http.Request) (*http.Response, error) {
	return f(req)
}

func upstreamTestResponse(status int, body string) *http.Response {
	return upstreamTestResponseWithHeader(status, body, http.Header{})
}

func upstreamTestResponseWithHeader(status int, body string, header http.Header) *http.Response {
	if header == nil {
		header = http.Header{}
	}
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     header,
	}
}

func TestBootstrapUsesDocumentHeadersOnly(t *testing.T) {
	client := &UpstreamClient{
		token:           "access-token",
		userAgent:       "test-agent",
		secCHUA:         `"Microsoft Edge";v="143", "Chromium";v="143", "Not A(Brand";v="24"`,
		secCHUAMobile:   "?0",
		secCHUAPlatform: `"Windows"`,
		client: upstreamDoerFunc(func(req *http.Request) (*http.Response, error) {
			if got := req.Header.Get("Authorization"); got != "" {
				t.Fatalf("bootstrap sent Authorization header: %q", got)
			}
			for _, name := range []string{
				"Origin",
				"Referer",
				"OAI-Device-Id",
				"OAI-Session-Id",
				"OAI-Language",
				"OAI-Client-Version",
				"OAI-Client-Build-Number",
				"X-OpenAI-Target-Path",
				"X-OpenAI-Target-Route",
			} {
				if got := req.Header.Get(name); got != "" {
					t.Fatalf("bootstrap sent %s header: %q", name, got)
				}
			}
			if got := req.Header.Get("Sec-Fetch-Mode"); got != "navigate" {
				t.Fatalf("Sec-Fetch-Mode = %q, want navigate", got)
			}
			if got := req.Header.Get("Sec-Fetch-Site"); got != "none" {
				t.Fatalf("Sec-Fetch-Site = %q, want none", got)
			}
			if got := req.Header.Get("Sec-Fetch-User"); got != "?1" {
				t.Fatalf("Sec-Fetch-User = %q, want ?1", got)
			}
			return upstreamTestResponse(200, `<html data-build="test-build"><script src="/c/test/_/x.js"></script></html>`), nil
		}),
	}
	if err := client.bootstrap(context.Background()); err != nil {
		t.Fatalf("bootstrap returned error: %v", err)
	}
}

func TestBootstrapRetriesRedirectAndThenSucceeds(t *testing.T) {
	attempts := 0
	client := &UpstreamClient{
		userAgent:       "test-agent",
		secCHUA:         `"Microsoft Edge";v="143"`,
		secCHUAMobile:   "?0",
		secCHUAPlatform: `"Windows"`,
		client: upstreamDoerFunc(func(req *http.Request) (*http.Response, error) {
			attempts++
			if attempts == 1 {
				h := http.Header{}
				h.Set("Location", "/auth/login")
				return upstreamTestResponseWithHeader(302, "", h), nil
			}
			return upstreamTestResponse(200, `<html data-build="retry-build"><script src="/c/retry/_/x.js"></script></html>`), nil
		}),
	}
	if err := client.bootstrap(context.Background()); err != nil {
		t.Fatalf("bootstrap returned error: %v", err)
	}
	if attempts != 2 {
		t.Fatalf("attempts = %d, want 2", attempts)
	}
	if client.dataBuild != "c/retry/_" {
		t.Fatalf("dataBuild = %q, want c/retry/_", client.dataBuild)
	}
}

func TestBootstrapRedirectErrorIncludesLocation(t *testing.T) {
	client := &UpstreamClient{
		userAgent:       "test-agent",
		secCHUA:         `"Microsoft Edge";v="143"`,
		secCHUAMobile:   "?0",
		secCHUAPlatform: `"Windows"`,
		client: upstreamDoerFunc(func(req *http.Request) (*http.Response, error) {
			h := http.Header{}
			h.Set("Location", "/auth/login")
			return upstreamTestResponseWithHeader(302, "", h), nil
		}),
	}
	err := client.bootstrap(context.Background())
	if err == nil {
		t.Fatal("expected bootstrap error")
	}
	if !strings.Contains(err.Error(), "bootstrap redirect: status=302") || !strings.Contains(err.Error(), "/auth/login") {
		t.Fatalf("error = %q, want redirect status and location", err.Error())
	}
}

func TestAuthenticatedChatRequirementsSolvesTurnstileWithEmptySourceP(t *testing.T) {
	dx := base64.StdEncoding.EncodeToString([]byte(`[[3,"ok"]]`))
	client := &UpstreamClient{
		token:           "access-token",
		userAgent:       "test-agent",
		deviceID:        "device-id",
		sessionID:       "session-id",
		secCHUA:         `"Microsoft Edge";v="143", "Chromium";v="143", "Not A(Brand";v="24"`,
		secCHUAMobile:   "?0",
		secCHUAPlatform: `"Windows"`,
		scriptSources:   []string{defaultPowScript},
		client: upstreamDoerFunc(func(req *http.Request) (*http.Response, error) {
			if req.URL.Path != "/backend-api/sentinel/chat-requirements" {
				t.Fatalf("path = %q, want /backend-api/sentinel/chat-requirements", req.URL.Path)
			}
			return upstreamTestResponse(200, `{"token":"requirements-token","turnstile":{"required":true,"dx":"`+dx+`"}}`), nil
		}),
	}
	got, err := client.chatRequirements(context.Background())
	if err != nil {
		t.Fatalf("chatRequirements returned error: %v", err)
	}
	if got.TurnstileToken != "b2s=" {
		t.Fatalf("TurnstileToken = %q, want b2s=", got.TurnstileToken)
	}
}

func TestConversationContentUsesStringTextPartForMultimodalMessages(t *testing.T) {
	storage := httptest.NewServer(stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		if r.Method != stdhttp.MethodPut || r.URL.Path != "/upload" {
			t.Fatalf("storage request = %s %s, want PUT /upload", r.Method, r.URL.Path)
		}
		w.WriteHeader(201)
	}))
	defer storage.Close()
	client := &UpstreamClient{
		token:           "access-token",
		userAgent:       "test-agent",
		deviceID:        "device-id",
		sessionID:       "session-id",
		secCHUA:         `"Microsoft Edge";v="143", "Chromium";v="143", "Not A(Brand";v="24"`,
		secCHUAMobile:   "?0",
		secCHUAPlatform: `"Windows"`,
		scriptSources:   []string{defaultPowScript},
		client: upstreamDoerFunc(func(req *http.Request) (*http.Response, error) {
			switch req.URL.Path {
			case "/backend-api/files":
				body := map[string]any{"file_id": "file-test", "upload_url": storage.URL + "/upload?sig=secret"}
				b, _ := json.Marshal(body)
				return upstreamTestResponse(200, string(b)), nil
			case "/backend-api/files/file-test/uploaded":
				return upstreamTestResponse(200, `{}`), nil
			default:
				t.Fatalf("unexpected upstream path %q", req.URL.Path)
			}
			return nil, nil
		}),
	}
	content, metadata, err := client.conversationContent(context.Background(), []any{
		map[string]any{"type": "text", "text": "describe this image"},
		map[string]any{"type": "image_url", "image_url": map[string]any{"url": "data:image/png;base64," + base64.StdEncoding.EncodeToString([]byte("fake-image"))}},
	})
	if err != nil {
		t.Fatalf("conversationContent returned error: %v", err)
	}
	if content["content_type"] != "multimodal_text" {
		t.Fatalf("content_type = %v, want multimodal_text", content["content_type"])
	}
	parts, ok := content["parts"].([]any)
	if !ok || len(parts) != 2 {
		t.Fatalf("parts = %#v, want two []any entries", content["parts"])
	}
	if first, ok := parts[0].(string); !ok || first != "describe this image" {
		t.Fatalf("parts[0] = %#v, want text string", parts[0])
	}
	imagePart, ok := parts[1].(map[string]any)
	if !ok || imagePart["content_type"] != "image_asset_pointer" {
		t.Fatalf("parts[1] = %#v, want image_asset_pointer object", parts[1])
	}
	attachments, ok := metadata["attachments"].([]map[string]any)
	if !ok || len(attachments) != 1 {
		t.Fatalf("attachments = %#v, want one attachment", metadata["attachments"])
	}
}

func TestParseJSONToolCalls(t *testing.T) {
	text := `{"tool_calls":[{"tool_name":"mcp__generate_image_lucid","parameters":{"prompt":"test","width":1024,"height":1024,"num_images":1}}]}`
	calls := parseToolCalls(text)
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %#v", calls)
	}
	if calls[0]["name"] != "mcp__generate_image_lucid" {
		t.Fatalf("expected name mcp__generate_image_lucid, got %q", calls[0]["name"])
	}
	input, _ := calls[0]["input"].(map[string]any)
	if input == nil {
		t.Fatal("expected input map")
	}
	if strAny(input["prompt"], "") != "test" {
		t.Fatalf("expected prompt=test, got %q", input["prompt"])
	}
}

func TestStripJSONToolCalls(t *testing.T) {
	text := `Some text {"tool_calls":[{"tool_name":"x"}]} more text`
	cleaned := stripToolMarkup(text)
	if !strings.Contains(cleaned, "Some text") || !strings.Contains(cleaned, "more text") {
		t.Fatalf("expected cleaned text to contain prose, got %q", cleaned)
	}
	if strings.Contains(cleaned, "tool_calls") || strings.Contains(cleaned, "tool_name") {
		t.Fatalf("expected tool markup removed, got %q", cleaned)
	}
}

func TestExtractToolIDsAcceptsBareFileIDsInImageToolMessages(t *testing.T) {
	conv := map[string]any{
		"mapping": map[string]any{
			"tool-node": map[string]any{
				"message": map[string]any{
					"author":   map[string]any{"role": "tool"},
					"metadata": map[string]any{"async_task_type": "image_gen"},
					"content": map[string]any{
						"content_type": "multimodal_text",
						"parts": []any{
							map[string]any{"asset_pointer": "file_result_123"},
							map[string]any{"asset_pointer": "sediment://sediment_result_456"},
						},
					},
				},
			},
		},
	}

	fileIDs, sedimentIDs := extractToolIDs(conv)
	if len(fileIDs) != 1 || fileIDs[0] != "file_result_123" {
		t.Fatalf("fileIDs = %#v, want [file_result_123]", fileIDs)
	}
	if len(sedimentIDs) != 1 || sedimentIDs[0] != "sediment_result_456" {
		t.Fatalf("sedimentIDs = %#v, want [sediment_result_456]", sedimentIDs)
	}
}

func TestExtractToolIDsAcceptsAssistantImageResultMessages(t *testing.T) {
	conv := map[string]any{
		"mapping": map[string]any{
			"assistant-node": map[string]any{
				"message": map[string]any{
					"author": map[string]any{"role": "assistant"},
					"content": map[string]any{
						"content_type": "multimodal_text",
						"parts": []any{
							map[string]any{"asset_pointer": "file-service://file_result_789"},
						},
					},
				},
			},
		},
	}

	fileIDs, sedimentIDs := extractToolIDs(conv)
	if len(fileIDs) != 1 || fileIDs[0] != "file_result_789" {
		t.Fatalf("fileIDs = %#v, want [file_result_789]", fileIDs)
	}
	if len(sedimentIDs) != 0 {
		t.Fatalf("sedimentIDs = %#v, want empty", sedimentIDs)
	}
}

func TestChatRequirementsDoesNotFailWhenTurnstileUnsolved(t *testing.T) {
	client := &UpstreamClient{
		token:           "access-token",
		userAgent:       "test-agent",
		deviceID:        "device-id",
		sessionID:       "session-id",
		secCHUA:         `"Microsoft Edge";v="143", "Chromium";v="143", "Not A(Brand";v="24"`,
		secCHUAMobile:   "?0",
		secCHUAPlatform: `"Windows"`,
		scriptSources:   []string{defaultPowScript},
		client: upstreamDoerFunc(func(req *http.Request) (*http.Response, error) {
			return upstreamTestResponse(200, `{"token":"requirements-token","turnstile":{"required":true}}`), nil
		}),
	}
	got, err := client.chatRequirements(context.Background())
	if err != nil {
		t.Fatalf("chatRequirements returned error: %v", err)
	}
	if got.Token != "requirements-token" {
		t.Fatalf("Token = %q, want requirements-token", got.Token)
	}
	if got.TurnstileToken != "" {
		t.Fatalf("TurnstileToken = %q, want empty", got.TurnstileToken)
	}
}
