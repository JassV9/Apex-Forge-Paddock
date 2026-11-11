-- Long-running work that core hands to the engine (docs/adr/0006-no-kafka-no-kubernetes.md).
-- Workers claim rows with SELECT ... FOR UPDATE SKIP LOCKED; there is no message broker.

-- +goose Up
CREATE TABLE jobs (
    id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    kind       TEXT        NOT NULL,
    payload    JSONB       NOT NULL,
    state      TEXT        NOT NULL DEFAULT 'queued'
               CHECK (state IN ('queued', 'running', 'done', 'failed')),
    attempts   INTEGER     NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    error      TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Claiming the oldest queued job must not scan finished ones.
CREATE INDEX jobs_queued_idx ON jobs (created_at) WHERE state = 'queued';

-- +goose Down
DROP TABLE jobs;
