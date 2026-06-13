package app

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"reflect"
	"sort"
	"strings"
	"time"
)

type traceContextKey struct{}

func newTraceID() string {
	return randID(4)
}

func withTraceID(ctx context.Context, id string) context.Context {
	if strings.TrimSpace(id) == "" {
		id = newTraceID()
	}
	return context.WithValue(ctx, traceContextKey{}, id)
}

func traceIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if id, ok := ctx.Value(traceContextKey{}).(string); ok {
		return strings.TrimSpace(id)
	}
	return ""
}

func traceEnabled() bool {
	v := strings.ToLower(strings.TrimSpace(firstNonEmpty(os.Getenv("CHATGPT2API_TRACE"), os.Getenv("CHATGPT2API_NETWORK_TRACE"))))
	switch v {
	case "0", "false", "off", "no", "quiet":
		return false
	default:
		return true
	}
}

func traceLogf(ctx context.Context, format string, args ...any) {
	if !traceEnabled() {
		return
	}
	id := traceIDFromContext(ctx)
	if id == "" {
		id = "no-trace"
	}
	log.Printf("[trace:%s] %s", id, fmt.Sprintf(format, args...))
}

func maskedValue(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if len(value) <= 8 {
		return "***"
	}
	return value[:4] + "..." + value[len(value)-4:]
}

func accountLabel(a Account) string {
	parts := []string{maskedValue(a.AccessToken)}
	if a.Email != nil && strings.TrimSpace(*a.Email) != "" {
		parts = append(parts, strings.TrimSpace(*a.Email))
	}
	if strings.TrimSpace(a.Type) != "" {
		parts = append(parts, "type="+strings.TrimSpace(a.Type))
	}
	if strings.TrimSpace(a.SourceType) != "" {
		parts = append(parts, "source="+strings.TrimSpace(a.SourceType))
	}
	return strings.Join(parts, " ")
}

func isSensitiveKey(key string) bool {
	k := strings.ToLower(strings.TrimSpace(key))
	return strings.Contains(k, "authorization") ||
		strings.Contains(k, "access_token") ||
		strings.Contains(k, "refresh_token") ||
		strings.Contains(k, "id_token") ||
		strings.Contains(k, "token") ||
		strings.Contains(k, "cookie") ||
		strings.Contains(k, "secret") ||
		strings.Contains(k, "password") ||
		strings.Contains(k, "auth-key") ||
		strings.Contains(k, "x-api-key") ||
		strings.Contains(k, "oai-device-id") ||
		strings.Contains(k, "oai-session-id")
}

func sanitizeTraceValue(key string, value any) any {
	if isSensitiveKey(key) {
		return maskedValue(strAny(value, ""))
	}
	lower := strings.ToLower(strings.TrimSpace(key))
	if lower == "file_data" || lower == "image_url" || lower == "b64_json" || lower == "result" {
		s := strAny(value, "")
		if len(s) > 120 {
			return fmt.Sprintf("<%d bytes omitted>", len(s))
		}
	}
	switch v := value.(type) {
	case map[string]any:
		return sanitizeTraceMap(v)
	case []any:
		out := make([]any, 0, len(v))
		for _, item := range v {
			out = append(out, sanitizeTraceValue("", item))
		}
		return out
	default:
		return value
	}
}

func sanitizeTraceMap(in map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range in {
		out[k] = sanitizeTraceValue(k, v)
	}
	return out
}

func traceBodyPreview(body any) string {
	if body == nil {
		return "<empty>"
	}
	var safe any
	if m, ok := body.(map[string]any); ok {
		safe = sanitizeTraceMap(m)
	} else {
		safe = sanitizeTraceValue("", body)
	}
	b, err := json.Marshal(safe)
	if err != nil {
		return fmt.Sprintf("<%T>", body)
	}
	text := string(b)
	if len(text) > 1600 {
		return text[:1600] + "...<truncated>"
	}
	return text
}

func traceHeaderPreview(headers any) string {
	if headers == nil {
		return "{}"
	}
	rv := reflect.ValueOf(headers)
	if rv.Kind() != reflect.Map {
		return fmt.Sprintf("<%T>", headers)
	}
	items := []string{}
	for _, key := range rv.MapKeys() {
		k := fmt.Sprint(key.Interface())
		if strings.EqualFold(k, "Header-Order:") || strings.EqualFold(k, "Header-Order") {
			continue
		}
		mv := rv.MapIndex(key)
		values := []string{}
		switch mv.Kind() {
		case reflect.Slice, reflect.Array:
			for i := 0; i < mv.Len(); i++ {
				values = append(values, fmt.Sprint(mv.Index(i).Interface()))
			}
		default:
			values = append(values, fmt.Sprint(mv.Interface()))
		}
		joined := strings.Join(values, ",")
		if isSensitiveKey(k) {
			joined = maskedValue(joined)
		}
		if len(joined) > 180 {
			joined = joined[:180] + "..."
		}
		items = append(items, k+"="+joined)
	}
	sort.Strings(items)
	return strings.Join(items, "; ")
}

type traceResponseWriter struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (w *traceResponseWriter) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
	}
	w.ResponseWriter.WriteHeader(status)
}

func (w *traceResponseWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	n, err := w.ResponseWriter.Write(b)
	w.bytes += n
	return n, err
}

func (w *traceResponseWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func traceHTTPDuration(start time.Time) string {
	return time.Since(start).Truncate(time.Millisecond).String()
}

func safeURLForLog(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return truncateText(raw, 160)
	}
	if u.RawQuery != "" {
		u.RawQuery = "<redacted>"
	}
	if u.User != nil {
		u.User = url.User("***")
	}
	return truncateText(u.String(), 220)
}
