package auth

import (
	"context"
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/opentap/opentap/internal/models"
	"github.com/opentap/opentap/pkg/response"
)

type apiKeyKey struct{}

func ProjectID(ctx context.Context) uuid.UUID {
	if key, ok := ctx.Value(apiKeyKey{}).(*models.APIKey); ok {
		return key.ProjectID
	}
	return uuid.Nil
}

type Middleware struct {
	repo Repository
}

func NewMiddleware(repo Repository) *Middleware {
	return &Middleware{repo: repo}
}

func (m *Middleware) Authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rawKey := bearer(r)
		if rawKey == "" {
			response.Error(w, http.StatusUnauthorized, "unauthorized", "Missing API key")
			return
		}

		apiKey, err := m.repo.ValidateKey(r.Context(), rawKey)
		if err != nil {
			response.Fail(w, err)
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

		ctx := context.WithValue(r.Context(), apiKeyKey{}, apiKey)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// FullAccess rejects ingest-only keys, which are safe to embed in client apps.
func FullAccess(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if key, _ := r.Context().Value(apiKeyKey{}).(*models.APIKey); key == nil || key.Scope != models.ScopeFull {
			response.Error(w, http.StatusForbidden, "forbidden", "This API key can only send events")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func AdminOnly(token string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if token == "" || subtle.ConstantTimeCompare([]byte(bearer(r)), []byte(token)) != 1 {
				response.Error(w, http.StatusUnauthorized, "unauthorized", "Invalid admin token")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// bearer accepts both "Bearer <key>" and a bare key.
func bearer(r *http.Request) string {
	return strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
}
