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
		response.Fail(w, err)
		return
	}

	response.JSON(w, http.StatusCreated, project)
}

func (h *Handler) ListProjects(w http.ResponseWriter, r *http.Request) {
	projects, err := h.service.List(r.Context())
	if err != nil {
		response.Fail(w, err)
		return
	}

	response.JSON(w, http.StatusOK, map[string]interface{}{
		"data": projects,
	})
}

func (h *Handler) GetProject(w http.ResponseWriter, r *http.Request) {
	projectID, ok := parseUUID(w, r, "id")
	if !ok {
		return
	}

	project, err := h.service.GetByID(r.Context(), projectID)
	if err != nil {
		response.Fail(w, err)
		return
	}
	if project == nil {
		response.Error(w, http.StatusNotFound, "not_found", "Project not found")
		return
	}

	response.JSON(w, http.StatusOK, project)
}

func (h *Handler) CreateKey(w http.ResponseWriter, r *http.Request) {
	projectID, ok := parseUUID(w, r, "id")
	if !ok {
		return
	}

	var req models.CreateAPIKeyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "bad_request", "Invalid JSON body")
		return
	}

	rawKey, apiKey, err := h.service.CreateKey(r.Context(), projectID, req)
	if err != nil {
		response.Fail(w, err)
		return
	}

	response.JSON(w, http.StatusCreated, map[string]interface{}{
		"id":         apiKey.ID,
		"key":        rawKey,
		"key_prefix": apiKey.KeyPrefix,
		"label":      apiKey.Label,
		"scope":      apiKey.Scope,
		"created_at": apiKey.CreatedAt,
	})
}

func (h *Handler) ListKeys(w http.ResponseWriter, r *http.Request) {
	projectID, ok := parseUUID(w, r, "id")
	if !ok {
		return
	}

	keys, err := h.service.ListKeys(r.Context(), projectID)
	if err != nil {
		response.Fail(w, err)
		return
	}

	response.JSON(w, http.StatusOK, map[string]interface{}{
		"data": keys,
	})
}

func (h *Handler) RevokeKey(w http.ResponseWriter, r *http.Request) {
	projectID, ok := parseUUID(w, r, "id")
	if !ok {
		return
	}
	keyID, ok := parseUUID(w, r, "key_id")
	if !ok {
		return
	}

	if err := h.service.RevokeKey(r.Context(), projectID, keyID); err != nil {
		response.Fail(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func parseUUID(w http.ResponseWriter, r *http.Request, param string) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, param))
	if err != nil {
		response.Error(w, http.StatusBadRequest, "bad_request", "Invalid "+param)
	}
	return id, err == nil
}
