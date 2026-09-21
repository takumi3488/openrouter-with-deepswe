package terminalbench

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"openrouter-with-deepswe/internal/postgres/sqlcgen"
)

type runStore struct {
	models []sqlcgen.Model
	rows   []sqlcgen.UpsertTerminalBenchScoreParams
}

func (s *runStore) ListVisibleModels(context.Context) ([]sqlcgen.Model, error) {
	return s.models, nil
}

func (s *runStore) UpsertTerminalBenchScore(_ context.Context, row sqlcgen.UpsertTerminalBenchScoreParams) error {
	s.rows = append(s.rows, row)
	if row.ReasoningEffort == "low" {
		return errors.New("simulated row failure")
	}
	return nil
}

func TestRunRetainsEveryAgentEffortAndContinuesFailures(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"leaderboard":{"name":"4-0-0"},"rows":[
			{"metadata":{"agent_display":{"label":"Codex"},"model_display":{"label":"GPT-6 Astra"},"model_org":{"label":"OpenAI"},"reasoning_effort":"high"},"metrics":{"accuracy":58.18,"accuracy_ci95_half_width":2.79},"status":"display"},
			{"metadata":{"agent_display":{"label":"Codex"},"model_display":{"label":"GPT-6 Astra"},"model_org":{"label":"OpenAI"},"reasoning_effort":"low"},"metrics":{"accuracy":50.61,"accuracy_ci95_half_width":2.75},"status":"display"}
		],"pagination":{"total":2,"page":1,"page_size":50,"total_pages":1}}`))
	}))
	defer srv.Close()

	store := &runStore{models: []sqlcgen.Model{{ID: "openai/gpt-6-astra", CanonicalSlug: "openai/gpt-6-astra-20260901"}}}
	err := Run(context.Background(), NewClient(srv.URL+"?leaderboard=4-0-0", srv.Client()), store)
	if err == nil {
		t.Fatal("Run() error = nil, want failed row error")
	}
	if len(store.rows) != 2 {
		t.Fatalf("upsert calls = %d, want 2", len(store.rows))
	}
	for _, row := range store.rows {
		if row.Leaderboard != "4-0-0" || row.ModelID != "openai/gpt-6-astra" {
			t.Fatalf("row identity = %+v", row)
		}
	}
}
