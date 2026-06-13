package app

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

func normalizeKeys(items []UserKey) []UserKey {
	out := make([]UserKey, 0, len(items))
	for _, k := range items {
		if k.ID == "" {
			k.ID = randID(6)
		}
		if k.Role == "" {
			k.Role = "user"
		}
		if k.Name == "" {
			if k.Role == "admin" {
				k.Name = "管理员密钥"
			} else {
				k.Name = "普通用户"
			}
		}
		if k.AccountTier == "" {
			k.AccountTier = "free"
		}
		if k.CreatedAt == "" {
			k.CreatedAt = nowISO()
		}
		if k.ImageDailyResetAt == "" {
			k.ImageDailyResetAt = todayKey()
		}
		if k.ImageMonthlyResetAt == "" {
			k.ImageMonthlyResetAt = monthKey()
		}
		if k.ChatDailyResetAt == "" {
			k.ChatDailyResetAt = todayKey()
		}
		if k.ChatMonthlyResetAt == "" {
			k.ChatMonthlyResetAt = monthKey()
		}
		if k.Role == "admin" {
			k.AccountTier = "premium"
			k.ImageDailyUnlimited = true
			k.ImageMonthlyUnlimited = true
			k.ImageTotalUnlimited = true
			k.ChatDailyUnlimited = true
			k.ChatMonthlyUnlimited = true
			k.ChatTotalUnlimited = true
		}
		out = append(out, k)
	}
	return out
}

func (s *Server) bearer(r *http.Request) string {
	h := r.Header.Get("Authorization")
	parts := strings.SplitN(h, " ", 2)
	if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
		return strings.TrimSpace(parts[1])
	}
	return ""
}

func (s *Server) requireIdentity(w http.ResponseWriter, r *http.Request) (*Identity, bool) {
	token := s.bearer(r)
	if token == "" {
		writeErr(w, 401, "密钥无效或已失效，请重新登录")
		return nil, false
	}
	if token == s.cfg.AuthKey {
		return &Identity{ID: "admin", Name: "管理员", Role: "admin", AccountTier: "premium", CanUsePaidImageAccounts: true, CanUseHighResolution: true}, true
	}
	keys := s.store.LoadAuthKeys()
	h := hashKey(token)
	for i, k := range keys {
		if !k.Enabled {
			continue
		}
		if k.KeyHash != "" && subtle.ConstantTimeCompare([]byte(k.KeyHash), []byte(h)) == 1 {
			now := nowISO()
			keys[i].LastUsedAt = &now
			_ = s.store.SaveAuthKeys(keys)
			prem := k.AccountTier == "premium"
			return &Identity{ID: k.ID, Name: k.Name, Role: k.Role, AccountTier: k.AccountTier, CanUsePaidImageAccounts: prem || k.Role == "admin", CanUseHighResolution: prem || k.Role == "admin"}, true
		}
	}
	writeErr(w, 401, "密钥无效或已失效，请重新登录")
	return nil, false
}

func (s *Server) requireAdmin(w http.ResponseWriter, r *http.Request) (*Identity, bool) {
	id, ok := s.requireIdentity(w, r)
	if !ok {
		return nil, false
	}
	if id.Role != "admin" {
		writeErr(w, 403, "需要管理员权限才能执行这个操作")
		return nil, false
	}
	return id, true
}

func publicKey(k UserKey) map[string]any {
	prem := k.Role == "admin" || k.AccountTier == "premium"
	res := map[string]any{"id": k.ID, "name": k.Name, "role": k.Role, "enabled": k.Enabled, "created_at": k.CreatedAt, "last_used_at": k.LastUsedAt, "account_tier": func() string {
		if k.Role == "admin" {
			return "premium"
		}
		return k.AccountTier
	}(), "can_use_paid_image_accounts": prem, "can_use_high_resolution": prem, "key_visible": k.Role == "user" && k.Key != ""}
	add := func(prefix string, quota, used int, unl bool) {
		res[prefix+"_quota"] = quota
		res[prefix+"_used"] = used
		res[prefix+"_unlimited"] = unl
		if unl {
			res[prefix+"_remaining"] = nil
		} else {
			rem := quota - used
			if rem < 0 {
				rem = 0
			}
			res[prefix+"_remaining"] = rem
		}
	}
	add("image_daily", k.ImageDailyQuota, k.ImageDailyUsed, k.ImageDailyUnlimited)
	add("image_monthly", k.ImageMonthlyQuota, k.ImageMonthlyUsed, k.ImageMonthlyUnlimited)
	add("image_total", k.ImageTotalQuota, k.ImageTotalUsed, k.ImageTotalUnlimited)
	add("chat_daily", k.ChatDailyQuota, k.ChatDailyUsed, k.ChatDailyUnlimited)
	add("chat_monthly", k.ChatMonthlyQuota, k.ChatMonthlyUsed, k.ChatMonthlyUnlimited)
	add("chat_total", k.ChatTotalQuota, k.ChatTotalUsed, k.ChatTotalUnlimited)
	return res
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	id, ok := s.requireIdentity(w, r)
	if !ok {
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "version": s.version(), "role": id.Role, "subject_id": id.ID, "name": id.Name, "account_tier": id.AccountTier, "can_use_paid_image_accounts": id.CanUsePaidImageAccounts, "can_use_high_resolution": id.CanUseHighResolution})
}

