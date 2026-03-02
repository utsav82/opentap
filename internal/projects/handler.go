package projects

import (
	"encoding/json"
	"net/http"

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

func (h *Handler) CreateProject(w http.ResponseWriter, r *http.Request) {
	var req models.CreateProjectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "bad_request", "Invalid JSON body")
		return
	}

	project, err := h.service.Create(r.Context(), req)
	if err != nil {
		if isValidationError(err) {
			response.Error(w, http.StatusBadRequest, "validation_error", err.Error())
			return
		}
		response.Error(w, http.StatusInternalServerError, "internal_error", "Failed to create project")
		return
	}

	response.JSON(w, http.StatusCreated, project)
}

func (h *Handler) ListProjects(w http.ResponseWriter, r *http.Request) {
	projects, err := h.service.List(r.Context())
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "internal_error", "Failed to list projects")
		return
	}

	if projects == nil {
		projects = []*models.Project{}
	}

	response.JSON(w, http.StatusOK, map[string]interface{}{
		"data": projects,
	})
}

func (h *Handler) GetProject(w http.ResponseWriter, r *http.Request) {
	projectID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		response.Error(w, http.StatusBadRequest, "bad_request", "Invalid project ID")
		return
	}

	project, err := h.service.GetByID(r.Context(), projectID)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "internal_error", "Failed to get project")
		return
	}
	if project == nil {
		response.Error(w, http.StatusNotFound, "not_found", "Project not found")
		return
	}

	response.JSON(w, http.StatusOK, project)
}

func isValidationError(err error) bool {
	return err != nil && len(err.Error()) > 11 && err.Error()[:11] == "validation:"
}
