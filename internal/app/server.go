package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type Server struct {
	root        string
	dataDir     string
	imagesDir   string
	webDist     string
	cfg         Config
	store       *Store
	mux         *http.ServeMux
	accMu       sync.Mutex
	authMu      sync.Mutex
	taskMu      sync.Mutex
	galleryMu   sync.Mutex
	logMu       sync.Mutex
	callStarts  map[string]time.Time
	taskCancels map[string]context.CancelFunc
	accountPool *accountPool
	logSvc      *logService
}

func NewServer(root string) (*Server, error) {
	root, _ = filepath.Abs(root)
	cfg, err := loadConfig(filepath.Join(root, "config.json"))
	if err != nil {
		return nil, err
	}
	if env := strings.TrimSpace(os.Getenv("CHATGPT2API_AUTH_KEY")); env != "" {
		cfg.AuthKey = env
	}
	if strings.TrimSpace(cfg.AuthKey) == "" {
		return nil, errors.New("auth-key 未设置")
	}
	if cfg.RefreshAccountIntervalMinute <= 0 {
		cfg.RefreshAccountIntervalMinute = 60
	}
	if cfg.ImageRetentionDays <= 0 {
		cfg.ImageRetentionDays = 15
	}
	if cfg.ImagePollTimeoutSecs <= 0 {
		cfg.ImagePollTimeoutSecs = 120
	}
	if cfg.ImageAccountConcurrency <= 0 {
		cfg.ImageAccountConcurrency = 3
	}
	if cfg.AIReview == nil {
		cfg.AIReview = map[string]any{"enabled": false}
	}
	s := &Server{root: root, dataDir: filepath.Join(root, "data"), imagesDir: filepath.Join(root, "data", "images"), webDist: filepath.Join(root, "web_dist"), cfg: cfg, callStarts: map[string]time.Time{}, taskCancels: map[string]context.CancelFunc{}, accountPool: newAccountPool(&cfg)}
	if err := os.MkdirAll(s.imagesDir, 0755); err != nil {
		return nil, err
	}
	s.logSvc = newLogService(s.dataDir)
	s.store = NewStore(s.dataDir)
	s.recoverUnfinishedTasks()
	s.cleanupOldTasks()
	s.mux = http.NewServeMux()
	s.routes()
	s.startLimitedAccountWatcher()
	return s, nil
}

func loadConfig(path string) (Config, error) {
	var raw map[string]any
	if err := ensureNotDir(path); err != nil {
		return Config{}, err
	}
	b, err := os.ReadFile(path)
	if err != nil {
		raw = map[string]any{}
	} else {
		_ = json.Unmarshal(b, &raw)
	}
	var cfg Config
	if b, _ := json.Marshal(raw); len(b) > 0 {
		_ = json.Unmarshal(b, &cfg)
	}
	cfg.Extra = raw
	return cfg, nil
}

func (s *Server) saveConfig() error {
	m := s.configMap(true)
	return writeJSONFile(filepath.Join(s.root, "config.json"), m)
}

func (s *Server) configMap(includeAuth bool) map[string]any {
	m := map[string]any{}
	for k, v := range s.cfg.Extra {
		m[k] = v
	}
	if includeAuth {
		m["auth-key"] = s.cfg.AuthKey
	} else {
		delete(m, "auth-key")
	}
	m["refresh_account_interval_minute"] = s.cfg.RefreshAccountIntervalMinute
	m["image_retention_days"] = s.cfg.ImageRetentionDays
	m["image_poll_timeout_secs"] = s.cfg.ImagePollTimeoutSecs
	m["auto_remove_rate_limited_accounts"] = s.cfg.AutoRemoveRateLimitedAccounts
	m["auto_remove_invalid_accounts"] = s.cfg.AutoRemoveInvalidAccounts
	m["log_levels"] = s.cfg.LogLevels
	m["proxy"] = s.cfg.Proxy
	m["base_url"] = s.cfg.BaseURL
	m["sensitive_words"] = s.cfg.SensitiveWords
	m["global_system_prompt"] = s.cfg.GlobalSystemPrompt
	m["ai_review"] = s.cfg.AIReview
	m["image_account_concurrency"] = s.cfg.ImageAccountConcurrency
	m["cleanup_protect_gallery"] = s.cfg.CleanupProtectGallery
	m["cleanup_protect_user_images"] = s.cfg.CleanupProtectUserImages
	if _, ok := m["backup"]; !ok {
		m["backup"] = disabledBackupSettings()
	}
	return m
}

func (s *Server) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		traceID := firstNonEmpty(r.Header.Get("X-Request-Id"), r.Header.Get("X-Trace-Id"), newTraceID())
		ctx := withTraceID(r.Context(), traceID)
		r = r.WithContext(ctx)
		tw := &traceResponseWriter{ResponseWriter: w}
		start := time.Now()
		traceLogf(ctx, "┌─ client request %s %s remote=%s ua=%q", r.Method, r.URL.RequestURI(), r.RemoteAddr, truncateText(r.UserAgent(), 160))
		defer func() {
			status := tw.status
			if status == 0 {
				status = http.StatusOK
			}
			traceLogf(ctx, "└─ client response status=%d bytes=%d duration=%s", status, tw.bytes, traceHTTPDuration(start))
		}()
		tw.Header().Set("Access-Control-Allow-Origin", "*")
		tw.Header().Set("Access-Control-Allow-Methods", "GET,POST,PUT,PATCH,DELETE,OPTIONS")
		tw.Header().Set("Access-Control-Allow-Headers", "Authorization,Content-Type,x-api-key,anthropic-version")
		if r.Method == http.MethodOptions {
			tw.WriteHeader(204)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/") || strings.HasPrefix(r.URL.Path, "/v1/") {
			tw.Header().Set("Cache-Control", "no-store")
		}
		s.mux.ServeHTTP(tw, r)
	})
}

