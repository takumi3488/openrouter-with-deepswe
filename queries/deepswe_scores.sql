-- name: UpsertDeepsweScore :exec
INSERT INTO deepswe_scores (
    model_id, harness, reasoning_effort, pass_rate, pass_at_1, pass_at_4,
    n_passed, n_attempted, mean_cost_usd, fetched_at
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, now())
ON CONFLICT (model_id, harness, reasoning_effort) DO UPDATE SET
    pass_rate = EXCLUDED.pass_rate,
    pass_at_1 = EXCLUDED.pass_at_1,
    pass_at_4 = EXCLUDED.pass_at_4,
    n_passed = EXCLUDED.n_passed,
    n_attempted = EXCLUDED.n_attempted,
    mean_cost_usd = EXCLUDED.mean_cost_usd,
    fetched_at = now();

-- name: GetScoresByModelIDs :many
SELECT * FROM deepswe_scores WHERE model_id = ANY(sqlc.arg(model_ids)::text[])
ORDER BY model_id, harness, reasoning_effort;
