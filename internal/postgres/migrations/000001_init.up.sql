CREATE TABLE models (
    id                TEXT PRIMARY KEY,
    canonical_slug    TEXT NOT NULL,
    name              TEXT NOT NULL,
    released_at       TIMESTAMPTZ NOT NULL,
    context_length    BIGINT NOT NULL DEFAULT 0,
    cheapest_provider TEXT,
    prompt_price      NUMERIC(24, 12) NOT NULL,
    completion_price  NUMERIC(24, 12) NOT NULL,
    favorite          BOOLEAN NOT NULL DEFAULT FALSE,
    hidden            BOOLEAN NOT NULL DEFAULT FALSE,
    last_seen_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

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
