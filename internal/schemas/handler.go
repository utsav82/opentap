package schemas

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/opentap/opentap/internal/auth"
	"github.com/opentap/opentap/pkg/response"
)

type Handler struct {
	service Service
}

func NewHandler(service Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) ListSchemas(w http.ResponseWriter, r *http.Request) {
	summaries, err := h.service.ListEventTypes(r.Context(), auth.ProjectID(r.Context()))
	if err != nil {
		response.Fail(w, err)
		return
	}

	response.JSON(w, http.StatusOK, map[string]interface{}{
		"data": summaries,
	})
}

func (h *Handler) GetSchema(w http.ResponseWriter, r *http.Request) {
	detail, err := h.service.GetEventSchema(r.Context(), auth.ProjectID(r.Context()), chi.URLParam(r, "event_name"))
	if err != nil {
		response.Fail(w, err)
		return
	}
	if detail == nil {
		response.Error(w, http.StatusNotFound, "not_found", "Event schema not found")
		return
	}

	response.JSON(w, http.StatusOK, detail)
}
