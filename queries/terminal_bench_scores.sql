-- name: UpsertTerminalBenchScore :exec
INSERT INTO terminal_bench_scores (
    model_id, leaderboard, agent, reasoning_effort, accuracy,
    accuracy_ci95_half_width, fetched_at
)
VALUES ($1, $2, $3, $4, $5, $6, now())
ON CONFLICT (model_id, leaderboard, agent, reasoning_effort) DO UPDATE SET
    accuracy = EXCLUDED.accuracy,
    accuracy_ci95_half_width = EXCLUDED.accuracy_ci95_half_width,
    fetched_at = now();

-- name: GetTerminalBenchScoresByModelIDs :many
SELECT model_id, leaderboard, agent, reasoning_effort, accuracy,
       accuracy_ci95_half_width, fetched_at
FROM terminal_bench_scores
WHERE model_id = ANY(sqlc.arg(model_ids)::text[])
ORDER BY model_id, leaderboard, agent, reasoning_effort;
