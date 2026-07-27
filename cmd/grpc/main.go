// Command grpc serves the ModelCatalogService gRPC API: toggling per-model
// favorite/hidden flags and listing models with their OpenRouter pricing
// and DeepSWE scores.
package main

import (
	"context"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"

	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	modelcatalogv1 "openrouter-with-deepswe/gen/modelcatalog/v1"
	"openrouter-with-deepswe/internal/config"
	"openrouter-with-deepswe/internal/postgres"
	"openrouter-with-deepswe/internal/server"
	"openrouter-with-deepswe/internal/telemetry"
)

func main() {
	if err := run(); err != nil {
		slog.Error("grpc: fatal", "error", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	shutdown, err := telemetry.Setup(ctx, "grpc", cfg.OTLPEndpoint)
	if err != nil {
		return err
	}
	defer func() {
		if err := shutdown(context.Background()); err != nil {
			slog.Warn("grpc: telemetry shutdown", "error", err)
		}
	}()

	pool, queries, err := postgres.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	lis, err := net.Listen("tcp", cfg.GRPCAddr)
	if err != nil {
		return err
	}

	grpcServer := grpc.NewServer(grpc.StatsHandler(otelgrpc.NewServerHandler()))
	modelcatalogv1.RegisterModelCatalogServiceServer(grpcServer, server.New(queries))
	reflection.Register(grpcServer)

	go func() {
		<-ctx.Done()
		slog.Info("grpc: shutting down")
		grpcServer.GracefulStop()
	}()

	slog.Info("grpc: listening", "addr", cfg.GRPCAddr)
	return grpcServer.Serve(lis)
}
