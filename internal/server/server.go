// Package server implements the ModelCatalogService gRPC API on top of the
// sqlc-generated Queries.
package server

import (
	"context"
	"errors"
	"math"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	modelcatalogv1 "openrouter-with-deepswe/gen/modelcatalog/v1"
	"openrouter-with-deepswe/internal/postgres/sqlcgen"
	"openrouter-with-deepswe/internal/terminalbench"
)

// Store is the subset of sqlcgen.Queries that Server needs.
type Store interface {
	SetFavorite(ctx context.Context, arg sqlcgen.SetFavoriteParams) (sqlcgen.Model, error)
	SetHidden(ctx context.Context, arg sqlcgen.SetHiddenParams) (sqlcgen.Model, error)
	ListVisibleModels(ctx context.Context) ([]sqlcgen.Model, error)
	ListFavoriteModels(ctx context.Context) ([]sqlcgen.Model, error)
	ListHiddenModels(ctx context.Context) ([]sqlcgen.Model, error)
	GetTerminalBenchScoresByModelIDs(ctx context.Context, modelIDs []string) ([]sqlcgen.TerminalBenchScore, error)
	UpsertTerminalBenchScore(ctx context.Context, arg sqlcgen.UpsertTerminalBenchScoreParams) error
}

// Server implements modelcatalogv1.ModelCatalogServiceServer.
type Server struct {
	modelcatalogv1.UnimplementedModelCatalogServiceServer
	store Store
}

// New builds a Server backed by store.
func New(store Store) *Server {
	return &Server{store: store}
}

func (s *Server) SetFavorite(ctx context.Context, req *modelcatalogv1.SetFavoriteRequest) (*modelcatalogv1.SetFavoriteResponse, error) {
	m, err := s.store.SetFavorite(ctx, sqlcgen.SetFavoriteParams{ID: req.GetModelId(), Favorite: req.GetFavorite()})
	if err != nil {
		return nil, mapStoreError(err, req.GetModelId(), "set favorite")
	}
	pm, err := s.toProtoModelWithScores(ctx, m)
	if err != nil {
		return nil, err
	}
	return &modelcatalogv1.SetFavoriteResponse{Model: pm}, nil
}

func (s *Server) SetHidden(ctx context.Context, req *modelcatalogv1.SetHiddenRequest) (*modelcatalogv1.SetHiddenResponse, error) {
	m, err := s.store.SetHidden(ctx, sqlcgen.SetHiddenParams{ID: req.GetModelId(), Hidden: req.GetHidden()})
	if err != nil {
		return nil, mapStoreError(err, req.GetModelId(), "set hidden")
	}
	pm, err := s.toProtoModelWithScores(ctx, m)
	if err != nil {
		return nil, err
	}
	return &modelcatalogv1.SetHiddenResponse{Model: pm}, nil
}

func (s *Server) ListModels(ctx context.Context, req *modelcatalogv1.ListModelsRequest) (*modelcatalogv1.ListModelsResponse, error) {
	var (
		models []sqlcgen.Model
		err    error
	)
	switch req.GetFilter() {
	case modelcatalogv1.ListModelsRequest_FILTER_FAVORITE:
		models, err = s.store.ListFavoriteModels(ctx)
	case modelcatalogv1.ListModelsRequest_FILTER_HIDDEN:
		models, err = s.store.ListHiddenModels(ctx)
	default:
		models, err = s.store.ListVisibleModels(ctx)
	}
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list models: %v", err)
	}

	ids := make([]string, len(models))
	for i, m := range models {
		ids[i] = m.ID
	}
	terminalBenchByModel, err := s.terminalBenchScoresByModelID(ctx, ids)
	if err != nil {
		return nil, err
	}

	out := make([]*modelcatalogv1.Model, len(models))
	for i, m := range models {
		out[i] = toProtoModel(m, terminalBenchByModel[m.ID])
	}
	return &modelcatalogv1.ListModelsResponse{Models: out}, nil
}

