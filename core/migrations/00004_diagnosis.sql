-- Phase 4: what a session's passes say about the car. Every diagnosis is kept: a later one
-- does not replace an earlier one, it is the newest row.

-- +goose Up
CREATE TABLE diagnosis (
    id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    session_id BIGINT      NOT NULL REFERENCES session (id) ON DELETE CASCADE,
    body       JSONB       NOT NULL,   -- contracts/diagnosis.schema.json, as the engine sent it
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX diagnosis_session_idx ON diagnosis (session_id, id DESC);

-- +goose Down
DROP TABLE diagnosis;
