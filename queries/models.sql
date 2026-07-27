-- name: UpsertModel :exec
-- Never touches favorite/hidden: those are user-controlled flags and must
-- survive re-imports from OpenRouter.
INSERT INTO models (
    id, canonical_slug, name, released_at, context_length,
    cheapest_provider, prompt_price, completion_price, last_seen_at
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, now())
ON CONFLICT (id) DO UPDATE SET
    canonical_slug = EXCLUDED.canonical_slug,
    name = EXCLUDED.name,
    released_at = EXCLUDED.released_at,
    context_length = EXCLUDED.context_length,
    cheapest_provider = EXCLUDED.cheapest_provider,
    prompt_price = EXCLUDED.prompt_price,
    completion_price = EXCLUDED.completion_price,
    last_seen_at = now(),
    updated_at = now();

-- name: ListFavoriteIDs :many
SELECT id FROM models WHERE favorite = TRUE;

-- name: SetFavorite :one
UPDATE models SET favorite = $2, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: SetHidden :one
UPDATE models SET hidden = $2, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: ListVisibleModels :many
SELECT * FROM models WHERE hidden = FALSE ORDER BY released_at DESC;

-- name: ListFavoriteModels :many
SELECT * FROM models WHERE favorite = TRUE ORDER BY released_at DESC;

-- name: ListModelsWithoutScores :many
SELECT * FROM models m
WHERE hidden = FALSE
  AND NOT EXISTS (SELECT 1 FROM deepswe_scores s WHERE s.model_id = m.id)
ORDER BY released_at DESC;
