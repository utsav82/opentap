package events

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/opentap/opentap/internal/auth"
	"github.com/opentap/opentap/internal/models"
	"github.com/opentap/opentap/internal/pagination"
	"github.com/opentap/opentap/pkg/response"
)

type Handler struct {
	service Service
}

func NewHandler(service Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) TrackEvent(w http.ResponseWriter, r *http.Request) {
	var req models.TrackEventRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "bad_request", "Invalid JSON body")
		return
	}

	id, err := h.service.TrackEvent(r.Context(), auth.ProjectID(r.Context()), req)
	if err != nil {
		response.Fail(w, err)
		return
	}

	response.JSON(w, http.StatusAccepted, map[string]interface{}{
		"id":     id,
		"status": "accepted",
	})
}

func (h *Handler) TrackBatch(w http.ResponseWriter, r *http.Request) {
	var req models.TrackBatchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "bad_request", "Invalid JSON body")
		return
	}

	ids, err := h.service.TrackBatch(r.Context(), auth.ProjectID(r.Context()), req)
	if err != nil {
		response.Fail(w, err)
		return
	}

	response.JSON(w, http.StatusAccepted, map[string]interface{}{
		"accepted": len(ids),
		"ids":      ids,
	})
}

func (h *Handler) ListEvents(w http.ResponseWriter, r *http.Request) {
	events, nextCursor, err := h.service.ListEvents(r.Context(), auth.ProjectID(r.Context()), pagination.Parse(r), ParseFilters(r))
	if err != nil {
		response.Fail(w, err)
		return
	}
	response.Paginated(w, events, nextCursor)
}

func (h *Handler) GetEvent(w http.ResponseWriter, r *http.Request) {
	eventID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		response.Error(w, http.StatusBadRequest, "bad_request", "Invalid event ID")
		return
	}

	event, err := h.service.GetEvent(r.Context(), auth.ProjectID(r.Context()), eventID)
	if err != nil {
		response.Fail(w, err)
		return
	}
	if event == nil {
		response.Error(w, http.StatusNotFound, "not_found", "Event not found")
		return
	}

	response.JSON(w, http.StatusOK, event)
}

func ParseFilters(r *http.Request) EventFilters {
	filters := EventFilters{
		EventName: r.URL.Query().Get("event_name"),
		UserID:    r.URL.Query().Get("user_id"),
	}
	if t, err := time.Parse(time.RFC3339, r.URL.Query().Get("start")); err == nil {
		filters.Start = &t
	}
	if t, err := time.Parse(time.RFC3339, r.URL.Query().Get("end")); err == nil {
		filters.End = &t
	}
	return filters
}
