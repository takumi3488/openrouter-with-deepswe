// Package openrouter selects Text-to-Text models from the OpenRouter model
// catalog and resolves each one's cheapest provider pricing.
package openrouter

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// Client talks to the OpenRouter public API (no authentication required for
// the endpoints this package uses).
type Client struct {
	baseURL string
	http    *http.Client
}

// NewClient builds a Client against baseURL (e.g.
// "https://openrouter.ai/api/v1"). A nil httpClient gets a default client
// with OpenTelemetry instrumentation and a 30s timeout.
func NewClient(baseURL string, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{
			Timeout:   30 * time.Second,
			Transport: otelhttp.NewTransport(http.DefaultTransport),
		}
	}
	return &Client{baseURL: strings.TrimRight(baseURL, "/"), http: httpClient}
}

// ListModels fetches the full model catalog.
func (c *Client) ListModels(ctx context.Context) ([]Model, error) {
	var out modelsResponse
	if err := c.getJSON(ctx, "/models", &out); err != nil {
		return nil, fmt.Errorf("list models: %w", err)
	}
	return out.Data, nil
}

// Endpoints fetches the per-provider pricing for a single model. modelID is
// used as-is (author/slug, optionally with a ":variant" suffix).
func (c *Client) Endpoints(ctx context.Context, modelID string) ([]Endpoint, error) {
	var out endpointsResponse
	// url.PathEscape would also escape the "/" separating author and slug,
	// so each segment is escaped individually instead.
	path := "/models/" + escapeSegments(modelID) + "/endpoints"
	if err := c.getJSON(ctx, path, &out); err != nil {
		return nil, fmt.Errorf("endpoints for %s: %w", modelID, err)
	}
	return out.Data.Endpoints, nil
}

func escapeSegments(id string) string {
	parts := strings.Split(id, "/")
	for i, p := range parts {
		parts[i] = url.PathEscape(p)
	}
	return strings.Join(parts, "/")
}

// getJSON performs a GET request and decodes a JSON body into out. A single
// 429 response is retried once after a short backoff.
func (c *Client) getJSON(ctx context.Context, path string, out any) error {
	fullURL := c.baseURL + path
	resp, err := c.doWithRetry(ctx, fullURL)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}

func (c *Client) doWithRetry(ctx context.Context, fullURL string) (*http.Response, error) {
	for attempt := 0; ; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, fullURL, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Accept", "application/json")

		resp, err := c.http.Do(req)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode == http.StatusTooManyRequests && attempt == 0 {
			_ = resp.Body.Close()
			select {
			case <-time.After(2 * time.Second):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
			continue
		}
		return resp, nil
	}
}
