package server_test

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	modelcatalogv1 "openrouter-with-deepswe/gen/modelcatalog/v1"
	"openrouter-with-deepswe/internal/postgres/sqlcgen"
	"openrouter-with-deepswe/internal/server"
	"openrouter-with-deepswe/internal/testdb"
)

// TestServer_EndToEnd exercises the real sqlc Queries against a live
// PostgreSQL container through the actual gRPC wire protocol (via bufconn),
// verifying the full SetFavorite -> ListModels(FILTER_FAVORITE) round trip
// that a real client would perform.
func TestServer_EndToEnd_SetFavoriteThenListFavorites(t *testing.T) {
	_, queries := testdb.New(t)
	ctx := context.Background()

	const id = "vendor/e2e-model"
	err := queries.UpsertModel(ctx, sqlcgen.UpsertModelParams{
		ID:              id,
		CanonicalSlug:   id,
		Name:            "E2E Model",
		ReleasedAt:      pgtype.Timestamptz{Time: time.Now(), Valid: true},
		ContextLength:   128000,
		PromptPrice:     "0.000001",
		CompletionPrice: "0.000002",
	})
	if err != nil {
		t.Fatalf("UpsertModel: %v", err)
	}

	lis := bufconn.Listen(1024 * 1024)
	grpcServer := grpc.NewServer()
	modelcatalogv1.RegisterModelCatalogServiceServer(grpcServer, server.New(queries))
	go func() {
		_ = grpcServer.Serve(lis)
	}()
	defer grpcServer.Stop()

	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = conn.Close() }()

	client := modelcatalogv1.NewModelCatalogServiceClient(conn)

	if _, err := client.SetFavorite(ctx, &modelcatalogv1.SetFavoriteRequest{ModelId: id, Favorite: true}); err != nil {
		t.Fatalf("SetFavorite: %v", err)
	}

	resp, err := client.ListModels(ctx, &modelcatalogv1.ListModelsRequest{Filter: modelcatalogv1.ListModelsRequest_FILTER_FAVORITE})
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}

	var found bool
	for _, m := range resp.Models {
		if m.Id == id {
			found = true
			if !m.Favorite {
				t.Error("returned model Favorite = false, want true")
			}
		}
	}
	if !found {
		t.Errorf("favorited model %q not found in ListModels(FILTER_FAVORITE) response", id)
	}
}

// TestServer_EndToEnd_BatchUpsertOverwritesManualScore pins the precedence
// rule: the manual RPC and the terminalbench batch write the same primary
// key, so a later batch run overwrites the manual row in place instead of
// adding a second one.
func TestServer_EndToEnd_BatchUpsertOverwritesManualScore(t *testing.T) {
	_, queries := testdb.New(t)
	ctx := context.Background()

	const id = "vendor/e2e-manual-score-model"
	err := queries.UpsertModel(ctx, sqlcgen.UpsertModelParams{
		ID:              id,
		CanonicalSlug:   id,
		Name:            "E2E Manual Score Model",
		ReleasedAt:      pgtype.Timestamptz{Time: time.Now(), Valid: true},
		ContextLength:   128000,
		PromptPrice:     "0.000001",
		CompletionPrice: "0.000002",
	})
	if err != nil {
		t.Fatalf("UpsertModel: %v", err)
	}

	srv := server.New(queries)
	resp, err := srv.UpsertTerminalBenchScore(ctx, &modelcatalogv1.UpsertTerminalBenchScoreRequest{
		ModelId:               id,
		Leaderboard:           "4-0-0",
		Agent:                 "agent-e2e",
		Accuracy:              41.5,
		AccuracyCi95HalfWidth: 3.5,
	})
	if err != nil {
		t.Fatalf("UpsertTerminalBenchScore: %v", err)
	}
	if got := resp.GetScore().GetReasoningEffort(); got != "default" {
		t.Fatalf("stored ReasoningEffort = %q, want %q", got, "default")
	}

	// Exactly what the terminalbench batch does for the same key.
	err = queries.UpsertTerminalBenchScore(ctx, sqlcgen.UpsertTerminalBenchScoreParams{
		ModelID:               id,
		Leaderboard:           "4-0-0",
		Agent:                 "agent-e2e",
		ReasoningEffort:       "default",
		Accuracy:              62.25,
		AccuracyCi95HalfWidth: 1.25,
	})
	if err != nil {
		t.Fatalf("batch UpsertTerminalBenchScore: %v", err)
	}

	scores, err := queries.GetTerminalBenchScoresByModelIDs(ctx, []string{id})
	if err != nil {
		t.Fatalf("GetTerminalBenchScoresByModelIDs: %v", err)
	}
	if len(scores) != 1 {
		t.Fatalf("scores for %q = %d, want 1 (batch must overwrite the manual row, not add one)", id, len(scores))
	}
	if scores[0].Accuracy != 62.25 || scores[0].AccuracyCi95HalfWidth != 1.25 {
		t.Errorf("score = %+v, want the batch values 62.25/1.25 to win", scores[0])
	}
}

// TestServer_EndToEnd_UnknownModelIsNotFound pins the assumption behind the
// 23503 mapping: pgx/v5 really surfaces *pgconn.PgError through the :exec
// path, so a foreign-key violation on a missing model becomes NOT_FOUND.
func TestServer_EndToEnd_UnknownModelIsNotFound(t *testing.T) {
	_, queries := testdb.New(t)
	ctx := context.Background()

	_, err := server.New(queries).UpsertTerminalBenchScore(ctx, &modelcatalogv1.UpsertTerminalBenchScoreRequest{
		ModelId:               "vendor/e2e-missing-model",
		Leaderboard:           "4-0-0",
		Agent:                 "agent-e2e-missing",
		Accuracy:              10,
		AccuracyCi95HalfWidth: 1,
	})
	if status.Code(err) != codes.NotFound {
		t.Fatalf("UpsertTerminalBenchScore() error = %v, want codes.NotFound", err)
	}
}
