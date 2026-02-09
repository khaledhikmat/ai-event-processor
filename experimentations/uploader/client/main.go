package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/khaledhikmat/ai-event-processor/services/adkclient"
	"google.golang.org/adk/session"
)

const (
	ServerURL = "http://localhost:8080"
	AppName   = "image_analyzer"
	UserID    = "user123"
	SessionID = "session123"
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
	// Step 1: Create a session (if not already created by client.go)
	if err := createSession(); err != nil {
		fmt.Printf("Ignoring session creation failed: %v\n", err)
		//return // Ignore
	}

	// Step 2: Upload an image
	imageFile := "image.png"
	if err := uploadImage(imageFile); err != nil {
		fmt.Printf("Upload failed: %v\n", err)
		return
	}
	fmt.Println("Image uploaded successfully!")

	// Step 3: Analyze the image
	question := "What colors are dominant in this image? What time of day was this photo taken?"
	analysis, err := analyzeImage(question)
	if err != nil {
		fmt.Printf("Analysis failed: %v\n", err)
		return
	}

	fmt.Printf("\nAnalysis:\n%s\n", analysis)
}

func uploadImage(filename string) error {
	// Read image file
	imageData, err := os.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("failed to read image: %w", err)
	}

	// Upload via custom endpoint
	url := fmt.Sprintf("%s/upload?app=%s&user=%s&session=%s&filename=%s",
		ServerURL, AppName, UserID, SessionID, filename)

	resp, err := http.Post(url, "image/jpeg", bytes.NewReader(imageData))
	if err != nil {
		return fmt.Errorf("upload request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("upload failed: %s", string(body))
	}

	return nil
}

func createSession() error {
	sessionURL := fmt.Sprintf("%s/api/apps/%s/users/%s/sessions/%s",
		ServerURL, AppName, UserID, SessionID)

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

func analyzeImage(question string) (string, error) {
	// Create the ADK request
	request := ADKRequest{
		AppName:   AppName,
		UserID:    UserID,
		SessionID: SessionID,
		NewMessage: NewMessage{
			Role: "user",
			Parts: []MessagePart{
				{Text: question},
			},
		},
	}

	// Marshal the request
	requestJSON, err := json.Marshal(request)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	// Send the request with 3-minute timeout
	endpoint := fmt.Sprintf("%s/api/run", ServerURL)
	client := &http.Client{
		Timeout: 3 * time.Minute,
	}
	fmt.Printf("Server endpoint URL: %s\n", endpoint)

	resp, err := client.Post(endpoint, "application/json", bytes.NewBuffer(requestJSON))
	if err != nil {
		return "", fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Read response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	// Check status code
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var restEvents []adkclient.RestEvent
	if err := json.Unmarshal(body, &restEvents); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
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

	// var result struct {
	// 	Events []struct {
	// 		Author  string `json:"author"`
	// 		Content struct {
	// 			Parts []struct {
	// 				Text string `json:"text"`
	// 			} `json:"parts"`
	// 		} `json:"content"`
	// 	} `json:"events"`
	// }

	// if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
	// 	return "", fmt.Errorf("failed to decode response: %w", err)
	// }

	// // Extract model response
	// var analysis string
	// for _, event := range result.Events {
	// 	if event.Author == "model" {
	// 		for _, part := range event.Content.Parts {
	// 			analysis += part.Text
	// 		}
	// 	}
	// }

	return "analysis", nil
}
