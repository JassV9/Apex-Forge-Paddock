-- Run context and AI provenance (docs/adr/0028-run-context-and-ai-provenance.md).
--
-- experiment.ai_hint_share: the share of the car's own AI tyre hint an AI experiment pins for
-- all its blocks; NULL drives on the share the agent's calibration learned.
--
-- corner_pass: what each pass was driven under, from the pass line's "context": the tyre hint
-- the AI was given (NULL when a human drove), the largest damage zone, the car's reset counter,
-- the share of samples with ABS, TC and DRS acting, and the clock of the t column.
--
-- run_context: one row per session with what its session line says (block_id NULL), and one
-- row per session and run block with the hint the experiment pinned and the range of hints its
-- passes were driven with. Ingest writes it; nothing changes a row's settings afterwards.
--
-- Down drops all of it; the spool files still hold every value.

-- +goose Up
ALTER TABLE experiment
    ADD COLUMN ai_hint_share DOUBLE PRECISION
        CHECK (ai_hint_share IS NULL OR ai_hint_share BETWEEN 0.5 AND 1.5);

ALTER TABLE corner_pass
    ADD COLUMN ai_hint       DOUBLE PRECISION,
    ADD COLUMN damage_max    DOUBLE PRECISION,
    ADD COLUMN reset_counter INTEGER,
    ADD COLUMN abs_share     DOUBLE PRECISION,
    ADD COLUMN tc_share      DOUBLE PRECISION,
    ADD COLUMN drs_share     DOUBLE PRECISION,
    ADD COLUMN clock         TEXT CHECK (clock IN ('physics', 'app'));

CREATE TABLE run_context (
    id                BIGSERIAL PRIMARY KEY,
    session_id        BIGINT NOT NULL REFERENCES session (id) ON DELETE CASCADE,
    block_id          BIGINT REFERENCES run_block (id) ON DELETE CASCADE,
    -- the session line (block_id NULL)
    csp_version       TEXT,
    csp_build         INTEGER,
    cars_count        INTEGER,
    ai_tyre_hint_base DOUBLE PRECISION,
    ai_learned_share  DOUBLE PRECISION,
    ai_applied_hint   DOUBLE PRECISION,
    ai_extra_grip     DOUBLE PRECISION,
    ai_aero_hint      DOUBLE PRECISION,
    ai_brake_hint     DOUBLE PRECISION,
    ai_level          DOUBLE PRECISION,
    ai_aggression     DOUBLE PRECISION,
    -- both: on the session line the share pinned when it started, on a block the experiment's
    ai_pinned_share   DOUBLE PRECISION,
    -- a block (block_id set): the hints its passes in this session were driven with
    ai_hint_min       DOUBLE PRECISION,
    ai_hint_max       DOUBLE PRECISION,
    passes            INTEGER NOT NULL DEFAULT 0,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE NULLS NOT DISTINCT (session_id, block_id)
);

CREATE INDEX run_context_block ON run_context (block_id) WHERE block_id IS NOT NULL;

-- +goose Down
DROP TABLE run_context;

ALTER TABLE corner_pass
    DROP COLUMN clock,
    DROP COLUMN drs_share,
    DROP COLUMN tc_share,
    DROP COLUMN abs_share,
    DROP COLUMN reset_counter,
    DROP COLUMN damage_max,
    DROP COLUMN ai_hint;

ALTER TABLE experiment DROP COLUMN ai_hint_share;
