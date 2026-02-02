package tools

import (
	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"

	"github.com/khaledhikmat/ai-event-processor/models"
	"github.com/khaledhikmat/ai-event-processor/services/events"
)

// **** Models
// CorrelationEventHistoryArgs defines the arguments for the get_event_history tool.
type CorrelationEventHistoryArgs struct {
	Location   string   `json:"location" description:"Event location to query for (building, floor, zone, or site)"`
	Minutes    int      `json:"minutes" description:"Time window in minutes to look back from now (e.g., 30 for last 30 minutes)"`
	EventTypes []string `json:"event_types,omitempty" description:"Optional: Specific event types to filter (e.g., 'video.detection', 'access.denied'). Leave empty to get all event types."`
	Severity   string   `json:"severity,omitempty" description:"Optional: Filter by severity level (CRITICAL, HIGH, MEDIUM, LOW). Leave empty for all severities."`
}

// CorrelationEventHistoryResult defines the response structure for the get_event_history tool.
type CorrelationEventHistoryResult struct {
	Events []models.SecurityEvent `json:"events"`
}

// **** Constructor
// NewCorrelationEventHistorTool creates a new ADK tool for retrieving events.
// Prevent the tool to instantiate services directly.
func NewCorrelationEventHistorTool(datasvc events.Service) (tool.Tool, *CorrelationEventHistoryProvider, error) {
	wp := &CorrelationEventHistoryProvider{
		DataService: datasvc,
	}
	t, err := functiontool.New(functiontool.Config{
		Name:        "get_events_history",
		Description: "Search for event history based on location, time window, event types, and severity.",
	}, wp.GetEventsHistory)
	return t, wp, err
}

// **** Methods
// CorrelationEventHistoryProvider implements the get_events_history tool.
type CorrelationEventHistoryProvider struct {
	DataService events.Service
}

// Close cleans up any resources.
func (wp *CorrelationEventHistoryProvider) Close() error {
	// Do not close the service as it is managed elsewhere.
	return nil
}

func (wp *CorrelationEventHistoryProvider) GetEventsHistory(_ tool.Context, args CorrelationEventHistoryArgs) (CorrelationEventHistoryResult, error) {
	events, err := wp.DataService.RetrieveEventsHistory(args.Location, args.Minutes, args.EventTypes, args.Severity)
	if err != nil {
		return CorrelationEventHistoryResult{}, err
	}

	return CorrelationEventHistoryResult{
		Events: events,
	}, nil
}
