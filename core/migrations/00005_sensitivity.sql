-- Phase 5: what each setup item does on each car. The rows are derived: core rebuilds them
-- from the finished experiments' results, so a new method (model_version) can re-derive
-- every one. A row is either measured on the car or a prior borrowed from another car.

-- +goose Up
CREATE TABLE sensitivity_effect (
    id                  BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    car_id              BIGINT  NOT NULL REFERENCES car (id) ON DELETE CASCADE,
    parameter_id        BIGINT  NOT NULL REFERENCES setup_parameter (id) ON DELETE CASCADE,
    direction           TEXT    NOT NULL CHECK (direction IN ('increase', 'decrease')),  -- of the raw value
    clicks              INTEGER NOT NULL CHECK (clicks > 0),
    channel             TEXT    NOT NULL,
    phase               TEXT,
    corner_class        TEXT    NOT NULL DEFAULT 'all',
    estimate            DOUBLE PRECISION NOT NULL,   -- variant minus baseline, per corner pass
    ci_low              DOUBLE PRECISION NOT NULL,
    ci_high             DOUBLE PRECISION NOT NULL,
    n_passes            INTEGER NOT NULL,
    effect_class        TEXT    NOT NULL,
    nonlinear           BOOLEAN NOT NULL DEFAULT FALSE,
    note                TEXT    NOT NULL DEFAULT '',
    is_prior            BOOLEAN NOT NULL,
    prior_source_car_id BIGINT  REFERENCES car (id) ON DELETE CASCADE,
    experiment_ids      BIGINT[] NOT NULL DEFAULT '{}',
    model_version       TEXT    NOT NULL,
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    -- A prior always says whose it is, and a measurement never borrows.
    CHECK (is_prior = (prior_source_car_id IS NOT NULL)),
    UNIQUE (car_id, parameter_id, direction, clicks, channel, corner_class)
);

-- +goose Down
DROP TABLE sensitivity_effect;
