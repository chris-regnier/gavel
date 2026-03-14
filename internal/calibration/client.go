package calibration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Client talks to the calibration server.
type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

// NewClient constructs a Client targeting baseURL, authenticating with apiKey,
// and applying timeout to every HTTP request.
//
// A trailing slash on baseURL is trimmed so that path concatenation is always
// consistent regardless of how the caller configures the URL.
func NewClient(baseURL, apiKey string, timeout time.Duration) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		httpClient: &http.Client{Timeout: timeout},
	}
}

// UploadEvents sends a batch of calibration events to POST /v1/events/batch.
//
// The request body is a JSON object with a "team_id" string and an "events"
// array. The server is expected to respond with HTTP 202 Accepted; any other
// status code is returned as an error.
func (c *Client) UploadEvents(ctx context.Context, teamID string, events []Event) error {
	body := struct {
		TeamID string  `json:"team_id"`
		Events []Event `json:"events"`
	}{TeamID: teamID, Events: events}

	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal events: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/events/batch", bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("upload events: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted {
		return fmt.Errorf("upload events: status %d", resp.StatusCode)
	}
	return nil
}

// GetCalibration fetches team-specific threshold overrides and cross-org signals
// from GET /v1/calibration/{teamID}.
//
// ruleIDs, when non-empty, restricts the response to the named rules via the
// "rules" query parameter (comma-separated). fileType is passed as the
// "file_type" query parameter so the server can filter few-shot examples.
//
// Returns a parsed CalibrationResponse on success or an error when the request
// fails, times out, or the server returns a non-200 status.
func (c *Client) GetCalibration(ctx context.Context, teamID string, ruleIDs []string, fileType string) (*CalibrationResponse, error) {
	url := fmt.Sprintf("%s/v1/calibration/%s?file_type=%s", c.baseURL, teamID, fileType)
	if len(ruleIDs) > 0 {
		url += "&rules=" + strings.Join(ruleIDs, ",")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("get calibration: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("get calibration: status %d", resp.StatusCode)
	}

	var cal CalibrationResponse
	if err := json.NewDecoder(resp.Body).Decode(&cal); err != nil {
		return nil, fmt.Errorf("decode calibration: %w", err)
	}
	return &cal, nil
}
