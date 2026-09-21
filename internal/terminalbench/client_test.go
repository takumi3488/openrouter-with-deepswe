package terminalbench

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientLeaderboardPaginatesAndSkipsHiddenRows(t *testing.T) {
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		var body struct {
			Name string `json:"name"`
			Page int    `json:"page"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if body.Name != "4-0-0" {
			t.Errorf("name = %q, want 4-0-0", body.Name)
		}
		row := jsonRowMap("Codex", "GPT-6 Astra", "OpenAI", "high", 58.18, 2.79, "display")
		if body.Page == 2 {
			row = jsonRowMap("Codex", "GPT-6 Astra", "OpenAI", "max", 57.88, 2.8, "display")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"leaderboard": map[string]any{"name": "4-0-0"},
			"rows":        []map[string]any{row},
			"pagination":  map[string]any{"total": 2, "page": body.Page, "page_size": 1, "total_pages": 2},
		})
	}))
	defer srv.Close()

	rows, err := NewClient(srv.URL+"?leaderboard=4-0-0", srv.Client()).Leaderboard(context.Background())
	if err != nil {
		t.Fatalf("Leaderboard() error = %v", err)
	}
	if len(rows) != 2 || rows[1].ReasoningEffort != "max" {
		t.Fatalf("rows = %+v, want both paginated rows", rows)
	}
	if requests != 2 {
		t.Fatalf("requests = %d, want 2", requests)
	}

	visible, _, err := parseRows(map[string]any{
		"leaderboard": map[string]any{"name": "4-0-0"},
		"rows": []any{
			jsonRowMap("Codex", "GPT-6 Astra", "OpenAI", "high", 58.18, 2.79, "display"),
			jsonRowMap("hidden", "GPT-6 Astra", "OpenAI", "high", 1, 1, "hide"),
		},
		"pagination": map[string]any{"total": 1, "page": 1, "page_size": 50, "total_pages": 1},
	}, "4-0-0", 1)
	if err != nil {
		t.Fatalf("parseRows() error = %v", err)
	}
	if len(visible) != 1 {
		t.Fatalf("visible rows = %d, want 1", len(visible))
	}
}

func TestParseRowsRejectsMissingMetric(t *testing.T) {
	row := jsonRowMap("Codex", "GPT-6 Astra", "OpenAI", "high", 58.18, 2.79, "display")
	delete(row["metrics"].(map[string]any), "accuracy_ci95_half_width")
	if _, _, err := parseRows(map[string]any{
		"leaderboard": map[string]any{"name": "4-0-0"},
		"rows":        []any{row},
		"pagination":  map[string]any{"total": 1, "page": 1, "page_size": 50, "total_pages": 1},
	}, "4-0-0", 1); err == nil {
		t.Fatal("parseRows() error = nil, want missing metric error")
	}
}
func TestParseRowsRejectsMissingPagination(t *testing.T) {
	payload := map[string]any{
		"leaderboard": map[string]any{"name": "4-0-0"},
		"rows":        []any{},
	}
	if _, _, err := parseRows(payload, "4-0-0", 1); err == nil {
		t.Fatal("parseRows() error = nil, want missing pagination error")
	}
}

func jsonRowMap(agent, model, provider, effort string, accuracy, ci float64, status string) map[string]any {
	return map[string]any{
		"metadata": map[string]any{
			"agent_display":    map[string]any{"label": agent},
			"model_display":    map[string]any{"label": model},
			"model_org":        map[string]any{"label": provider},
			"reasoning_effort": effort,
		},
		"metrics": map[string]any{"accuracy": accuracy, "accuracy_ci95_half_width": ci},
		"status":  status,
	}
}
