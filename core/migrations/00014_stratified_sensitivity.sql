-- A stratified vehicle sensitivity profile with provenance, priors apart
-- (docs/adr/0033-stratified-vehicle-sensitivity-profile.md). 00013 and 00015 are taken by changes
-- written in parallel.
--
-- sensitivity_derivation / sensitivity_measurement: each finished one-item experiment re-derived
-- once per engine version (POST /sensitivity/measure): its finding and, per pooled channel and
-- speed class, the block-level estimate with its standard error and degrees of freedom. Append
-- only; a failed derivation is kept with its reason so it is not retried on every read.
--
-- sensitivity_effect: the car's own pooled rows only. The stratum (track, driver, AI hint bucket,
-- baseline value, click size, phase), the knowledge level, the pooling method and the provenance
-- (experiments, engine versions, stale) are columns; is_prior and its lender are gone, and a row
-- must name its experiments. The rows are derived, so Up deletes them: the next read rebuilds
-- them with the strata.
--
-- sensitivity_prior: another car's effect lent to this car. No effect_class and no knowledge
-- level, so nothing can read it as a measurement.
--
-- sensitivity_profile: per car, a hash of the last pooled reply; a read writes the car's rows only
-- when it changed.
--
-- Down drops the new tables and columns and restores is_prior; the rows are derived and are
-- deleted both ways (an older core rebuilds them on its next read).

-- +goose Up
CREATE TABLE sensitivity_derivation (
    id             BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    experiment_id  BIGINT NOT NULL REFERENCES experiment (id) ON DELETE CASCADE,
    engine_version TEXT   NOT NULL,
    status         TEXT   NOT NULL CHECK (status IN ('derived', 'failed')),
    finding_class  TEXT CHECK (finding_class IN (
        'invalid', 'confirmed_improvement', 'confirmed_regression', 'neutral',
        'context_dependent', 'likely_improvement', 'likely_regression', 'insufficient_evidence')),
    reason         TEXT   NOT NULL DEFAULT '',
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (status = 'failed' OR finding_class IS NOT NULL),
    UNIQUE (experiment_id, engine_version)
);

CREATE TABLE sensitivity_measurement (
    id             BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    derivation_id  BIGINT NOT NULL REFERENCES sensitivity_derivation (id) ON DELETE CASCADE,
    channel        TEXT   NOT NULL,
    corner_class   TEXT   NOT NULL CHECK (corner_class IN ('all', 'slow', 'medium', 'fast')),
    phase          TEXT   NOT NULL DEFAULT 'corner',
    estimate       DOUBLE PRECISION NOT NULL,
    standard_error DOUBLE PRECISION NOT NULL CHECK (standard_error >= 0),
    df             INTEGER NOT NULL CHECK (df >= 1),
    n_passes       INTEGER NOT NULL CHECK (n_passes >= 0),
    blocks         INTEGER NOT NULL CHECK (blocks >= 0),
    corners        INTEGER NOT NULL CHECK (corners >= 0),
    UNIQUE (derivation_id, channel, corner_class, phase)
);

DELETE FROM sensitivity_effect;
ALTER TABLE sensitivity_effect
    DROP CONSTRAINT sensitivity_effect_check,
    DROP CONSTRAINT sensitivity_effect_car_id_parameter_id_direction_clicks_cha_key,
    DROP COLUMN prior_source_car_id,
    DROP COLUMN is_prior,
    ADD COLUMN track           TEXT NOT NULL,
    ADD COLUMN track_name      TEXT NOT NULL DEFAULT '',
    ADD COLUMN driver          TEXT NOT NULL CHECK (driver IN ('ai', 'human')),
    ADD COLUMN hint_bucket     TEXT NOT NULL,
    ADD COLUMN baseline_value  DOUBLE PRECISION,
    ADD COLUMN click_step      DOUBLE PRECISION CHECK (click_step > 0),
    ADD COLUMN knowledge_level TEXT NOT NULL
        CHECK (knowledge_level IN ('confirmed', 'preliminary', 'neutral', 'insufficient')),
    ADD COLUMN method          TEXT NOT NULL CHECK (method IN ('single', 'half_normal', 'hksj')),
    ADD COLUMN experiments     INTEGER NOT NULL CHECK (experiments >= 1),
    ADD COLUMN standard_error  DOUBLE PRECISION NOT NULL CHECK (standard_error >= 0),
    ADD COLUMN p_increase      DOUBLE PRECISION NOT NULL CHECK (p_increase BETWEEN 0 AND 1),
    ADD COLUMN pi_low          DOUBLE PRECISION,
    ADD COLUMN pi_high         DOUBLE PRECISION,
    ADD COLUMN tau             DOUBLE PRECISION CHECK (tau >= 0),
    ADD COLUMN margin          DOUBLE PRECISION CHECK (margin > 0),
    ADD COLUMN stale           BOOLEAN NOT NULL,
    ADD COLUMN engine_versions TEXT[] NOT NULL DEFAULT '{}',
    -- a measured row names its experiments; a prior is not a row of this table
    ADD CONSTRAINT sensitivity_effect_measured CHECK (cardinality(experiment_ids) > 0),
    ADD CONSTRAINT sensitivity_effect_stratum UNIQUE NULLS NOT DISTINCT (car_id, parameter_id,
        direction, clicks, channel, corner_class, phase, track, driver, hint_bucket,
        baseline_value, click_step);

