package tools

import (
	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"

	"github.com/khaledhikmat/ai-event-processor/models"
	"github.com/khaledhikmat/ai-event-processor/services/events"
)

// **** Models
// CorrelationProximityCalculationArgs defines the arguments for the get_proximity_calculation tool.
type CorrelationProximityCalculationArgs struct {
	Event1 models.SecurityEvent `json:"event1" description:"First event for proximity calculation"`
	Event2 models.SecurityEvent `json:"event2" description:"Second event for proximity calculation"`
}

// CorrelationProximityCalculationResult defines the response structure for the get_proximity_calculation tool.
type CorrelationProximityCalculationResult struct {
	TimeDiff     float64 `json:"timeDiff"`
	HasProximity bool    `json:"hasProximity"`
	Description  string  `json:"description"`
}

// **** Constructor
// NewCorrelationProximityCalculationTool creates a new ADK tool for retrieving events.
// Prevent the tool to instantiate services directly.
func NewCorrelationProximityCalculationTool(datasvc events.Service) (tool.Tool, *CorrelationProximityCalculationProvider, error) {
	wp := &CorrelationProximityCalculationProvider{
		DataService: datasvc,
	}
	t, err := functiontool.New(functiontool.Config{
		Name:        "get_proximity_calculation",
		Description: "Calculate proximity between two events.",
	}, wp.GetProximityCalculation)
	return t, wp, err
}

// **** Methods
// CorrelationProximityCalculationProvider implements the get_proximity_calculation tool.
type CorrelationProximityCalculationProvider struct {
	DataService events.Service
}

// Close cleans up any resources.
func (wp *CorrelationProximityCalculationProvider) Close() error {
	// Do not close the service as it is managed elsewhere.
	return nil
}

func (wp *CorrelationProximityCalculationProvider) GetProximityCalculation(_ tool.Context, args CorrelationProximityCalculationArgs) (CorrelationProximityCalculationResult, error) {
	timeDiff, hasSpatialProximity, description, err := wp.DataService.CalculateProximity(args.Event1, args.Event2)
	if err != nil {
		return CorrelationProximityCalculationResult{}, err
	}

	return CorrelationProximityCalculationResult{
		TimeDiff:     timeDiff,
		HasProximity: hasSpatialProximity,
		Description:  description,
	}, nil
}
