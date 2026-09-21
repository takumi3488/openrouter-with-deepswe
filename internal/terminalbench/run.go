package terminalbench

import (
	"context"
	"fmt"
	"log/slog"

	"openrouter-with-deepswe/internal/postgres/sqlcgen"
)

// Store is the persistence contract needed by Run.
type Store interface {
	ListVisibleModels(context.Context) ([]sqlcgen.Model, error)
	UpsertTerminalBenchScore(context.Context, sqlcgen.UpsertTerminalBenchScoreParams) error
}

// Run fetches the selected leaderboard once, matches every visible model, and
// upserts every matching (agent, reasoning effort) row. A bad database row does
// not prevent other rows from being attempted; the run reports failures after
// processing all targets.
func Run(ctx context.Context, client *Client, store Store) error {
	models, err := store.ListVisibleModels(ctx)
	if err != nil {
		return fmt.Errorf("terminalbench: list visible models: %w", err)
	}
	if len(models) == 0 {
		slog.InfoContext(ctx, "no visible models available for Terminal-Bench scores")
		return nil
	}

	rows, err := client.Leaderboard(ctx)
	if err != nil {
		return fmt.Errorf("terminalbench: fetch leaderboard: %w", err)
	}
	index := BuildIndex(rows)
	leaderboard, _ := client.selector()

	var unmatched, failed int
	for _, model := range models {
		matched, ok := MatchModel(index, model.ID, model.CanonicalSlug)
		if !ok {
			unmatched++
			slog.InfoContext(ctx, "model not found on Terminal-Bench leaderboard", "model_id", model.ID)
			continue
		}
		for _, row := range matched {
			if err := store.UpsertTerminalBenchScore(ctx, sqlcgen.UpsertTerminalBenchScoreParams{
				ModelID:               model.ID,
				Leaderboard:           leaderboard,
				Agent:                 row.Agent,
				ReasoningEffort:       EffortOrDefault(row.ReasoningEffort),
				Accuracy:              row.Accuracy,
				AccuracyCi95HalfWidth: row.AccuracyCi95HalfWidth,
			}); err != nil {
				failed++
				slog.ErrorContext(ctx, "upsert Terminal-Bench score failed", "model_id", model.ID, "agent", row.Agent, "reasoning_effort", row.ReasoningEffort, "error", err)
			}
		}
	}

	slog.InfoContext(ctx, "Terminal-Bench score sync complete", "targets", len(models), "unmatched", unmatched, "failed", failed)
	if failed > 0 {
		return fmt.Errorf("terminalbench: %d score row(s) failed to upsert", failed)
	}
	return nil
}
