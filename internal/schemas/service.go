package schemas

import (
	"context"

	"github.com/google/uuid"
)

type PropertyInfo struct {
	Name                string `json:"name"`
	Type                string `json:"type"`
	DistinctValuesCount int    `json:"distinct_values_count"`
}

type EventDetail struct {
	EventName  string         `json:"event_name"`
	Properties []PropertyInfo `json:"properties"`
}

type Service interface {
	ListEventTypes(ctx context.Context, projectID uuid.UUID) ([]EventSummary, error)
	GetEventSchema(ctx context.Context, projectID uuid.UUID, eventName string) (*EventDetail, error)
}

type service struct {
	repo Repository
}

func NewService(repo Repository) Service {
	return &service{repo: repo}
}

func (s *service) ListEventTypes(ctx context.Context, projectID uuid.UUID) ([]EventSummary, error) {
	return s.repo.ListEventTypes(ctx, projectID)
}

func (s *service) GetEventSchema(ctx context.Context, projectID uuid.UUID, eventName string) (*EventDetail, error) {
	schemas, err := s.repo.ListByEvent(ctx, projectID, eventName)
	if err != nil {
		return nil, err
	}

	if len(schemas) == 0 {
		return nil, nil
	}

	properties := make([]PropertyInfo, 0, len(schemas))
	for _, schema := range schemas {
		properties = append(properties, PropertyInfo{
			Name:                schema.PropertyName,
			Type:                schema.PropertyType,
			DistinctValuesCount: schema.DistinctValuesCount,
		})
	}

	return &EventDetail{
		EventName:  eventName,
		Properties: properties,
	}, nil
}

// InferType infers the JSON type of a value.
func InferType(val interface{}) string {
	switch val.(type) {
	case float64, int, int64, float32:
		return "number"
	case bool:
		return "boolean"
	case string:
		return "string"
	case []interface{}:
		return "array"
	case map[string]interface{}:
		return "object"
	case nil:
		return "null"
	default:
		return "string"
	}
}

// mergeType keeps the known type when null values appear and reports conflicting types as mixed.
func mergeType(a, b string) string {
	switch {
	case a == b || b == "null":
		return a
	case a == "null":
		return b
	}
	return "mixed"
}
