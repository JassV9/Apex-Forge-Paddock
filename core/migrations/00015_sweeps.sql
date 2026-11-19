-- Sensitivity discovery sweeps (docs/adr/0032-sensitivity-discovery-sweeps.md).
--
-- experiment.purpose: why an experiment was driven. improve (every experiment until now): the
-- champion follows the verdict. sweep: a step of a sweep, measured and never promoted; the
-- baseline goes back on the car whatever the verdict. recheck: the champion against the
-- original baseline (the existing re-checks are marked so; kind keeps saying the same).
--
-- sweep: one setting of one car swept step by step, each step its own experiment against one
-- baseline at one pinned AI hint. requested and planned are JSON arrays of clicks (planned is
-- requested clipped to the item's range), skipped says in words what clipping changed. curve
-- is the engine's classification once every step is measured.
--
-- sweep_step: the experiment of each step. One step per clicks value.
--
-- Down drops the sweeps and the purpose; the experiments themselves stay in the ledger.

-- +goose Up
ALTER TABLE experiment
    ADD COLUMN purpose TEXT NOT NULL DEFAULT 'improve'
        CHECK (purpose IN ('improve', 'sweep', 'recheck'));

UPDATE experiment SET purpose = 'recheck' WHERE kind = 'recheck';

CREATE TABLE sweep (
    id                  BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    session_id          BIGINT NOT NULL REFERENCES session (id),
    car_id              BIGINT NOT NULL REFERENCES car (id),
    track_id            BIGINT NOT NULL REFERENCES track (id),
    parameter           TEXT NOT NULL,
    label               TEXT NOT NULL,
    baseline_version_id BIGINT NOT NULL REFERENCES setup_version (id),
    ai_hint_share       DOUBLE PRECISION NOT NULL CHECK (ai_hint_share BETWEEN 0.5 AND 1.5),
    requested           JSONB NOT NULL,
    planned             JSONB NOT NULL,
    skipped             JSONB NOT NULL DEFAULT '[]',
    state               TEXT NOT NULL DEFAULT 'running'
        CHECK (state IN ('running', 'waiting', 'done', 'stopped', 'failed')),
    message             TEXT NOT NULL DEFAULT '',
    curve               JSONB,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- one sweep at a time per car: two would fight over the car
CREATE UNIQUE INDEX sweep_one_open_per_car ON sweep (car_id) WHERE state IN ('running', 'waiting');

CREATE TABLE sweep_step (
    sweep_id      BIGINT NOT NULL REFERENCES sweep (id),
    clicks        INTEGER NOT NULL CHECK (clicks <> 0),
    experiment_id BIGINT NOT NULL UNIQUE REFERENCES experiment (id),
    -- the step's response cells the curve uses (ADR 0031), NULL until they were read
    response      JSONB,
    added_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (sweep_id, clicks)
);

-- +goose Down
DROP TABLE sweep_step;
DROP TABLE sweep;
ALTER TABLE experiment DROP COLUMN purpose;
