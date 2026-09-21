package terminalbench

import "testing"

func TestMatchModelUsesProviderAndClaudePrefix(t *testing.T) {
	index := BuildIndex([]LeaderboardRow{
		{Agent: "Codex", Model: "GPT-6 Astra", Provider: "OpenAI", ReasoningEffort: "high"},
		{Agent: "Claude Code", Model: "Opus 5", Provider: "Anthropic", ReasoningEffort: "max"},
		{Agent: "Other", Model: "GPT-6 Astra", Provider: "Other", ReasoningEffort: "high"},
	})

	rows, ok := MatchModel(index, "openai/gpt-6-astra:batch", "openai/gpt-6-astra-20260901")
	if !ok || len(rows) != 1 || rows[0].Agent != "Codex" {
		t.Fatalf("OpenAI match = %+v, %v", rows, ok)
	}
	rows, ok = MatchModel(index, "anthropic/claude-opus-5", "")
	if !ok || len(rows) != 1 || rows[0].Agent != "Claude Code" {
		t.Fatalf("Anthropic match = %+v, %v", rows, ok)
	}
	rows, ok = MatchModel(index, "other/gpt-6-astra", "")
	if !ok || len(rows) != 1 || rows[0].Agent != "Other" {
		t.Fatalf("Other provider match = %+v, %v", rows, ok)
	}
	if _, ok := MatchModel(index, "unknown/gpt-6-astra", ""); ok {
		t.Fatal("cross-provider model matched, want no match")
	}
}

func TestEffortOrDefault(t *testing.T) {
	if got := EffortOrDefault(""); got != DefaultEffort {
		t.Fatalf("EffortOrDefault(empty) = %q, want %q", got, DefaultEffort)
	}
	if got := EffortOrDefault("high"); got != "high" {
		t.Fatalf("EffortOrDefault(high) = %q, want high", got)
	}
}
