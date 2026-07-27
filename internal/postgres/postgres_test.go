package postgres_test

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"openrouter-with-deepswe/internal/postgres/sqlcgen"
	"openrouter-with-deepswe/internal/testdb"
)

func upsertParams(id string, released time.Time) sqlcgen.UpsertModelParams {
	return sqlcgen.UpsertModelParams{
		ID:              id,
		CanonicalSlug:   id,
		Name:            "Test Model " + id,
		ReleasedAt:      pgtype.Timestamptz{Time: released.UTC(), Valid: true},
		ContextLength:   128000,
		PromptPrice:     "0.000001",
		CompletionPrice: "0.000002",
	}
}

func TestUpsertModel_DoesNotOverwriteFlags(t *testing.T) {
	_, q := testdb.New(t)
	ctx := context.Background()

	const id = "vendor/model-a"
	if err := q.UpsertModel(ctx, upsertParams(id, time.Now())); err != nil {
		t.Fatalf("initial UpsertModel: %v", err)
	}

	if _, err := q.SetFavorite(ctx, sqlcgen.SetFavoriteParams{ID: id, Favorite: true}); err != nil {
		t.Fatalf("SetFavorite: %v", err)
	}
	if _, err := q.SetHidden(ctx, sqlcgen.SetHiddenParams{ID: id, Hidden: true}); err != nil {
		t.Fatalf("SetHidden: %v", err)
	}

	// Re-import: prices/name change, flags must survive untouched.
	params := upsertParams(id, time.Now())
	params.PromptPrice = "0.0000005"
	if err := q.UpsertModel(ctx, params); err != nil {
		t.Fatalf("second UpsertModel: %v", err)
	}

	models, err := q.ListFavoriteModels(ctx)
	if err != nil {
		t.Fatalf("ListFavoriteModels: %v", err)
	}
	var found *sqlcgen.Model
	for i := range models {
		if models[i].ID == id {
			found = &models[i]
		}
	}
	if found == nil {
		t.Fatal("model lost its favorite flag after re-import")
	}
	if !found.Hidden {
		t.Error("model lost its hidden flag after re-import")
	}
	// NUMERIC(24,12) round-trips the value, but not its original string
	// formatting (Postgres pads to the column's fixed scale), so compare
	// numerically rather than by exact string.
	if got, err := strconv.ParseFloat(found.PromptPrice, 64); err != nil || got != 0.0000005 {
		t.Errorf("PromptPrice = %q (parsed %v, err %v), want updated price 0.0000005", found.PromptPrice, got, err)
	}
}

func TestSetFavorite_UnknownModelReturnsErrNoRows(t *testing.T) {
	_, q := testdb.New(t)
	ctx := context.Background()

	_, err := q.SetFavorite(ctx, sqlcgen.SetFavoriteParams{ID: "does-not-exist", Favorite: true})
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("SetFavorite() error = %v, want pgx.ErrNoRows", err)
	}
}

func TestListVisibleModels_ExcludesHidden(t *testing.T) {
	_, q := testdb.New(t)
	ctx := context.Background()

	must(t, q.UpsertModel(ctx, upsertParams("vendor/visible", time.Now())))
	must(t, q.UpsertModel(ctx, upsertParams("vendor/hidden", time.Now())))
	if _, err := q.SetHidden(ctx, sqlcgen.SetHiddenParams{ID: "vendor/hidden", Hidden: true}); err != nil {
		t.Fatalf("SetHidden: %v", err)
	}

	models, err := q.ListVisibleModels(ctx)
	if err != nil {
		t.Fatalf("ListVisibleModels: %v", err)
	}
	ids := idSet(models)
	if !ids["vendor/visible"] {
		t.Error("expected visible model to be listed")
	}
	if ids["vendor/hidden"] {
		t.Error("hidden model must not be listed")
	}
}

func TestListModelsWithoutScores(t *testing.T) {
	_, q := testdb.New(t)
	ctx := context.Background()

	must(t, q.UpsertModel(ctx, upsertParams("vendor/scored", time.Now())))
	must(t, q.UpsertModel(ctx, upsertParams("vendor/unscored", time.Now())))
	must(t, q.UpsertModel(ctx, upsertParams("vendor/hidden-unscored", time.Now())))
	if _, err := q.SetHidden(ctx, sqlcgen.SetHiddenParams{ID: "vendor/hidden-unscored", Hidden: true}); err != nil {
		t.Fatalf("SetHidden: %v", err)
	}

	must(t, q.UpsertDeepsweScore(ctx, sqlcgen.UpsertDeepsweScoreParams{
		ModelID:         "vendor/scored",
		Harness:         "mini-swe-agent",
		ReasoningEffort: "high",
		PassRate:        ptr(0.5),
	}))

	targets, err := q.ListModelsWithoutScores(ctx)
	if err != nil {
		t.Fatalf("ListModelsWithoutScores: %v", err)
	}
	ids := idSet(targets)
	if ids["vendor/scored"] {
		t.Error("scored model must not be a target")
	}
	if !ids["vendor/unscored"] {
		t.Error("unscored visible model should be a target")
	}
	if ids["vendor/hidden-unscored"] {
		t.Error("hidden model must not be a target even without scores")
	}
}

func TestUpsertDeepsweScore_UpdatesOnReRun(t *testing.T) {
	_, q := testdb.New(t)
	ctx := context.Background()

	must(t, q.UpsertModel(ctx, upsertParams("vendor/scored-model", time.Now())))

	must(t, q.UpsertDeepsweScore(ctx, sqlcgen.UpsertDeepsweScoreParams{
		ModelID:         "vendor/scored-model",
		Harness:         "mini-swe-agent",
		ReasoningEffort: "high",
		PassRate:        ptr(0.5),
	}))
	must(t, q.UpsertDeepsweScore(ctx, sqlcgen.UpsertDeepsweScoreParams{
		ModelID:         "vendor/scored-model",
		Harness:         "mini-swe-agent",
		ReasoningEffort: "high",
		PassRate:        ptr(0.75),
	}))

	scores, err := q.GetScoresByModelIDs(ctx, []string{"vendor/scored-model"})
	if err != nil {
		t.Fatalf("GetScoresByModelIDs: %v", err)
	}
	if len(scores) != 1 {
		t.Fatalf("got %d scores, want 1 (re-run should update, not duplicate)", len(scores))
	}
	if scores[0].PassRate == nil || *scores[0].PassRate != 0.75 {
		t.Errorf("PassRate = %v, want 0.75", scores[0].PassRate)
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func idSet(models []sqlcgen.Model) map[string]bool {
	out := make(map[string]bool, len(models))
	for _, m := range models {
		out[m.ID] = true
	}
	return out
}

func ptr[T any](v T) *T { return &v }