func (s *Server) handleAuthMe(w http.ResponseWriter, r *http.Request) {
	id, ok := s.requireIdentity(w, r)
	if !ok {
		return
	}
	if id.Role == "admin" {
		payload := map[string]any{"id": id.ID, "name": id.Name, "role": "admin", "account_tier": "premium", "can_use_paid_image_accounts": true, "can_use_high_resolution": true}
		for _, k := range []string{"image_daily", "image_monthly", "image_total", "chat_daily", "chat_monthly", "chat_total"} {
			payload[k+"_quota"] = 0
			payload[k+"_used"] = 0
			payload[k+"_unlimited"] = true
			payload[k+"_remaining"] = nil
		}
		writeJSON(w, 200, map[string]any{"identity": payload})
		return
	}
	for _, k := range s.store.LoadAuthKeys() {
		if k.ID == id.ID {
			writeJSON(w, 200, map[string]any{"identity": publicKey(k)})
			return
		}
	}
	writeErr(w, 404, "用户不存在")
}

func (s *Server) handleAuthUsers(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdmin(w, r); !ok {
		return
	}
	switch r.Method {
	case http.MethodGet:
		keys := s.store.LoadAuthKeys()
		items := []map[string]any{}
		for _, k := range keys {
			if k.Role == "user" {
				items = append(items, publicKey(k))
			}
		}
		writeJSON(w, 200, map[string]any{"items": items})
	case http.MethodPost:
		var body map[string]any
		if !readBody(w, r, &body) {
			return
		}
		raw := strings.TrimSpace(strAny(body["key"], ""))
		if raw == "" {
			raw = "sk-" + randID(24)
		}
		role := "user"
		tier := strings.ToLower(strings.TrimSpace(strAny(body["account_tier"], "free")))
		if tier != "premium" {
			tier = "free"
		}
		k := UserKey{ID: randID(6), Name: strings.TrimSpace(strAny(body["name"], "普通用户")), Role: role, KeyHash: hashKey(raw), Key: raw, AccountTier: tier, Enabled: true, CreatedAt: nowISO(), ImageDailyQuota: intAny(body["image_daily_quota"], 0), ImageDailyUnlimited: boolAny(body["image_daily_unlimited"], true), ImageMonthlyQuota: intAny(body["image_monthly_quota"], 0), ImageMonthlyUnlimited: boolAny(body["image_monthly_unlimited"], true), ImageTotalQuota: intAny(body["image_total_quota"], 0), ImageTotalUnlimited: boolAny(body["image_total_unlimited"], false), ChatDailyQuota: intAny(body["chat_daily_quota"], 0), ChatDailyUnlimited: boolAny(body["chat_daily_unlimited"], true), ChatMonthlyQuota: intAny(body["chat_monthly_quota"], 0), ChatMonthlyUnlimited: boolAny(body["chat_monthly_unlimited"], true), ChatTotalQuota: intAny(body["chat_total_quota"], 0), ChatTotalUnlimited: boolAny(body["chat_total_unlimited"], true), ImageDailyResetAt: todayKey(), ImageMonthlyResetAt: monthKey(), ChatDailyResetAt: todayKey(), ChatMonthlyResetAt: monthKey()}
		keys := s.store.LoadAuthKeys()
		keys = append(keys, k)
		_ = s.store.SaveAuthKeys(keys)
		items := []map[string]any{}
		for _, it := range keys {
			if it.Role == "user" {
				items = append(items, publicKey(it))
			}
		}
		writeJSON(w, 200, map[string]any{"item": publicKey(k), "key": raw, "items": items})
	default:
		writeErr(w, 405, "method not allowed")
	}
}

