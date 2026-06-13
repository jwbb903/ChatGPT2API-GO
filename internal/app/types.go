package app

import "time"

type Config struct {
	AuthKey                       string         `json:"auth-key"`
	RefreshAccountIntervalMinute  int            `json:"refresh_account_interval_minute"`
	ImageRetentionDays            int            `json:"image_retention_days"`
	ImagePollTimeoutSecs          int            `json:"image_poll_timeout_secs"`
	AutoRemoveRateLimitedAccounts bool           `json:"auto_remove_rate_limited_accounts"`
	AutoRemoveInvalidAccounts     bool           `json:"auto_remove_invalid_accounts"`
	LogLevels                     []string       `json:"log_levels"`
	Proxy                         string         `json:"proxy"`
	BaseURL                       string         `json:"base_url"`
	SensitiveWords                []string       `json:"sensitive_words"`
	GlobalSystemPrompt            string         `json:"global_system_prompt"`
	AIReview                      map[string]any `json:"ai_review"`
	ImageAccountConcurrency       int            `json:"image_account_concurrency"`
	CleanupProtectGallery         bool           `json:"cleanup_protect_gallery"`
	CleanupProtectUserImages      bool           `json:"cleanup_protect_user_images"`
	Extra                         map[string]any `json:"-"`
}

type Account struct {
	AccessToken       string            `json:"access_token"`
	Type              string            `json:"type"`
	SourceType        string            `json:"source_type,omitempty"`
	ExportType        string            `json:"export_type,omitempty"`
	Status            string            `json:"status"`
	Quota             int               `json:"quota"`
	InitialQuota      int               `json:"initial_quota,omitempty"`
	ImageQuotaUnknown bool              `json:"image_quota_unknown,omitempty"`
	Email             *string           `json:"email,omitempty"`
	UserID            *string           `json:"user_id,omitempty"`
	LimitsProgress    []map[string]any  `json:"limits_progress,omitempty"`
	DefaultModelSlug  *string           `json:"default_model_slug,omitempty"`
	RestoreAt         *string           `json:"restore_at,omitempty"`
	RateLimitedAt     *string           `json:"rate_limited_at,omitempty"`
	RateLimitResetAt  *string           `json:"rate_limit_reset_at,omitempty"`
	Success           int               `json:"success"`
	Fail              int               `json:"fail"`
	LastUsedAt        *string           `json:"last_used_at,omitempty"`
	Mailbox           map[string]any    `json:"mailbox,omitempty"`
	Password          *string           `json:"password,omitempty"`
	RefreshToken      *string           `json:"refresh_token,omitempty"`
	IDToken           *string           `json:"id_token,omitempty"`
	AccountID         *string           `json:"account_id,omitempty"`
	ExpiresAt         any               `json:"expires_at,omitempty"`
	ClientID          *string           `json:"client_id,omitempty"`
	CreatedAt         *string           `json:"created_at,omitempty"`
	FP                map[string]string `json:"fp,omitempty"`
}

type UserKey struct {
	ID                    string  `json:"id"`
	Name                  string  `json:"name"`
	Role                  string  `json:"role"`
	KeyHash               string  `json:"key_hash,omitempty"`
	Key                   string  `json:"key,omitempty"`
	AccountTier           string  `json:"account_tier"`
	Enabled               bool    `json:"enabled"`
	CreatedAt             string  `json:"created_at,omitempty"`
	LastUsedAt            *string `json:"last_used_at"`
	ImageDailyQuota       int     `json:"image_daily_quota"`
	ImageDailyUsed        int     `json:"image_daily_used"`
	ImageDailyUnlimited   bool    `json:"image_daily_unlimited"`
	ImageDailyResetAt     string  `json:"image_daily_reset_at,omitempty"`
	ImageMonthlyQuota     int     `json:"image_monthly_quota"`
	ImageMonthlyUsed      int     `json:"image_monthly_used"`
	ImageMonthlyUnlimited bool    `json:"image_monthly_unlimited"`
	ImageMonthlyResetAt   string  `json:"image_monthly_reset_at,omitempty"`
	ImageTotalQuota       int     `json:"image_total_quota"`
	ImageTotalUsed        int     `json:"image_total_used"`
	ImageTotalUnlimited   bool    `json:"image_total_unlimited"`
	ChatDailyQuota        int     `json:"chat_daily_quota"`
	ChatDailyUsed         int     `json:"chat_daily_used"`
	ChatDailyUnlimited    bool    `json:"chat_daily_unlimited"`
	ChatDailyResetAt      string  `json:"chat_daily_reset_at,omitempty"`
	ChatMonthlyQuota      int     `json:"chat_monthly_quota"`
	ChatMonthlyUsed       int     `json:"chat_monthly_used"`
	ChatMonthlyUnlimited  bool    `json:"chat_monthly_unlimited"`
	ChatMonthlyResetAt    string  `json:"chat_monthly_reset_at,omitempty"`
	ChatTotalQuota        int     `json:"chat_total_quota"`
	ChatTotalUsed         int     `json:"chat_total_used"`
	ChatTotalUnlimited    bool    `json:"chat_total_unlimited"`
}

type Identity struct {
	ID                      string `json:"id"`
	Name                    string `json:"name"`
	Role                    string `json:"role"`
	AccountTier             string `json:"account_tier"`
	CanUsePaidImageAccounts bool   `json:"can_use_paid_image_accounts"`
	CanUseHighResolution    bool   `json:"can_use_high_resolution"`
}

type LogItem struct {
	ID      string         `json:"id"`
	Time    string         `json:"time"`
	Type    string         `json:"type"`
	Summary string         `json:"summary,omitempty"`
	Detail  map[string]any `json:"detail,omitempty"`
}

type ImageTask struct {
	ID         string           `json:"id"`
	OwnerID    string           `json:"owner_id,omitempty"`
	Status     string           `json:"status"`
	Mode       string           `json:"mode"`
	Model      string           `json:"model,omitempty"`
	Size       string           `json:"size,omitempty"`
	Resolution string           `json:"resolution,omitempty"`
	CreatedAt  string           `json:"created_at"`
	UpdatedAt  string           `json:"updated_at"`
	Data       []map[string]any `json:"data,omitempty"`
	Error      string           `json:"error,omitempty"`
}

type GalleryItem struct {
	ID            string `json:"id"`
	ImageRel      string `json:"image_rel"`
	PublisherID   string `json:"publisher_id"`
	PublisherName string `json:"publisher_name"`
	Prompt        string `json:"prompt"`
	Model         string `json:"model"`
	Size          string `json:"size"`
	Width         int    `json:"width"`
	Height        int    `json:"height"`
	IsEdit        bool   `json:"is_edit"`
	CreatedAt     int64  `json:"created_at"`
	Status        string `json:"status"`
}

func nowISO() string { return time.Now().UTC().Format(time.RFC3339Nano) }
