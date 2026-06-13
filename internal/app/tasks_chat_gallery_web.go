package app

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func parseTimeAny(value string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339, value); err == nil {
		return t, nil
	}
	if t, err := time.Parse("2006-01-02 15:04:05", value); err == nil {
		return t, nil
	}
	return time.Time{}, errors.New("invalid time")
}

func parseCSV(value string) []string {
	parts := strings.Split(value, ",")
	out := []string{}
	for _, part := range parts {
		if item := strings.TrimSpace(part); item != "" {
			out = append(out, item)
		}
	}
	return out
}

func (s *Server) handleImageTasks(w http.ResponseWriter, r *http.Request) {
	id, ok := s.requireIdentity(w, r)
	if !ok {
		return
	}
	wanted := parseCSV(r.URL.Query().Get("ids"))
	wantMap := map[string]bool{}
	for _, taskID := range wanted {
		wantMap[taskID] = true
	}
	items := []ImageTask{}
	seen := map[string]bool{}
	for _, task := range s.store.LoadTasks() {
		if task.OwnerID != "" && task.OwnerID != id.ID && id.Role != "admin" {
			continue
		}
		if len(wantMap) > 0 && !wantMap[task.ID] {
			continue
		}
		items = append(items, task)
		seen[task.ID] = true
	}
	missing := []string{}
	for _, taskID := range wanted {
		if !seen[taskID] {
			missing = append(missing, taskID)
		}
	}
	writeJSON(w, 200, map[string]any{"items": items, "missing_ids": missing})
}
func (s *Server) saveTask(t ImageTask) {
	s.upsertTask(t)
}
func (s *Server) upsertTask(t ImageTask) {
	t.UpdatedAt = nowISO()
	tasks := s.store.LoadTasks()
	for i := range tasks {
		if tasks[i].ID == t.ID {
			tasks[i] = t
			_ = s.store.SaveTasks(tasks)
			return
		}
	}
	tasks = append([]ImageTask{t}, tasks...)
	_ = s.store.SaveTasks(tasks)
}
func (s *Server) setTaskCancel(id string, cancel context.CancelFunc) {
	s.taskMu.Lock()
	defer s.taskMu.Unlock()
	s.taskCancels[id] = cancel
}
func (s *Server) popTaskCancel(id string) context.CancelFunc {
	s.taskMu.Lock()
	defer s.taskMu.Unlock()
	cancel := s.taskCancels[id]
	delete(s.taskCancels, id)
	return cancel
}
func (s *Server) recoverUnfinishedTasks() {
	tasks := s.store.LoadTasks()
	changed := false
	for i := range tasks {
		if tasks[i].Status == "running" || tasks[i].Status == "queued" {
			tasks[i].Status = "error"
			tasks[i].Error = "server restarted"
			tasks[i].UpdatedAt = nowISO()
			changed = true
		}
	}
	if changed {
		_ = s.store.SaveTasks(tasks)
		if s.logSvc != nil {
			s.logSvc.add("system", "恢复未完成图片任务", map[string]any{"status": "recovered"})
		}
	}
}

func (s *Server) cleanupOldTasks() {
	days := s.cfg.ImageRetentionDays
	if days <= 0 {
		return
	}
	cutoff := time.Now().AddDate(0, 0, -days)
	tasks := s.store.LoadTasks()
	out := tasks[:0]
	removed := 0
	for _, task := range tasks {
		if task.Status != "success" && task.Status != "error" && task.Status != "canceled" {
			out = append(out, task)
			continue
		}
		updated, err := parseTimeAny(task.UpdatedAt)
		if err != nil || updated.After(cutoff) {
			out = append(out, task)
			continue
		}
		removed++
	}
	if removed > 0 {
		_ = s.store.SaveTasks(out)
		if s.logSvc != nil {
			s.logSvc.add("system", "清理旧图片任务", map[string]any{"removed": removed, "retention_days": days})
		}
	}
}

