package server

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	modelcatalogv1 "openrouter-with-deepswe/gen/modelcatalog/v1"
	"openrouter-with-deepswe/internal/postgres/sqlcgen"
)

type fakeStore struct {
	models  map[string]sqlcgen.Model
	scores  map[string][]sqlcgen.DeepsweScore
	visible []string // ordered ids returned by ListVisibleModels
	fav     []string // ordered ids returned by ListFavoriteModels
	hidden  []string // ordered ids returned by ListHiddenModels
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		models: map[string]sqlcgen.Model{},
		scores: map[string][]sqlcgen.DeepsweScore{},
	}
}

func (f *fakeStore) SetFavorite(ctx context.Context, arg sqlcgen.SetFavoriteParams) (sqlcgen.Model, error) {
	m, ok := f.models[arg.ID]
	if !ok {
		return sqlcgen.Model{}, pgx.ErrNoRows
	}
	m.Favorite = arg.Favorite
	f.models[arg.ID] = m
	return m, nil
}

func (f *fakeStore) SetHidden(ctx context.Context, arg sqlcgen.SetHiddenParams) (sqlcgen.Model, error) {
	m, ok := f.models[arg.ID]
	if !ok {
		return sqlcgen.Model{}, pgx.ErrNoRows
	}
	m.Hidden = arg.Hidden
	f.models[arg.ID] = m
	return m, nil
}

func (f *fakeStore) ListVisibleModels(ctx context.Context) ([]sqlcgen.Model, error) {
	return f.byIDs(f.visible), nil
}

func (f *fakeStore) ListFavoriteModels(ctx context.Context) ([]sqlcgen.Model, error) {
	return f.byIDs(f.fav), nil
}

func (f *fakeStore) ListHiddenModels(ctx context.Context) ([]sqlcgen.Model, error) {
	return f.byIDs(f.hidden), nil
}

func (f *fakeStore) byIDs(ids []string) []sqlcgen.Model {
	out := make([]sqlcgen.Model, 0, len(ids))
	for _, id := range ids {
		out = append(out, f.models[id])
	}
	return out
}

func (f *fakeStore) GetScoresByModelIDs(ctx context.Context, ids []string) ([]sqlcgen.DeepsweScore, error) {
	var out []sqlcgen.DeepsweScore
	for _, id := range ids {
		out = append(out, f.scores[id]...)
	}
	return out, nil
}

func testModel(id string, favorite, hidden bool) sqlcgen.Model {
	return sqlcgen.Model{
		ID:              id,
		CanonicalSlug:   id,
		Name:            "Name " + id,
		ReleasedAt:      pgtype.Timestamptz{Time: time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC), Valid: true},
		ContextLength:   128000,
		PromptPrice:     "0.000001",
		CompletionPrice: "0.000002",
		Favorite:        favorite,
		Hidden:          hidden,
	}
}

func TestSetFavorite_Success(t *testing.T) {
	store := newFakeStore()
	store.models["vendor/a"] = testModel("vendor/a", false, false)
	srv := New(store)

	resp, err := srv.SetFavorite(context.Background(), &modelcatalogv1.SetFavoriteRequest{ModelId: "vendor/a", Favorite: true})
	if err != nil {
		t.Fatalf("SetFavorite() error = %v", err)
	}
	if !resp.Model.Favorite {
		t.Error("resp.Model.Favorite = false, want true")
	}
}

func TestSetFavorite_NotFound(t *testing.T) {
	store := newFakeStore()
	srv := New(store)

	_, err := srv.SetFavorite(context.Background(), &modelcatalogv1.SetFavoriteRequest{ModelId: "vendor/missing", Favorite: true})
	if status.Code(err) != codes.NotFound {
		t.Fatalf("SetFavorite() error = %v, want codes.NotFound", err)
	}
}

func TestSetHidden_Success(t *testing.T) {
	store := newFakeStore()
	store.models["vendor/a"] = testModel("vendor/a", false, false)
	srv := New(store)

	resp, err := srv.SetHidden(context.Background(), &modelcatalogv1.SetHiddenRequest{ModelId: "vendor/a", Hidden: true})
	if err != nil {
		t.Fatalf("SetHidden() error = %v", err)
	}
	if !resp.Model.Hidden {
		t.Error("resp.Model.Hidden = false, want true")
	}
}

