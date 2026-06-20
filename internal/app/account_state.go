package app

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

func isRateLimitErrorText(err error) bool {
	if err == nil {
		return false
	}
	text := strings.ToLower(err.Error())
	return strings.Contains(text, "status=429") ||
		strings.Contains(text, "http 429") ||
		strings.Contains(text, "too many requests") ||
		strings.Contains(text, "rate_limit_exceeded") ||
		strings.Contains(text, "usage_limit_reached") ||
		strings.Contains(text, "free plan limit") ||
		strings.Contains(text, "limit for image generation") ||
		strings.Contains(text, "image generations requests") ||
		strings.Contains(text, "when the limit resets")
}

func isInvalidTokenErrorText(err error) bool {
	if err == nil {
		return false
	}
	text := strings.ToLower(err.Error())
	return strings.Contains(text, "status=401") ||
		strings.Contains(text, "http 401") ||
		strings.Contains(text, "token_invalidated") ||
		strings.Contains(text, "token_revoked") ||
		strings.Contains(text, "invalidated oauth token") ||
		strings.Contains(text, "authentication token has been invalidated")
}

func isUpstreamBlockErrorText(err error) bool {
	if err == nil {
		return false
	}
	text := strings.ToLower(err.Error())
	return (strings.Contains(text, "status=403") || strings.Contains(text, "http 403")) &&
		(strings.Contains(text, "<html") || strings.Contains(text, "<body") || strings.Contains(text, "meta http-equiv") || strings.Contains(text, "something seems to have gone wrong"))
}

func isTurnstileRequirementErrorText(err error) bool {
	if err == nil {
		return false
	}
	text := strings.ToLower(err.Error())
	return strings.Contains(text, "turnstile") && strings.Contains(text, "required")
}

func rateLimitRestoreDelay(err error) time.Duration {
	if err == nil {
		return 5 * time.Minute
	}
	text := strings.ToLower(err.Error())
	hours := 0
	minutes := 0
	if m := regexp.MustCompile(`(\d+)\s*hours?`).FindStringSubmatch(text); len(m) > 1 {
		hours, _ = strconv.Atoi(m[1])
	}
	if m := regexp.MustCompile(`(\d+)\s*minutes?`).FindStringSubmatch(text); len(m) > 1 {
		minutes, _ = strconv.Atoi(m[1])
	}
	if hours > 0 || minutes > 0 {
		return time.Duration(hours)*time.Hour + time.Duration(minutes)*time.Minute
	}
	return 5 * time.Minute
}

func (s *Server) markAccountSuccess(token string, image bool) {
	if token == "" {
		return
	}
	now := nowISO()
	_ = s.store.UpdateAccounts(func(accounts []Account) []Account {
		for i := range accounts {
			if accounts[i].AccessToken != token {
				continue
			}
			accounts[i].Success++
			accounts[i].LastUsedAt = &now
			if image && !accounts[i].ImageQuotaUnknown && accounts[i].Quota > 0 {
				accounts[i].Quota--
			}
			if accounts[i].Status == "限流" && (accounts[i].ImageQuotaUnknown || accounts[i].Quota > 0) {
				accounts[i].Status = "正常"
				accounts[i].RestoreAt = nil
				accounts[i].RateLimitedAt = nil
				accounts[i].RateLimitResetAt = nil
			}
			return accounts
		}
		return accounts
	})
}

func (s *Server) markAccountFailure(token string, err error, image bool) {
	if token == "" {
		return
	}
	now := nowISO()
	_ = s.store.UpdateAccounts(func(accounts []Account) []Account {
		for i := range accounts {
			if accounts[i].AccessToken != token {
				continue
			}
			accounts[i].Fail++
			accounts[i].LastUsedAt = &now
			if isRateLimitErrorText(err) {
				accounts[i].Status = "限流"
				reset := time.Now().UTC().Add(rateLimitRestoreDelay(err)).Format(time.RFC3339)
				accounts[i].RestoreAt = &reset
				accounts[i].RateLimitResetAt = &reset
				accounts[i].RateLimitedAt = &now
				if s.cfg.AutoRemoveRateLimitedAccounts {
					accounts = append(accounts[:i], accounts[i+1:]...)
				}
			} else if isInvalidTokenErrorText(err) {
				accounts[i].Status = "异常"
				accounts[i].Quota = 0
				if s.cfg.AutoRemoveInvalidAccounts {
					accounts = append(accounts[:i], accounts[i+1:]...)
				}
			}
			return accounts
		}
		return accounts
	})
}
