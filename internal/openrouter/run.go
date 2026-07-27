package openrouter

import (
	"context"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"golang.org/x/sync/errgroup"

	"openrouter-with-deepswe/internal/postgres/sqlcgen"
)

// Store is the subset of sqlcgen.Queries that Run needs.
type Store interface {
	ListFavoriteIDs(ctx context.Context) ([]string, error)
	UpsertModel(ctx context.Context, arg sqlcgen.UpsertModelParams) error
}

// Options controls Run's behavior. Zero values fall back to the defaults
// documented on each field.
type Options struct {
	// WeightInput and WeightOutput weight prompt vs completion price when
	// picking the cheapest provider. Default: 3 and 1.
	WeightInput  float64
	WeightOutput float64
	// Concurrency bounds how many /endpoints requests run at once. Default: 4.
	Concurrency int
	// Now returns the current time, used for the "released within one
	// month" check. Defaults to time.Now; overridable for tests.
	Now func() time.Time
}

func (o Options) withDefaults() Options {
	if o.WeightInput == 0 {
		o.WeightInput = 3
	}
	if o.WeightOutput == 0 {
		o.WeightOutput = 1
	}
	if o.Concurrency <= 0 {
		o.Concurrency = 4
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	return o
}

// Run selects Text-to-Text models that are recently released or favorited,
// resolves each one's cheapest provider pricing, and upserts the result.
// Favorite/hidden flags are never touched (see sqlcgen.UpsertModel).
//
// A per-model endpoints lookup failure does not abort the batch: it falls
// back to the model's own listed pricing with no provider recorded. Run
// only returns an error if the final upsert itself fails for one or more
// models.
func Run(ctx context.Context, client *Client, store Store, opts Options) error {
	opts = opts.withDefaults()

	favoriteIDs, err := store.ListFavoriteIDs(ctx)
	if err != nil {
		return fmt.Errorf("openrouter: list favorite ids: %w", err)
	}
	favorites := make(map[string]bool, len(favoriteIDs))
	for _, id := range favoriteIDs {
		favorites[id] = true
	}

	models, err := client.ListModels(ctx)
	if err != nil {
		return fmt.Errorf("openrouter: list models: %w", err)
	}
	warnMissingFavorites(ctx, models, favorites)

	candidates := SelectCandidates(models, opts.Now(), favorites)
	slog.InfoContext(ctx, "selected candidate models", "count", len(candidates), "total", len(models))

	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(opts.Concurrency)
	var failed atomic.Int64

	for _, m := range candidates {
		g.Go(func() error {
			provider, prompt, completion, ok := resolvePricing(gctx, client, m, opts.WeightInput, opts.WeightOutput)
			if !ok {
				// Neither the endpoints API nor the model's own listing had
				// a usable price (e.g. OpenRouter's meta "auto router",
				// which reports "-1" to mean "depends on what it routes
				// to"). There is nothing meaningful to store.
				slog.WarnContext(gctx, "no usable pricing for model, skipping", "model_id", m.ID)
				return nil
			}
			err := store.UpsertModel(gctx, sqlcgen.UpsertModelParams{
				ID:               m.ID,
				CanonicalSlug:    m.CanonicalSlug,
				Name:             m.Name,
				ReleasedAt:       pgtype.Timestamptz{Time: time.Unix(m.Created, 0).UTC(), Valid: true},
				ContextLength:    m.ContextLength,
				CheapestProvider: provider,
				PromptPrice:      prompt,
				CompletionPrice:  completion,
			})
			if err != nil {
				failed.Add(1)
				slog.ErrorContext(gctx, "upsert model failed", "model_id", m.ID, "error", err)
			}
			// Never abort the whole batch over a single model's failure.
			return nil
		})
	}
	_ = g.Wait()

	if n := failed.Load(); n > 0 {
		return fmt.Errorf("openrouter: %d model(s) failed to upsert", n)
	}
	return nil
}

func warnMissingFavorites(ctx context.Context, models []Model, favorites map[string]bool) {
	seen := make(map[string]bool, len(models))
	for _, m := range models {
		seen[m.ID] = true
	}
	for id := range favorites {
		if !seen[id] {
			slog.WarnContext(ctx, "favorite model no longer listed by OpenRouter", "model_id", id)
		}
	}
}

// resolvePricing looks up m's provider endpoints and returns the cheapest
// one's name and pricing. On any failure (request error, or no valid
// endpoint), it falls back to m's own listed pricing with no provider name.
// ok is false if no usable price was found anywhere, in which case the
// other return values are meaningless and must not be stored.
func resolvePricing(ctx context.Context, client *Client, m Model, weightInput, weightOutput float64) (provider *string, prompt, completion string, ok bool) {
	eps, err := client.Endpoints(ctx, m.ID)
	if err != nil {
		slog.WarnContext(ctx, "endpoints lookup failed, falling back to listed pricing", "model_id", m.ID, "error", err)
		return fallbackPricing(m)
	}
	best, found := CheapestEndpoint(eps, weightInput, weightOutput)
	if !found {
		slog.WarnContext(ctx, "no valid endpoint pricing, falling back to listed pricing", "model_id", m.ID)
		return fallbackPricing(m)
	}
	name := best.ProviderName
	return &name, best.Pricing.Prompt, best.Pricing.Completion, true
}

// fallbackPricing validates and returns m's own listed pricing, for use
// when no per-provider endpoint pricing is available.
func fallbackPricing(m Model) (provider *string, prompt, completion string, ok bool) {
	if _, _, valid := validPricing(m.Pricing); !valid {
		return nil, "", "", false
	}
	return nil, m.Pricing.Prompt, m.Pricing.Completion, true
}