func (s *Server) routes() {
	s.mux.HandleFunc("/auth/login", s.handleLogin)
	s.mux.HandleFunc("/version", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"version": s.version()})
	})
	s.mux.HandleFunc("/api/auth/me", s.handleAuthMe)
	s.mux.HandleFunc("/api/auth/users", s.handleAuthUsers)
	s.mux.HandleFunc("/api/auth/users/", s.handleAuthUserID)
	s.mux.HandleFunc("/api/accounts", s.handleAccounts)
	s.mux.HandleFunc("/api/accounts/refresh", s.handleAccountsRefresh)
	s.mux.HandleFunc("/api/accounts/update", s.handleAccountsUpdate)
	s.mux.HandleFunc("/api/settings", s.handleSettings)
	s.mux.HandleFunc("/api/storage/info", s.handleStorageInfo)
	s.mux.HandleFunc("/api/system/status", s.handleSystemStatus)
	s.mux.HandleFunc("/api/transport/status", s.handleSystemStatus)
	s.mux.HandleFunc("/api/proxy", s.handleProxy)
	s.mux.HandleFunc("/api/proxy/test", s.handleProxyTest)
	s.mux.HandleFunc("/api/logs", s.handleLogs)
	s.mux.HandleFunc("/api/logs/delete", s.handleLogsDelete)
	s.mux.HandleFunc("/api/images", s.handleImages)
	s.mux.HandleFunc("/api/me/images", s.handleMyImages)
	s.mux.HandleFunc("/api/images/owners", s.handleImageOwners)
	s.mux.HandleFunc("/api/images/delete", s.handleImageDelete)
	s.mux.HandleFunc("/api/images/download/", s.handleImageDownloadSingle)
	s.mux.HandleFunc("/api/images/download", s.handleImageDownload)
	s.mux.HandleFunc("/api/images/tags", s.handleImageTags)
	s.mux.HandleFunc("/api/images/tags/", s.handleImageTagDelete)
	s.mux.HandleFunc("/image-thumbnails/", s.handleThumbnail)
	s.mux.HandleFunc("/api/gallery/feed", s.handleGalleryFeed)
	s.mux.HandleFunc("/api/gallery/publish", s.handleGalleryPublish)
	s.mux.HandleFunc("/api/gallery/published/batch", s.handleGalleryPublishedBatch)
	s.mux.HandleFunc("/api/gallery/items/", s.handleGalleryItem)
	s.mux.HandleFunc("/api/gallery/published", s.handleGalleryPublished)
	s.mux.HandleFunc("/api/image-tasks", s.handleImageTasks)
	s.mux.HandleFunc("/api/image-tasks/generations", s.handleImageTaskGeneration)
	s.mux.HandleFunc("/api/image-tasks/edits", s.handleImageTaskEdit)
	s.mux.HandleFunc("/api/image-tasks/cancel", s.handleImageTaskCancel)
	s.mux.HandleFunc("/api/chat/stream", s.handleChatStream)
	s.mux.HandleFunc("/api/chat/account-types", s.handleChatAccountTypes)
	s.mux.HandleFunc("/api/chat/conversations", s.handleChatConversations)
	s.mux.HandleFunc("/api/chat/conversations/", s.handleChatConversationID)
	s.mux.HandleFunc("/api/cpa/pools", s.handleCPAPools)
	s.mux.HandleFunc("/api/cpa/pools/", s.handleCPAPoolID)
	s.mux.HandleFunc("/api/sub2api/servers", s.handleSub2APIServers)
	s.mux.HandleFunc("/api/sub2api/servers/", s.handleSub2APIServerID)
	s.mux.HandleFunc("/api/register", s.handleRegisterDisabled)
	s.mux.HandleFunc("/api/register/", s.handleRegisterDisabled)
	s.mux.HandleFunc("/api/backup/test", s.handleBackupDisabled)
	s.mux.HandleFunc("/api/backups", s.handleBackupsDisabled)
	s.mux.HandleFunc("/api/backups/", s.handleBackupDisabled)
	s.mux.HandleFunc("/api/video/metadata", s.handleVideoMetadata)
	s.mux.HandleFunc("/api/video/cover", s.handleVideoCover)
	s.mux.HandleFunc("/v1/models", s.handleV1Models)
	s.mux.HandleFunc("/v1/images/generations", s.handleV1ImagesGenerations)
	s.mux.HandleFunc("/v1/images/edits", s.handleV1ImagesEdits)
	s.mux.HandleFunc("/v1/chat/completions", s.handleV1ChatCompletions)
	s.mux.HandleFunc("/v1/responses", s.handleV1Responses)
	s.mux.HandleFunc("/v1/messages", s.handleV1Messages)
	s.mux.Handle("/images/", http.StripPrefix("/images/", http.FileServer(http.Dir(s.imagesDir))))
	s.mux.HandleFunc("/", s.handleWeb)
}

func (s *Server) version() string {
	b, err := os.ReadFile(filepath.Join(s.root, "VERSION"))
	if err == nil && strings.TrimSpace(string(b)) != "" {
		return strings.TrimSpace(string(b))
	}
	return "go-0.1.0"
}

func (s *Server) baseURL(r *http.Request) string {
	if strings.TrimSpace(s.cfg.BaseURL) != "" {
		return strings.TrimRight(s.cfg.BaseURL, "/")
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if xf := r.Header.Get("X-Forwarded-Proto"); xf != "" {
		scheme = strings.Split(xf, ",")[0]
	}
	return fmt.Sprintf("%s://%s", scheme, r.Host)
}
