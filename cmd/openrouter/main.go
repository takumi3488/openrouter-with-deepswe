// Command openrouter selects Text-to-Text models from OpenRouter that were
// either released within the last month or are marked favorite, resolves
// each one's cheapest provider pricing, and upserts the result into the
// database. It is meant to run as a one-shot batch (e.g. on a schedule).
package main

import (
	"context"
	"log/slog"
	"os"

	"openrouter-with-deepswe/internal/config"
	"openrouter-with-deepswe/internal/openrouter"
	"openrouter-with-deepswe/internal/postgres"
	"openrouter-with-deepswe/internal/telemetry"
)

func main() {
	if err := run(); err != nil {
		slog.Error("openrouter: fatal", "error", err)
		os.Exit(1)
	}
}

func run() error {
	ctx := context.Background()

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	shutdown, err := telemetry.Setup(ctx, "openrouter", cfg.OTLPEndpoint)
	if err != nil {
		return err
	}
	defer func() {
		if err := shutdown(ctx); err != nil {
			slog.Warn("openrouter: telemetry shutdown", "error", err)
		}
	}()

	tracer := telemetry.Tracer("openrouter-with-deepswe/cmd/openrouter")
	ctx, span := tracer.Start(ctx, "openrouter.Run")
	defer span.End()

	pool, queries, err := postgres.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	client := openrouter.NewClient(cfg.OpenRouterBaseURL, nil)

	return openrouter.Run(ctx, client, queries, openrouter.Options{
		WeightInput:  cfg.PriceWeightInput,
		WeightOutput: cfg.PriceWeightOutput,
		Concurrency:  cfg.EndpointConcurrency,
	})
}
