package openrouter

import (
	"testing"
	"time"
)

func textToTextModel() Model {
	return Model{
		ID: "vendor/text-model",
		Architecture: Architecture{
			InputModalities:  []string{"text"},
			OutputModalities: []string{"text"},
		},
	}
}

func TestIsTextToText(t *testing.T) {
	cases := []struct {
		name string
		arch Architecture
		want bool
	}{
		{"text only", Architecture{InputModalities: []string{"text"}, OutputModalities: []string{"text"}}, true},
		{"text+image input, text output", Architecture{InputModalities: []string{"text", "image"}, OutputModalities: []string{"text"}}, true},
		{"text input, image output", Architecture{InputModalities: []string{"text"}, OutputModalities: []string{"image"}}, false},
		{"image input only", Architecture{InputModalities: []string{"image"}, OutputModalities: []string{"text"}}, false},
		{"no modalities", Architecture{}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := Model{Architecture: c.arch}
			if got := IsTextToText(m); got != c.want {
				t.Errorf("IsTextToText(%+v) = %v, want %v", c.arch, got, c.want)
			}
		})
	}
}

func TestIsRecentOrFavorite(t *testing.T) {
	now := time.Date(2026, 7, 27, 0, 0, 0, 0, time.UTC)
	cutoff := now.AddDate(0, -1, 0)

	cases := []struct {
		name     string
		created  time.Time
		favorite bool
		want     bool
	}{
		{"released yesterday, not favorite", now.AddDate(0, 0, -1), false, true},
		{"released exactly at cutoff boundary (not after), not favorite", cutoff, false, false},
		{"released one day before cutoff, not favorite", cutoff.AddDate(0, 0, -1), false, false},
		{"released long ago, favorite", now.AddDate(-2, 0, 0), true, true},
		{"released long ago, not favorite", now.AddDate(-2, 0, 0), false, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := Model{ID: "m", Created: c.created.Unix()}
			favorites := map[string]bool{}
			if c.favorite {
				favorites["m"] = true
			}
			if got := IsRecentOrFavorite(m, now, favorites); got != c.want {
				t.Errorf("IsRecentOrFavorite() = %v, want %v", got, c.want)
			}
		})
	}
}

func TestSelectCandidates(t *testing.T) {
	now := time.Date(2026, 7, 27, 0, 0, 0, 0, time.UTC)

	recentT2T := textToTextModel()
	recentT2T.ID = "recent-t2t"
	recentT2T.Created = now.AddDate(0, 0, -1).Unix()

	oldT2TFavorite := textToTextModel()
	oldT2TFavorite.ID = "old-t2t-favorite"
	oldT2TFavorite.Created = now.AddDate(-1, 0, 0).Unix()

	oldT2TNotFavorite := textToTextModel()
	oldT2TNotFavorite.ID = "old-t2t-not-favorite"
	oldT2TNotFavorite.Created = now.AddDate(-1, 0, 0).Unix()

	recentNonT2T := Model{
		ID:      "recent-non-t2t",
		Created: now.AddDate(0, 0, -1).Unix(),
		Architecture: Architecture{
			InputModalities:  []string{"text"},
			OutputModalities: []string{"image"},
		},
	}

	models := []Model{recentT2T, oldT2TFavorite, oldT2TNotFavorite, recentNonT2T}
	favorites := map[string]bool{"old-t2t-favorite": true}

	got := SelectCandidates(models, now, favorites)

	gotIDs := make(map[string]bool, len(got))
	for _, m := range got {
		gotIDs[m.ID] = true
	}

	want := map[string]bool{"recent-t2t": true, "old-t2t-favorite": true}
	if len(gotIDs) != len(want) {
		t.Fatalf("SelectCandidates() = %v, want %v", gotIDs, want)
	}
	for id := range want {
		if !gotIDs[id] {
			t.Errorf("SelectCandidates() missing expected id %q, got %v", id, gotIDs)
		}
	}
}

func TestCheapestEndpoint(t *testing.T) {
	eps := []Endpoint{
		{ProviderName: "Baidu", Pricing: Pricing{Prompt: "0.0000002072", Completion: "0.0000003108"}, Status: 0},
		{ProviderName: "SiliconFlow-degraded", Pricing: Pricing{Prompt: "0.000000259", Completion: "0.00000042"}, Status: -2},
		{ProviderName: "Broken-price", Pricing: Pricing{Prompt: "not-a-number", Completion: "0.0000001"}, Status: 0},
		{ProviderName: "DeepInfra", Pricing: Pricing{Prompt: "0.00000026", Completion: "0.00000038"}, Status: 0},
	}

	best, ok := CheapestEndpoint(eps, 3, 1)
	if !ok {
		t.Fatal("CheapestEndpoint() ok = false, want true")
	}
	if best.ProviderName != "Baidu" {
		t.Errorf("CheapestEndpoint() = %q, want Baidu", best.ProviderName)
	}
}

func TestCheapestEndpoint_NoValidEndpoints(t *testing.T) {
	eps := []Endpoint{
		{ProviderName: "Degraded", Status: -1, Pricing: Pricing{Prompt: "0.001", Completion: "0.001"}},
		{ProviderName: "Unparseable", Status: 0, Pricing: Pricing{Prompt: "nan", Completion: "0.001"}},
		{ProviderName: "Negative", Status: 0, Pricing: Pricing{Prompt: "-1", Completion: "0.001"}},
	}
	if _, ok := CheapestEndpoint(eps, 3, 1); ok {
		t.Error("CheapestEndpoint() ok = true, want false")
	}
}

func TestCheapestEndpoint_WeightsChooseDifferentWinner(t *testing.T) {
	// A is cheap on input, expensive on output; B is the reverse.
	eps := []Endpoint{
		{ProviderName: "A", Pricing: Pricing{Prompt: "1", Completion: "10"}, Status: 0},
		{ProviderName: "B", Pricing: Pricing{Prompt: "10", Completion: "1"}, Status: 0},
	}

	// Weighting input heavily (3:1) favors A, which has the cheap input price.
	if best, _ := CheapestEndpoint(eps, 3, 1); best.ProviderName != "A" {
		t.Errorf("weighted toward input: got %q, want A", best.ProviderName)
	}
	// Weighting output heavily (1:3) favors B, which has the cheap output price.
	if best, _ := CheapestEndpoint(eps, 1, 3); best.ProviderName != "B" {
		t.Errorf("weighted toward output: got %q, want B", best.ProviderName)
	}
}