CREATE TABLE sensitivity_prior (
    id                 BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    car_id             BIGINT NOT NULL REFERENCES car (id) ON DELETE CASCADE,
    parameter_id       BIGINT NOT NULL REFERENCES setup_parameter (id) ON DELETE CASCADE,
    direction          TEXT   NOT NULL CHECK (direction IN ('increase', 'decrease')),
    clicks             INTEGER NOT NULL CHECK (clicks > 0),
    channel            TEXT   NOT NULL,
    corner_class       TEXT   NOT NULL,
    phase              TEXT   NOT NULL,
    estimate           DOUBLE PRECISION NOT NULL,
    ci_low             DOUBLE PRECISION NOT NULL,
    ci_high            DOUBLE PRECISION NOT NULL,
    standard_error     DOUBLE PRECISION NOT NULL CHECK (standard_error >= 0),
    source_car_id      BIGINT NOT NULL REFERENCES car (id) ON DELETE CASCADE,
    source_track       TEXT   NOT NULL,
    source_experiments INTEGER NOT NULL CHECK (source_experiments >= 1),
    weight             DOUBLE PRECISION NOT NULL CHECK (weight > 0 AND weight <= 1),
    note               TEXT   NOT NULL,
    model_version      TEXT   NOT NULL,
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (source_car_id <> car_id),
    UNIQUE (car_id, parameter_id, direction, clicks, channel, corner_class, phase)
);

CREATE TABLE sensitivity_profile (
    car_id        BIGINT PRIMARY KEY REFERENCES car (id) ON DELETE CASCADE,
    inputs_hash   TEXT NOT NULL,
    model_version TEXT NOT NULL,
    built_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE sensitivity_profile;
DROP TABLE sensitivity_prior;
DELETE FROM sensitivity_effect;
ALTER TABLE sensitivity_effect
    DROP CONSTRAINT sensitivity_effect_stratum,
    DROP CONSTRAINT sensitivity_effect_measured,
    DROP COLUMN engine_versions,
    DROP COLUMN stale,
    DROP COLUMN margin,
    DROP COLUMN tau,
    DROP COLUMN pi_high,
    DROP COLUMN pi_low,
    DROP COLUMN p_increase,
    DROP COLUMN standard_error,
    DROP COLUMN experiments,
    DROP COLUMN method,
    DROP COLUMN knowledge_level,
    DROP COLUMN click_step,
    DROP COLUMN baseline_value,
    DROP COLUMN hint_bucket,
    DROP COLUMN driver,
    DROP COLUMN track_name,
    DROP COLUMN track,
    ADD COLUMN is_prior BOOLEAN NOT NULL,
    ADD COLUMN prior_source_car_id BIGINT REFERENCES car (id) ON DELETE CASCADE,
    ADD CONSTRAINT sensitivity_effect_check CHECK (is_prior = (prior_source_car_id IS NOT NULL)),
    ADD CONSTRAINT sensitivity_effect_car_id_parameter_id_direction_clicks_cha_key
        UNIQUE (car_id, parameter_id, direction, clicks, channel, corner_class);
DROP TABLE sensitivity_measurement;
DROP TABLE sensitivity_derivation;
