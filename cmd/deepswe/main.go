// Command deepswe matches visible models that have no recorded DeepSWE
// score against the live DeepSWE leaderboard, and upserts every
// (harness, reasoning effort) row found for each match. It is meant to run
// as a one-shot batch (e.g. on a schedule).
package main

import (
	"context"
	"log/slog"
	"os"

	"openrouter-with-deepswe/internal/config"
	"openrouter-with-deepswe/internal/deepswe"
	"openrouter-with-deepswe/internal/postgres"
	"openrouter-with-deepswe/internal/telemetry"
)

func main() {
	if err := run(); err != nil {
		slog.Error("deepswe: fatal", "error", err)
		os.Exit(1)
	}
}

func run() error {
	ctx := context.Background()

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	shutdown, err := telemetry.Setup(ctx, "deepswe", cfg.OTLPEndpoint)
	if err != nil {
		return err
	}
	defer func() {
		if err := shutdown(ctx); err != nil {
			slog.Warn("deepswe: telemetry shutdown", "error", err)
		}
	}()

	tracer := telemetry.Tracer("openrouter-with-deepswe/cmd/deepswe")
	ctx, span := tracer.Start(ctx, "deepswe.Run")
	defer span.End()

	pool, queries, err := postgres.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	client := deepswe.NewClient(cfg.DeepSWELeaderboardURL, nil)

	return deepswe.Run(ctx, client, queries)
}
