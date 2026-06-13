package app

import (
	"bytes"
	"compress/zlib"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

func (s *Server) messagesFromBodyWithFiles(b map[string]any) ([]map[string]any, error) {
	messages := messagesFromBody(b)
	out := make([]map[string]any, 0, len(messages)+1)
	if strings.TrimSpace(s.cfg.GlobalSystemPrompt) != "" {
		out = append(out, map[string]any{"role": "system", "content": strings.TrimSpace(s.cfg.GlobalSystemPrompt)})
	}
	for _, message := range messages {
		item := map[string]any{}
		for key, value := range message {
			item[key] = value
		}
		content, err := expandInputFiles(item["content"])
		if err != nil {
			return nil, err
		}
		item["content"] = content
		out = append(out, item)
	}
	return out, nil
}

func expandInputFiles(content any) (any, error) {
	parts, ok := content.([]any)
	if !ok {
		return content, nil
	}
	out := make([]any, 0, len(parts))
	for _, part := range parts {
		block, ok := part.(map[string]any)
		if !ok {
			out = append(out, part)
			continue
		}
		typ := strings.TrimSpace(strAny(block["type"], ""))
		if typ != "input_file" && typ != "file" {
			out = append(out, part)
			continue
		}
		text, err := extractInputFileText(block)
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(text) != "" {
			out = append(out, map[string]any{"type": "text", "text": text})
		}
	}
	return out, nil
}

func extractInputFileText(block map[string]any) (string, error) {
	name := firstNonEmpty(strAny(block["file_name"], ""), strAny(block["filename"], ""), strAny(block["name"], ""), "attachment")
	raw := firstNonEmpty(strAny(block["file_data"], ""), strAny(block["data"], ""), strAny(block["content"], ""))
	if raw == "" {
		if file, ok := block["file"].(map[string]any); ok {
			raw = firstNonEmpty(strAny(file["file_data"], ""), strAny(file["data"], ""), strAny(file["content"], ""))
			name = firstNonEmpty(strAny(file["file_name"], ""), strAny(file["filename"], ""), strAny(file["name"], ""), name)
		}
	}
	if raw == "" {
		return "", fmt.Errorf("input_file %q requires file_data as a data URL", name)
	}
	data, mimeType, err := decodeInputFileData(raw)
	if err != nil {
		return "", fmt.Errorf("input_file %q: %w", name, err)
	}
	if len(data) > 4<<20 {
		return "", fmt.Errorf("input_file %q is too large; max supported text extraction size is 4MB", name)
	}
	ext := strings.ToLower(filepath.Ext(name))
	mimeType = strings.ToLower(strings.TrimSpace(firstNonEmpty(strAny(block["mime_type"], ""), strAny(block["mimeType"], ""), mimeType)))
	switch {
	case ext == ".txt" || ext == ".md" || ext == ".markdown" || strings.HasPrefix(mimeType, "text/") || strings.Contains(mimeType, "markdown"):
		return formatExtractedFileText(name, string(data)), nil
	case ext == ".pdf" || strings.Contains(mimeType, "pdf"):
		text, hasImages := extractPDFText(data)
		if strings.TrimSpace(text) == "" {
			if hasImages {
				return "", fmt.Errorf("PDF %q appears to contain embedded images or scanned pages; extracting image-only PDF content is not supported. Please upload a text-based PDF, TXT, or MD file", name)
			}
			return "", fmt.Errorf("PDF %q did not contain extractable text. Please upload a text-based PDF, TXT, or MD file", name)
		}
		return formatExtractedFileText(name, text), nil
	default:
		return "", fmt.Errorf("input_file %q has unsupported type %q; supported files are PDF, TXT, and MD", name, firstNonEmpty(mimeType, ext))
	}
}

func decodeInputFileData(raw string) ([]byte, string, error) {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "data:") {
		head, payload, ok := strings.Cut(raw, ",")
		if !ok {
			return nil, "", errors.New("invalid data URL")
		}
		mimeType := strings.TrimPrefix(strings.Split(strings.TrimPrefix(head, "data:"), ";")[0], " ")
		if strings.Contains(head, ";base64") {
			data, err := base64.StdEncoding.DecodeString(payload)
			if err != nil {
				return nil, mimeType, err
			}
			return data, mimeType, nil
		}
		return []byte(payload), mimeType, nil
	}
	data, err := base64.StdEncoding.DecodeString(raw)
	if err == nil && len(data) > 0 {
		return data, "", nil
	}
	return []byte(raw), "text/plain", nil
}

func formatExtractedFileText(name, text string) string {
	text = strings.TrimSpace(normalizeWhitespace(text))
	if text == "" {
		return ""
	}
	if len([]rune(text)) > 20000 {
		runes := []rune(text)
		text = string(runes[:20000]) + "\n[文件内容已截断，仅保留前 20000 字符]"
	}
	return fmt.Sprintf("\n\n[File: %s]\n%s\n[/File]", name, text)
}

