// Package terminalbench fetches Terminal-Bench leaderboard rows from Harbor and
// records their per-agent, per-effort scores against visible OpenRouter models.
package terminalbench

// LeaderboardRow is one visible Terminal-Bench result. Accuracy values are
// percentages (0..100), not fractions.
type LeaderboardRow struct {
	Agent                 string
	Model                 string
	Provider              string
	ReasoningEffort       string
	Accuracy              float64
	AccuracyCi95HalfWidth float64
}
