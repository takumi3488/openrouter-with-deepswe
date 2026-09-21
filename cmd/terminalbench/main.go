// Command terminalbench matches visible OpenRouter models against the public
// Terminal-Bench 4.0 leaderboard and stores every agent/effort score row.
package main

import (
	"context"
	"log/slog"
	"os"

	"openrouter-with-deepswe/internal/config"
	"openrouter-with-deepswe/internal/postgres"
	"openrouter-with-deepswe/internal/telemetry"
	"openrouter-with-deepswe/internal/terminalbench"
)

func main() {
	if err := run(); err != nil {
		slog.Error("terminalbench: fatal", "error", err)
		os.Exit(1)
	}
}

func run() error {
	ctx := context.Background()

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	shutdown, err := telemetry.Setup(ctx, "terminalbench", cfg.OTLPEndpoint)
	if err != nil {
		return err
	}
	defer func() {
		if err := shutdown(ctx); err != nil {
			slog.Warn("terminalbench: telemetry shutdown", "error", err)
		}
	}()

	tracer := telemetry.Tracer("openrouter-with-deepswe/cmd/terminalbench")
	ctx, span := tracer.Start(ctx, "terminalbench.Run")
	defer span.End()

	pool, queries, err := postgres.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	client := terminalbench.NewClient(cfg.TerminalBenchLeaderboardURL, nil)
	return terminalbench.Run(ctx, client, queries)
}
