package auth

import (
	"context"
	"net/http"
	"strings"

	"github.com/opentap/opentap/pkg/response"
)

type Middleware struct {
	repo Repository
}

func NewMiddleware(repo Repository) *Middleware {
	return &Middleware{repo: repo}
}

func (m *Middleware) Authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			response.Error(w, http.StatusUnauthorized, "unauthorized", "Missing Authorization header")
			return
		}

		// Support "Bearer <key>" or just "<key>"
		rawKey := authHeader
		if strings.HasPrefix(authHeader, "Bearer ") {
			rawKey = strings.TrimPrefix(authHeader, "Bearer ")
		}

		rawKey = strings.TrimSpace(rawKey)
		if rawKey == "" {
			response.Error(w, http.StatusUnauthorized, "unauthorized", "Missing API key")
			return
		}

		apiKey, err := m.repo.ValidateKey(r.Context(), rawKey)
		if err != nil {
			response.Error(w, http.StatusInternalServerError, "internal_error", "Failed to validate API key")
			return
		}

		if apiKey == nil {
			response.Error(w, http.StatusUnauthorized, "unauthorized", "Invalid API key")
			return
		}

		if apiKey.RevokedAt != nil {
			response.Error(w, http.StatusForbidden, "forbidden", "API key has been revoked")
			return
		}

		// Inject project_id into context
		ctx := context.WithValue(r.Context(), "project_id", apiKey.ProjectID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
