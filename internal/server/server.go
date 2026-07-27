// Package server implements the ModelCatalogService gRPC API on top of the
// sqlc-generated Queries.
package server

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	modelcatalogv1 "openrouter-with-deepswe/gen/modelcatalog/v1"
	"openrouter-with-deepswe/internal/postgres/sqlcgen"
)

// Store is the subset of sqlcgen.Queries that Server needs.
type Store interface {
	SetFavorite(ctx context.Context, arg sqlcgen.SetFavoriteParams) (sqlcgen.Model, error)
	SetHidden(ctx context.Context, arg sqlcgen.SetHiddenParams) (sqlcgen.Model, error)
	ListVisibleModels(ctx context.Context) ([]sqlcgen.Model, error)
	ListFavoriteModels(ctx context.Context) ([]sqlcgen.Model, error)
	GetScoresByModelIDs(ctx context.Context, modelIDs []string) ([]sqlcgen.DeepsweScore, error)
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
	if req.GetFilter() == modelcatalogv1.ListModelsRequest_FILTER_FAVORITE {
		models, err = s.store.ListFavoriteModels(ctx)
	} else {
		models, err = s.store.ListVisibleModels(ctx)
	}
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list models: %v", err)
	}

	ids := make([]string, len(models))
	for i, m := range models {
		ids[i] = m.ID
	}
	scoresByModel, err := s.scoresByModelID(ctx, ids)
	if err != nil {
		return nil, err
	}

	out := make([]*modelcatalogv1.Model, len(models))
	for i, m := range models {
		out[i] = toProtoModel(m, scoresByModel[m.ID])
	}
	return &modelcatalogv1.ListModelsResponse{Models: out}, nil
}

func (s *Server) toProtoModelWithScores(ctx context.Context, m sqlcgen.Model) (*modelcatalogv1.Model, error) {
	scores, err := s.store.GetScoresByModelIDs(ctx, []string{m.ID})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get scores: %v", err)
	}
	return toProtoModel(m, scores), nil
}

func (s *Server) scoresByModelID(ctx context.Context, ids []string) (map[string][]sqlcgen.DeepsweScore, error) {
	scores, err := s.store.GetScoresByModelIDs(ctx, ids)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get scores: %v", err)
	}
	byModel := make(map[string][]sqlcgen.DeepsweScore, len(ids))
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

func toProtoModel(m sqlcgen.Model, scores []sqlcgen.DeepsweScore) *modelcatalogv1.Model {
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
	for _, sc := range scores {
		pm.DeepsweScores = append(pm.DeepsweScores, toProtoScore(sc))
	}
	return pm
}

func toProtoScore(sc sqlcgen.DeepsweScore) *modelcatalogv1.DeepSweScore {
	return &modelcatalogv1.DeepSweScore{
		Harness:         sc.Harness,
		ReasoningEffort: sc.ReasoningEffort,
		PassRate:        derefFloat(sc.PassRate),
		PassAt_1:        derefFloat(sc.PassAt1),
		PassAt_4:        derefFloat(sc.PassAt4),
		MeanCostUsd:     derefFloat(sc.MeanCostUsd),
	}
}

func derefFloat(f *float64) float64 {
	if f == nil {
		return 0
	}
	return *f
}
