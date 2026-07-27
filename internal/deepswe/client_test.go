package deepswe

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestClient_Leaderboard(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, err := os.ReadFile("testdata/leaderboard.json")
		if err != nil {
			t.Fatalf("read fixture: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(data)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, srv.Client())
	rows, err := client.Leaderboard(context.Background())
	if err != nil {
		t.Fatalf("Leaderboard() error = %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("Leaderboard() returned %d rows, want 3", len(rows))
	}
	if rows[0].Model != "claude-opus-5" || rows[0].ReasoningEffort != "max" {
		t.Errorf("rows[0] = %+v", rows[0])
	}
}

func TestClient_Leaderboard_NonOKStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, srv.Client())
	if _, err := client.Leaderboard(context.Background()); err == nil {
		t.Fatal("Leaderboard() error = nil, want non-nil")
	}
}

func TestClient_Leaderboard_InvalidJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("{not json"))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, srv.Client())
	if _, err := client.Leaderboard(context.Background()); err == nil {
		t.Fatal("Leaderboard() error = nil, want non-nil")
	}
}
