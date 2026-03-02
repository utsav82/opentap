package users

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/opentap/opentap/internal/events"
	"github.com/opentap/opentap/internal/models"
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
	projectID := r.Context().Value("project_id").(uuid.UUID)
	params := parseCursorParams(r)

	users, nextCursor, err := h.service.ListUsers(r.Context(), projectID, params)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "internal_error", "Failed to list users")
		return
	}

	if users == nil {
		users = []*models.User{}
	}

	response.JSON(w, http.StatusOK, response.PaginatedResponse{
		Data:       users,
		NextCursor: nextCursor,
		HasMore:    nextCursor != "",
	})
}

func (h *Handler) GetUser(w http.ResponseWriter, r *http.Request) {
	projectID := r.Context().Value("project_id").(uuid.UUID)
	userID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		response.Error(w, http.StatusBadRequest, "bad_request", "Invalid user ID")
		return
	}

	user, err := h.service.GetUser(r.Context(), projectID, userID)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "internal_error", "Failed to get user")
		return
	}
	if user == nil {
		response.Error(w, http.StatusNotFound, "not_found", "User not found")
		return
	}

	response.JSON(w, http.StatusOK, user)
}

func (h *Handler) GetUserEvents(w http.ResponseWriter, r *http.Request) {
	projectID := r.Context().Value("project_id").(uuid.UUID)
	userUUID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		response.Error(w, http.StatusBadRequest, "bad_request", "Invalid user ID")
		return
	}

	// Look up user to get the external user_id
	user, err := h.service.GetUser(r.Context(), projectID, userUUID)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "internal_error", "Failed to get user")
		return
	}
	if user == nil {
		response.Error(w, http.StatusNotFound, "not_found", "User not found")
		return
	}

	params := parseCursorParams(r)
	filters := events.EventFilters{
		EventName: r.URL.Query().Get("event_name"),
	}

	evts, nextCursor, err := h.eventService.ListUserEvents(r.Context(), projectID, user.UserID, params, filters)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "internal_error", "Failed to list user events")
		return
	}

	if evts == nil {
		evts = []*models.Event{}
	}

	response.JSON(w, http.StatusOK, response.PaginatedResponse{
		Data:       evts,
		NextCursor: nextCursor,
		HasMore:    nextCursor != "",
	})
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
