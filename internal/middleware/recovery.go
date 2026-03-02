package middleware

import (
	"net/http"

	"github.com/opentap/opentap/pkg/response"
	"github.com/rs/zerolog/log"
)

func Recovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				log.Error().
					Interface("panic", err).
					Str("method", r.Method).
					Str("path", r.URL.Path).
					Msg("panic recovered")

				response.Error(w, http.StatusInternalServerError, "internal_error", "Internal server error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}
