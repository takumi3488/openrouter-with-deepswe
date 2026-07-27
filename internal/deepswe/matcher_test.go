package deepswe

import "testing"

func TestOrSlug(t *testing.T) {
	cases := []struct{ id, want string }{
		{"anthropic/claude-opus-5", "claude-opus-5"},
		{"deepseek/deepseek-r1:free", "deepseek-r1"},
		{"deepseek/deepseek-r1:extended", "deepseek-r1"},
		{"no-provider-prefix", "no-provider-prefix"},
		{"vendor/sub/model", "model"},
	}
	for _, c := range cases {
		if got := orSlug(c.id); got != c.want {
			t.Errorf("orSlug(%q) = %q, want %q", c.id, got, c.want)
		}
	}
}

func TestNormalize(t *testing.T) {
	cases := []struct{ in, want string }{
		{"claude-3.5-sonnet", "claude-3-5-sonnet"},
		{"Claude-Opus-5", "claude-opus-5"},
		{"gpt-5.6-sol", "gpt-5-6-sol"},
	}
	for _, c := range cases {
		if got := normalize(c.in); got != c.want {
			t.Errorf("normalize(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestMatchModel(t *testing.T) {
	rows := []LeaderboardRow{
		{Model: "claude-opus-5", Harness: "mini-swe-agent", ReasoningEffort: "max"},
		{Model: "claude-opus-5", Harness: "mini-swe-agent", ReasoningEffort: "high"},
		{Model: "claude-3-5-sonnet", Harness: "mini-swe-agent", ReasoningEffort: ""},
	}
	index := BuildIndex(rows)

	t.Run("matches by id slug", func(t *testing.T) {
		got, ok := MatchModel(index, "anthropic/claude-opus-5", "anthropic/claude-opus-5-20260101")
		if !ok {
			t.Fatal("MatchModel() ok = false, want true")
		}
		if len(got) != 2 {
			t.Errorf("MatchModel() returned %d rows, want 2", len(got))
		}
	})

	t.Run("falls back to canonical slug, with dot normalization", func(t *testing.T) {
		got, ok := MatchModel(index, "anthropic/claude-3.5-sonnet-fast", "anthropic/claude-3.5-sonnet")
		if !ok {
			t.Fatal("MatchModel() ok = false, want true")
		}
		if len(got) != 1 {
			t.Errorf("MatchModel() returned %d rows, want 1", len(got))
		}
	})

	t.Run("no match", func(t *testing.T) {
		if _, ok := MatchModel(index, "vendor/unknown-model", "vendor/unknown-model"); ok {
			t.Error("MatchModel() ok = true, want false")
		}
	})

	t.Run("variant suffix is stripped before matching", func(t *testing.T) {
		got, ok := MatchModel(index, "anthropic/claude-opus-5:free", "anthropic/claude-opus-5-20260101")
		if !ok || len(got) != 2 {
			t.Errorf("MatchModel() = %v, %v, want 2 rows matched", got, ok)
		}
	})
}

func TestEffortOrDefault(t *testing.T) {
	if got := EffortOrDefault(""); got != DefaultEffort {
		t.Errorf("EffortOrDefault(\"\") = %q, want %q", got, DefaultEffort)
	}
	if got := EffortOrDefault("high"); got != "high" {
		t.Errorf("EffortOrDefault(\"high\") = %q, want \"high\"", got)
	}
}
