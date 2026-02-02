package events

import "github.com/khaledhikmat/ai-event-processor/models"

type Service interface {
	RetrieveEventsHistory(location string, minutes int, eventType []string, severity string) ([]models.SecurityEvent, error)
	RetrieveEntityHistory(entity string, minutes int) ([]models.SecurityEvent, error)
	CalculateProximity(event1, event2 models.SecurityEvent) (float64, bool, string, error)
}
