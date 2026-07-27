package deepswe

import (
	"context"
	"fmt"
	"log/slog"

	"openrouter-with-deepswe/internal/postgres/sqlcgen"
)

// Store is the subset of sqlcgen.Queries that Run needs.
type Store interface {
	ListModelsWithoutScores(ctx context.Context) ([]sqlcgen.Model, error)
	UpsertDeepsweScore(ctx context.Context, arg sqlcgen.UpsertDeepsweScoreParams) error
}

// Run finds visible models with no recorded DeepSWE score, matches them
// against the live leaderboard, and upserts every (harness, reasoning
// effort) row found for each match. Models with no leaderboard entry are
// simply logged and skipped: since the target query is "no score yet",
// they are automatically retried the next time Run executes, once DeepSWE
// publishes results for them.
func Run(ctx context.Context, client *Client, store Store) error {
	targets, err := store.ListModelsWithoutScores(ctx)
	if err != nil {
		return fmt.Errorf("deepswe: list models without scores: %w", err)
	}
	if len(targets) == 0 {
		slog.InfoContext(ctx, "no models pending a DeepSWE score")
		return nil
	}

	rows, err := client.Leaderboard(ctx)
	if err != nil {
		return fmt.Errorf("deepswe: fetch leaderboard: %w", err)
	}
	index := BuildIndex(rows)

	var failed int
	var unmatched int
	for _, m := range targets {
		matched, ok := MatchModel(index, m.ID, m.CanonicalSlug)
		if !ok {
			unmatched++
			slog.InfoContext(ctx, "model not found on DeepSWE leaderboard, will retry later", "model_id", m.ID)
			continue
		}
		for _, row := range matched {
			err := store.UpsertDeepsweScore(ctx, sqlcgen.UpsertDeepsweScoreParams{
				ModelID:         m.ID,
				Harness:         row.Harness,
				ReasoningEffort: EffortOrDefault(row.ReasoningEffort),
				PassRate:        ptr(row.PassRate),
				PassAt1:         ptr(row.PassAt1),
				PassAt4:         ptr(row.PassAt4),
				NPassed:         ptr(row.NPassed),
				NAttempted:      ptr(row.NAttempted),
				MeanCostUsd:     ptr(row.MeanCostUsd),
			})
			if err != nil {
				failed++
				slog.ErrorContext(ctx, "upsert deepswe score failed",
					"model_id", m.ID, "harness", row.Harness, "reasoning_effort", row.ReasoningEffort, "error", err)
			}
		}
	}

	slog.InfoContext(ctx, "deepswe score sync complete",
		"targets", len(targets), "unmatched", unmatched, "failed", failed)

	if failed > 0 {
		return fmt.Errorf("deepswe: %d score row(s) failed to upsert", failed)
	}
	return nil
}

func ptr[T any](v T) *T { return &v }