func extractPDFText(data []byte) (string, bool) {
	hasImages := bytes.Contains(data, []byte("/Subtype /Image")) || bytes.Contains(data, []byte("/Subtype/Image"))
	chunks := [][]byte{data}
	streamRe := regexp.MustCompile(`(?s)(<<.*?>>)\s*stream\r?\n?(.*?)\r?\n?endstream`)
	for _, match := range streamRe.FindAllSubmatch(data, -1) {
		dict, stream := match[1], bytes.Trim(match[2], "\r\n")
		if bytes.Contains(dict, []byte("/FlateDecode")) {
			if decoded, err := inflatePDFStream(stream); err == nil && len(decoded) > 0 {
				chunks = append(chunks, decoded)
				continue
			}
		}
		chunks = append(chunks, stream)
	}
	parts := []string{}
	for _, chunk := range chunks {
		parts = append(parts, extractPDFStrings(chunk)...)
	}
	return normalizeWhitespace(strings.Join(parts, " ")), hasImages
}

func inflatePDFStream(data []byte) ([]byte, error) {
	r, err := zlib.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return io.ReadAll(io.LimitReader(r, 4<<20))
}

func extractPDFStrings(data []byte) []string {
	out := []string{}
	literalRe := regexp.MustCompile(`\((?:\\.|[^\\)])*\)`)
	for _, raw := range literalRe.FindAll(data, -1) {
		if s := decodePDFLiteralString(string(raw[1 : len(raw)-1])); isUsefulPDFText(s) {
			out = append(out, s)
		}
	}
	hexRe := regexp.MustCompile(`<([0-9A-Fa-f\s]{4,})>`)
	for _, match := range hexRe.FindAllSubmatch(data, -1) {
		compact := strings.Join(strings.Fields(string(match[1])), "")
		if len(compact)%2 == 1 {
			compact += "0"
		}
		b, err := hex.DecodeString(compact)
		if err != nil || len(b) == 0 {
			continue
		}
		if s := decodeMaybeUTF16BE(b); isUsefulPDFText(s) {
			out = append(out, s)
		}
	}
	return out
}

func decodePDFLiteralString(raw string) string {
	var b strings.Builder
	for i := 0; i < len(raw); i++ {
		ch := raw[i]
		if ch != '\\' || i+1 >= len(raw) {
			b.WriteByte(ch)
			continue
		}
		i++
		switch raw[i] {
		case 'n':
			b.WriteByte('\n')
		case 'r':
			b.WriteByte('\r')
		case 't':
			b.WriteByte('\t')
		case 'b':
			b.WriteByte('\b')
		case 'f':
			b.WriteByte('\f')
		case '(', ')', '\\':
			b.WriteByte(raw[i])
		case '\n', '\r':
			for i+1 < len(raw) && (raw[i+1] == '\n' || raw[i+1] == '\r') {
				i++
			}
		default:
			if raw[i] >= '0' && raw[i] <= '7' {
				end := i + 1
				for end < len(raw) && end < i+3 && raw[end] >= '0' && raw[end] <= '7' {
					end++
				}
				if v, err := strconv.ParseInt(raw[i:end], 8, 32); err == nil {
					b.WriteRune(rune(v))
				}
				i = end - 1
			} else {
				b.WriteByte(raw[i])
			}
		}
	}
	return b.String()
}

func decodeMaybeUTF16BE(data []byte) string {
	if len(data) >= 2 && data[0] == 0xfe && data[1] == 0xff {
		data = data[2:]
	}
	if len(data)%2 == 0 && len(data) >= 2 {
		units := make([]uint16, 0, len(data)/2)
		zeroHigh := 0
		for i := 0; i < len(data); i += 2 {
			if data[i] == 0 {
				zeroHigh++
			}
			units = append(units, uint16(data[i])<<8|uint16(data[i+1]))
		}
		if zeroHigh > len(units)/2 || (len(data) >= 2 && data[0] == 0xfe && data[1] == 0xff) {
			return string(utf16.Decode(units))
		}
	}
	if utf8.Valid(data) {
		return string(data)
	}
	runes := make([]rune, 0, len(data))
	for _, b := range data {
		runes = append(runes, rune(b))
	}
	return string(runes)
}

func isUsefulPDFText(s string) bool {
	s = strings.TrimSpace(s)
	if len([]rune(s)) < 2 {
		return false
	}
	letters := 0
	for _, r := range s {
		if r >= 32 && r != 127 {
			letters++
		}
	}
	return letters >= 2
}

func normalizeWhitespace(s string) string {
	s = strings.ReplaceAll(s, "\x00", "")
	return strings.TrimSpace(regexp.MustCompile(`[ \t\r\n]+`).ReplaceAllString(s, " "))
}

