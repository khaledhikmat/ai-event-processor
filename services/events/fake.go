package events

import (
	_ "embed"
	"encoding/json"
	"math"
	"strings"
	"time"

	"github.com/khaledhikmat/ai-event-processor/models"
	"github.com/khaledhikmat/ai-event-processor/services/lgr"
)

//go:embed data/security-events.json
var jsonData string

type FakeService struct {
	events []models.SecurityEvent
}

func NewFakeService() (*FakeService, error) {
	var events []models.SecurityEvent
	if err := json.Unmarshal([]byte(jsonData), &events); err != nil {
		return nil, err
	}

	lgr.Logger.Info("Loaded fake events", "count", len(events))
	return &FakeService{events: events}, nil
}

// RetrieveEventsHistory retrieves events filtered by location, time window, event types, and severity
func (s *FakeService) RetrieveEventsHistory(location string, minutes int, eventTypes []string, severity string) ([]models.SecurityEvent, error) {
	result := []models.SecurityEvent{} // Initialize as empty slice, not nil
	cutoffTime := time.Now().Add(-time.Duration(minutes) * time.Minute)

	for _, event := range s.events {
		// Filter by time
		if event.Timestamp.Before(cutoffTime) {
			continue
		}

		// Filter by location (check if any location field matches)
		if location != "" {
			locationMatch := strings.Contains(strings.ToLower(event.Location.Site), strings.ToLower(location)) ||
				strings.Contains(strings.ToLower(event.Location.Building), strings.ToLower(location)) ||
				strings.Contains(strings.ToLower(event.Location.Floor), strings.ToLower(location)) ||
				strings.Contains(strings.ToLower(event.Location.Zone), strings.ToLower(location))

			if !locationMatch {
				continue
			}
		}

		// Filter by event types
		if len(eventTypes) > 0 {
			typeMatch := false
			for _, et := range eventTypes {
				if strings.Contains(strings.ToLower(event.EventType), strings.ToLower(et)) {
					typeMatch = true
					break
				}
			}
			if !typeMatch {
				continue
			}
		}

		// Filter by severity
		if severity != "" && !strings.EqualFold(event.Severity, severity) {
			continue
		}

		result = append(result, event)
	}

	return result, nil
}

// RetrieveEntityHistory retrieves all events involving a specific entity within the time window
func (s *FakeService) RetrieveEntityHistory(entity string, minutes int) ([]models.SecurityEvent, error) {
	result := []models.SecurityEvent{} // Initialize as empty slice, not nil
	cutoffTime := time.Now().Add(-time.Duration(minutes) * time.Minute)

	for _, event := range s.events {
		// Filter by time
		if event.Timestamp.Before(cutoffTime) {
			continue
		}

		// Check if entity is involved in this event
		entityMatch := false
		for _, e := range event.Entities {
			if strings.Contains(strings.ToLower(e.EntityID), strings.ToLower(entity)) ||
				strings.Contains(strings.ToLower(e.EntityType), strings.ToLower(entity)) {
				entityMatch = true
				break
			}
		}

		if entityMatch {
			result = append(result, event)
		}
	}

	return result, nil
}

// CalculateProximity calculates spatial and temporal proximity between two events
func (s *FakeService) CalculateProximity(event1, event2 models.SecurityEvent) (float64, bool, string, error) {
	// Calculate temporal proximity (in minutes)
	timeDiff := math.Abs(event1.Timestamp.Sub(event2.Timestamp).Minutes())

	// Calculate spatial proximity (Euclidean distance between coordinates if available)
	var spatialDistance float64
	var hasSpatialProximity bool

	if event1.Location.Coordinates.Lat != 0 && event1.Location.Coordinates.Lng != 0 &&
		event2.Location.Coordinates.Lat != 0 && event2.Location.Coordinates.Lng != 0 {
		// Simple Euclidean distance (not geographically accurate for large distances)
		latDiff := event1.Location.Coordinates.Lat - event2.Location.Coordinates.Lat
		lngDiff := event1.Location.Coordinates.Lng - event2.Location.Coordinates.Lng
		spatialDistance = math.Sqrt(latDiff*latDiff + lngDiff*lngDiff)
		hasSpatialProximity = true
	} else {
		// Check if same location hierarchy
		if event1.Location.Site == event2.Location.Site &&
			event1.Location.Building == event2.Location.Building {
			hasSpatialProximity = true
			if event1.Location.Floor == event2.Location.Floor {
				if event1.Location.Zone == event2.Location.Zone {
					spatialDistance = 0.0 // Same zone
				} else {
					spatialDistance = 1.0 // Same floor, different zone
				}
			} else {
				spatialDistance = 2.0 // Same building, different floor
			}
		} else if event1.Location.Site == event2.Location.Site {
			hasSpatialProximity = true
			spatialDistance = 3.0 // Same site, different building
		} else {
			hasSpatialProximity = false
			spatialDistance = 999.0 // Different sites
		}
	}

	// Determine proximity description
	var description string
	if timeDiff < 5 && spatialDistance < 1.0 {
		description = "Very close temporal and spatial proximity"
	} else if timeDiff < 15 && spatialDistance < 2.0 {
		description = "Close temporal and spatial proximity"
	} else if timeDiff < 30 && spatialDistance < 3.0 {
		description = "Moderate temporal and spatial proximity"
	} else {
		description = "Low temporal or spatial proximity"
	}

	return timeDiff, hasSpatialProximity, description, nil
}
