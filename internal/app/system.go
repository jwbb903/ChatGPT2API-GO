package app

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"
)

func (s *Server) handleSettings(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdmin(w, r); !ok {
		return
	}
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, 200, map[string]any{"config": s.configMap(false)})
	case http.MethodPost:
		var body map[string]any
		if !readBody(w, r, &body) {
			return
		}
		for k, v := range body {
			s.cfg.Extra[k] = v
		}
		if v, ok := body["proxy"]; ok {
			s.cfg.Proxy = strAny(v, "")
		}
		if v, ok := body["base_url"]; ok {
			s.cfg.BaseURL = strings.TrimRight(strAny(v, ""), "/")
		}
		if v, ok := body["global_system_prompt"]; ok {
			s.cfg.GlobalSystemPrompt = strAny(v, "")
		}
		if v, ok := body["refresh_account_interval_minute"]; ok {
			s.cfg.RefreshAccountIntervalMinute = intAny(v, s.cfg.RefreshAccountIntervalMinute)
		}
		if v, ok := body["image_retention_days"]; ok {
			s.cfg.ImageRetentionDays = intAny(v, s.cfg.ImageRetentionDays)
		}
		if v, ok := body["image_poll_timeout_secs"]; ok {
			s.cfg.ImagePollTimeoutSecs = intAny(v, s.cfg.ImagePollTimeoutSecs)
		}
		if v, ok := body["auto_remove_invalid_accounts"]; ok {
			s.cfg.AutoRemoveInvalidAccounts = boolAny(v, false)
		}
		if v, ok := body["auto_remove_rate_limited_accounts"]; ok {
			s.cfg.AutoRemoveRateLimitedAccounts = boolAny(v, false)
		}
		_ = s.saveConfig()
		writeJSON(w, 200, map[string]any{"config": s.configMap(false)})
	default:
		writeErr(w, 405, "method not allowed")
	}
}
func (s *Server) handleStorageInfo(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdmin(w, r); !ok {
		return
	}
	writeJSON(w, 200, map[string]any{"backend": map[string]any{"type": "json", "description": "本地 JSON 文件存储", "file_path": s.store.path("accounts.json"), "auth_keys_file_path": s.store.path("auth_keys.json"), "gallery_file_path": s.store.path("gallery.json")}, "health": map[string]any{"status": "healthy", "backend": "json"}})
}
func (s *Server) handleSystemStatus(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdmin(w, r); !ok {
		return
	}
	transport := strings.ToLower(strings.TrimSpace(os.Getenv("CHATGPT2API_UPSTREAM_TRANSPORT")))
	if transport == "" {
		transport = "tls-client"
	}
	bin := strings.TrimSpace(os.Getenv("CHATGPT2API_CURL_IMPERSONATE_BIN"))
	if bin == "" && (transport == "curl" || transport == "curl-impersonate" || transport == "impersonate") {
		if p, err := s.ensureCurlImpersonateBinary(); err == nil {
			bin = p
		}
	}
	binOK := false
	if bin != "" {
		if st, err := os.Stat(bin); err == nil && !st.IsDir() && st.Mode()&0111 != 0 {
			binOK = true
		}
	}
	writeJSON(w, 200, map[string]any{"ok": true, "version": s.version(), "storage": "json", "transport": transport, "curl_impersonate_bin": bin, "curl_impersonate_executable": binOK, "accounts": len(s.store.LoadAccounts()), "tasks": len(s.store.LoadTasks())})
}

