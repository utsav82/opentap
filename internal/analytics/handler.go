package analytics

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/opentap/opentap/internal/auth"
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

	points, err := h.service.EventCountOverTime(r.Context(), auth.ProjectID(r.Context()), query)
	if err != nil {
		response.Fail(w, err)
		return
	}

	response.JSON(w, http.StatusOK, map[string]interface{}{
		"data": points,
	})
}

func (h *Handler) TopEvents(w http.ResponseWriter, r *http.Request) {
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

	counts, err := h.service.TopEvents(r.Context(), auth.ProjectID(r.Context()), timeWindow, limit)
	if err != nil {
		response.Fail(w, err)
		return
	}

	response.JSON(w, http.StatusOK, map[string]interface{}{
		"data": counts,
	})
}

func (h *Handler) DAU(w http.ResponseWriter, r *http.Request) {
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

	points, err := h.service.DAU(r.Context(), auth.ProjectID(r.Context()), query)
	if err != nil {
		response.Fail(w, err)
		return
	}

	response.JSON(w, http.StatusOK, map[string]interface{}{
		"data": points,
	})
}

func (h *Handler) Funnel(w http.ResponseWriter, r *http.Request) {
	steps := strings.Split(r.URL.Query().Get("steps"), ",")
	for i := range steps {
		steps[i] = strings.TrimSpace(steps[i])
	}

	result, err := h.service.Funnel(r.Context(), auth.ProjectID(r.Context()), steps, r.URL.Query().Get("time_window"))
	if err != nil {
		response.Fail(w, err)
		return
	}

	response.JSON(w, http.StatusOK, map[string]interface{}{
		"data": result,
	})
}

func (h *Handler) Retention(w http.ResponseWriter, r *http.Request) {
	query := RetentionQuery{
		StartEvent:  r.URL.Query().Get("start_event"),
		ReturnEvent: r.URL.Query().Get("return_event"),
		Interval:    r.URL.Query().Get("interval"),
		Periods:     8,
	}
	if query.Interval == "" {
		query.Interval = "week"
	}
	if p := r.URL.Query().Get("periods"); p != "" {
		query.Periods, _ = strconv.Atoi(p)
	}

	cohorts, err := h.service.Retention(r.Context(), auth.ProjectID(r.Context()), query)
	if err != nil {
		response.Fail(w, err)
		return
	}

	response.JSON(w, http.StatusOK, map[string]interface{}{
		"data": cohorts,
	})
}
