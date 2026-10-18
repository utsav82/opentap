package users

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/opentap/opentap/internal/auth"
	"github.com/opentap/opentap/internal/events"
	"github.com/opentap/opentap/internal/models"
	"github.com/opentap/opentap/internal/pagination"
	"github.com/opentap/opentap/pkg/response"
)

type Handler struct {
	service      Service
	eventService events.Service
}

func NewHandler(service Service, eventService events.Service) *Handler {
	return &Handler{service: service, eventService: eventService}
}

func (h *Handler) ListUsers(w http.ResponseWriter, r *http.Request) {
	users, nextCursor, err := h.service.ListUsers(r.Context(), auth.ProjectID(r.Context()), pagination.Parse(r))
	if err != nil {
		response.Fail(w, err)
		return
	}
	response.Paginated(w, users, nextCursor)
}

func (h *Handler) GetUser(w http.ResponseWriter, r *http.Request) {
	user, ok := h.user(w, r)
	if ok {
		response.JSON(w, http.StatusOK, user)
	}
}

func (h *Handler) GetUserEvents(w http.ResponseWriter, r *http.Request) {
	user, ok := h.user(w, r)
	if !ok {
		return
	}

	filters := events.ParseFilters(r)
	filters.UserID = user.UserID
	evts, nextCursor, err := h.eventService.ListEvents(r.Context(), user.ProjectID, pagination.Parse(r), filters)
	if err != nil {
		response.Fail(w, err)
		return
	}
	response.Paginated(w, evts, nextCursor)
}

func (h *Handler) Identify(w http.ResponseWriter, r *http.Request) {
	var req models.IdentifyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "bad_request", "Invalid JSON body")
		return
	}

	merged, err := h.service.Identify(r.Context(), auth.ProjectID(r.Context()), req)
	if err != nil {
		response.Fail(w, err)
		return
	}

	response.JSON(w, http.StatusOK, map[string]interface{}{
		"user_id":       req.UserID,
		"merged_events": merged,
	})
}

func (h *Handler) user(w http.ResponseWriter, r *http.Request) (*models.User, bool) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		response.Error(w, http.StatusBadRequest, "bad_request", "Invalid user ID")
		return nil, false
	}

	user, err := h.service.GetUser(r.Context(), auth.ProjectID(r.Context()), id)
	if err != nil {
		response.Fail(w, err)
		return nil, false
	}
	if user == nil {
		response.Error(w, http.StatusNotFound, "not_found", "User not found")
		return nil, false
	}
	return user, true
}
