// Package config reads process configuration from environment variables,
// applying sane defaults so every binary in this repository runs with zero
// required configuration beyond DATABASE_URL.
package config

import (
	"fmt"
	"os"
	"strconv"
)

// Config holds settings shared across cmd/openrouter, cmd/grpc and
// cmd/deepswe. Each binary only reads the fields it needs.
type Config struct {
	// DatabaseURL is a PostgreSQL connection string, e.g.
	// "postgres://app:app@localhost:5432/app?sslmode=disable".
	DatabaseURL string

	// OpenRouterBaseURL is the base URL of the OpenRouter API. Overridable
	// for tests.
	OpenRouterBaseURL string

	// DeepSWELeaderboardURL is the URL of the DeepSWE leaderboard JSON
	// artifact. Overridable for tests.
	DeepSWELeaderboardURL string

	// PriceWeightInput and PriceWeightOutput weight prompt vs completion
	// price when picking the cheapest provider for a model.
	PriceWeightInput  float64
	PriceWeightOutput float64

	// EndpointConcurrency bounds how many OpenRouter /endpoints requests run
	// concurrently.
	EndpointConcurrency int

	// GRPCAddr is the listen address of cmd/grpc.
	GRPCAddr string

	// OTLPEndpoint is the OpenTelemetry OTLP/gRPC collector endpoint.
	OTLPEndpoint string
}

// Load reads configuration from the environment. DATABASE_URL is required;
// every other field falls back to a default.
func Load() (Config, error) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		return Config{}, fmt.Errorf("config: DATABASE_URL is required")
	}

	weightIn, err := floatEnv("PRICE_WEIGHT_INPUT", 3)
	if err != nil {
		return Config{}, err
	}
	weightOut, err := floatEnv("PRICE_WEIGHT_OUTPUT", 1)
	if err != nil {
		return Config{}, err
	}
	concurrency, err := intEnv("ENDPOINT_CONCURRENCY", 4)
	if err != nil {
		return Config{}, err
	}

	return Config{
		DatabaseURL:           dbURL,
		OpenRouterBaseURL:     stringEnv("OPENROUTER_BASE_URL", "https://openrouter.ai/api/v1"),
		DeepSWELeaderboardURL: stringEnv("DEEPSWE_LEADERBOARD_URL", "https://deepswe.datacurve.ai/artifacts/v1.1/leaderboard-live.json"),
		PriceWeightInput:      weightIn,
		PriceWeightOutput:     weightOut,
		EndpointConcurrency:   concurrency,
		GRPCAddr:              stringEnv("GRPC_ADDR", ":50051"),
		OTLPEndpoint:          stringEnv("OTEL_EXPORTER_OTLP_ENDPOINT", "localhost:4317"),
	}, nil
}

func stringEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func floatEnv(key string, def float64) (float64, error) {
	v := os.Getenv(key)
	if v == "" {
		return def, nil
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return 0, fmt.Errorf("config: invalid %s: %w", key, err)
	}
	return f, nil
}

func intEnv(key string, def int) (int, error) {
	v := os.Getenv(key)
	if v == "" {
		return def, nil
	}
	i, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("config: invalid %s: %w", key, err)
	}
	return i, nil
}
