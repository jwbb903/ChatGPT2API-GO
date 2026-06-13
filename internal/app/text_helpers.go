package app

import (
	"context"
	"net/http"
	"time"
)

func (s *Server) collectUpstreamText(r *http.Request, messages []map[string]any, model string) (string, error) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()
	return s.collectTextWithRetry(ctx, messages, model)
}
