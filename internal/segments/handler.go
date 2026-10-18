package segments

import (
	"encoding/csv"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/opentap/opentap/internal/auth"
	"github.com/opentap/opentap/internal/models"
	"github.com/opentap/opentap/internal/pagination"
	"github.com/opentap/opentap/pkg/response"
	"github.com/rs/zerolog/log"
)

type Handler struct {
	service Service
}

func NewHandler(service Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) CreateSegment(w http.ResponseWriter, r *http.Request) {
	var req models.CreateSegmentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "bad_request", "Invalid JSON body")
		return
	}

	segment, err := h.service.Create(r.Context(), auth.ProjectID(r.Context()), req)
	if err != nil {
		response.Fail(w, err)
		return
	}

	response.JSON(w, http.StatusCreated, segment)
}

func (h *Handler) GetSegment(w http.ResponseWriter, r *http.Request) {
	segmentID, ok := parseID(w, r)
	if !ok {
		return
	}

	segment, err := h.service.GetByID(r.Context(), auth.ProjectID(r.Context()), segmentID)
	if err != nil {
		response.Fail(w, err)
		return
	}
	if segment == nil {
		response.Error(w, http.StatusNotFound, "not_found", "Segment not found")
		return
	}

	response.JSON(w, http.StatusOK, segment)
}

func (h *Handler) ListSegments(w http.ResponseWriter, r *http.Request) {
	segments, nextCursor, err := h.service.List(r.Context(), auth.ProjectID(r.Context()), pagination.Parse(r))
	if err != nil {
		response.Fail(w, err)
		return
	}
	response.Paginated(w, segments, nextCursor)
}

func (h *Handler) UpdateSegment(w http.ResponseWriter, r *http.Request) {
	segmentID, ok := parseID(w, r)
	if !ok {
		return
	}

	var req models.UpdateSegmentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "bad_request", "Invalid JSON body")
		return
	}

	segment, err := h.service.Update(r.Context(), auth.ProjectID(r.Context()), segmentID, req)
	if err != nil {
		response.Fail(w, err)
		return
	}

	response.JSON(w, http.StatusOK, segment)
}

func (h *Handler) DeleteSegment(w http.ResponseWriter, r *http.Request) {
	segmentID, ok := parseID(w, r)
	if !ok {
		return
	}

	if err := h.service.Delete(r.Context(), auth.ProjectID(r.Context()), segmentID); err != nil {
		response.Fail(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) GetSegmentUsers(w http.ResponseWriter, r *http.Request) {
	segmentID, ok := parseID(w, r)
	if !ok {
		return
	}

	users, nextCursor, err := h.service.GetUsers(r.Context(), auth.ProjectID(r.Context()), segmentID, pagination.Parse(r))
	if err != nil {
		response.Fail(w, err)
		return
	}
	response.Paginated(w, users, nextCursor)
}

func (h *Handler) ExportSegmentUsers(w http.ResponseWriter, r *http.Request) {
	segmentID, ok := parseID(w, r)
	if !ok {
		return
	}

	projectID := auth.ProjectID(r.Context())
	segment, err := h.service.GetByID(r.Context(), projectID, segmentID)
	if err != nil {
		response.Fail(w, err)
		return
	}
	if segment == nil {
		response.Error(w, http.StatusNotFound, "not_found", "Segment not found")
		return
	}

	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", `attachment; filename="segment-`+segmentID.String()+`.csv"`)
	out := csv.NewWriter(w)
	out.Write([]string{"user_id", "first_seen", "last_seen", "event_count"})
	err = h.service.ExportUsers(r.Context(), projectID, segmentID, func(u *models.User) error {
		return out.Write([]string{u.UserID, u.FirstSeen.Format(time.RFC3339), u.LastSeen.Format(time.RFC3339), strconv.FormatInt(u.EventCount, 10)})
	})
	out.Flush()
	if err != nil {
		// Headers are already sent, so the client sees a truncated file.
		log.Error().Err(err).Str("segment", segmentID.String()).Msg("segment export failed")
	}
}

func parseID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		response.Error(w, http.StatusBadRequest, "bad_request", "Invalid segment ID")
	}
	return id, err == nil
}
