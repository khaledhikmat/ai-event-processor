package tools

import (
	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"

	"github.com/khaledhikmat/ai-event-processor/models"
	"github.com/khaledhikmat/ai-event-processor/services/events"
)

// **** Models
// CorrelationEntityHistoryArgs defines the arguments for the get_entity_history tool.
type CorrelationEntityHistoryArgs struct {
	Entity  string `json:"entity" description:"Entity to query for"`
	Minutes int    `json:"minutes" description:"Time window in minutes for events"`
}

// CorrelationEntityHistoryResult defines the response structure for the get_entity_history tool.
type CorrelationEntityHistoryResult struct {
	Events []models.SecurityEvent `json:"events"`
}

// **** Constructor
// NewCorrelationEntityHistoryTool creates a new ADK tool for retrieving events.
// Prevent the tool to instantiate services directly.
func NewCorrelationEntityHistoryTool(datasvc events.Service) (tool.Tool, *CorrelationEntityHistoryProvider, error) {
	wp := &CorrelationEntityHistoryProvider{
		DataService: datasvc,
	}
	t, err := functiontool.New(functiontool.Config{
		Name:        "get_entity_history",
		Description: "Search for entity history based on entity and time window.",
	}, wp.GetEntityHistory)
	return t, wp, err
}

// **** Methods
// CorrelationEntityHistoryProvider implements the get_entity_history tool.
type CorrelationEntityHistoryProvider struct {
	DataService events.Service
}

// Close cleans up any resources.
func (wp *CorrelationEntityHistoryProvider) Close() error {
	// Do not close the service as it is managed elsewhere.
	return nil
}

func (wp *CorrelationEntityHistoryProvider) GetEntityHistory(_ tool.Context, args CorrelationEntityHistoryArgs) (CorrelationEntityHistoryResult, error) {
	events, err := wp.DataService.RetrieveEntityHistory(args.Entity, args.Minutes)
	if err != nil {
		return CorrelationEntityHistoryResult{}, err
	}

	return CorrelationEntityHistoryResult{
		Events: events,
	}, nil
}
