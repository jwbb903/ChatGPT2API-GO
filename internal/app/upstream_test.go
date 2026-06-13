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
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     http.Header{},
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
