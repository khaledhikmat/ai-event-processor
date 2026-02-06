package adkclient

import (
	"strings"

	"google.golang.org/adk/session"
)

// GetText extracts all text content from an event
func GetText(event *session.Event) string {
	if event.Content == nil || len(event.Content.Parts) == 0 {
		return ""
	}

	var texts []string
	for _, part := range event.Content.Parts {
		if part.Text != "" {
			texts = append(texts, part.Text)
		}
	}

	return strings.Join(texts, "\n")
}

// ExtractAgentOutputs extracts all values from StateDelta
func ExtractAgentOutputs(events []*session.Event) map[string]string {
	outputs := make(map[string]string)

	for _, event := range events {
		if event.Actions.StateDelta != nil {
			for key, value := range event.Actions.StateDelta {
				if strValue, ok := value.(string); ok && strValue != "" {
					outputs[key] = strValue
				}
			}
		}
	}

	return outputs
}

// GetFinalResponse returns the text from the last final (non-partial) event
func GetFinalResponse(events []*session.Event) string {
	for i := len(events) - 1; i >= 0; i-- {
		event := events[i]
		if event.IsFinalResponse() {
			return GetText(event)
		}
	}
	return ""
}
