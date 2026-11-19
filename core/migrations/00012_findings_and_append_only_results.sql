-- What an experiment found, beside what the ledger decided (docs/adr/0029-block-level-inference-
-- and-finding-classes.md), and results kept for every verdict with the engine version that
-- computed them (docs/adr/0030-append-only-results-and-engine-version.md). 00011 is taken by a
-- change written in parallel.
--
-- verdict: the finding class and what to do next, the confounders and flags, whether the driver
-- adapted, the look, the margin and the passes behind the numbers. NULL on every verdict judged
-- before this migration and on the driver's and the training program's decisions; the view
-- verdict_finding derives a finding for old engine verdicts from their decision and reason.
--
-- result: rows are appended per engine verdict (verdict_id) and never replaced. Rows from before
-- this migration have no verdict_id; they are an experiment's current result until a verdict with
-- results of its own is added. current_result is what the ledger shows and pools.
--
-- Down drops the new columns and views. Results superseded by a later verdict are deleted first,
-- so the table means what it meant before (one set per experiment, the latest). That history
-- exists only since this migration.

-- +goose Up
ALTER TABLE verdict
    ADD COLUMN finding_class TEXT CHECK (finding_class IN (
        'invalid', 'confirmed_improvement', 'confirmed_regression', 'neutral',
        'context_dependent', 'likely_improvement', 'likely_regression', 'insufficient_evidence')),
    ADD COLUMN recommendation TEXT CHECK (recommendation IN (
        'continue', 'larger_perturbation', 'investigate_context', 'switch_metric',
        'stop_neutral', 'stop_invalid')),
    ADD COLUMN flags           TEXT[] NOT NULL DEFAULT '{}',
    ADD COLUMN confounders     JSONB,
    ADD COLUMN driver_adapted  BOOLEAN,
    ADD COLUMN engine_version  TEXT,
    ADD COLUMN passes_used     INTEGER CHECK (passes_used >= 0),
    ADD COLUMN passes_excluded INTEGER CHECK (passes_excluded >= 0),
    ADD COLUMN look            INTEGER CHECK (look BETWEEN 0 AND 3),
    ADD COLUMN margin          DOUBLE PRECISION CHECK (margin >= 0);

ALTER TABLE result
    ADD COLUMN verdict_id     BIGINT REFERENCES verdict (id) ON DELETE CASCADE,
    ADD COLUMN engine_version TEXT;
CREATE INDEX result_verdict_idx ON result (experiment_id, verdict_id);

-- The results the ledger shows: those of the experiment's latest verdict that has any, or the
-- rows from before verdicts carried their own.
CREATE VIEW current_result AS
SELECT r.*
FROM result r
WHERE r.verdict_id IS NOT DISTINCT FROM (
    SELECT max(latest.verdict_id) FROM result latest WHERE latest.experiment_id = r.experiment_id);

-- Every verdict with a finding: its own, or for an engine verdict judged before ADR 0029 the one
-- its decision and reason imply (derived = true; another method judged it). The driver's and the
-- program's decisions have none.
CREATE VIEW verdict_finding AS
SELECT v.id, v.experiment_id, v.decided_by,
       COALESCE(v.finding_class, CASE
           WHEN v.decided_by <> 'engine' THEN NULL
           WHEN v.decision IN ('accepted', 'tradeoff') THEN 'confirmed_improvement'
           WHEN v.decision = 'rejected' AND v.reason LIKE 'Worse on %' THEN 'confirmed_regression'
           WHEN v.decision = 'rejected' AND v.reason LIKE 'No meaningful effect%' THEN 'neutral'
           WHEN v.decision = 'inconclusive' AND v.reason LIKE 'Not enough passes%' THEN 'invalid'
           WHEN v.decision IN ('rejected', 'inconclusive') THEN 'insufficient_evidence'
       END) AS finding_class,
       v.finding_class IS NULL AND v.decided_by = 'engine' AS derived
FROM verdict v;

-- +goose Down
DROP VIEW verdict_finding;
DROP VIEW current_result;
DELETE FROM result r
WHERE r.verdict_id IS DISTINCT FROM (
    SELECT max(latest.verdict_id) FROM result latest WHERE latest.experiment_id = r.experiment_id);
DROP INDEX result_verdict_idx;
ALTER TABLE result
    DROP COLUMN engine_version,
    DROP COLUMN verdict_id;
ALTER TABLE verdict
    DROP COLUMN margin,
    DROP COLUMN look,
    DROP COLUMN passes_excluded,
    DROP COLUMN passes_used,
    DROP COLUMN engine_version,
    DROP COLUMN driver_adapted,
    DROP COLUMN confounders,
    DROP COLUMN flags,
    DROP COLUMN recommendation,
    DROP COLUMN finding_class;
