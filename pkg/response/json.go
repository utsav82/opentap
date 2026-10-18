package response

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/opentap/opentap/internal/models"
	"github.com/rs/zerolog/log"
)

type ErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type ErrorResponse struct {
	Error ErrorBody `json:"error"`
}

type PaginatedResponse struct {
	Data       interface{} `json:"data"`
	NextCursor string      `json:"next_cursor,omitempty"`
	HasMore    bool        `json:"has_more"`
}

func JSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func Paginated(w http.ResponseWriter, data interface{}, nextCursor string) {
	JSON(w, http.StatusOK, PaginatedResponse{Data: data, NextCursor: nextCursor, HasMore: nextCursor != ""})
}

func Error(w http.ResponseWriter, status int, code, message string) {
	JSON(w, status, ErrorResponse{
		Error: ErrorBody{Code: code, Message: message},
	})
}

// Fail maps service errors to HTTP responses, hiding internal details.
func Fail(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, models.ErrValidation):
		Error(w, http.StatusBadRequest, "validation_error", err.Error())
	case errors.Is(err, models.ErrNotFound):
		Error(w, http.StatusNotFound, "not_found", err.Error())
	default:
		log.Error().Err(err).Msg("request failed")
		Error(w, http.StatusInternalServerError, "internal_error", "Internal server error")
	}
}
