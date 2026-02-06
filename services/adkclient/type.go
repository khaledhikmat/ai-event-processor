package adkclient

import (
	"time"

	"google.golang.org/adk/model"
	"google.golang.org/adk/session"
	"google.golang.org/genai"
)

// RestEvent represents the JSON format returned by ADK REST API
// This is the ONLY place where you define the REST API structure
type RestEvent struct {
	ID                 string                   `json:"id"`
	Time               int64                    `json:"time"`
	InvocationID       string                   `json:"invocationId"`
	Branch             string                   `json:"branch"`
	Author             string                   `json:"author"`
	Partial            bool                     `json:"partial"`
	LongRunningToolIDs []string                 `json:"longRunningToolIds"`
	Content            *genai.Content           `json:"content"`
	GroundingMetadata  *genai.GroundingMetadata `json:"groundingMetadata"`
	TurnComplete       bool                     `json:"turnComplete"`
	Interrupted        bool                     `json:"interrupted"`
	ErrorCode          string                   `json:"errorCode"`
	ErrorMessage       string                   `json:"errorMessage"`
	Actions            RestEventActions         `json:"actions"`
}

type RestEventActions struct {
	StateDelta    map[string]any   `json:"stateDelta"`
	ArtifactDelta map[string]int64 `json:"artifactDelta"`
}

// ToSessionEvent converts REST format to session.Event
// This uses ADK's native types, so you benefit from their updates
func (e *RestEvent) ToSessionEvent() *session.Event {
	return &session.Event{
		ID:                 e.ID,
		Timestamp:          time.Unix(e.Time, 0),
		InvocationID:       e.InvocationID,
		Branch:             e.Branch,
		Author:             e.Author,
		LongRunningToolIDs: e.LongRunningToolIDs,
		LLMResponse: model.LLMResponse{
			Content:           e.Content,
			GroundingMetadata: e.GroundingMetadata,
			Partial:           e.Partial,
			TurnComplete:      e.TurnComplete,
			Interrupted:       e.Interrupted,
			ErrorCode:         e.ErrorCode,
			ErrorMessage:      e.ErrorMessage,
		},
		Actions: session.EventActions{
			StateDelta:    e.Actions.StateDelta,
			ArtifactDelta: e.Actions.ArtifactDelta,
		},
	}
}
