-- Phase 3: setups as immutable versions, and experiments with their plan, results, verdict
-- and full history.

-- +goose Up
CREATE TABLE setup_parameter (
    id          BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    car_id      BIGINT  NOT NULL REFERENCES car (id) ON DELETE CASCADE,
    name        TEXT    NOT NULL,
    label       TEXT    NOT NULL,
    tab         TEXT    NOT NULL DEFAULT '',
    kind        TEXT,
    min         DOUBLE PRECISION NOT NULL,
    max         DOUBLE PRECISION NOT NULL,
    step        DOUBLE PRECISION NOT NULL CHECK (step > 0),
    shown_scale DOUBLE PRECISION NOT NULL DEFAULT 1,
    role        TEXT,
    axle        TEXT,
    excluded    BOOLEAN NOT NULL DEFAULT FALSE,
    UNIQUE (car_id, name)
);

-- Immutable: a version is its values. The same values on the same car are the same version.
CREATE TABLE setup_version (
    id           BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    car_id       BIGINT      NOT NULL REFERENCES car (id),
    content_hash TEXT        NOT NULL,               -- core's hash of the values
    agent_hash   TEXT,                               -- the agent's fingerprint, once it reports one
    parent_id    BIGINT      REFERENCES setup_version (id),
    "values"     JSONB       NOT NULL,
    file_name    TEXT,
    created_by   TEXT        NOT NULL,               -- 'agent' or 'experiment <id>'
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (car_id, content_hash)
);
CREATE INDEX setup_version_agent_hash_idx ON setup_version (car_id, agent_hash);

CREATE TABLE hypothesis (
    id           BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    car_id       BIGINT      NOT NULL REFERENCES car (id),
    track_id     BIGINT      NOT NULL REFERENCES track (id),
    statement    TEXT        NOT NULL,
    phase        TEXT,
    corner_class TEXT,
    evidence     JSONB       NOT NULL DEFAULT '[]',   -- metric ids
    alternatives JSONB       NOT NULL DEFAULT '[]',
    source       TEXT        NOT NULL CHECK (source IN ('rules', 'llm', 'human')),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE experiment (
    id                  BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    hypothesis_id       BIGINT      NOT NULL REFERENCES hypothesis (id),
    kind                TEXT        NOT NULL CHECK (kind IN ('ofat', 'factorial', 'recheck')),
    baseline_version_id BIGINT      NOT NULL REFERENCES setup_version (id),
    parameter_id        BIGINT      REFERENCES setup_parameter (id),  -- the one knob (ofat)
    clicks              INTEGER     NOT NULL DEFAULT 0,
    state               TEXT        NOT NULL CHECK (state IN (
        'proposed', 'planned', 'running', 'evaluating', 'accepted', 'rejected', 'tradeoff',
        'inconclusive', 'reverted', 'aborted', 'discarded')),
    success_test        JSONB       NOT NULL,          -- frozen once the experiment is planned
    guardrails          JSONB       NOT NULL,
    mde                 JSONB,                          -- the plan summary
    extensions          INTEGER     NOT NULL DEFAULT 0 CHECK (extensions BETWEEN 0 AND 2),
    plan_revision       INTEGER     NOT NULL DEFAULT 0,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE experiment_arm (
    id               BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    experiment_id    BIGINT  NOT NULL REFERENCES experiment (id) ON DELETE CASCADE,
    setup_version_id BIGINT  NOT NULL REFERENCES setup_version (id),
    label_code       TEXT    NOT NULL,                 -- what the driver sees: blind
    is_baseline      BOOLEAN NOT NULL,
    UNIQUE (experiment_id, label_code)
);

CREATE TABLE run_block (
    id               BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    experiment_id    BIGINT  NOT NULL REFERENCES experiment (id) ON DELETE CASCADE,
    arm_id           BIGINT  NOT NULL REFERENCES experiment_arm (id),
    "order"          INTEGER NOT NULL CHECK ("order" >= 1),
    laps_target      INTEGER NOT NULL CHECK (laps_target >= 0),
    condition_window JSONB   NOT NULL DEFAULT '{}',
    state            TEXT    NOT NULL DEFAULT 'queued',
    UNIQUE (experiment_id, "order")
);

CREATE TABLE prediction (
    id            BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    experiment_id BIGINT NOT NULL REFERENCES experiment (id) ON DELETE CASCADE,
    channel       TEXT   NOT NULL,
    phase         TEXT,
    corner_class  TEXT,
    direction     TEXT   NOT NULL CHECK (direction IN ('decrease', 'increase')),
    low           DOUBLE PRECISION NOT NULL,
    high          DOUBLE PRECISION NOT NULL
);

CREATE TABLE result (
    id            BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    experiment_id BIGINT  NOT NULL REFERENCES experiment (id) ON DELETE CASCADE,
    channel       TEXT    NOT NULL,
    role          TEXT    NOT NULL,
    phase         TEXT,
    corner_class  TEXT    NOT NULL DEFAULT 'all',
    estimate      DOUBLE PRECISION NOT NULL,
    ci_low        DOUBLE PRECISION NOT NULL,
    ci_high       DOUBLE PRECISION NOT NULL,
    n_passes      INTEGER NOT NULL,
    effect_class  TEXT    NOT NULL
);

CREATE TABLE verdict (
    id               BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    experiment_id    BIGINT      NOT NULL REFERENCES experiment (id) ON DELETE CASCADE,
    decision         TEXT        NOT NULL CHECK (decision IN (
        'accepted', 'rejected', 'tradeoff', 'inconclusive', 'reverted', 'aborted')),
    reason           TEXT        NOT NULL,
    prediction_hits  INTEGER     NOT NULL DEFAULT 0,
    prediction_total INTEGER     NOT NULL DEFAULT 0,
    decided_by       TEXT        NOT NULL CHECK (decided_by IN ('engine', 'driver')),
    decided_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Every state transition, in order: the ledger's audit trail.
CREATE TABLE experiment_event (
    id            BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    experiment_id BIGINT      NOT NULL REFERENCES experiment (id) ON DELETE CASCADE,
    from_state    TEXT        NOT NULL,
    to_state      TEXT        NOT NULL,
    reason        TEXT        NOT NULL,
    at            TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- The setup currently trusted for a car on a track, and how many experiments were accepted
-- since it was last re-checked against the original baseline.
CREATE TABLE champion (
    car_id                BIGINT  NOT NULL REFERENCES car (id),
    track_id              BIGINT  NOT NULL REFERENCES track (id),
    setup_version_id      BIGINT  NOT NULL REFERENCES setup_version (id),
    original_version_id   BIGINT  NOT NULL REFERENCES setup_version (id),
    accepted_since_check  INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (car_id, track_id)
);

ALTER TABLE corner_pass
    ADD COLUMN setup_version_id BIGINT REFERENCES setup_version (id),
    ADD COLUMN block_id         BIGINT REFERENCES run_block (id),
    ADD COLUMN block_lap        INTEGER;
CREATE INDEX corner_pass_block_idx ON corner_pass (block_id);

-- +goose Down
DROP INDEX corner_pass_block_idx;
ALTER TABLE corner_pass
    DROP COLUMN block_lap,
    DROP COLUMN block_id,
    DROP COLUMN setup_version_id;
DROP TABLE champion;
DROP TABLE experiment_event;
DROP TABLE verdict;
DROP TABLE result;
DROP TABLE prediction;
DROP TABLE run_block;
DROP TABLE experiment_arm;
DROP TABLE experiment;
DROP TABLE hypothesis;
DROP TABLE setup_version;
DROP TABLE setup_parameter;
