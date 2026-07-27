package openrouter

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
)

func TestClient_ListModels(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		serveFixture(t, w, "testdata/models.json")
	}))
	defer srv.Close()

	client := NewClient(srv.URL, nil)
	models, err := client.ListModels(context.Background())
	if err != nil {
		t.Fatalf("ListModels() error = %v", err)
	}
	if len(models) != 4 {
		t.Fatalf("ListModels() returned %d models, want 4", len(models))
	}
	if models[0].ID != "anthropic/claude-opus-5-fast" {
		t.Errorf("models[0].ID = %q", models[0].ID)
	}
	if models[0].Pricing.Prompt != "0.00001" {
		t.Errorf("models[0].Pricing.Prompt = %q", models[0].Pricing.Prompt)
	}
}

func TestClient_Endpoints(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		serveFixture(t, w, "testdata/endpoints.json")
	}))
	defer srv.Close()

	client := NewClient(srv.URL, nil)
	eps, err := client.Endpoints(context.Background(), "deepseek/deepseek-v3.2")
	if err != nil {
		t.Fatalf("Endpoints() error = %v", err)
	}
	if want := "/models/deepseek/deepseek-v3.2/endpoints"; gotPath != want {
		t.Errorf("requested path = %q, want %q", gotPath, want)
	}
	if len(eps) != 4 {
		t.Fatalf("Endpoints() returned %d endpoints, want 4", len(eps))
	}
}

func TestClient_Endpoints_VariantSuffixPreserved(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		serveFixture(t, w, "testdata/endpoints.json")
	}))
	defer srv.Close()

	client := NewClient(srv.URL, nil)
	if _, err := client.Endpoints(context.Background(), "deepseek/deepseek-r1:free"); err != nil {
		t.Fatalf("Endpoints() error = %v", err)
	}
	if want := "/models/deepseek/deepseek-r1:free/endpoints"; gotPath != want {
		t.Errorf("requested path = %q, want %q", gotPath, want)
	}
}

func TestClient_ListModels_NonOKStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("boom"))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, nil)
	if _, err := client.ListModels(context.Background()); err == nil {
		t.Fatal("ListModels() error = nil, want non-nil")
	}
}

func TestClient_ListModels_InvalidJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("{not json"))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, nil)
	if _, err := client.ListModels(context.Background()); err == nil {
		t.Fatal("ListModels() error = nil, want non-nil")
	}
}

func TestClient_RetriesOnce429(t *testing.T) {
	var attempts atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if attempts.Add(1) == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		serveFixture(t, w, "testdata/models.json")
	}))
	defer srv.Close()

	client := NewClient(srv.URL, srv.Client())
	models, err := client.ListModels(context.Background())
	if err != nil {
		t.Fatalf("ListModels() error = %v", err)
	}
	if len(models) != 4 {
		t.Fatalf("ListModels() returned %d models, want 4", len(models))
	}
	if got := attempts.Load(); got != 2 {
		t.Errorf("attempts = %d, want 2", got)
	}
}

func TestClient_DoesNotRetryTwice(t *testing.T) {
	var attempts atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, srv.Client())
	if _, err := client.ListModels(context.Background()); err == nil {
		t.Fatal("ListModels() error = nil, want non-nil")
	}
	if got := attempts.Load(); got != 2 {
		t.Errorf("attempts = %d, want 2 (1 initial + 1 retry, no more)", got)
	}
}

func serveFixture(t *testing.T, w http.ResponseWriter, path string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture %s: %v", path, err)
	}
	w.Header().Set("Content-Type", "application/json")
	if _, err := w.Write(data); err != nil {
		t.Fatalf("write fixture %s: %v", path, err)
	}
}
