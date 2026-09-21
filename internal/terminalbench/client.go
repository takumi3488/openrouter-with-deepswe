package terminalbench

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strings"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

const (
	defaultHarborPageURL = "https://hub.harborframework.com/datasets/terminal-bench/terminal-bench/latest?leaderboard=4-0-0&tab=leaderboard"
	harborSupabaseURL    = "https://ofhuhcpkvzjlejydnvyd.supabase.co/functions/v1/leaderboard-read"
	harborPublishableKey = "sb_publishable_Z-vuQbpvpG-PStjbh4yE0Q_e-d3MTIH"
	packageSelector      = "terminal-bench/terminal-bench"
	defaultLeaderboard   = "4-0-0"
)

// Client reads public Harbor leaderboard rows without a browser or JavaScript
// runtime. The configured URL supplies the selected leaderboard name; requests
// use Harbor's stable public leaderboard-read edge function.
type Client struct {
	url  string
	http *http.Client
}

// NewClient builds a Harbor client. A nil HTTP client gets a 30-second client
// with OpenTelemetry instrumentation. A custom URL is useful for tests and may
// point directly at an edge-function-compatible endpoint.
func NewClient(leaderboardURL string, httpClient *http.Client) *Client {
	if leaderboardURL == "" {
		leaderboardURL = defaultHarborPageURL
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second, Transport: otelhttp.NewTransport(http.DefaultTransport)}
	}
	return &Client{url: leaderboardURL, http: httpClient}
}

// Leaderboard fetches all visible rows for the selected Harbor leaderboard.
// The edge function returns canonical pages; every page is retained.
func (c *Client) Leaderboard(ctx context.Context) ([]LeaderboardRow, error) {
	name, endpoint := c.selector()
	var rows []LeaderboardRow
	for page := 1; ; page++ {
		batch, totalPages, err := c.page(ctx, endpoint, name, page)
		if err != nil {
			return nil, err
		}
		rows = append(rows, batch...)
		if page >= totalPages {
			break
		}
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("terminalbench: Harbor returned no visible rows for leaderboard %q", name)
	}
	return rows, nil
}

func (c *Client) selector() (name, endpoint string) {
	u, err := url.Parse(c.url)
	if err != nil {
		return defaultLeaderboard, c.url
	}
	name = u.Query().Get("leaderboard")
	if name == "" {
		name = defaultLeaderboard
	}
	// The production config is the Hub page URL. Tests and operators may point
	// directly at a compatible edge function endpoint.
	if u.Hostname() == "hub.harborframework.com" {
		return name, harborSupabaseURL
	}
	return name, c.url
}

