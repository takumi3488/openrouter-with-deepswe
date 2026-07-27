package openrouter

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"openrouter-with-deepswe/internal/postgres/sqlcgen"
)

// fakeStore is an in-memory Store for testing Run without a database.
type fakeStore struct {
	mu        sync.Mutex
	favorites []string
	upserts   map[string]sqlcgen.UpsertModelParams
	failIDs   map[string]bool
}

func newFakeStore(favorites []string) *fakeStore {
	return &fakeStore{favorites: favorites, upserts: map[string]sqlcgen.UpsertModelParams{}}
}

func (f *fakeStore) ListFavoriteIDs(ctx context.Context) ([]string, error) {
	return f.favorites, nil
}

func (f *fakeStore) UpsertModel(ctx context.Context, arg sqlcgen.UpsertModelParams) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failIDs[arg.ID] {
		return errUpsertFailed
	}
	f.upserts[arg.ID] = arg
	return nil
}

var errUpsertFailed = &fakeUpsertError{}

type fakeUpsertError struct{}

func (*fakeUpsertError) Error() string { return "fake upsert error" }

func TestRun_SelectsAndUpsertsCandidates(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/models", func(w http.ResponseWriter, r *http.Request) {
		serveFixture(t, w, "testdata/models.json")
	})
	mux.HandleFunc("/models/deepseek/deepseek-v3.2/endpoints", func(w http.ResponseWriter, r *http.Request) {
		serveFixture(t, w, "testdata/endpoints.json")
	})
	// anthropic/claude-opus-5-fast has no dedicated endpoints fixture: fall
	// through to a generic empty-endpoints response so its listed pricing
	// is used as a fallback, exercising that path too.
	mux.HandleFunc("/models/anthropic/claude-opus-5-fast/endpoints", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"endpoints":[]}}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	client := NewClient(srv.URL, srv.Client())
	// deepseek/deepseek-v3.2 was released in 2020 in the fixture; mark it
	// favorite so it is selected despite being old.
	store := newFakeStore([]string{"deepseek/deepseek-v3.2"})

	now := time.Date(2026, 7, 27, 0, 0, 0, 0, time.UTC)
	err := Run(context.Background(), client, store, Options{Now: func() time.Time { return now }})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	store.mu.Lock()
	defer store.mu.Unlock()

	if len(store.upserts) != 2 {
		t.Fatalf("upserts = %v, want 2 entries (claude-opus-5-fast, deepseek-v3.2)", keysOf(store.upserts))
	}

	claude, ok := store.upserts["anthropic/claude-opus-5-fast"]
	if !ok {
		t.Fatal("claude-opus-5-fast was not upserted")
	}
	if claude.CheapestProvider != nil {
		t.Errorf("claude fallback CheapestProvider = %v, want nil", *claude.CheapestProvider)
	}
	if claude.PromptPrice != "0.00001" {
		t.Errorf("claude fallback PromptPrice = %q, want listed price 0.00001", claude.PromptPrice)
	}

	deepseek, ok := store.upserts["deepseek/deepseek-v3.2"]
	if !ok {
		t.Fatal("deepseek-v3.2 was not upserted despite being favorited")
	}
	if deepseek.CheapestProvider == nil || *deepseek.CheapestProvider != "Baidu" {
		t.Errorf("deepseek CheapestProvider = %v, want Baidu", deepseek.CheapestProvider)
	}

	if _, ok := store.upserts["openai/gpt-image-old"]; ok {
		t.Error("gpt-image-old should have been excluded (not text-to-text)")
	}

	if _, ok := store.upserts["openrouter/auto-beta"]; ok {
		t.Error("auto-beta should have been skipped: its \"-1\" listed price is a sentinel, not a real price, and no endpoint fixture backs it")
	}
}

func TestRun_ReturnsErrorWhenUpsertFails(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/models", func(w http.ResponseWriter, r *http.Request) {
		serveFixture(t, w, "testdata/models.json")
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"endpoints":[]}}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	client := NewClient(srv.URL, srv.Client())
	store := newFakeStore(nil)
	store.failIDs = map[string]bool{"anthropic/claude-opus-5-fast": true}

	now := time.Date(2026, 7, 27, 0, 0, 0, 0, time.UTC)
	err := Run(context.Background(), client, store, Options{Now: func() time.Time { return now }})
	if err == nil {
		t.Fatal("Run() error = nil, want non-nil when an upsert fails")
	}
}

func keysOf(m map[string]sqlcgen.UpsertModelParams) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