func (s *Server) handleAuthUserID(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdmin(w, r); !ok {
		return
	}
	rest := strings.TrimPrefix(r.URL.Path, "/api/auth/users/")
	parts := strings.Split(strings.Trim(rest, "/"), "/")
	if parts[0] == "" {
		writeErr(w, 404, "not found")
		return
	}
	id := parts[0]
	keys := s.store.LoadAuthKeys()
	idx := -1
	for i, k := range keys {
		if k.ID == id && k.Role == "user" {
			idx = i
			break
		}
	}
	if idx < 0 {
		writeErr(w, 404, "这条用户密钥不存在，可能已经被删除")
		return
	}
	if len(parts) > 1 && parts[1] == "key" {
		writeJSON(w, 200, map[string]any{"key": keys[idx].Key, "key_visible": keys[idx].Key != ""})
		return
	}
	if len(parts) > 1 && parts[1] == "regenerate" {
		raw := "sk-" + randID(24)
		if r.Method == http.MethodPost {
			var b map[string]any
			_ = json.NewDecoder(r.Body).Decode(&b)
			if v := strings.TrimSpace(strAny(b["key"], "")); v != "" {
				raw = v
			}
		}
		keys[idx].Key = raw
		keys[idx].KeyHash = hashKey(raw)
		_ = s.store.SaveAuthKeys(keys)
		writeJSON(w, 200, map[string]any{"item": publicKey(keys[idx]), "key": raw, "items": s.publicUserKeys()})
		return
	}
	switch r.Method {
	case http.MethodDelete:
		keys = append(keys[:idx], keys[idx+1:]...)
		_ = s.store.SaveAuthKeys(keys)
		writeJSON(w, 200, map[string]any{"items": s.publicUserKeys()})
	case http.MethodPost:
		var b map[string]any
		if !readBody(w, r, &b) {
			return
		}
		k := keys[idx]
		if v, ok := b["name"]; ok {
			k.Name = strAny(v, k.Name)
		}
		if v, ok := b["enabled"]; ok {
			k.Enabled = boolAny(v, k.Enabled)
		}
		if v := strings.TrimSpace(strAny(b["key"], "")); v != "" {
			k.Key = v
			k.KeyHash = hashKey(v)
		}
		if v, ok := b["account_tier"]; ok {
			t := strings.ToLower(strAny(v, "free"))
			if t != "premium" {
				t = "free"
			}
			k.AccountTier = t
		}
		applyQuotaUpdate(&k, b)
		keys[idx] = k
		_ = s.store.SaveAuthKeys(keys)
		writeJSON(w, 200, map[string]any{"item": publicKey(k), "items": s.publicUserKeys()})
	default:
		writeErr(w, 405, "method not allowed")
	}
}

func (s *Server) publicUserKeys() []map[string]any {
	out := []map[string]any{}
	for _, k := range s.store.LoadAuthKeys() {
		if k.Role == "user" {
			out = append(out, publicKey(k))
		}
	}
	return out
}
func applyQuotaUpdate(k *UserKey, b map[string]any) {
	if v, ok := b["image_daily_quota"]; ok {
		k.ImageDailyQuota = intAny(v, k.ImageDailyQuota)
	}
	if v, ok := b["image_daily_unlimited"]; ok {
		k.ImageDailyUnlimited = boolAny(v, k.ImageDailyUnlimited)
	}
	if v, ok := b["image_monthly_quota"]; ok {
		k.ImageMonthlyQuota = intAny(v, k.ImageMonthlyQuota)
	}
	if v, ok := b["image_monthly_unlimited"]; ok {
		k.ImageMonthlyUnlimited = boolAny(v, k.ImageMonthlyUnlimited)
	}
	if v, ok := b["image_total_quota"]; ok {
		k.ImageTotalQuota = intAny(v, k.ImageTotalQuota)
	}
	if v, ok := b["image_total_unlimited"]; ok {
		k.ImageTotalUnlimited = boolAny(v, k.ImageTotalUnlimited)
	}
	if v, ok := b["chat_daily_quota"]; ok {
		k.ChatDailyQuota = intAny(v, k.ChatDailyQuota)
	}
	if v, ok := b["chat_daily_unlimited"]; ok {
		k.ChatDailyUnlimited = boolAny(v, k.ChatDailyUnlimited)
	}
	if v, ok := b["chat_monthly_quota"]; ok {
		k.ChatMonthlyQuota = intAny(v, k.ChatMonthlyQuota)
	}
	if v, ok := b["chat_monthly_unlimited"]; ok {
		k.ChatMonthlyUnlimited = boolAny(v, k.ChatMonthlyUnlimited)
	}
	if v, ok := b["chat_total_quota"]; ok {
		k.ChatTotalQuota = intAny(v, k.ChatTotalQuota)
	}
	if v, ok := b["chat_total_unlimited"]; ok {
		k.ChatTotalUnlimited = boolAny(v, k.ChatTotalUnlimited)
	}
	if boolAny(b["reset_image_daily_used"], false) {
		k.ImageDailyUsed = 0
		k.ImageDailyResetAt = todayKey()
	}
	if boolAny(b["reset_image_monthly_used"], false) {
		k.ImageMonthlyUsed = 0
		k.ImageMonthlyResetAt = monthKey()
	}
	if boolAny(b["reset_image_total_used"], false) {
		k.ImageTotalUsed = 0
	}
	if boolAny(b["reset_chat_daily_used"], false) {
		k.ChatDailyUsed = 0
		k.ChatDailyResetAt = todayKey()
	}
	if boolAny(b["reset_chat_monthly_used"], false) {
		k.ChatMonthlyUsed = 0
		k.ChatMonthlyResetAt = monthKey()
	}
	if boolAny(b["reset_chat_total_used"], false) {
		k.ChatTotalUsed = 0
	}
}

