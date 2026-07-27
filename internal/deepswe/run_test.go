package deepswe

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"

	"openrouter-with-deepswe/internal/postgres/sqlcgen"
)

type fakeStore struct {
	mu      sync.Mutex
	targets []sqlcgen.Model
	scores  []sqlcgen.UpsertDeepsweScoreParams
	failOn  string // ModelID that always fails to upsert
}

func (f *fakeStore) ListModelsWithoutScores(ctx context.Context) ([]sqlcgen.Model, error) {
	return f.targets, nil
}

func (f *fakeStore) UpsertDeepsweScore(ctx context.Context, arg sqlcgen.UpsertDeepsweScoreParams) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failOn != "" && arg.ModelID == f.failOn {
		return errors.New("fake upsert error")
	}
	f.scores = append(f.scores, arg)
	return nil
}

func leaderboardServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, err := os.ReadFile("testdata/leaderboard.json")
		if err != nil {
			t.Fatalf("read fixture: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(data)
	}))
}

func TestRun_MatchesAndUpsertsAllEffortRows(t *testing.T) {
	srv := leaderboardServer(t)
	defer srv.Close()

	store := &fakeStore{
		targets: []sqlcgen.Model{
			{ID: "anthropic/claude-opus-5", CanonicalSlug: "anthropic/claude-opus-5-20260101"},
			{ID: "vendor/unknown-model", CanonicalSlug: "vendor/unknown-model"},
		},
	}
	client := NewClient(srv.URL, srv.Client())

	if err := Run(context.Background(), client, store); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	store.mu.Lock()
	defer store.mu.Unlock()

	if len(store.scores) != 2 {
		t.Fatalf("upserted %d scores, want 2 (max + high for claude-opus-5)", len(store.scores))
	}
	for _, s := range store.scores {
		if s.ModelID != "anthropic/claude-opus-5" {
			t.Errorf("unexpected score for model %q", s.ModelID)
		}
	}
}

func TestRun_NoTargetsIsNoop(t *testing.T) {
	store := &fakeStore{}
	// No HTTP server: Run must return before ever calling the client if
	// there are no targets.
	client := NewClient("http://127.0.0.1:0/unreachable", http.DefaultClient)

	if err := Run(context.Background(), client, store); err != nil {
		t.Fatalf("Run() error = %v, want nil (no targets, should short-circuit)", err)
	}
}

func TestRun_EmptyReasoningEffortMapsToDefault(t *testing.T) {
	srv := leaderboardServer(t)
	defer srv.Close()

	store := &fakeStore{
		targets: []sqlcgen.Model{
			{ID: "moonshot/kimi-k2-7-code", CanonicalSlug: "moonshot/kimi-k2-7-code"},
		},
	}
	client := NewClient(srv.URL, srv.Client())

	if err := Run(context.Background(), client, store); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	store.mu.Lock()
	defer store.mu.Unlock()
	if len(store.scores) != 1 {
		t.Fatalf("upserted %d scores, want 1", len(store.scores))
	}
	if store.scores[0].ReasoningEffort != DefaultEffort {
		t.Errorf("ReasoningEffort = %q, want %q", store.scores[0].ReasoningEffort, DefaultEffort)
	}
}

func TestRun_ReturnsErrorWhenUpsertFails(t *testing.T) {
	srv := leaderboardServer(t)
	defer srv.Close()

	store := &fakeStore{
		targets: []sqlcgen.Model{
			{ID: "anthropic/claude-opus-5", CanonicalSlug: "anthropic/claude-opus-5"},
		},
		failOn: "anthropic/claude-opus-5",
	}
	client := NewClient(srv.URL, srv.Client())

	if err := Run(context.Background(), client, store); err == nil {
		t.Fatal("Run() error = nil, want non-nil when upserts fail")
	}
}