func (c *Client) page(ctx context.Context, endpoint, name string, page int) ([]LeaderboardRow, int, error) {
	body, err := json.Marshal(map[string]any{
		"package":   packageSelector,
		"name":      name,
		"page":      page,
		"page_size": 50,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("terminalbench: encode Harbor request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, 0, fmt.Errorf("terminalbench: build Harbor request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	if endpoint == harborSupabaseURL {
		req.Header.Set("apikey", harborPublishableKey)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("terminalbench: fetch Harbor leaderboard page %d: %w", page, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		message, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return nil, 0, fmt.Errorf("terminalbench: Harbor leaderboard page %d returned %s: %s", page, resp.Status, strings.TrimSpace(string(message)))
	}
	payload, err := decodePayload(resp.Body)
	if err != nil {
		return nil, 0, fmt.Errorf("terminalbench: decode Harbor leaderboard page %d: %w", page, err)
	}
	return parseRows(payload, name, page)
}

func decodePayload(r io.Reader) (any, error) {
	data, err := io.ReadAll(io.LimitReader(r, 32<<20))
	if err != nil {
		return nil, err
	}
	var payload any
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, fmt.Errorf("response is not Harbor JSON: %w", err)
	}
	return payload, nil
}
func parseRows(payload any, expectedName string, requestedPage int) ([]LeaderboardRow, int, error) {
	m, ok := payload.(map[string]any)
	if !ok {
		return nil, 0, errors.New("harbor payload is not an object")
	}
	leaderboard, ok := m["leaderboard"].(map[string]any)
	if !ok {
		return nil, 0, errors.New("harbor payload has no leaderboard object")
	}
	name, ok := leaderboard["name"].(string)
	if !ok || name == "" {
		return nil, 0, errors.New("harbor leaderboard has no name")
	}
	if name != expectedName {
		return nil, 0, fmt.Errorf("harbor returned leaderboard %q, want %q", name, expectedName)
	}
	rowsValue, ok := m["rows"]
	if !ok {
		return nil, 0, errors.New("harbor payload has no rows array")
	}
	rows, ok := rowsValue.([]any)
	if !ok {
		return nil, 0, errors.New("harbor payload rows is not an array")
	}
	pagination, ok := m["pagination"].(map[string]any)
	if !ok {
		return nil, 0, errors.New("harbor payload has no pagination object")
	}
	total, ok := numberAsInt(pagination["total"])
	if !ok || total < 0 {
		return nil, 0, errors.New("harbor pagination has invalid total")
	}
	page, ok := numberAsInt(pagination["page"])
	if !ok || page != requestedPage {
		return nil, 0, fmt.Errorf("harbor pagination page %d does not match request page %d", page, requestedPage)
	}
	pageSize, ok := numberAsInt(pagination["page_size"])
	if !ok || pageSize < 1 {
		return nil, 0, errors.New("harbor pagination has invalid page_size")
	}
	totalPages, ok := numberAsInt(pagination["total_pages"])
	if !ok || totalPages < 1 {
		return nil, 0, errors.New("harbor pagination has invalid total_pages")
	}
	out := make([]LeaderboardRow, 0, len(rows))
	for i, raw := range rows {
		row, ok := raw.(map[string]any)
		if !ok {
			return nil, 0, fmt.Errorf("harbor row %d is not an object", i)
		}
		statusValue, present := row["status"]
		status := ""
		if present {
			var ok bool
			status, ok = statusValue.(string)
			if !ok {
				return nil, 0, fmt.Errorf("harbor row %d has non-string status", i)
			}
		}
		switch status {
		case "hide", "hidden":
			continue
		case "", "display":
		default:
			return nil, 0, fmt.Errorf("harbor row %d has unexpected status %q", i, status)
		}
		parsed, err := parseRow(row)
		if err != nil {
			return nil, 0, fmt.Errorf("harbor row %d: %w", i, err)
		}
		out = append(out, parsed)
	}
	return out, totalPages, nil
}

func parseRow(row map[string]any) (LeaderboardRow, error) {
	metadata, ok := row["metadata"].(map[string]any)
	if !ok {
		return LeaderboardRow{}, errors.New("missing metadata")
	}
	metrics, ok := row["metrics"].(map[string]any)
	if !ok {
		return LeaderboardRow{}, errors.New("missing metrics")
	}
	agent := nestedLabel(metadata, "agent_display")
	model := nestedLabel(metadata, "model_display")
	provider := nestedLabel(metadata, "model_org")
	if agent == "" || model == "" || provider == "" {
		return LeaderboardRow{}, errors.New("missing agent, model, or model organization label")
	}
	accuracy, ok := metrics["accuracy"].(float64)
	if !ok || math.IsNaN(accuracy) || math.IsInf(accuracy, 0) || accuracy < 0 || accuracy > 100 {
		return LeaderboardRow{}, errors.New("invalid metrics.accuracy")
	}
	ci, ok := metrics["accuracy_ci95_half_width"].(float64)
	if !ok || math.IsNaN(ci) || math.IsInf(ci, 0) || ci < 0 || ci > 100 {
		return LeaderboardRow{}, errors.New("invalid metrics.accuracy_ci95_half_width")
	}
	effort, ok := metadata["reasoning_effort"].(string)
	if !ok {
		return LeaderboardRow{}, errors.New("missing metadata.reasoning_effort")
	}
	return LeaderboardRow{
		Agent:                 agent,
		Model:                 model,
		Provider:              provider,
		ReasoningEffort:       effort,
		Accuracy:              accuracy,
		AccuracyCi95HalfWidth: ci,
	}, nil
}

func nestedLabel(metadata map[string]any, key string) string {
	value, ok := metadata[key].(map[string]any)
	if !ok {
		return ""
	}
	label, _ := value["label"].(string)
	return strings.TrimSpace(label)
}

func numberAsInt(value any) (int, bool) {
	switch n := value.(type) {
	case float64:
		if math.Trunc(n) != n || n > float64(int(^uint(0)>>1)) || n < float64(-int(^uint(0)>>1)-1) {
			return 0, false
		}
		return int(n), true
	case int:
		return n, true
	default:
		return 0, false
	}
}