func (s *Server) consumeImage(id *Identity, n int) bool {
	if id.Role == "admin" {
		return true
	}
	return s.consumeQuota(id.ID, n, true)
}
func (s *Server) consumeChat(id *Identity, n int) bool {
	if id.Role == "admin" {
		return true
	}
	return s.consumeQuota(id.ID, n, false)
}
func (s *Server) consumeQuota(id string, n int, image bool) bool {
	if n < 1 {
		n = 1
	}
	keys := s.store.LoadAuthKeys()
	for i, k := range keys {
		if k.ID != id {
			continue
		}
		resetPeriods(&k)
		if image {
			if !enough(k.ImageDailyQuota, k.ImageDailyUsed, k.ImageDailyUnlimited, n) || !enough(k.ImageMonthlyQuota, k.ImageMonthlyUsed, k.ImageMonthlyUnlimited, n) || !enough(k.ImageTotalQuota, k.ImageTotalUsed, k.ImageTotalUnlimited, n) {
				return false
			}
			k.ImageDailyUsed += n
			k.ImageMonthlyUsed += n
			k.ImageTotalUsed += n
		} else {
			if !enough(k.ChatDailyQuota, k.ChatDailyUsed, k.ChatDailyUnlimited, n) || !enough(k.ChatMonthlyQuota, k.ChatMonthlyUsed, k.ChatMonthlyUnlimited, n) || !enough(k.ChatTotalQuota, k.ChatTotalUsed, k.ChatTotalUnlimited, n) {
				return false
			}
			k.ChatDailyUsed += n
			k.ChatMonthlyUsed += n
			k.ChatTotalUsed += n
		}
		keys[i] = k
		_ = s.store.SaveAuthKeys(keys)
		return true
	}
	return false
}
func (s *Server) refundImage(id *Identity, n int) {
	if id == nil || id.Role == "admin" || n <= 0 {
		return
	}
	s.refundQuota(id.ID, n, true)
}
func (s *Server) refundChat(id *Identity, n int) {
	if id == nil || id.Role == "admin" || n <= 0 {
		return
	}
	s.refundQuota(id.ID, n, false)
}
func (s *Server) refundQuota(id string, n int, image bool) {
	keys := s.store.LoadAuthKeys()
	for i, k := range keys {
		if k.ID != id {
			continue
		}
		if image {
			k.ImageDailyUsed = maxInt(0, k.ImageDailyUsed-n)
			k.ImageMonthlyUsed = maxInt(0, k.ImageMonthlyUsed-n)
			k.ImageTotalUsed = maxInt(0, k.ImageTotalUsed-n)
		} else {
			k.ChatDailyUsed = maxInt(0, k.ChatDailyUsed-n)
			k.ChatMonthlyUsed = maxInt(0, k.ChatMonthlyUsed-n)
			k.ChatTotalUsed = maxInt(0, k.ChatTotalUsed-n)
		}
		keys[i] = k
		_ = s.store.SaveAuthKeys(keys)
		return
	}
}
func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
func enough(q, u int, unl bool, n int) bool { return unl || q-u >= n }
func resetPeriods(k *UserKey) {
	if k.ImageDailyResetAt != todayKey() {
		k.ImageDailyUsed = 0
		k.ImageDailyResetAt = todayKey()
	}
	if k.ChatDailyResetAt != todayKey() {
		k.ChatDailyUsed = 0
		k.ChatDailyResetAt = todayKey()
	}
	if k.ImageMonthlyResetAt != monthKey() {
		k.ImageMonthlyUsed = 0
		k.ImageMonthlyResetAt = monthKey()
	}
	if k.ChatMonthlyResetAt != monthKey() {
		k.ChatMonthlyUsed = 0
		k.ChatMonthlyResetAt = monthKey()
	}
}

var _ = time.Now
