// Package deepswe fetches the DeepSWE benchmark leaderboard and matches its
// rows against OpenRouter model IDs to record per-effort scores.
package deepswe

// LeaderboardRow is a single (model, harness, reasoning effort) row from the
// DeepSWE live leaderboard artifact.
type LeaderboardRow struct {
	// Model is DeepSWE's own slug, without any provider prefix (e.g.
	// "claude-opus-5"), not an OpenRouter model ID.
	Model   string `json:"model"`
	Harness string `json:"harness"`
	// ReasoningEffort is empty when DeepSWE recorded no effort level for
	// this row (single-effort models).
	ReasoningEffort string  `json:"reasoning_effort"`
	PassRate        float64 `json:"pass_rate"`
	PassAt1         float64 `json:"pass_at_1"`
	PassAt4         float64 `json:"pass_at_4"`
	NPassed         int64   `json:"n_passed"`
	NAttempted      int64   `json:"n_attempted"`
	MeanCostUsd     float64 `json:"mean_cost_usd"`
}

type leaderboardResponse struct {
	Rows []LeaderboardRow `json:"rows"`
}
