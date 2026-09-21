CREATE TABLE deepswe_scores (
    model_id         TEXT NOT NULL REFERENCES models (id) ON DELETE CASCADE,
    harness          TEXT NOT NULL,
    reasoning_effort TEXT NOT NULL,
    pass_rate        DOUBLE PRECISION,
    pass_at_1        DOUBLE PRECISION,
    pass_at_4        DOUBLE PRECISION,
    n_passed         BIGINT,
    n_attempted      BIGINT,
    mean_cost_usd    DOUBLE PRECISION,
    fetched_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (model_id, harness, reasoning_effort)
);
