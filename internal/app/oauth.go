package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	http "github.com/bogdanfinn/fhttp"
)

const codexOAuthClientID = "app_EMoamEEZ73f0CkXaXp7hrann"
const codexOAuthTokenURL = "https://auth.openai.com/oauth/token"

func (s *Server) refreshOAuthAccessToken(ctx context.Context, oldToken string) (string, error) {
	accounts := s.store.LoadAccounts()
	idx := -1
	var account Account
	for i, a := range accounts {
		if a.AccessToken == oldToken {
			idx = i
			account = a
			break
		}
	}
	if idx < 0 || account.RefreshToken == nil || strings.TrimSpace(*account.RefreshToken) == "" {
		return "", fmt.Errorf("refresh_token not found")
	}
	client, err := NewUpstreamClient("", s.cfg.Proxy, s.ensureCurlImpersonateBinary)
	if err != nil {
		return "", err
	}
	clientID := codexOAuthClientID
	if account.ClientID != nil && strings.TrimSpace(*account.ClientID) != "" {
		clientID = strings.TrimSpace(*account.ClientID)
	}
	form := url.Values{}
	form.Set("client_id", clientID)
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", strings.TrimSpace(*account.RefreshToken))
	form.Set("scope", "openid profile email")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, codexOAuthTokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := client.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("oauth refresh failed: status=%d", resp.StatusCode)
	}
	var payload map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", err
	}
	newToken := strings.TrimSpace(strAny(payload["access_token"], ""))
	if newToken == "" {
		return "", fmt.Errorf("oauth refresh response missing access_token")
	}
	refreshToken := strings.TrimSpace(strAny(payload["refresh_token"], ""))
	if refreshToken == "" && account.RefreshToken != nil {
		refreshToken = *account.RefreshToken
	}
	idToken := strings.TrimSpace(strAny(payload["id_token"], ""))
	expiresIn := intAny(payload["expires_in"], 0)
	account.AccessToken = newToken
	account.RefreshToken = &refreshToken
	if idToken != "" {
		account.IDToken = &idToken
	}
	account.SourceType = "codex"
	account.ClientID = &clientID
	if expiresIn > 0 {
		account.ExpiresAt = time.Now().Unix() + int64(expiresIn)
	}
	if account.AccountID == nil || *account.AccountID == "" {
		if accountID := chatGPTAccountID(newToken); accountID != "" {
			account.AccountID = &accountID
		}
	}
	accounts[idx] = account
	_ = s.store.SaveAccounts(accounts)
	return newToken, nil
}

func (s *Server) upstreamClientForTokenWithRefresh(ctx context.Context, token string) (*UpstreamClient, error) {
	client, err := NewUpstreamClientForAccount(s.accountByToken(token), s.cfg.Proxy, s.ensureCurlImpersonateBinary)
	if err == nil {
		return client, nil
	}
	newToken, refreshErr := s.refreshOAuthAccessToken(ctx, token)
	if refreshErr != nil {
		return nil, err
	}
	return NewUpstreamClientForAccount(s.accountByToken(newToken), s.cfg.Proxy, s.ensureCurlImpersonateBinary)
}