func (s *Server) chatCompletionMessages(body map[string]any) ([]map[string]any, error) {
	messages, err := s.messagesFromBodyWithFiles(body)
	if err != nil {
		return nil, err
	}
	toolPrompt := buildChatToolPrompt(body["tools"], body["tool_choice"])
	if toolPrompt == "" {
		return messages, nil
	}
	return append([]map[string]any{{"role": "system", "content": toolPrompt}}, messages...), nil
}

func (s *Server) responseMessagesFromBody(body map[string]any) ([]map[string]any, error) {
	messages := []map[string]any{}
	if strings.TrimSpace(s.cfg.GlobalSystemPrompt) != "" {
		messages = append(messages, map[string]any{"role": "system", "content": strings.TrimSpace(s.cfg.GlobalSystemPrompt)})
	}
	if instructions := strings.TrimSpace(strAny(body["instructions"], "")); instructions != "" {
		messages = append(messages, map[string]any{"role": "system", "content": instructions})
	}
	input := body["input"]
	switch v := input.(type) {
	case string:
		if strings.TrimSpace(v) != "" {
			messages = append(messages, map[string]any{"role": "user", "content": strings.TrimSpace(v)})
		}
	case map[string]any:
		item := map[string]any{"role": firstNonEmpty(strAny(v["role"], ""), "user"), "content": v["content"]}
		content, err := expandInputFiles(item["content"])
		if err != nil {
			return nil, err
		}
		item["content"] = content
		messages = append(messages, item)
	case []any:
		if responseInputIsContentBlocks(v) {
			content, err := expandInputFiles(v)
			if err != nil {
				return nil, err
			}
			messages = append(messages, map[string]any{"role": "user", "content": content})
		} else {
			for _, raw := range v {
				m, ok := raw.(map[string]any)
				if !ok {
					continue
				}
				item := map[string]any{"role": firstNonEmpty(strAny(m["role"], ""), "user"), "content": m["content"]}
				content, err := expandInputFiles(item["content"])
				if err != nil {
					return nil, err
				}
				item["content"] = content
				messages = append(messages, item)
			}
		}
	}
	if len(messages) == 0 {
		return s.messagesFromBodyWithFiles(body)
	}
	return messages, nil
}

func messagesPlainText(messages []map[string]any) string {
	parts := []string{}
	for _, message := range messages {
		if text := strings.TrimSpace(messageTextAny(message["content"])); text != "" {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, "\n")
}

func responseInputIsContentBlocks(items []any) bool {
	if len(items) == 0 {
		return false
	}
	for _, raw := range items {
		m, ok := raw.(map[string]any)
		if !ok {
			return false
		}
		if strings.TrimSpace(strAny(m["role"], "")) != "" {
			return false
		}
		if strings.TrimSpace(strAny(m["type"], "")) == "" {
			return false
		}
	}
	return true
}

func buildChatToolPrompt(tools any, toolChoice any) string {
	if isToolChoiceNone(toolChoice) {
		return ""
	}
	prompt := buildToolPrompt(tools)
	if prompt == "" {
		return ""
	}
	switch choice := toolChoice.(type) {
	case string:
		if strings.TrimSpace(choice) == "required" {
			prompt += "\n- The current request requires a tool call."
		}
	case map[string]any:
		name := ""
		if fn, ok := choice["function"].(map[string]any); ok {
			name = strAny(fn["name"], "")
		}
		name = firstNonEmpty(name, strAny(choice["name"], ""))
		if strings.TrimSpace(name) != "" {
			prompt += "\n- The current request must call the tool named " + strings.TrimSpace(name) + "."
		}
	}
	return prompt
}

func isToolChoiceNone(toolChoice any) bool {
	if strings.TrimSpace(strAny(toolChoice, "")) == "none" {
		return true
	}
	if m, ok := toolChoice.(map[string]any); ok && strings.TrimSpace(strAny(m["type"], "")) == "none" {
		return true
	}
	return false
}

func openAIToolCallsFromText(text string, tools any) []map[string]any {
	if !toolsListNonEmpty(tools) {
		return nil
	}
	parsed := parseToolCalls(text)
	calls := make([]map[string]any, 0, len(parsed))
	for i, call := range parsed {
		name := strings.TrimSpace(strAny(call["name"], ""))
		if name == "" {
			continue
		}
		args, _ := json.Marshal(call["input"])
		calls = append(calls, map[string]any{
			"id":   fmt.Sprintf("call_%s_%d", randID(4), i),
			"type": "function",
			"function": map[string]any{
				"name":      name,
				"arguments": string(args),
			},
		})
	}
	return calls
}

func chatCompletionMessage(content string, toolCalls []map[string]any) map[string]any {
	message := map[string]any{"role": "assistant", "content": content}
	if len(toolCalls) > 0 {
		if strings.TrimSpace(content) == "" {
			message["content"] = nil
		}
		message["tool_calls"] = toolCalls
	}
	return message
}

func chatCompletionFinishReason(toolCalls []map[string]any) string {
	if len(toolCalls) > 0 {
		return "tool_calls"
	}
	return "stop"
}
