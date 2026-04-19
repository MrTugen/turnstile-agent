// Package edge talks to turnstile-edge's scan endpoint to decide whether a
// given RFID UID should be granted access.
package edge

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/MrTugen/turnstile-agent/internal/uid"
)

// Client posts scan payloads to the edge and decodes the decision.
type Client struct {
	url        string
	apiKey     string
	deviceName string
	http       *http.Client
}

// Decision is the result of an edge verify call.
type Decision struct {
	Granted bool
	Reason  string
}

// Options configure a Client. RequestTimeout is applied as the HTTP client's
// Timeout; pass context.WithTimeout on the call for per-request deadlines.
type Options struct {
	URL            string
	APIKey         string
	DeviceName     string
	RequestTimeout time.Duration
}

// New constructs a Client with its own keep-alive-enabled HTTP client.
func New(opts Options) *Client {
	return &Client{
		url:        opts.URL,
		apiKey:     opts.APIKey,
		deviceName: opts.DeviceName,
		http:       &http.Client{Timeout: opts.RequestTimeout},
	}
}

type scanRequest struct {
	UID        string `json:"uid"`
	DeviceName string `json:"deviceName"`
	Timestamp  int64  `json:"timestamp"`
}

// scanResponse accepts any of the three boolean field names the Python agent
// historically tolerated. Any one set to true grants access.
type scanResponse struct {
	Granted *bool  `json:"granted,omitempty"`
	Access  *bool  `json:"access,omitempty"`
	Allowed *bool  `json:"allowed,omitempty"`
	Reason  string `json:"reason,omitempty"`
}

// VerifyScan sends the UID to edge and returns the access decision.
func (c *Client) VerifyScan(ctx context.Context, rawUID string) (Decision, error) {
	normalized := uid.Normalize(rawUID)

	body, err := json.Marshal(scanRequest{
		UID:        normalized,
		DeviceName: c.deviceName,
		Timestamp:  time.Now().UnixMilli(),
	})
	if err != nil {
		return Decision{}, fmt.Errorf("marshal scan request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, bytes.NewReader(body))
	if err != nil {
		return Decision{}, fmt.Errorf("build scan request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("X-API-Key", c.apiKey)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return Decision{}, fmt.Errorf("send scan request: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return Decision{}, fmt.Errorf("read scan response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Decision{}, fmt.Errorf("edge returned %s: %s", resp.Status, string(raw))
	}

	var decoded scanResponse
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return Decision{}, fmt.Errorf("decode scan response: %w (body: %s)", err, string(raw))
	}

	granted := (decoded.Granted != nil && *decoded.Granted) ||
		(decoded.Access != nil && *decoded.Access) ||
		(decoded.Allowed != nil && *decoded.Allowed)

	reason := decoded.Reason
	if reason == "" {
		reason = "edge_response"
	}

	return Decision{Granted: granted, Reason: reason}, nil
}
