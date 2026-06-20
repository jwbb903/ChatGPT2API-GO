package app

import (
	"strings"
	"testing"
)

func TestNormalizePromptMessagesMapsDeveloperAndMergesSystem(t *testing.T) {
	messages := normalizePromptMessages([]map[string]any{
		{"role": "system", "content": "global"},
		{"role": "developer", "content": "dev rules"},
		{"role": "user", "content": "hello"},
	})
	if len(messages) != 2 {
		t.Fatalf("len = %d, want 2: %#v", len(messages), messages)
	}
	if messages[0]["role"] != "system" {
		t.Fatalf("first role = %#v, want system", messages[0]["role"])
	}
	content := messageTextAny(messages[0]["content"])
	if !strings.Contains(content, "global") || !strings.Contains(content, "dev rules") {
		t.Fatalf("system content = %q, want both system parts", content)
	}
}

func TestPrependOrMergeSystemPromptMergesIntoExistingSystem(t *testing.T) {
	messages := prependOrMergeSystemPrompt([]map[string]any{
		{"role": "system", "content": "base"},
		{"role": "user", "content": "hello"},
	}, "tools")
	if len(messages) != 2 {
		t.Fatalf("len = %d, want 2", len(messages))
	}
	content := messageTextAny(messages[0]["content"])
	if !strings.Contains(content, "base") || !strings.Contains(content, "tools") {
		t.Fatalf("merged content = %q", content)
	}
}

func TestUpstreamConversationConvertsSystemAndDeveloperToUserText(t *testing.T) {
	for _, role := range []string{"system", "developer"} {
		gotRole, content := upstreamConversationRoleAndContent(map[string]any{"role": role, "content": "follow rules"})
		if gotRole != "user" {
			t.Fatalf("role %s converted to %s, want user", role, gotRole)
		}
		text := messageTextAny(content)
		if !strings.Contains(text, "System instructions:") || !strings.Contains(text, "follow rules") {
			t.Fatalf("content = %q", text)
		}
	}
}

func TestRetryableBootstrapErrorText(t *testing.T) {
	if !isRetryableBootstrapError(errString("bootstrap redirect: status=302 location=/auth/login")) {
		t.Fatal("expected bootstrap 302 to be retryable")
	}
	if isRetryableBootstrapError(errString("bootstrap failed: status=403 body=forbidden")) {
		t.Fatal("expected bootstrap 403 to be non-retryable")
	}
}

type errString string

func (e errString) Error() string { return string(e) }
