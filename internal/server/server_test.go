package server

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	modelcatalogv1 "openrouter-with-deepswe/gen/modelcatalog/v1"
	"openrouter-with-deepswe/internal/postgres/sqlcgen"
)

type fakeStore struct {
	models        map[string]sqlcgen.Model
	terminalBench map[string][]sqlcgen.TerminalBenchScore
	terminalErr   error
	visible       []string // ordered ids returned by ListVisibleModels
	fav           []string // ordered ids returned by ListFavoriteModels
	hidden        []string // ordered ids returned by ListHiddenModels
	upserted      []sqlcgen.UpsertTerminalBenchScoreParams
	upsertErr     error
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		models:        map[string]sqlcgen.Model{},
		terminalBench: map[string][]sqlcgen.TerminalBenchScore{},
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

func (f *fakeStore) GetTerminalBenchScoresByModelIDs(ctx context.Context, ids []string) ([]sqlcgen.TerminalBenchScore, error) {
	if f.terminalErr != nil {
		return nil, f.terminalErr
	}
	var out []sqlcgen.TerminalBenchScore
	for _, id := range ids {
		out = append(out, f.terminalBench[id]...)
	}
	return out, nil
}

func (f *fakeStore) UpsertTerminalBenchScore(ctx context.Context, arg sqlcgen.UpsertTerminalBenchScoreParams) error {
	f.upserted = append(f.upserted, arg)
	return f.upsertErr
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

func addTestScores(store *fakeStore, modelID string) {
	store.terminalBench[modelID] = []sqlcgen.TerminalBenchScore{
		{ModelID: modelID, Leaderboard: "4-0-0", Agent: "agent-a", ReasoningEffort: "low", Accuracy: 72.5, AccuracyCi95HalfWidth: 2.5},
		{ModelID: modelID, Leaderboard: "4-0-0", Agent: "agent-b", ReasoningEffort: "high", Accuracy: 80, AccuracyCi95HalfWidth: 1.5},
	}
}

func TestSetFavorite_Success(t *testing.T) {
	store := newFakeStore()
	store.models["vendor/a"] = testModel("vendor/a", false, false)
	addTestScores(store, "vendor/a")
	srv := New(store)

	resp, err := srv.SetFavorite(context.Background(), &modelcatalogv1.SetFavoriteRequest{ModelId: "vendor/a", Favorite: true})
	if err != nil {
		t.Fatalf("SetFavorite() error = %v", err)
	}
	if !resp.Model.Favorite {
		t.Error("resp.Model.Favorite = false, want true")
	}
	if len(resp.Model.TerminalBenchScores) != 2 {
		t.Fatalf("TerminalBenchScores = %d, want 2", len(resp.Model.TerminalBenchScores))
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
	addTestScores(store, "vendor/a")
	srv := New(store)

	resp, err := srv.SetHidden(context.Background(), &modelcatalogv1.SetHiddenRequest{ModelId: "vendor/a", Hidden: true})
	if err != nil {
		t.Fatalf("SetHidden() error = %v", err)
	}
	if !resp.Model.Hidden {
		t.Error("resp.Model.Hidden = false, want true")
	}
	if len(resp.Model.TerminalBenchScores) != 2 {
		t.Fatalf("TerminalBenchScores = %d, want 2", len(resp.Model.TerminalBenchScores))
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

func TestListModels_IncludesTerminalBenchScores(t *testing.T) {
	store := newFakeStore()
	store.models["vendor/a"] = testModel("vendor/a", false, false)
	store.visible = []string{"vendor/a"}
	addTestScores(store, "vendor/a")
	srv := New(store)

	resp, err := srv.ListModels(context.Background(), &modelcatalogv1.ListModelsRequest{})
	if err != nil {
		t.Fatalf("ListModels() error = %v", err)
	}
	model := resp.Models[0]
	if len(model.TerminalBenchScores) != 2 {
		t.Fatalf("TerminalBenchScores = %+v, want 2 entries", model.TerminalBenchScores)
	}
	if model.TerminalBenchScores[0].Agent != "agent-a" || model.TerminalBenchScores[0].ReasoningEffort != "low" ||
		model.TerminalBenchScores[1].Agent != "agent-b" || model.TerminalBenchScores[1].ReasoningEffort != "high" {
		t.Errorf("Terminal-Bench scores = %+v", model.TerminalBenchScores)
	}
}

func TestSetModel_ScoreErrorsPropagate(t *testing.T) {
	tests := []struct {
		name string
		call func(*Server) error
	}{
		{
			name: "favorite",
			call: func(s *Server) error {
				_, err := s.SetFavorite(context.Background(), &modelcatalogv1.SetFavoriteRequest{ModelId: "vendor/a", Favorite: true})
				return err
			},
		},
		{
			name: "hidden",
			call: func(s *Server) error {
				_, err := s.SetHidden(context.Background(), &modelcatalogv1.SetHiddenRequest{ModelId: "vendor/a", Hidden: true})
				return err
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newFakeStore()
			store.models["vendor/a"] = testModel("vendor/a", false, false)
			store.terminalErr = errors.New("terminal score query failed")
			err := tt.call(New(store))
			if status.Code(err) != codes.Internal {
				t.Fatalf("score error code = %v, want %v", status.Code(err), codes.Internal)
			}
		})
	}
}

func TestListModels_ScoreErrorsPropagate(t *testing.T) {
	tests := []struct {
		name string
		set  func(*fakeStore)
	}{
		{name: "terminal bench", set: func(store *fakeStore) { store.terminalErr = errors.New("terminal score query failed") }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newFakeStore()
			store.models["vendor/a"] = testModel("vendor/a", false, false)
			store.visible = []string{"vendor/a"}
			tt.set(store)
			_, err := New(store).ListModels(context.Background(), &modelcatalogv1.ListModelsRequest{})
			if status.Code(err) != codes.Internal {
				t.Fatalf("score error code = %v, want %v", status.Code(err), codes.Internal)
			}
		})
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

func TestUpsertTerminalBenchScore_StoresAndEchoesScore(t *testing.T) {
	store := newFakeStore()
	srv := New(store)

	resp, err := srv.UpsertTerminalBenchScore(context.Background(), &modelcatalogv1.UpsertTerminalBenchScoreRequest{
		ModelId:               "vendor/a",
		Leaderboard:           "4-0-0",
		Agent:                 "agent-a",
		ReasoningEffort:       "high",
		Accuracy:              72.5,
		AccuracyCi95HalfWidth: 2.5,
	})
	if err != nil {
		t.Fatalf("UpsertTerminalBenchScore() error = %v", err)
	}

	want := sqlcgen.UpsertTerminalBenchScoreParams{
		ModelID: "vendor/a", Leaderboard: "4-0-0", Agent: "agent-a",
		ReasoningEffort: "high", Accuracy: 72.5, AccuracyCi95HalfWidth: 2.5,
	}
	if len(store.upserted) != 1 || store.upserted[0] != want {
		t.Fatalf("store params = %+v, want exactly one %+v", store.upserted, want)
	}
	got := resp.GetScore()
	if got.GetLeaderboard() != "4-0-0" || got.GetAgent() != "agent-a" || got.GetReasoningEffort() != "high" ||
		got.GetAccuracy() != 72.5 || got.GetAccuracyCi95HalfWidth() != 2.5 {
		t.Errorf("response score = %+v, want the stored values echoed back", got)
	}
}

// Manual rows must land on the same "default" reasoning-effort sentinel the
// terminalbench batch writes; otherwise a later batch run would insert a
// second row instead of overwriting the manual one.
func TestUpsertTerminalBenchScore_EmptyReasoningEffortBecomesDefault(t *testing.T) {
	store := newFakeStore()
	srv := New(store)

	resp, err := srv.UpsertTerminalBenchScore(context.Background(), &modelcatalogv1.UpsertTerminalBenchScoreRequest{
		ModelId: "vendor/a", Leaderboard: "4-0-0", Agent: "agent-a", ReasoningEffort: "  ", Accuracy: 10,
	})
	if err != nil {
		t.Fatalf("UpsertTerminalBenchScore() error = %v", err)
	}
	if store.upserted[0].ReasoningEffort != "default" {
		t.Errorf("stored ReasoningEffort = %q, want %q", store.upserted[0].ReasoningEffort, "default")
	}
	if resp.GetScore().GetReasoningEffort() != "default" {
		t.Errorf("response ReasoningEffort = %q, want %q", resp.GetScore().GetReasoningEffort(), "default")
	}
}

// Padded input must be stored trimmed: an untrimmed key occupies a primary
// key the batch can never write, leaving a permanently-manual duplicate row.
func TestUpsertTerminalBenchScore_TrimsKeysBeforeStoring(t *testing.T) {
	store := newFakeStore()
	srv := New(store)

	resp, err := srv.UpsertTerminalBenchScore(context.Background(), &modelcatalogv1.UpsertTerminalBenchScoreRequest{
		ModelId:               " m1 ",
		Leaderboard:           " 4-0-0 ",
		Agent:                 " Codex ",
		ReasoningEffort:       " high ",
		Accuracy:              72.5,
		AccuracyCi95HalfWidth: 2.5,
	})
	if err != nil {
		t.Fatalf("UpsertTerminalBenchScore() error = %v", err)
	}

	want := sqlcgen.UpsertTerminalBenchScoreParams{
		ModelID: "m1", Leaderboard: "4-0-0", Agent: "Codex",
		ReasoningEffort: "high", Accuracy: 72.5, AccuracyCi95HalfWidth: 2.5,
	}
	if len(store.upserted) != 1 || store.upserted[0] != want {
		t.Fatalf("store params = %+v, want exactly one %+v", store.upserted, want)
	}
	got := resp.GetScore()
	if got.GetLeaderboard() != "4-0-0" || got.GetAgent() != "Codex" || got.GetReasoningEffort() != "high" {
		t.Errorf("response score = %+v, want the trimmed values echoed back", got)
	}
}

func TestUpsertTerminalBenchScore_InvalidArguments(t *testing.T) {
	valid := func() *modelcatalogv1.UpsertTerminalBenchScoreRequest {
		return &modelcatalogv1.UpsertTerminalBenchScoreRequest{
			ModelId: "vendor/a", Leaderboard: "4-0-0", Agent: "agent-a", Accuracy: 50, AccuracyCi95HalfWidth: 1,
		}
	}
	tests := []struct {
		name   string
		mutate func(*modelcatalogv1.UpsertTerminalBenchScoreRequest)
	}{
		{"empty model id", func(r *modelcatalogv1.UpsertTerminalBenchScoreRequest) { r.ModelId = "" }},
		{"whitespace-only model id", func(r *modelcatalogv1.UpsertTerminalBenchScoreRequest) { r.ModelId = "\t " }},
		{"empty leaderboard", func(r *modelcatalogv1.UpsertTerminalBenchScoreRequest) { r.Leaderboard = " " }},
		{"empty agent", func(r *modelcatalogv1.UpsertTerminalBenchScoreRequest) { r.Agent = "" }},
		{"negative accuracy", func(r *modelcatalogv1.UpsertTerminalBenchScoreRequest) { r.Accuracy = -1 }},
		{"accuracy above 100", func(r *modelcatalogv1.UpsertTerminalBenchScoreRequest) { r.Accuracy = 101 }},
		{"ci half width above 100", func(r *modelcatalogv1.UpsertTerminalBenchScoreRequest) { r.AccuracyCi95HalfWidth = 101 }},
		{"nan accuracy", func(r *modelcatalogv1.UpsertTerminalBenchScoreRequest) { r.Accuracy = math.NaN() }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newFakeStore()
			req := valid()
			tt.mutate(req)

			_, err := New(store).UpsertTerminalBenchScore(context.Background(), req)
			if status.Code(err) != codes.InvalidArgument {
				t.Fatalf("UpsertTerminalBenchScore() error = %v, want codes.InvalidArgument", err)
			}
			if len(store.upserted) != 0 {
				t.Errorf("store was called with %+v, want no call on invalid input", store.upserted)
			}
		})
	}
}

func TestUpsertTerminalBenchScore_StoreErrorBecomesInternal(t *testing.T) {
	store := newFakeStore()
	store.upsertErr = errors.New("boom")
	srv := New(store)

	_, err := srv.UpsertTerminalBenchScore(context.Background(), &modelcatalogv1.UpsertTerminalBenchScoreRequest{
		ModelId: "vendor/a", Leaderboard: "4-0-0", Agent: "agent-a", Accuracy: 50,
	})
	if status.Code(err) != codes.Internal {
		t.Fatalf("UpsertTerminalBenchScore() error = %v, want codes.Internal", err)
	}
}

func TestUpsertTerminalBenchScore_ForeignKeyViolationBecomesNotFound(t *testing.T) {
	store := newFakeStore()
	store.upsertErr = &pgconn.PgError{Code: "23503", Message: "insert or update violates foreign key constraint"}
	srv := New(store)

	_, err := srv.UpsertTerminalBenchScore(context.Background(), &modelcatalogv1.UpsertTerminalBenchScoreRequest{
		ModelId: "vendor/missing", Leaderboard: "4-0-0", Agent: "agent-a", Accuracy: 50,
	})
	if status.Code(err) != codes.NotFound {
		t.Fatalf("UpsertTerminalBenchScore() error = %v, want codes.NotFound", err)
	}
}
