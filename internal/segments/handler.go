package segments

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

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

func (h *Handler) CreateSegment(w http.ResponseWriter, r *http.Request) {
	projectID := r.Context().Value("project_id").(uuid.UUID)

	var req models.CreateSegmentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "bad_request", "Invalid JSON body")
		return
	}

	segment, err := h.service.Create(r.Context(), projectID, req)
	if err != nil {
		if isValidationError(err) {
			response.Error(w, http.StatusBadRequest, "validation_error", err.Error())
			return
		}
		response.Error(w, http.StatusInternalServerError, "internal_error", "Failed to create segment")
		return
	}

	response.JSON(w, http.StatusCreated, segment)
}

func (h *Handler) GetSegment(w http.ResponseWriter, r *http.Request) {
	projectID := r.Context().Value("project_id").(uuid.UUID)
	segmentID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		response.Error(w, http.StatusBadRequest, "bad_request", "Invalid segment ID")
		return
	}

	segment, err := h.service.GetByID(r.Context(), projectID, segmentID)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "internal_error", "Failed to get segment")
		return
	}
	if segment == nil {
		response.Error(w, http.StatusNotFound, "not_found", "Segment not found")
		return
	}

	response.JSON(w, http.StatusOK, segment)
}

func (h *Handler) ListSegments(w http.ResponseWriter, r *http.Request) {
	projectID := r.Context().Value("project_id").(uuid.UUID)
	params := parseCursorParams(r)

	segments, nextCursor, err := h.service.List(r.Context(), projectID, params)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "internal_error", "Failed to list segments")
		return
	}

	if segments == nil {
		segments = []*models.Segment{}
	}

	response.JSON(w, http.StatusOK, response.PaginatedResponse{
		Data:       segments,
		NextCursor: nextCursor,
		HasMore:    nextCursor != "",
	})
}

func (h *Handler) UpdateSegment(w http.ResponseWriter, r *http.Request) {
	projectID := r.Context().Value("project_id").(uuid.UUID)
	segmentID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		response.Error(w, http.StatusBadRequest, "bad_request", "Invalid segment ID")
		return
	}

	var req models.UpdateSegmentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "bad_request", "Invalid JSON body")
		return
	}

	segment, err := h.service.Update(r.Context(), projectID, segmentID, req)
	if err != nil {
		if err.Error() == "not_found" {
			response.Error(w, http.StatusNotFound, "not_found", "Segment not found")
			return
		}
		if isValidationError(err) {
			response.Error(w, http.StatusBadRequest, "validation_error", err.Error())
			return
		}
		response.Error(w, http.StatusInternalServerError, "internal_error", "Failed to update segment")
		return
	}

	response.JSON(w, http.StatusOK, segment)
}

func (h *Handler) DeleteSegment(w http.ResponseWriter, r *http.Request) {
	projectID := r.Context().Value("project_id").(uuid.UUID)
	segmentID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		response.Error(w, http.StatusBadRequest, "bad_request", "Invalid segment ID")
		return
	}

	if err := h.service.Delete(r.Context(), projectID, segmentID); err != nil {
		if strings.Contains(err.Error(), "not found") {
			response.Error(w, http.StatusNotFound, "not_found", "Segment not found")
			return
		}
		response.Error(w, http.StatusInternalServerError, "internal_error", "Failed to delete segment")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) GetSegmentUsers(w http.ResponseWriter, r *http.Request) {
	segmentID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		response.Error(w, http.StatusBadRequest, "bad_request", "Invalid segment ID")
		return
	}

	params := parseCursorParams(r)
	users, nextCursor, err := h.service.GetUsers(r.Context(), segmentID, params)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "internal_error", "Failed to get segment users")
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

func isValidationError(err error) bool {
	return err != nil && len(err.Error()) > 11 && err.Error()[:11] == "validation:"
}
