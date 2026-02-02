package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

const (
	baseURL        = "http://localhost:8081"
	appName        = "root_agent"
	userID         = "testuser"
	sessionID      = "testsession"
	pauseDuration  = 100 * time.Second
	eventsFilePath = "./input-events.json"
)

type SecurityEvent struct {
	EventID      string                 `json:"event_id"`
	EventType    string                 `json:"event_type"`
	SourceSystem string                 `json:"source_system"`
	Timestamp    string                 `json:"timestamp"`
	Location     map[string]interface{} `json:"location"`
	Severity     string                 `json:"severity"`
	Entities     []interface{}          `json:"entities"`
	Payload      map[string]interface{} `json:"payload"`
	Metadata     map[string]interface{} `json:"metadata"`
}

type ADKRequest struct {
	AppName    string     `json:"appName"`
	UserID     string     `json:"userId"`
	SessionID  string     `json:"sessionId"`
	NewMessage NewMessage `json:"newMessage"`
}

type NewMessage struct {
	Role  string        `json:"role"`
	Parts []MessagePart `json:"parts"`
}

type MessagePart struct {
	Text string `json:"text"`
}

func main() {
	fmt.Println("Security Event Test Script")
	fmt.Println("===========================")

	// Load events from file
	events, err := loadEvents(eventsFilePath)
	if err != nil {
		fmt.Printf("Failed to load events: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Loaded %d events from %s\n\n", len(events), eventsFilePath)

	// Process each event
	for i, event := range events {
		fmt.Printf("[%d/%d] Processing event: %s (%s)\n", i+1, len(events), event.EventID, event.EventType)
		fmt.Printf("        Severity: %s, Location: %s/%s\n",
			event.Severity,
			event.Location["building"],
			event.Location["zone"])

		if err := sendEvent(event, i+1); err != nil {
			fmt.Printf("❌ Error sending event: %v\n\n", err)
			//continue
			os.Exit(1)
		}

		fmt.Printf("✅ Event sent successfully\n")

		// Pause before next event (except after the last one)
		if i < len(events)-1 {
			fmt.Printf("⏸️  Pausing for %v before next event...\n\n", pauseDuration)
			time.Sleep(pauseDuration)
		}
	}

	fmt.Printf("\n✅ All %d events processed successfully!\n", len(events))
}

func createSession() error {
	sessionURL := fmt.Sprintf("%s/api/apps/%s/users/%s/sessions/%s",
		baseURL, appName, userID, sessionID)

	fmt.Printf("Creating session at: %s\n", sessionURL)

	resp, err := http.Post(sessionURL, "application/json", nil)
	if err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("session creation failed with status %d: %s", resp.StatusCode, string(body))
	}

	fmt.Printf("✅ Session created successfully\n\n")
	return nil
}

func createSessionWithState(eventJSON string) error {
	sessionURL := fmt.Sprintf("%s/api/apps/%s/users/%s/sessions/%s",
		baseURL, appName, userID, sessionID)

	// Create session with initial state
	sessionPayload := map[string]interface{}{
		"state": map[string]interface{}{
			"event_data": eventJSON,
		},
	}

	payloadJSON, err := json.Marshal(sessionPayload)
	if err != nil {
		return fmt.Errorf("failed to marshal session payload: %w", err)
	}

	fmt.Printf("Creating session with state payload: %s\n", string(payloadJSON))

	resp, err := http.Post(sessionURL, "application/json", bytes.NewBuffer(payloadJSON))
	if err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("session creation failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Verify the session was created with state
	body, _ := io.ReadAll(resp.Body)
	fmt.Printf("Session created successfully. Response: %s\n", string(body))

	// Double-check by fetching the session
	return verifySessionState()
}

func verifySessionState() error {
	sessionURL := fmt.Sprintf("%s/api/apps/%s/users/%s/sessions/%s",
		baseURL, appName, userID, sessionID)

	resp, err := http.Get(sessionURL)
	if err != nil {
		return fmt.Errorf("failed to get session: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	fmt.Printf("Session state verification: %s\n\n", string(body))

	return nil
}

func deleteSession() error {
	sessionURL := fmt.Sprintf("%s/api/apps/%s/users/%s/sessions/%s",
		baseURL, appName, userID, sessionID)

	req, err := http.NewRequest(http.MethodDelete, sessionURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create delete request: %w", err)
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to delete session: %w", err)
	}
	defer resp.Body.Close()

	// Ignore errors - session might not exist
	return nil
}

func loadEvents(filePath string) ([]SecurityEvent, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read events file: %w", err)
	}

	var events []SecurityEvent
	if err := json.Unmarshal(data, &events); err != nil {
		return nil, fmt.Errorf("failed to parse events JSON: %w", err)
	}

	return events, nil
}

func sendEvent(event SecurityEvent, eventNum int) error {
	// Marshal the event to JSON string
	eventJSON, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	// Delete existing session (ignore errors if it doesn't exist)
	deleteSession()

	// Create session with event data in initial state
	if err := createSessionWithState(string(eventJSON)); err != nil {
		return fmt.Errorf("failed to create session with state: %w", err)
	}

	// Create the ADK request
	request := ADKRequest{
		AppName:   appName,
		UserID:    userID,
		SessionID: sessionID,
		NewMessage: NewMessage{
			Role: "user",
			Parts: []MessagePart{
				{Text: fmt.Sprintf("Analyze this security event #%d", eventNum)},
			},
		},
	}

	// Marshal the request
	requestJSON, err := json.Marshal(request)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	// Send the request with 3-minute timeout
	endpoint := fmt.Sprintf("%s/api/run", baseURL)
	client := &http.Client{
		Timeout: 3 * time.Minute,
	}
	resp, err := client.Post(endpoint, "application/json", bytes.NewBuffer(requestJSON))
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Read response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	// Check status code
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("request failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Parse and extract final agent outputs
	var events []map[string]interface{}
	if err := json.Unmarshal(body, &events); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	// Extract state delta with agent outputs
	agentOutputs := make(map[string]string)
	for _, event := range events {
		if actions, ok := event["actions"].(map[string]interface{}); ok {
			if stateDelta, ok := actions["stateDelta"].(map[string]interface{}); ok {
				for key, value := range stateDelta {
					if strValue, ok := value.(string); ok && strValue != "" {
						agentOutputs[key] = strValue
					}
				}
			}
		}
	}

	// Pretty print just the agent outputs
	fmt.Printf("\n        === Agent Analysis Results ===\n\n")
	if triageOutput, ok := agentOutputs["triage_output"]; ok {
		fmt.Printf("        📊 TRIAGE:\n%s\n\n", triageOutput)
	}
	if corrOutput, ok := agentOutputs["correlation_output"]; ok {
		fmt.Printf("        🔗 CORRELATION:\n%s\n\n", corrOutput)
	}
	if runbooksOutput, ok := agentOutputs["runbooks_output"]; ok {
		fmt.Printf("        📖 RUNBOOKS:\n%s\n\n", runbooksOutput)
	}

	// The final response is in the last event's content
	for i := len(events) - 1; i >= 0; i-- {
		if content, ok := events[i]["content"].(map[string]interface{}); ok {
			if parts, ok := content["parts"].([]interface{}); ok {
				for _, part := range parts {
					if partMap, ok := part.(map[string]interface{}); ok {
						if text, ok := partMap["text"].(string); ok && text != "" {
							fmt.Printf("        🎯 FINAL RECOMMENDATIONS:\n%s\n\n", text)
							return nil
						}
					}
				}
			}
		}
	}

	return nil
}
