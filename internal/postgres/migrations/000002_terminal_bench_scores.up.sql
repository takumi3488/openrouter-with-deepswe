CREATE TABLE terminal_bench_scores (
    model_id                 TEXT NOT NULL REFERENCES models (id) ON DELETE CASCADE,
    leaderboard              TEXT NOT NULL,
    agent                    TEXT NOT NULL,
    reasoning_effort         TEXT NOT NULL,
    accuracy                 DOUBLE PRECISION NOT NULL CHECK (accuracy >= 0 AND accuracy <= 100),
    accuracy_ci95_half_width DOUBLE PRECISION NOT NULL CHECK (accuracy_ci95_half_width >= 0 AND accuracy_ci95_half_width <= 100),
    fetched_at               TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (model_id, leaderboard, agent, reasoning_effort)
);