func (s *Server) handleProxy(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdmin(w, r); !ok {
		return
	}
	if r.Method == http.MethodGet {
		writeJSON(w, 200, map[string]any{"proxy": map[string]any{"url": s.cfg.Proxy, "enabled": s.cfg.Proxy != ""}})
		return
	}
	if r.Method == http.MethodPost {
		var b map[string]any
		if !readBody(w, r, &b) {
			return
		}
		s.cfg.Proxy = strAny(b["url"], strAny(b["proxy"], s.cfg.Proxy))
		s.cfg.Extra["proxy"] = s.cfg.Proxy
		_ = s.saveConfig()
		writeJSON(w, 200, map[string]any{"proxy": map[string]any{"url": s.cfg.Proxy, "enabled": s.cfg.Proxy != ""}})
		return
	}
	writeErr(w, 405, "method not allowed")
}
func (s *Server) handleProxyTest(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdmin(w, r); !ok {
		return
	}
	target := "https://chatgpt.com/backend-api/me"
	client := &http.Client{Timeout: 15 * time.Second}
	if strings.TrimSpace(s.cfg.Proxy) != "" {
		proxyURL, err := url.Parse(s.cfg.Proxy)
		if err != nil {
			writeJSON(w, 200, map[string]any{"result": map[string]any{"ok": false, "message": err.Error()}})
			return
		}
		client.Transport = &http.Transport{Proxy: http.ProxyURL(proxyURL)}
	}
	req, _ := http.NewRequestWithContext(r.Context(), http.MethodGet, target, nil)
	resp, err := client.Do(req)
	if err != nil {
		writeJSON(w, 200, map[string]any{"result": map[string]any{"ok": false, "message": err.Error()}})
		return
	}
	defer resp.Body.Close()
	ok := resp.StatusCode > 0 && resp.StatusCode < 500
	writeJSON(w, 200, map[string]any{"result": map[string]any{"ok": ok, "status": resp.StatusCode, "message": resp.Status}})
}

func (s *Server) handleLogs(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdmin(w, r); !ok {
		return
	}
	typ := strings.TrimSpace(r.URL.Query().Get("type"))
	startDate := strings.TrimSpace(r.URL.Query().Get("start_date"))
	endDate := strings.TrimSpace(r.URL.Query().Get("end_date"))
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	endpoint := strings.TrimSpace(r.URL.Query().Get("endpoint"))
	model := strings.TrimSpace(r.URL.Query().Get("model"))
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	items := s.logSvc.listFiltered(typ, startDate, endDate, status, endpoint, model, query, 200)
	writeJSON(w, 200, map[string]any{"items": items})
}
func (s *Server) handleLogsDelete(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdmin(w, r); !ok {
		return
	}
	var b struct {
		IDs []string `json:"ids"`
	}
	if !readBody(w, r, &b) {
		return
	}
	removed := s.logSvc.delete(b.IDs)
	writeJSON(w, 200, map[string]any{"removed": removed})
}

func disabledBackupSettings() map[string]any {
	return map[string]any{"enabled": false, "provider": "disabled", "account_id": "", "access_key_id": "", "secret_access_key": "", "bucket": "", "prefix": "backups", "interval_minutes": 1440, "rotation_keep": 0, "encrypt": false, "passphrase": "", "include": map[string]any{"config": true, "register": false, "cpa": true, "sub2api": true, "logs": true, "image_tasks": true, "accounts_snapshot": true, "auth_keys_snapshot": true, "chat_conversations_snapshot": true, "images": false}}
}
func (s *Server) handleBackupDisabled(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdmin(w, r); !ok {
		return
	}
	writeJSON(w, 200, map[string]any{"result": map[string]any{"ok": false, "status": 0, "message": "R2 备份已在 Go 精简版中禁用"}, "ok": false})
}
func (s *Server) handleBackupsDisabled(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdmin(w, r); !ok {
		return
	}
	writeJSON(w, 200, map[string]any{"items": []any{}, "state": map[string]any{"running": false, "last_status": "disabled"}, "settings": disabledBackupSettings()})
}
func (s *Server) handleRegisterDisabled(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdmin(w, r); !ok {
		return
	}
	writeJSON(w, 200, map[string]any{"register": map[string]any{"enabled": false, "running": false, "logs": []any{}, "message": "注册机已在 Go 版中移除"}})
}
func (s *Server) handleVideoMetadata(w http.ResponseWriter, r *http.Request) {
	meta, err := bilibiliMetadata(r.URL.Query().Get("url"))
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, meta)
}
func (s *Server) handleVideoCover(w http.ResponseWriter, r *http.Request) {
	u := strings.TrimSpace(r.URL.Query().Get("url"))
	parsed, err := url.Parse(u)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		writeErr(w, 400, "url is required")
		return
	}
	host := strings.ToLower(parsed.Hostname())
	if !(strings.HasSuffix(host, "hdslb.com") || strings.HasSuffix(host, "bilibili.com") || strings.HasSuffix(host, "bilivideo.com")) {
		writeErr(w, 400, "unsupported cover host")
		return
	}
	req, _ := http.NewRequestWithContext(r.Context(), http.MethodGet, u, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0 Safari/537.36")
	req.Header.Set("Referer", "https://www.bilibili.com/")
	req.Header.Set("Accept", "image/avif,image/webp,image/apng,image/svg+xml,image/*,*/*;q=0.8")
	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		writeErr(w, 502, err.Error())
		return
	}
	defer resp.Body.Close()
	ct := strings.Split(resp.Header.Get("Content-Type"), ";")[0]
	if resp.StatusCode < 200 || resp.StatusCode >= 300 || !strings.HasPrefix(ct, "image/") {
		writeErr(w, 502, fmt.Sprintf("cover fetch failed: %d", resp.StatusCode))
		return
	}
	w.Header().Set("Content-Type", ct)
	_, _ = io.Copy(w, resp.Body)
}

