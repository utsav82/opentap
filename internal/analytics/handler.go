package analytics

import (
	"net/http"
	"strconv"

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

func (h *Handler) EventCountOverTime(w http.ResponseWriter, r *http.Request) {
	projectID := r.Context().Value("project_id").(uuid.UUID)

	query := models.AnalyticsQuery{
		EventName:  r.URL.Query().Get("event_name"),
		TimeWindow: r.URL.Query().Get("time_window"),
		Interval:   r.URL.Query().Get("interval"),
	}

	if query.TimeWindow == "" {
		query.TimeWindow = "7d"
	}
	if query.Interval == "" {
		query.Interval = "day"
	}

	points, err := h.service.EventCountOverTime(r.Context(), projectID, query)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "internal_error", "Failed to query analytics")
		return
	}

	if points == nil {
		points = []TimeSeriesPoint{}
	}

	response.JSON(w, http.StatusOK, map[string]interface{}{
		"data": points,
	})
}

func (h *Handler) TopEvents(w http.ResponseWriter, r *http.Request) {
	projectID := r.Context().Value("project_id").(uuid.UUID)

	timeWindow := r.URL.Query().Get("time_window")
	if timeWindow == "" {
		timeWindow = "7d"
	}

	limit := 10
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 50 {
			limit = parsed
		}
	}

	counts, err := h.service.TopEvents(r.Context(), projectID, timeWindow, limit)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "internal_error", "Failed to query analytics")
		return
	}

	if counts == nil {
		counts = []EventCount{}
	}

	response.JSON(w, http.StatusOK, map[string]interface{}{
		"data": counts,
	})
}

func (h *Handler) DAU(w http.ResponseWriter, r *http.Request) {
	projectID := r.Context().Value("project_id").(uuid.UUID)

	query := models.AnalyticsQuery{
		TimeWindow: r.URL.Query().Get("time_window"),
		Interval:   r.URL.Query().Get("interval"),
	}

	if query.TimeWindow == "" {
		query.TimeWindow = "7d"
	}
	if query.Interval == "" {
		query.Interval = "day"
	}

	points, err := h.service.DAU(r.Context(), projectID, query)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "internal_error", "Failed to query analytics")
		return
	}

	if points == nil {
		points = []TimeSeriesPoint{}
	}

	response.JSON(w, http.StatusOK, map[string]interface{}{
		"data": points,
	})
}
