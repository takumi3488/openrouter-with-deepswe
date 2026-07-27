package server_test

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
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