func bilibiliMetadata(raw string) (map[string]any, error) {
	u := strings.TrimSpace(raw)
	if u == "" {
		return nil, fmt.Errorf("url is required")
	}
	parsed, err := url.Parse(u)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return nil, fmt.Errorf("unsupported url")
	}
	host := strings.ToLower(parsed.Hostname())
	if host == "b23.tv" || host == "bili2233.cn" {
		client := &http.Client{Timeout: 15 * time.Second}
		req, _ := http.NewRequest(http.MethodGet, u, nil)
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0 Safari/537.36")
		resp, err := client.Do(req)
		if err == nil {
			u = resp.Request.URL.String()
			_ = resp.Body.Close()
			parsed, _ = url.Parse(u)
			host = strings.ToLower(parsed.Hostname())
		}
	}
	if !(host == "bilibili.com" || host == "www.bilibili.com" || host == "m.bilibili.com" || strings.HasSuffix(host, ".bilibili.com")) {
		return nil, fmt.Errorf("unsupported video host")
	}
	segs := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	videoID := ""
	for i := 0; i+1 < len(segs); i++ {
		if strings.ToLower(segs[i]) == "video" {
			videoID = segs[i+1]
			break
		}
	}
	if !regexp.MustCompile(`(?i)^(BV[0-9A-Za-z]+|av\d+)$`).MatchString(videoID) {
		return nil, fmt.Errorf("unsupported bilibili video id")
	}
	page := parsed.Query().Get("p")
	if page == "" {
		page = parsed.Query().Get("page")
	}
	if page == "" {
		page = "1"
	}
	apiURL := "https://api.bilibili.com/x/web-interface/view?"
	isAV := strings.HasPrefix(strings.ToLower(videoID), "av")
	if isAV {
		apiURL += "aid=" + url.QueryEscape(strings.TrimPrefix(strings.TrimPrefix(videoID, "av"), "AV"))
	} else {
		apiURL += "bvid=" + url.QueryEscape(videoID)
	}
	req, _ := http.NewRequest(http.MethodGet, apiURL, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0 Safari/537.36")
	req.Header.Set("Referer", "https://www.bilibili.com/")
	req.Header.Set("Accept", "application/json")
	resp, err := (&http.Client{Timeout: 20 * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var payload map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	if intAny(payload["code"], -1) != 0 {
		return nil, fmt.Errorf("%s", firstNonEmpty(strAny(payload["message"], ""), "bilibili metadata failed"))
	}
	data, _ := payload["data"].(map[string]any)
	bvid := strAny(data["bvid"], "")
	aid := strAny(data["aid"], "")
	canonical := firstNonEmpty(bvid, "av"+aid, videoID)
	embedQuery := "bvid=" + url.QueryEscape(bvid)
	if bvid == "" {
		embedQuery = "aid=" + url.QueryEscape(aid)
	}
	rawThumb := strAny(data["pic"], "")
	thumb := ""
	if rawThumb != "" {
		thumb = "/api/video/cover?url=" + url.QueryEscape(rawThumb)
	}
	return map[string]any{"provider": "bilibili", "id": canonical, "title": strAny(data["title"], ""), "thumb_url": thumb, "raw_thumb_url": rawThumb, "watch_url": "https://www.bilibili.com/video/" + canonical, "embed_url": "https://player.bilibili.com/player.html?" + embedQuery + "&p=" + url.QueryEscape(page) + "&autoplay=1"}, nil
}

func (s *Server) handleCPAPools(w http.ResponseWriter, r *http.Request) {
	s.handleNamedList(w, r, "cpa_pools.json", "pools", "pool")
}
func (s *Server) handleSub2APIServers(w http.ResponseWriter, r *http.Request) {
	s.handleNamedList(w, r, "sub2api_servers.json", "servers", "server")
}
func (s *Server) handleNamedList(w http.ResponseWriter, r *http.Request, file, listKey, itemKey string) {
	if _, ok := s.requireAdmin(w, r); !ok {
		return
	}
	items := s.store.LoadList(file)
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, 200, map[string]any{listKey: items})
	case http.MethodPost:
		var b map[string]any
		if !readBody(w, r, &b) {
			return
		}
		b["id"] = randID(6)
		items = append(items, b)
		_ = s.store.SaveList(file, items)
		writeJSON(w, 200, map[string]any{itemKey: b, listKey: items})
	default:
		writeErr(w, 405, "method not allowed")
	}
}
func (s *Server) handleCPAPoolID(w http.ResponseWriter, r *http.Request) {
	s.handleNamedListID(w, r, "/api/cpa/pools/", "cpa_pools.json", "pools", "pool")
}
func (s *Server) handleSub2APIServerID(w http.ResponseWriter, r *http.Request) {
	s.handleNamedListID(w, r, "/api/sub2api/servers/", "sub2api_servers.json", "servers", "server")
}
func (s *Server) handleNamedListID(w http.ResponseWriter, r *http.Request, prefix, file, listKey, itemKey string) {
	if _, ok := s.requireAdmin(w, r); !ok {
		return
	}
	rest := strings.TrimPrefix(r.URL.Path, prefix)
	parts := strings.Split(strings.Trim(rest, "/"), "/")
	id := parts[0]
	if len(parts) > 1 {
		switch parts[1] {
		case "files":
			writeJSON(w, 200, map[string]any{"pool_id": id, "files": []any{}})
			return
		case "import":
			writeJSON(w, 200, map[string]any{"import_job": nil})
			return
		case "groups":
			writeJSON(w, 200, map[string]any{"server_id": id, "groups": []any{}})
			return
		case "accounts":
			writeJSON(w, 200, map[string]any{"server_id": id, "accounts": []any{}})
			return
		}
	}
	items := s.store.LoadList(file)
	idx := -1
	for i, it := range items {
		if strAny(it["id"], "") == id {
			idx = i
			break
		}
	}
	if idx < 0 {
		writeErr(w, 404, "not found")
		return
	}
	if r.Method == http.MethodDelete {
		items = append(items[:idx], items[idx+1:]...)
		_ = s.store.SaveList(file, items)
		writeJSON(w, 200, map[string]any{listKey: items})
		return
	}
	if r.Method == http.MethodPost {
		var b map[string]any
		if !readBody(w, r, &b) {
			return
		}
		for k, v := range b {
			items[idx][k] = v
		}
		_ = s.store.SaveList(file, items)
		writeJSON(w, 200, map[string]any{itemKey: items[idx], listKey: items})
		return
	}
	writeErr(w, 405, "method not allowed")
}
