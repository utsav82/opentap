package schemas

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/opentap/opentap/pkg/response"
)

type Handler struct {
	service Service
}

func NewHandler(service Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) ListSchemas(w http.ResponseWriter, r *http.Request) {
	projectID := r.Context().Value("project_id").(uuid.UUID)

	summaries, err := h.service.ListEventTypes(r.Context(), projectID)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "internal_error", "Failed to list schemas")
		return
	}

	if summaries == nil {
		summaries = []EventSummary{}
	}

	response.JSON(w, http.StatusOK, map[string]interface{}{
		"data": summaries,
	})
}

func (h *Handler) GetSchema(w http.ResponseWriter, r *http.Request) {
	projectID := r.Context().Value("project_id").(uuid.UUID)
	eventName := chi.URLParam(r, "event_name")

	detail, err := h.service.GetEventSchema(r.Context(), projectID, eventName)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "internal_error", "Failed to get schema")
		return
	}
	if detail == nil {
		response.Error(w, http.StatusNotFound, "not_found", "Event schema not found")
		return
	}

	response.JSON(w, http.StatusOK, detail)
}
