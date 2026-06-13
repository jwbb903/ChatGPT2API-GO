package app

import "strings"

func (s *Server) mergeDynamicModels(result map[string]any) map[string]any {
	data, _ := result["data"].([]map[string]any)
	if data == nil {
		if raw, ok := result["data"].([]any); ok {
			for _, item := range raw {
				if m, ok := item.(map[string]any); ok {
					data = append(data, m)
				}
			}
		}
	}
	seen := map[string]bool{}
	for _, item := range data {
		seen[strings.TrimSpace(strAny(item["id"], ""))] = true
	}
	accounts := s.store.LoadAccounts()
	hasWeb := false
	codexTypes := map[string]bool{}
	for _, a := range accounts {
		if a.AccessToken == "" {
			continue
		}
		if strings.ToLower(a.SourceType) == "codex" {
			codexTypes[strings.ToLower(a.Type)] = true
		} else {
			hasWeb = true
		}
	}
	add := func(id string) {
		if id != "" && !seen[id] {
			seen[id] = true
			data = append(data, map[string]any{"id": id, "object": "model", "created": 0, "owned_by": "chatgpt2api", "permission": []any{}, "root": id, "parent": nil})
		}
	}
	if hasWeb {
		add("gpt-image-2")
	}
	if codexTypes["plus"] || codexTypes["team"] || codexTypes["pro"] {
		add("codex-gpt-image-2")
	}
	if codexTypes["plus"] {
		add("plus-codex-gpt-image-2")
	}
	if codexTypes["team"] {
		add("team-codex-gpt-image-2")
	}
	if codexTypes["pro"] {
		add("pro-codex-gpt-image-2")
	}
	result["data"] = data
	return result
}
