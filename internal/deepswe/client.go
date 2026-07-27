package deepswe

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// Client fetches the DeepSWE live leaderboard artifact (no authentication
// required).
type Client struct {
	url  string
	http *http.Client
}

// NewClient builds a Client that fetches leaderboardURL, e.g.
// "https://deepswe.datacurve.ai/artifacts/v1.1/leaderboard-live.json". A nil
// httpClient gets a default client with OpenTelemetry instrumentation and a
// 30s timeout.
func NewClient(leaderboardURL string, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{
			Timeout:   30 * time.Second,
			Transport: otelhttp.NewTransport(http.DefaultTransport),
		}
	}
	return &Client{url: leaderboardURL, http: httpClient}
}

// Leaderboard fetches and decodes the full leaderboard in one request.
func (c *Client) Leaderboard(ctx context.Context) ([]LeaderboardRow, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url, nil)
	if err != nil {
		return nil, fmt.Errorf("deepswe: build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("deepswe: fetch leaderboard: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return nil, fmt.Errorf("deepswe: unexpected status %d: %s", resp.StatusCode, string(body))
	}

	var out leaderboardResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("deepswe: decode leaderboard: %w", err)
	}
	return out.Rows, nil
}