// UpsertTerminalBenchScore manually records a Terminal-Bench score for a
// model. String keys are trimmed before validation and storage so a manual
// row lands on exactly the primary key the terminalbench batch writes. The
// batch unconditionally re-upserts every row it finds for a visible model,
// so a later batch run overwrites a manual row sharing that key; manual rows
// on hidden models, or whose key the leaderboard never produces, persist.
func (s *Server) UpsertTerminalBenchScore(ctx context.Context, req *modelcatalogv1.UpsertTerminalBenchScoreRequest) (*modelcatalogv1.UpsertTerminalBenchScoreResponse, error) {
	modelID := strings.TrimSpace(req.GetModelId())
	leaderboard := strings.TrimSpace(req.GetLeaderboard())
	agent := strings.TrimSpace(req.GetAgent())
	for _, field := range [][2]string{{"model_id", modelID}, {"leaderboard", leaderboard}, {"agent", agent}} {
		if field[1] == "" {
			return nil, status.Errorf(codes.InvalidArgument, "%s must not be empty", field[0])
		}
	}
	inRange := func(name string, value float64) error {
		if math.IsNaN(value) || value < 0 || value > 100 {
			return status.Errorf(codes.InvalidArgument, "%s must be between 0 and 100, got %v", name, value)
		}
		return nil
	}
	if err := inRange("accuracy", req.GetAccuracy()); err != nil {
		return nil, err
	}
	if err := inRange("accuracy_ci95_half_width", req.GetAccuracyCi95HalfWidth()); err != nil {
		return nil, err
	}

	params := sqlcgen.UpsertTerminalBenchScoreParams{
		ModelID:               modelID,
		Leaderboard:           leaderboard,
		Agent:                 agent,
		ReasoningEffort:       terminalbench.EffortOrDefault(strings.TrimSpace(req.GetReasoningEffort())),
		Accuracy:              req.GetAccuracy(),
		AccuracyCi95HalfWidth: req.GetAccuracyCi95HalfWidth(),
	}
	if err := s.store.UpsertTerminalBenchScore(ctx, params); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23503" {
			return nil, status.Errorf(codes.NotFound, "model %q not found", modelID)
		}
		return nil, mapStoreError(err, modelID, "upsert terminal bench score")
	}
	return &modelcatalogv1.UpsertTerminalBenchScoreResponse{Score: &modelcatalogv1.TerminalBenchScore{
		Leaderboard:           params.Leaderboard,
		Agent:                 params.Agent,
		ReasoningEffort:       params.ReasoningEffort,
		Accuracy:              params.Accuracy,
		AccuracyCi95HalfWidth: params.AccuracyCi95HalfWidth,
	}}, nil
}

func (s *Server) toProtoModelWithScores(ctx context.Context, m sqlcgen.Model) (*modelcatalogv1.Model, error) {
	terminalBenchScores, err := s.store.GetTerminalBenchScoresByModelIDs(ctx, []string{m.ID})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get terminal bench scores: %v", err)
	}
	return toProtoModel(m, terminalBenchScores), nil
}

func (s *Server) terminalBenchScoresByModelID(ctx context.Context, ids []string) (map[string][]sqlcgen.TerminalBenchScore, error) {
	scores, err := s.store.GetTerminalBenchScoresByModelIDs(ctx, ids)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get terminal bench scores: %v", err)
	}
	byModel := make(map[string][]sqlcgen.TerminalBenchScore, len(ids))
	for _, sc := range scores {
		byModel[sc.ModelID] = append(byModel[sc.ModelID], sc)
	}
	return byModel, nil
}

func mapStoreError(err error, modelID, action string) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return status.Errorf(codes.NotFound, "model %q not found", modelID)
	}
	return status.Errorf(codes.Internal, "%s: %v", action, err)
}

func toProtoModel(m sqlcgen.Model, terminalBenchScores []sqlcgen.TerminalBenchScore) *modelcatalogv1.Model {
	var provider string
	if m.CheapestProvider != nil {
		provider = *m.CheapestProvider
	}

	pm := &modelcatalogv1.Model{
		Id:               m.ID,
		Name:             m.Name,
		CheapestProvider: provider,
		PromptPrice:      m.PromptPrice,
		CompletionPrice:  m.CompletionPrice,
		Favorite:         m.Favorite,
		Hidden:           m.Hidden,
		ContextLength:    m.ContextLength,
		ReleasedAt:       timestamppb.New(m.ReleasedAt.Time),
	}
	for _, sc := range terminalBenchScores {
		pm.TerminalBenchScores = append(pm.TerminalBenchScores, toProtoTerminalBenchScore(sc))
	}
	return pm
}

func toProtoTerminalBenchScore(sc sqlcgen.TerminalBenchScore) *modelcatalogv1.TerminalBenchScore {
	return &modelcatalogv1.TerminalBenchScore{
		Leaderboard:           sc.Leaderboard,
		Agent:                 sc.Agent,
		ReasoningEffort:       sc.ReasoningEffort,
		Accuracy:              sc.Accuracy,
		AccuracyCi95HalfWidth: sc.AccuracyCi95HalfWidth,
	}
}
