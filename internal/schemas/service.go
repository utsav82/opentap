package schemas

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/opentap/opentap/internal/models"
)

type EventSummary struct {
	EventName     string    `json:"event_name"`
	PropertyCount int       `json:"property_count"`
	FirstSeen     time.Time `json:"first_seen"`
	LastSeen      time.Time `json:"last_seen"`
}

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
	schemas, err := s.repo.ListByProject(ctx, projectID)
	if err != nil {
		return nil, err
	}

	// Group by event_name
	summaryMap := make(map[string]*EventSummary)
	for _, schema := range schemas {
		if existing, ok := summaryMap[schema.EventName]; ok {
			existing.PropertyCount++
			if schema.FirstSeen.Before(existing.FirstSeen) {
				existing.FirstSeen = schema.FirstSeen
			}
			if schema.LastSeen.After(existing.LastSeen) {
				existing.LastSeen = schema.LastSeen
			}
		} else {
			summaryMap[schema.EventName] = &EventSummary{
				EventName:     schema.EventName,
				PropertyCount: 1,
				FirstSeen:     schema.FirstSeen,
				LastSeen:      schema.LastSeen,
			}
		}
	}

	summaries := make([]EventSummary, 0, len(summaryMap))
	for _, s := range summaryMap {
		summaries = append(summaries, *s)
	}
	return summaries, nil
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

// ExtractSchemas extracts property schemas from an event.
func ExtractSchemas(projectID uuid.UUID, eventName string, properties map[string]interface{}, now time.Time) []*models.EventSchema {
	var schemas []*models.EventSchema
	for key, val := range properties {
		schemas = append(schemas, &models.EventSchema{
			ProjectID:           projectID,
			EventName:           eventName,
			PropertyName:        key,
			PropertyType:        InferType(val),
			DistinctValuesCount: 1,
			FirstSeen:           now,
			LastSeen:            now,
		})
	}
	return schemas
}