func (s *Server) updateTaskStatus(id, status, errText string, data []map[string]any) {
	tasks := s.store.LoadTasks()
	for i := range tasks {
		if tasks[i].ID == id {
			tasks[i].Status = status
			tasks[i].Error = errText
			tasks[i].Data = data
			tasks[i].UpdatedAt = nowISO()
			_ = s.store.SaveTasks(tasks)
			return
		}
	}
}
func (s *Server) handleImageTaskGeneration(w http.ResponseWriter, r *http.Request) {
	id, ok := s.requireIdentity(w, r)
	if !ok {
		return
	}
	var b struct {
		ClientTaskID string `json:"client_task_id"`
		Prompt       string `json:"prompt"`
		Model        string `json:"model"`
		N            int    `json:"n"`
		Size         string `json:"size"`
		Resolution   string `json:"resolution"`
	}
	if !readBody(w, r, &b) {
		return
	}
	if b.N < 1 {
		b.N = 1
	}
	if b.N > 4 {
		b.N = 4
	}
	t := ImageTask{ID: firstNonEmpty(strings.TrimSpace(b.ClientTaskID), randID(8)), OwnerID: id.ID, Status: "running", Mode: "generate", Model: b.Model, Size: b.Size, Resolution: b.Resolution, CreatedAt: nowISO(), UpdatedAt: nowISO()}
	callID := s.logCallStart(id, "/api/image-tasks/generations", b.Model, "文生图任务", b.Prompt)
	if err := s.checkContent(b.Prompt); err != nil {
		t.Status = "error"
		t.Error = err.Error()
		s.logCallFailure(callID, "/api/image-tasks/generations", b.Model, "文生图任务", err, nil)
		s.saveTask(t)
		writeJSON(w, 200, t)
		return
	}
	if !s.consumeImage(id, b.N) {
		t.Status = "error"
		t.Error = "画图额度不足"
		s.saveTask(t)
		writeJSON(w, 200, t)
		return
	}
	s.saveTask(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	s.setTaskCancel(t.ID, cancel)
	go func(task ImageTask, identity *Identity) {
		defer s.popTaskCancel(task.ID)
		defer cancel()
		data := []map[string]any{}
		for i := 0; i < b.N; i++ {
			items, err := s.generateImageWithPool(ctx, b.Prompt, b.Model, b.Size, b.Resolution, nil)
			if err != nil {
				s.refundImage(identity, b.N)
				if ctx.Err() != nil {
					s.updateTaskStatus(task.ID, "canceled", "canceled", nil)
					s.logCallFailure(callID, "/api/image-tasks/generations", b.Model, "文生图任务", errors.New("canceled"), map[string]any{"task_id": task.ID})
				} else {
					s.updateTaskStatus(task.ID, "error", err.Error(), nil)
					s.logCallFailure(callID, "/api/image-tasks/generations", b.Model, "文生图任务", err, map[string]any{"task_id": task.ID})
				}
				return
			}
			for _, res := range items {
				rel, url, err := s.saveImage(r, res.Bytes)
				if err != nil {
					s.refundImage(identity, b.N)
					s.updateTaskStatus(task.ID, "error", err.Error(), nil)
					return
				}
				s.recordOwner(identity, rel)
				s.recordPrompt(rel, b.Prompt, false)
				data = append(data, map[string]any{"url": url, "b64_json": base64.StdEncoding.EncodeToString(res.Bytes), "revised_prompt": firstNonEmpty(res.RevisedPrompt, b.Prompt)})
				break
			}
		}
		s.updateTaskStatus(task.ID, "success", "", data)
		s.logCallSuccess(callID, "/api/image-tasks/generations", b.Model, "文生图任务", map[string]any{"task_id": task.ID, "image_count": len(data)})
	}(t, id)
	writeJSON(w, 200, t)
}
func (s *Server) handleImageTaskEdit(w http.ResponseWriter, r *http.Request) {
	id, ok := s.requireIdentity(w, r)
	if !ok {
		return
	}
	_ = r.ParseMultipartForm(64 << 20)
	prompt := r.FormValue("prompt")
	model := r.FormValue("model")
	if model == "" {
		model = "gpt-image-2"
	}
	t := ImageTask{ID: firstNonEmpty(strings.TrimSpace(r.FormValue("client_task_id")), randID(8)), OwnerID: id.ID, Status: "running", Mode: "edit", Model: model, Size: r.FormValue("size"), Resolution: r.FormValue("resolution"), CreatedAt: nowISO(), UpdatedAt: nowISO()}
	callID := s.logCallStart(id, "/api/image-tasks/edits", model, "图生图任务", prompt)
	if err := s.checkContent(prompt); err != nil {
		t.Status = "error"
		t.Error = err.Error()
		s.logCallFailure(callID, "/api/image-tasks/edits", model, "图生图任务", err, nil)
		s.saveTask(t)
		writeJSON(w, 200, t)
		return
	}
	if !s.consumeImage(id, 1) {
		t.Status = "error"
		t.Error = "画图额度不足"
		s.saveTask(t)
		writeJSON(w, 200, t)
		return
	}
	inputs := [][]byte{}
	for _, key := range []string{"image", "image[]"} {
		for _, fh := range r.MultipartForm.File[key] {
			f, err := fh.Open()
			if err != nil {
				continue
			}
			b, _ := io.ReadAll(f)
			_ = f.Close()
			if len(b) > 0 {
				inputs = append(inputs, b)
			}
		}
	}
	if len(inputs) == 0 {
		s.refundImage(id, 1)
		t.Status = "error"
		t.Error = "image file is required"
		s.saveTask(t)
		writeJSON(w, 200, t)
		return
	}
	s.saveTask(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	s.setTaskCancel(t.ID, cancel)
	go func(task ImageTask, identity *Identity) {
		defer s.popTaskCancel(task.ID)
		defer cancel()
		items, err := s.generateImageWithPool(ctx, prompt, model, r.FormValue("size"), r.FormValue("resolution"), inputs)
		if err != nil {
			s.refundImage(identity, 1)
			if ctx.Err() != nil {
				s.updateTaskStatus(task.ID, "canceled", "canceled", nil)
				s.logCallFailure(callID, "/api/image-tasks/edits", model, "图生图任务", errors.New("canceled"), map[string]any{"task_id": task.ID})
			} else {
				s.updateTaskStatus(task.ID, "error", err.Error(), nil)
				s.logCallFailure(callID, "/api/image-tasks/edits", model, "图生图任务", err, map[string]any{"task_id": task.ID})
			}
			return
		}
		data := []map[string]any{}
		for _, res := range items {
			rel, url, err := s.saveImage(r, res.Bytes)
			if err != nil {
				s.refundImage(identity, 1)
				s.updateTaskStatus(task.ID, "error", err.Error(), nil)
				return
			}
			s.recordOwner(identity, rel)
			s.recordPrompt(rel, prompt, true)
			data = append(data, map[string]any{"url": url, "b64_json": base64.StdEncoding.EncodeToString(res.Bytes), "revised_prompt": firstNonEmpty(res.RevisedPrompt, prompt)})
			break
		}
		s.updateTaskStatus(task.ID, "success", "", data)
		s.logCallSuccess(callID, "/api/image-tasks/edits", model, "图生图任务", map[string]any{"task_id": task.ID, "image_count": len(data)})
	}(t, id)
	writeJSON(w, 200, t)
}
func (s *Server) handleImageTaskCancel(w http.ResponseWriter, r *http.Request) {
	identity, ok := s.requireIdentity(w, r)
	if !ok {
		return
	}
	var b map[string]any
	_ = json.NewDecoder(r.Body).Decode(&b)
	ids := []string{}
	if arr, ok := b["ids"].([]any); ok {
		for _, item := range arr {
			if v := strings.TrimSpace(strAny(item, "")); v != "" {
				ids = append(ids, v)
			}
		}
	}
	if v := strings.TrimSpace(strAny(b["id"], "")); v != "" {
		ids = append(ids, v)
	}
	canceled := []string{}
	skipped := []string{}
	missing := []string{}
	tasks := s.store.LoadTasks()
	byID := map[string]int{}
	for i, t := range tasks {
		byID[t.ID] = i
	}
	for _, id := range ids {
		idx, ok := byID[id]
		if !ok {
			missing = append(missing, id)
			continue
		}
		if tasks[idx].OwnerID != "" && tasks[idx].OwnerID != identity.ID && identity.Role != "admin" {
			skipped = append(skipped, id)
			continue
		}
		if tasks[idx].Status != "running" && tasks[idx].Status != "queued" && tasks[idx].Status != "pending" {
			skipped = append(skipped, id)
			continue
		}
		if cancel := s.popTaskCancel(id); cancel != nil {
			cancel()
		}
		tasks[idx].Status = "canceled"
		tasks[idx].Error = "canceled"
		tasks[idx].UpdatedAt = nowISO()
		canceled = append(canceled, id)
	}
	_ = s.store.SaveTasks(tasks)
	writeJSON(w, 200, map[string]any{"canceled": canceled, "skipped": skipped, "missing_ids": missing})
}

func (s *Server) handleChatStream(w http.ResponseWriter, r *http.Request) {
	id, ok := s.requireIdentity(w, r)
	if !ok {
		return
	}
	var b map[string]any
	_ = json.NewDecoder(r.Body).Decode(&b)
	model := strAny(b["model"], "auto")
	requestText := extractPrompt(b)
	callID := s.logCallStart(id, "/api/chat/stream", model, "聊天", requestText)
	if isImageChatRequest(b) {
		s.handleChatStreamImage(w, r, id, b, callID)
		return
	}
	if err := s.checkContent(requestText); err != nil {
		s.logCallFailure(callID, "/api/chat/stream", model, "聊天", err, nil)
		writeErr(w, 400, err.Error())
		return
	}
	if !s.consumeChat(id, 1) {
		writeErr(w, 402, "对话额度不足")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()
	requestedUpstreamCID := strings.TrimSpace(strAny(b["upstream_conversation_id"], strAny(b["conversation_id"], "")))
	_ = requestedUpstreamCID
	upstreamCID := ""
	messages, err := s.messagesFromBodyWithFiles(b)
	if err != nil {
		s.refundChat(id, 1)
		s.logCallFailure(callID, "/api/chat/stream", model, "聊天", err, nil)
		writeErr(w, 400, err.Error())
		return
	}
	events, errs := s.streamChatWithRetry(ctx, messages, model, "", "")
	w.Header().Set("Content-Type", "text/event-stream")
	text := ""
	cid := "conv_" + randID(8)
	upstreamMessageID := ""
	currentNode := ""
	accountToken := ""
	fileIDs := []string{}
	sedimentIDs := []string{}
	toolInvoked := any(nil)
	turnUseCase := ""
	blocked := false
	for ev := range events {
		if ev.ConversationID != "" {
			upstreamCID = ev.ConversationID
		}
		if ev.MessageID != "" {
			upstreamMessageID = ev.MessageID
		}
		if ev.CurrentNode != "" {
			currentNode = ev.CurrentNode
		}
		if ev.AccountToken != "" {
			accountToken = ev.AccountToken
		}
		fileIDs = unique(append(fileIDs, ev.FileIDs...))
		sedimentIDs = unique(append(sedimentIDs, ev.SedimentIDs...))
		if ev.ToolInvoked != nil {
			toolInvoked = ev.ToolInvoked
		}
		if ev.TurnUseCase != "" {
			turnUseCase = ev.TurnUseCase
		}
		if ev.Blocked {
			blocked = true
		}
		if ev.Delta == "" {
			continue
		}
		text += ev.Delta
		sse(w, map[string]any{"type": "conversation.delta", "delta": ev.Delta, "text": text, "conversation_id": firstNonEmpty(upstreamCID, cid), "upstream_conversation_id": upstreamCID, "message_id": upstreamMessageID, "current_node": currentNode, "file_ids": fileIDs, "sediment_ids": sedimentIDs, "blocked": ev.Blocked, "tool_invoked": ev.ToolInvoked, "turn_use_case": ev.TurnUseCase, "done": false})
	}
	if err := <-errs; err != nil {
		s.refundChat(id, 1)
		s.logCallFailure(callID, "/api/chat/stream", model, "聊天", err, map[string]any{"stream": true})
		sse(w, map[string]any{"type": "conversation.error", "error": err.Error(), "done": true})
		sseDone(w)
		return
	}
	doneCID := firstNonEmpty(upstreamCID, cid)
	if doneCID != "" && accountToken != "" && (len(fileIDs) > 0 || len(sedimentIDs) > 0 || toolInvoked == true) {
		imageText, ok := s.resolveChatStreamImages(ctx, r, id, accountToken, doneCID, fileIDs, sedimentIDs, text, len(extractChatImages(b)) > 0)
		if ok {
			text = imageText
			sse(w, map[string]any{"type": "conversation.delta", "delta": imageText, "text": imageText, "conversation_id": doneCID, "upstream_conversation_id": doneCID, "message_id": upstreamMessageID, "current_node": currentNode, "file_ids": fileIDs, "sediment_ids": sedimentIDs, "blocked": blocked, "tool_invoked": toolInvoked, "turn_use_case": turnUseCase, "done": false})
		}
	}
	s.upsertChatConversationFromStream(id, b, doneCID, upstreamMessageID, currentNode, "", text)
	s.logCallSuccess(callID, "/api/chat/stream", model, "聊天", map[string]any{"stream": true, "output_tokens": approxTokens(text), "file_ids": fileIDs, "sediment_ids": sedimentIDs})
	sse(w, map[string]any{"type": "conversation.done", "text": text, "conversation_id": doneCID, "upstream_conversation_id": doneCID, "message_id": upstreamMessageID, "current_node": currentNode, "file_ids": fileIDs, "sediment_ids": sedimentIDs, "blocked": blocked, "tool_invoked": toolInvoked, "turn_use_case": turnUseCase, "done": true})
	sseDone(w)
}
func (s *Server) resolveChatStreamImages(ctx context.Context, r *http.Request, id *Identity, token, conversationID string, fileIDs, sedimentIDs []string, fallbackText string, isEdit bool) (string, bool) {
	client, err := NewUpstreamClientForAccount(s.accountByToken(token), s.cfg.Proxy, s.ensureCurlImpersonateBinary)
	if err != nil {
		return fallbackText, false
	}
	if len(fileIDs) == 0 && len(sedimentIDs) == 0 && conversationID != "" {
		f, sed := client.pollImageIDs(ctx, conversationID, time.Duration(s.cfg.ImagePollTimeoutSecs)*time.Second)
		fileIDs = append(fileIDs, f...)
		sedimentIDs = append(sedimentIDs, sed...)
	}
	urls, err := client.resolveImageURLs(ctx, conversationID, fileIDs, sedimentIDs)
	if err != nil || len(urls) == 0 {
		return fallbackText, false
	}
	data := []map[string]any{}
	for _, u := range urls {
		bytes, err := client.download(ctx, u)
		if err != nil {
			continue
		}
		rel, url, err := s.saveImage(r, bytes)
		if err != nil {
			continue
		}
		s.recordOwner(id, rel)
		s.recordPrompt(rel, fallbackText, isEdit)
		data = append(data, map[string]any{"url": url, "b64_json": base64.StdEncoding.EncodeToString(bytes), "revised_prompt": fallbackText})
	}
	if len(data) == 0 {
		return fallbackText, false
	}
	return buildChatImageMarkdown(data), true
}

func (s *Server) handleChatStreamImage(w http.ResponseWriter, r *http.Request, id *Identity, b map[string]any, callID string) {
	model := strAny(b["model"], "gpt-image-2")
	if !isSupportedImageModel(model) {
		model = "gpt-image-2"
	}
	prompt := extractChatPrompt(b)
	if prompt == "" {
		err := errors.New("prompt is required")
		s.logCallFailure(callID, "/api/chat/stream", model, "聊天", err, nil)
		writeErr(w, 400, err.Error())
		return
	}
	if err := s.checkContent(prompt); err != nil {
		s.logCallFailure(callID, "/api/chat/stream", model, "聊天", err, nil)
		writeErr(w, 400, err.Error())
		return
	}
	if !s.consumeImage(id, 1) {
		writeErr(w, 402, "画图额度不足")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()
	items, err := s.generateImageWithPool(ctx, prompt, model, strAny(b["size"], ""), strAny(b["resolution"], ""), extractChatImages(b))
	w.Header().Set("Content-Type", "text/event-stream")
	cid := "conv_" + randID(8)
	if err != nil {
		s.refundImage(id, 1)
		s.logCallFailure(callID, "/api/chat/stream", model, "聊天", err, map[string]any{"image": true})
		sse(w, map[string]any{"type": "conversation.error", "error": err.Error(), "done": true})
		sseDone(w)
		return
	}
	data := []map[string]any{}
	for _, res := range items {
		rel, url, err := s.saveImage(r, res.Bytes)
		if err != nil {
			s.refundImage(id, 1)
			sse(w, map[string]any{"type": "conversation.error", "error": err.Error(), "done": true})
			sseDone(w)
			return
		}
		s.recordOwner(id, rel)
		s.recordPrompt(rel, prompt, len(extractChatImages(b)) > 0)
		data = append(data, map[string]any{"url": url, "b64_json": base64.StdEncoding.EncodeToString(res.Bytes), "revised_prompt": firstNonEmpty(res.RevisedPrompt, prompt)})
		break
	}
	text := buildChatImageMarkdown(data)
	s.logCallSuccess(callID, "/api/chat/stream", model, "聊天", map[string]any{"image": true, "image_count": len(data)})
	sse(w, map[string]any{"type": "conversation.delta", "delta": text, "text": text, "conversation_id": cid, "done": false})
	sse(w, map[string]any{"type": "conversation.done", "text": text, "conversation_id": cid, "done": true})
	sseDone(w)
}

func (s *Server) upsertChatConversationFromStream(id *Identity, b map[string]any, upstreamCID, messageID, currentNode, token, assistantText string) {
	convID := strings.TrimSpace(strAny(b["id"], strAny(b["conversation_local_id"], "")))
	if convID == "" {
		return
	}
	item := map[string]any{}
	for k, v := range b {
		item[k] = v
	}
	item["id"] = convID
	item["owner_id"] = id.ID
	item["upstream_conversation_id"] = upstreamCID
	item["upstream_message_id"] = messageID
	item["current_node"] = currentNode
	item["upstream_account_token"] = token
	item["updated_at"] = nowISO()
	if item["created_at"] == nil || strings.TrimSpace(strAny(item["created_at"], "")) == "" {
		item["created_at"] = nowISO()
	}
	if strings.TrimSpace(strAny(item["title"], "")) == "" {
		item["title"] = truncateText(extractPrompt(b), 40)
	}
	if assistantText != "" {
		item["last_text"] = truncateText(assistantText, 500)
	}
	items := s.store.LoadList("chat_conversations.json")
	out := []map[string]any{item}
	for _, it := range items {
		if strAny(it["id"], "") != convID {
			out = append(out, it)
		}
	}
	_ = s.store.SaveList("chat_conversations.json", out)
}

func (s *Server) handleChatAccountTypes(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireIdentity(w, r); !ok {
		return
	}
	types := map[string]bool{}
	for _, a := range s.store.LoadAccounts() {
		types[a.Type] = true
	}
	arr := []map[string]any{}
	keys := make([]string, 0, len(types))
	for t := range types {
		if t != "" {
			keys = append(keys, t)
		}
	}
	sort.Strings(keys)
	for _, t := range keys {
		arr = append(arr, map[string]any{"type": t, "label": t})
	}
	if len(arr) == 0 {
		arr = []map[string]any{{"type": "free", "label": "free"}}
	}
	writeJSON(w, 200, map[string]any{"items": arr})
}
func (s *Server) handleChatConversations(w http.ResponseWriter, r *http.Request) {
	identity, ok := s.requireIdentity(w, r)
	if !ok {
		return
	}
	if r.Method == http.MethodGet {
		items := []map[string]any{}
		for _, it := range s.store.LoadList("chat_conversations.json") {
			if strAny(it["owner_id"], identity.ID) == identity.ID || identity.Role == "admin" {
				items = append(items, it)
			}
		}
		writeJSON(w, 200, map[string]any{"items": items})
		return
	}
	if r.Method == http.MethodPost {
		var b map[string]any
		if !readBody(w, r, &b) {
			return
		}
		if strings.TrimSpace(strAny(b["id"], "")) == "" {
			b["id"] = "conv_" + randID(8)
		}
		b["owner_id"] = identity.ID
		b["updated_at"] = nowISO()
		if strings.TrimSpace(strAny(b["created_at"], "")) == "" {
			b["created_at"] = nowISO()
		}
		items := s.store.LoadList("chat_conversations.json")
		out := []map[string]any{b}
		for _, it := range items {
			if strAny(it["id"], "") != strAny(b["id"], "") {
				out = append(out, it)
			}
		}
		_ = s.store.SaveList("chat_conversations.json", out)
		writeJSON(w, 200, map[string]any{"item": b})
		return
	}
	writeErr(w, 405, "method not allowed")
}
func (s *Server) handleChatConversationID(w http.ResponseWriter, r *http.Request) {
	identity, ok := s.requireIdentity(w, r)
	if !ok {
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/chat/conversations/")
	items := s.store.LoadList("chat_conversations.json")
	out := []map[string]any{}
	deleted := false
	for _, it := range items {
		owner := strAny(it["owner_id"], "")
		if strAny(it["id"], "") == id && (owner == "" || owner == identity.ID || identity.Role == "admin") {
			deleted = true
			upstreamCID := strings.TrimSpace(strAny(it["upstream_conversation_id"], ""))
			upstreamToken := strings.TrimSpace(strAny(it["upstream_account_token"], ""))
			if upstreamCID != "" && upstreamToken != "" {
				go func(cid, token string) {
					ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
					defer cancel()
					if c, err := NewUpstreamClientForAccount(s.accountByToken(token), s.cfg.Proxy, s.ensureCurlImpersonateBinary); err == nil {
						c.DeleteConversation(ctx, cid)
					}
				}(upstreamCID, upstreamToken)
			}
			continue
		}
		out = append(out, it)
	}
	_ = s.store.SaveList("chat_conversations.json", out)
	writeJSON(w, 200, map[string]any{"ok": deleted})
}

func (s *Server) publicGalleryItem(r *http.Request, it GalleryItem) map[string]any {
	return map[string]any{"id": it.ID, "image_rel": it.ImageRel, "image_url": s.baseURL(r) + "/images/" + it.ImageRel, "url": s.baseURL(r) + "/images/" + it.ImageRel, "publisher_name": it.PublisherName, "prompt": it.Prompt, "model": it.Model, "size": it.Size, "width": it.Width, "height": it.Height, "is_edit": it.IsEdit, "created_at": it.CreatedAt, "status": it.Status}
}
func (s *Server) handleGalleryFeed(w http.ResponseWriter, r *http.Request) {
	items := s.store.LoadGallery()
	out := []map[string]any{}
	for _, it := range items {
		if it.Status == "" || it.Status == "visible" {
			out = append(out, s.publicGalleryItem(r, it))
		}
	}
	writeJSON(w, 200, map[string]any{"items": out, "total": len(out)})
}
func (s *Server) handleGalleryPublish(w http.ResponseWriter, r *http.Request) {
	id, ok := s.requireIdentity(w, r)
	if !ok {
		return
	}
	var b map[string]any
	if !readBody(w, r, &b) {
		return
	}
	rel := relClean(strAny(b["image_rel"], strAny(b["path"], strAny(b["url"], ""))))
	rel = relFromURL(rel)
	if rel == "" {
		writeErr(w, 400, "image_rel is required")
		return
	}
	it := GalleryItem{ID: randID(8), ImageRel: rel, PublisherID: id.ID, PublisherName: id.Name, Prompt: strAny(b["prompt"], ""), Model: strAny(b["model"], "gpt-image-2"), Size: strAny(b["size"], ""), CreatedAt: time.Now().Unix(), Status: "visible"}
	items := s.store.LoadGallery()
	items = append([]GalleryItem{it}, items...)
	_ = s.store.SaveGallery(items)
	writeJSON(w, 200, map[string]any{"item": s.publicGalleryItem(r, it)})
}
func (s *Server) handleGalleryPublishedBatch(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireIdentity(w, r); !ok {
		return
	}
	var b struct {
		Paths     []string `json:"paths"`
		ImageRels []string `json:"image_rels"`
	}
	_ = json.NewDecoder(r.Body).Decode(&b)
	set := map[string]bool{}
	for _, it := range s.store.LoadGallery() {
		set[it.ImageRel] = true
	}
	res := map[string]bool{}
	for _, p := range append(b.Paths, b.ImageRels...) {
		rel := relClean(p)
		res[rel] = set[rel]
	}
	writeJSON(w, 200, map[string]any{"items": res})
}
func (s *Server) handleGalleryPublished(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireIdentity(w, r); !ok {
		return
	}
	writeJSON(w, 200, map[string]any{"items": s.store.LoadGallery()})
}
func (s *Server) handleGalleryItem(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/gallery/items/")
	action := ""
	if strings.Contains(id, "/") {
		parts := strings.SplitN(id, "/", 2)
		id = parts[0]
		action = parts[1]
	}
	items := s.store.LoadGallery()
	idx := -1
	for i, it := range items {
		if it.ID == id {
			idx = i
			break
		}
	}
	if idx < 0 {
		writeErr(w, 404, "not found")
		return
	}
	if r.Method == http.MethodGet {
		writeJSON(w, 200, map[string]any{"item": s.publicGalleryItem(r, items[idx])})
		return
	}
	if _, ok := s.requireIdentity(w, r); !ok {
		return
	}
	if r.Method == http.MethodDelete {
		items = append(items[:idx], items[idx+1:]...)
		_ = s.store.SaveGallery(items)
		writeJSON(w, 200, map[string]any{"ok": true})
		return
	}
	if r.Method == http.MethodPost && action == "hide" {
		items[idx].Status = "hidden"
		_ = s.store.SaveGallery(items)
		writeJSON(w, 200, map[string]any{"ok": true})
		return
	}
	if r.Method == http.MethodPost && action == "unhide" {
		items[idx].Status = "visible"
		_ = s.store.SaveGallery(items)
		writeJSON(w, 200, map[string]any{"ok": true})
		return
	}
	writeErr(w, 405, "method not allowed")
}

func (s *Server) handleWeb(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		path := filepath.Join(s.webDist, filepath.Clean(r.URL.Path))
		if st, err := os.Stat(path); err == nil && !st.IsDir() {
			http.ServeFile(w, r, path)
			return
		}
	}
	index := filepath.Join(s.webDist, "index.html")
	if _, err := os.Stat(index); err == nil {
		http.ServeFile(w, r, index)
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "service": "chatgpt2api-go", "hint": "web_dist/index.html not found; build frontend to enable SPA"})
}
