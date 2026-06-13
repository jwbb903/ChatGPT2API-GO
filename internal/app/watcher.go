package app

import (
	"context"
	"log"
	"time"
)

func (s *Server) startLimitedAccountWatcher() {
	interval := time.Duration(s.cfg.RefreshAccountIntervalMinute) * time.Minute
	if interval <= 0 {
		interval = 60 * time.Minute
	}
	go func() {
		for {
			time.Sleep(interval)
			accounts := s.store.LoadAccounts()
			var limited []string
			for _, a := range accounts {
				if a.Status == "限流" && a.AccessToken != "" {
					limited = append(limited, a.AccessToken)
				}
			}
			if len(limited) == 0 {
				continue
			}
			log.Printf("[account-limited-watcher] checking %d limited accounts", len(limited))
			for _, token := range limited {
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				client, err := NewUpstreamClientForAccount(s.accountByToken(token), s.cfg.Proxy, s.ensureCurlImpersonateBinary)
				if err != nil {
					cancel()
					continue
				}
				info, err := client.GetUserInfo(ctx)
				cancel()
				if err != nil {
					continue
				}
				updated := s.store.LoadAccounts()
				for i := range updated {
					if updated[i].AccessToken == token {
						mergeAccount(&updated[i], info)
						updated[i].AccessToken = token
						if info.Status == "正常" && (info.ImageQuotaUnknown || info.Quota > 0) {
							updated[i].Status = "正常"
							updated[i].RestoreAt = nil
							updated[i].RateLimitedAt = nil
							updated[i].RateLimitResetAt = nil
						}
						break
					}
				}
				_ = s.store.SaveAccounts(updated)
			}
		}
	}()
}
