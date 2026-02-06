package adkclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"google.golang.org/adk/session"
)

type Client struct {
	BaseURL    string
	HTTPClient *http.Client
}

func NewClient(baseURL string) *Client {
	return &Client{
		BaseURL: baseURL,
		HTTPClient: &http.Client{
			Timeout: 3 * time.Minute,
		},
	}
}

// RunAgent sends a message to an agent and returns session events
func (c *Client) RunAgent(ctx context.Context, req *RunRequest) ([]*session.Event, error) {
	// Marshal request
	reqJSON, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Send HTTP request
	endpoint := fmt.Sprintf("%s/api/run", c.BaseURL)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewBuffer(reqJSON))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Read response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("request failed with status %d: %s",
			resp.StatusCode, string(body))
	}

	// Parse REST events
	var restEvents []RestEvent
	if err := json.Unmarshal(body, &restEvents); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	// Convert to session events
	events := make([]*session.Event, len(restEvents))
	for i, re := range restEvents {
		events[i] = re.ToSessionEvent()
	}

	return events, nil
}

type RunRequest struct {
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
