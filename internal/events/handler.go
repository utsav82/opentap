package events

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/opentap/opentap/internal/models"
	"github.com/opentap/opentap/pkg/response"
)

type Handler struct {
	service Service
}

func NewHandler(service Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) TrackEvent(w http.ResponseWriter, r *http.Request) {
	projectID := r.Context().Value("project_id").(uuid.UUID)

	var req models.TrackEventRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "bad_request", "Invalid JSON body")
		return
	}

	id, err := h.service.TrackEvent(r.Context(), projectID, req)
	if err != nil {
		if isValidationError(err) {
			response.Error(w, http.StatusBadRequest, "validation_error", err.Error())
			return
		}
		response.Error(w, http.StatusInternalServerError, "internal_error", "Failed to process event")
		return
	}

	response.JSON(w, http.StatusAccepted, map[string]interface{}{
		"id":     id,
		"status": "accepted",
	})
}

func (h *Handler) TrackBatch(w http.ResponseWriter, r *http.Request) {
	projectID := r.Context().Value("project_id").(uuid.UUID)

	var req models.TrackBatchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "bad_request", "Invalid JSON body")
		return
	}

	ids, err := h.service.TrackBatch(r.Context(), projectID, req)
	if err != nil {
		if isValidationError(err) {
			response.Error(w, http.StatusBadRequest, "validation_error", err.Error())
			return
		}
		response.Error(w, http.StatusInternalServerError, "internal_error", "Failed to process events")
		return
	}

	response.JSON(w, http.StatusAccepted, map[string]interface{}{
		"accepted": len(ids),
		"ids":      ids,
	})
}

func (h *Handler) ListEvents(w http.ResponseWriter, r *http.Request) {
	projectID := r.Context().Value("project_id").(uuid.UUID)
	params := parseCursorParams(r)
	filters := parseEventFilters(r)

	events, nextCursor, err := h.service.ListEvents(r.Context(), projectID, params, filters)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "internal_error", "Failed to list events")
		return
	}

	if events == nil {
		events = []*models.Event{}
	}

	response.JSON(w, http.StatusOK, response.PaginatedResponse{
		Data:       events,
		NextCursor: nextCursor,
		HasMore:    nextCursor != "",
	})
}

func (h *Handler) GetEvent(w http.ResponseWriter, r *http.Request) {
	projectID := r.Context().Value("project_id").(uuid.UUID)
	eventID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		response.Error(w, http.StatusBadRequest, "bad_request", "Invalid event ID")
		return
	}

	event, err := h.service.GetEvent(r.Context(), projectID, eventID)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "internal_error", "Failed to get event")
		return
	}
	if event == nil {
		response.Error(w, http.StatusNotFound, "not_found", "Event not found")
		return
	}

	response.JSON(w, http.StatusOK, event)
}

func parseCursorParams(r *http.Request) models.CursorParams {
	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 100 {
			limit = parsed
		}
	}
	return models.CursorParams{
		Cursor: r.URL.Query().Get("cursor"),
		Limit:  limit,
	}
}

func parseEventFilters(r *http.Request) EventFilters {
	filters := EventFilters{
		EventName: r.URL.Query().Get("event_name"),
		UserID:    r.URL.Query().Get("user_id"),
	}
	if start := r.URL.Query().Get("start"); start != "" {
		if t, err := time.Parse(time.RFC3339, start); err == nil {
			filters.Start = &t
		}
	}
	if end := r.URL.Query().Get("end"); end != "" {
		if t, err := time.Parse(time.RFC3339, end); err == nil {
			filters.End = &t
		}
	}
	return filters
}

func isValidationError(err error) bool {
	return err != nil && len(err.Error()) > 11 && err.Error()[:11] == "validation:"
}
