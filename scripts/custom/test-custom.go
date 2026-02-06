package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/khaledhikmat/ai-event-processor/services/adkclient"
	"google.golang.org/adk/session"
)

const (
	baseURL       = "http://localhost:8081"
	appName       = "test_agent"
	userID        = "testuser"
	sessionID     = "testsession"
	pauseDuration = 100 * time.Second
)

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
	fmt.Println("Custom Test Script")
	fmt.Println("===========================")

	createSession()

	// Process each event
	if err := sendText("test message"); err != nil {
		fmt.Printf("❌ Error sending event: %v\n\n", err)
		//continue
		os.Exit(1)
	}
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

func sendText(text string) error {
	// Create the ADK request
	request := ADKRequest{
		AppName:   appName,
		UserID:    userID,
		SessionID: sessionID,
		NewMessage: NewMessage{
			Role: "user",
			Parts: []MessagePart{
				{Text: text},
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

	// Parse REST events
	var restEvents []adkclient.RestEvent
	if err := json.Unmarshal(body, &restEvents); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	// Convert to session events
	events := make([]*session.Event, len(restEvents))
	for i, re := range restEvents {
		events[i] = re.ToSessionEvent()
	}

	for i, event := range events {
		fmt.Printf("Event %d: ID=%s, Time=%s, Author=%s, Partial=%t, Content=%s\n",
			i+1, event.ID, event.Timestamp.Format(time.RFC3339), event.Author, event.Partial, adkclient.GetText(event))
	}

	// Extract outputs using helper
	outputs := adkclient.ExtractAgentOutputs(events)

	if triageOutput, ok := outputs["triage_output"]; ok {
		fmt.Printf("📊 TRIAGE:\n%s\n\n", triageOutput)
	}

	if corrOutput, ok := outputs["correlation_output"]; ok {
		fmt.Printf("🔗 CORRELATION:\n%s\n\n", corrOutput)
	}

	if runbooksOutput, ok := outputs["runbooks_output"]; ok {
		fmt.Printf("📖 RUNBOOKS:\n%s\n\n", runbooksOutput)
	}

	// Get final response
	finalResponse := adkclient.GetFinalResponse(events)
	if finalResponse != "" {
		fmt.Printf("🎯 FINAL RESPONSE:\n%s\n\n", finalResponse)
	}

	// Access individual events with native types
	for _, event := range events {
		// All session.Event methods available
		if event.IsFinalResponse() {
			log.Printf("[%s] - Final event: %s", event.Timestamp, adkclient.GetText(event))
		}

		// Access state delta
		for key, value := range event.Actions.StateDelta {
			log.Printf("State change: %s = %v", key, value)
		}
	}

	return nil
}