func TestSetHidden_NotFound(t *testing.T) {
	store := newFakeStore()
	srv := New(store)

	_, err := srv.SetHidden(context.Background(), &modelcatalogv1.SetHiddenRequest{ModelId: "vendor/missing", Hidden: true})
	if status.Code(err) != codes.NotFound {
		t.Fatalf("SetHidden() error = %v, want codes.NotFound", err)
	}
}

func TestListModels_DefaultFilterListsVisible(t *testing.T) {
	store := newFakeStore()
	store.models["vendor/a"] = testModel("vendor/a", false, false)
	store.visible = []string{"vendor/a"}
	srv := New(store)

	resp, err := srv.ListModels(context.Background(), &modelcatalogv1.ListModelsRequest{})
	if err != nil {
		t.Fatalf("ListModels() error = %v", err)
	}
	if len(resp.Models) != 1 || resp.Models[0].Id != "vendor/a" {
		t.Fatalf("ListModels() = %+v, want [vendor/a]", resp.Models)
	}
}

func TestListModels_FavoriteFilter(t *testing.T) {
	store := newFakeStore()
	store.models["vendor/a"] = testModel("vendor/a", true, true) // hidden favorite must still show up
	store.fav = []string{"vendor/a"}
	srv := New(store)

	resp, err := srv.ListModels(context.Background(), &modelcatalogv1.ListModelsRequest{Filter: modelcatalogv1.ListModelsRequest_FILTER_FAVORITE})
	if err != nil {
		t.Fatalf("ListModels() error = %v", err)
	}
	if len(resp.Models) != 1 || resp.Models[0].Id != "vendor/a" {
		t.Fatalf("ListModels() = %+v, want [vendor/a]", resp.Models)
	}
	if !resp.Models[0].Hidden {
		t.Error("hidden favorite model should still be returned with Hidden = true")
	}
}

func TestListModels_HiddenFilter(t *testing.T) {
	store := newFakeStore()
	store.models["vendor/a"] = testModel("vendor/a", false, true)
	store.hidden = []string{"vendor/a"}
	srv := New(store)

	resp, err := srv.ListModels(context.Background(), &modelcatalogv1.ListModelsRequest{Filter: modelcatalogv1.ListModelsRequest_FILTER_HIDDEN})
	if err != nil {
		t.Fatalf("ListModels() error = %v", err)
	}
	if len(resp.Models) != 1 || resp.Models[0].Id != "vendor/a" {
		t.Fatalf("ListModels() = %+v, want [vendor/a]", resp.Models)
	}
	if !resp.Models[0].Hidden {
		t.Error("hidden model should be returned with Hidden = true")
	}
}

func TestListModels_IncludesDeepSweScores(t *testing.T) {
	store := newFakeStore()
	store.models["vendor/a"] = testModel("vendor/a", false, false)
	store.visible = []string{"vendor/a"}
	passRate := 0.75
	store.scores["vendor/a"] = []sqlcgen.DeepsweScore{
		{ModelID: "vendor/a", Harness: "mini-swe-agent", ReasoningEffort: "high", PassRate: &passRate},
	}
	srv := New(store)

	resp, err := srv.ListModels(context.Background(), &modelcatalogv1.ListModelsRequest{})
	if err != nil {
		t.Fatalf("ListModels() error = %v", err)
	}
	scores := resp.Models[0].DeepsweScores
	if len(scores) != 1 {
		t.Fatalf("DeepsweScores = %+v, want 1 entry", scores)
	}
	if scores[0].ReasoningEffort != "high" || scores[0].PassRate != 0.75 {
		t.Errorf("scores[0] = %+v", scores[0])
	}
}

func TestListModels_CheapestProviderNilBecomesEmptyString(t *testing.T) {
	store := newFakeStore()
	m := testModel("vendor/a", false, false)
	m.CheapestProvider = nil
	store.models["vendor/a"] = m
	store.visible = []string{"vendor/a"}
	srv := New(store)

	resp, err := srv.ListModels(context.Background(), &modelcatalogv1.ListModelsRequest{})
	if err != nil {
		t.Fatalf("ListModels() error = %v", err)
	}
	if resp.Models[0].CheapestProvider != "" {
		t.Errorf("CheapestProvider = %q, want empty string", resp.Models[0].CheapestProvider)
	}
}
