package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

func (s *Server) checkContent(text string) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	lower := strings.ToLower(text)
	for _, word := range s.cfg.SensitiveWords {
		w := strings.ToLower(strings.TrimSpace(word))
		if w != "" && strings.Contains(lower, w) {
			return fmt.Errorf("请求包含敏感词：%s", word)
		}
	}
	if !boolAny(s.cfg.AIReview["enabled"], false) {
		return nil
	}
	baseURL := strings.TrimRight(strAny(s.cfg.AIReview["base_url"], ""), "/")
	apiKey := strings.TrimSpace(strAny(s.cfg.AIReview["api_key"], ""))
	model := strings.TrimSpace(strAny(s.cfg.AIReview["model"], ""))
	promptTpl := strings.TrimSpace(strAny(s.cfg.AIReview["prompt"], ""))
	if baseURL == "" || apiKey == "" || model == "" {
		return nil
	}
	if promptTpl == "" {
		promptTpl = "请判断下面内容是否安全，安全只回复 SAFE，不安全只回复 BLOCK，并给出简短原因：\n\n{{text}}"
	}
	prompt := strings.ReplaceAll(promptTpl, "{{text}}", text)
	body := map[string]any{
		"model":    model,
		"messages": []map[string]any{{"role": "user", "content": prompt}},
		"stream":   false,
	}
	payload, _ := json.Marshal(body)
	req, err := http.NewRequest(http.MethodPost, baseURL+"/v1/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return nil
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	var result map[string]any
	if json.NewDecoder(resp.Body).Decode(&result) != nil {
		return nil
	}
	content := strings.ToLower(extractReviewContent(result))
	if strings.Contains(content, "block") || strings.Contains(content, "不安全") || strings.Contains(content, "拒绝") {
		return fmt.Errorf("AI 内容审查未通过：%s", extractReviewContent(result))
	}
	return nil
}

func extractReviewContent(result map[string]any) string {
	choices, _ := result["choices"].([]any)
	if len(choices) == 0 {
		return ""
	}
	choice, _ := choices[0].(map[string]any)
	message, _ := choice["message"].(map[string]any)
	return strAny(message["content"], "")
}
