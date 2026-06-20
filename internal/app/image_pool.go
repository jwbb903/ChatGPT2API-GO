package app

import (
	"context"
	"errors"
)

func (s *Server) generateImageWithPool(ctx context.Context, prompt, model, size, resolution string, refs [][]byte) ([]upstreamImageResult, error) {
	accounts := s.store.LoadAccounts()
	maxAttempts := len(accounts)
	if maxAttempts < 1 {
		maxAttempts = 1
	}
	excluded := map[string]bool{}
	var lastErr error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		traceLogf(ctx, "├─ image account attempt %d/%d model=%s resolution=%s excluded=%d", attempt+1, maxAttempts, model, resolution, len(excluded))
		account, err := s.pickAccountExcluding(model, resolution, excluded)
		if err != nil {
			if lastErr != nil {
				return nil, lastErr
			}
			return nil, err
		}
		client, actualAccount, err := s.upstreamClientForImageAccount(model, resolution, account)
		if err != nil {
			s.accountPool.releaseToken(account.AccessToken)
			if lastErr != nil {
				return nil, lastErr
			}
			return nil, err
		}
		poolToken := account.AccessToken
		token := actualAccount.AccessToken
		traceLogf(ctx, "│  ├─ selected image account %s", accountLabel(actualAccount))
		excluded[poolToken] = true
		excluded[token] = true
		items, err := client.GenerateImage(ctx, prompt, model, size, resolution, refs)
		s.accountPool.releaseToken(poolToken)
		if err == nil {
			traceLogf(ctx, "└─ image account attempt %d success images=%d", attempt+1, len(items))
			s.markAccountSuccess(token, true)
			return items, nil
		}
		traceLogf(ctx, "│  └─ image account attempt %d failed error=%v", attempt+1, err)
		s.markAccountFailure(token, err, true)
		lastErr = err
		if !shouldRetryImageAccount(err) {
			return nil, err
		}
		traceLogf(ctx, "│  ├─ retry with another image account")
	}
	if lastErr == nil {
		lastErr = errors.New("no available image quota")
	}
	return nil, lastErr
}

func shouldRetryImageAccount(err error) bool {
	if err == nil {
		return false
	}
	return isRateLimitErrorText(err) || isInvalidTokenErrorText(err) || isUpstreamBlockErrorText(err) || isTurnstileRequirementErrorText(err)
}
