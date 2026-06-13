package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type logService struct {
	mu   sync.Mutex
	path string
}

func newLogService(dataDir string) *logService {
	path := filepath.Join(dataDir, "logs.jsonl")
	return &logService{path: path}
}

func (l *logService) add(typ, summary string, detail map[string]any) {
	item := map[string]any{
		"id":      randID(8),
		"time":    time.Now().Format("2006-01-02 15:04:05"),
		"type":    typ,
		"summary": summary,
		"detail":  detail,
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	f, err := os.OpenFile(l.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	data, _ := json.Marshal(item)
	f.Write(data)
	f.Write([]byte{'\n'})
}

func (s *Server) logCallStart(id *Identity, endpoint, model, action, requestText string) string {
	callID := randID(8)
	s.logMu.Lock()
	if s.callStarts != nil {
		s.callStarts[callID] = time.Now()
	}
	s.logMu.Unlock()
	detail := map[string]any{"call_id": callID, "endpoint": endpoint, "model": model, "action": action, "status": "started"}
	if id != nil {
		detail["subject_id"] = id.ID
		detail["role"] = id.Role
		detail["name"] = id.Name
	}
	if strings.TrimSpace(requestText) != "" {
		detail["request_text"] = truncateText(requestText, 500)
	}
	s.logSvc.add("call", action+"开始", detail)
	return callID
}

func (s *Server) logCallSuccess(callID, endpoint, model, action string, extra map[string]any) {
	detail := map[string]any{"call_id": callID, "endpoint": endpoint, "model": model, "action": action, "status": "success"}
	if duration := s.callDurationMS(callID); duration >= 0 {
		detail["duration_ms"] = duration
	}
	for k, v := range extra {
		detail[k] = v
	}
	s.logSvc.add("call", action+"完成", detail)
}

func (s *Server) logCallFailure(callID, endpoint, model, action string, err error, extra map[string]any) {
	detail := map[string]any{"call_id": callID, "endpoint": endpoint, "model": model, "action": action, "status": "failed"}
	if duration := s.callDurationMS(callID); duration >= 0 {
		detail["duration_ms"] = duration
	}
	if err != nil {
		detail["error"] = err.Error()
	}
	for k, v := range extra {
		detail[k] = v
	}
	s.logSvc.add("call", action+"失败", detail)
}

func (s *Server) callDurationMS(callID string) int64 {
	if callID == "" {
		return -1
	}
	s.logMu.Lock()
	defer s.logMu.Unlock()
	start, ok := s.callStarts[callID]
	if !ok {
		return -1
	}
	delete(s.callStarts, callID)
	return time.Since(start).Milliseconds()
}

func truncateText(s string, n int) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) <= n {
		return string(r)
	}
	return string(r[:n]) + "..."
}

func (l *logService) list(typ, startDate, endDate string, limit int) []map[string]any {
	return l.listFiltered(typ, startDate, endDate, "", "", "", "", limit)
}

func (l *logService) listFiltered(typ, startDate, endDate, status, endpoint, model, query string, limit int) []map[string]any {
	l.mu.Lock()
	defer l.mu.Unlock()
	if limit <= 0 {
		limit = 200
	}
	b, err := os.ReadFile(l.path)
	if err != nil {
		return []map[string]any{}
	}
	lines := strings.Split(strings.TrimRight(string(b), "\n"), "\n")
	// 从后往前读，取 limit 条
	var items []map[string]any
	for i := len(lines) - 1; i >= 0 && len(items) < limit; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		var item map[string]any
		if json.Unmarshal([]byte(line), &item) != nil {
			continue
		}
		t := strings.TrimSpace(strAny(item["time"], ""))
		day := ""
		if len(t) >= 10 {
			day = t[:10]
		}
		if typ != "" && strAny(item["type"], "") != typ {
			continue
		}
		detail, _ := item["detail"].(map[string]any)
		if status != "" && strAny(detail["status"], "") != status {
			continue
		}
		if endpoint != "" && strAny(detail["endpoint"], "") != endpoint {
			continue
		}
		if model != "" && strAny(detail["model"], "") != model {
			continue
		}
		if query != "" {
			blob := strings.ToLower(strAny(item["summary"], "") + " " + strAny(detail["request_text"], "") + " " + strAny(detail["error"], ""))
			if !strings.Contains(blob, strings.ToLower(query)) {
				continue
			}
		}
		if startDate != "" && day < startDate {
			continue
		}
		if endDate != "" && day > endDate {
			continue
		}
		if item["id"] == nil || strAny(item["id"], "") == "" {
			item["id"] = randID(8)
		}
		items = append(items, item)
	}
	// 反转恢复时间顺序
	sort.Slice(items, func(i, j int) bool {
		return strAny(items[i]["time"], "") > strAny(items[j]["time"], "")
	})
	return items
}

func (l *logService) delete(ids []string) int {
	if len(ids) == 0 {
		return 0
	}
	del := map[string]bool{}
	for _, id := range ids {
		del[id] = true
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	b, err := os.ReadFile(l.path)
	if err != nil {
		return 0
	}
	lines := strings.Split(strings.TrimRight(string(b), "\n"), "\n")
	var kept []string
	removed := 0
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var item map[string]any
		if json.Unmarshal([]byte(line), &item) != nil {
			kept = append(kept, line)
			continue
		}
		id := strAny(item["id"], "")
		if del[id] {
			removed++
			continue
		}
		kept = append(kept, line)
	}
	if err := os.WriteFile(l.path, []byte(strings.Join(kept, "\n")+"\n"), 0644); err != nil {
		return 0
	}
	return removed
}
