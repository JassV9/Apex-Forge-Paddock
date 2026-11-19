-- What the car did, stored with the verdict that measured it (docs/adr/0034-vehicle-response-in-
-- verdicts.md). The engine judges the cells the changed parameter is expected to move (metric,
-- frozen phase, speed regime); each becomes a result row of role 'response', appended under its
-- verdict like every other result (ADR 0030). The regime is corner_class (per corner, frozen from
-- the baseline, ADR 0031); the phase is the existing phase column.
--
-- result gains the response finding, whether the cell is a direct measurement or a proxy, and the
-- margin it was judged against. A response row always has all three; no other row has a finding.
--
-- current_result keeps meaning what the ledger shows and pools: the time, guardrail, driver and
-- prediction results of the latest verdict with results. Response rows are left out of it, so a
-- reader that pools channels never mixes in per-phase cells; current_response is their view.
--
-- Down deletes the response rows, drops current_response and the columns, then puts
-- current_result back as 00012 made it (replaced in place, so a view built on it survives).

-- +goose Up
ALTER TABLE result
    ADD COLUMN response_finding TEXT CHECK (response_finding IN (
        'increase', 'decrease', 'neutral', 'likely_increase', 'likely_decrease', 'insufficient')),
    ADD COLUMN expected TEXT CHECK (expected IN ('direct', 'proxy')),
    ADD COLUMN margin   DOUBLE PRECISION CHECK (margin > 0),
    ADD CONSTRAINT result_response_complete CHECK (
        (role = 'response') = (response_finding IS NOT NULL)
        AND (role = 'response') = (expected IS NOT NULL)
        AND (role = 'response') = (margin IS NOT NULL));

CREATE OR REPLACE VIEW current_result AS
SELECT r.id, r.experiment_id, r.channel, r.role, r.phase, r.corner_class, r.estimate, r.ci_low,
       r.ci_high, r.n_passes, r.effect_class, r.verdict_id, r.engine_version
FROM result r
WHERE r.role <> 'response'
  AND r.verdict_id IS NOT DISTINCT FROM (
    SELECT max(latest.verdict_id) FROM result latest WHERE latest.experiment_id = r.experiment_id);

-- The response findings of each experiment's latest verdict with results.
CREATE VIEW current_response AS
SELECT r.*
FROM result r
WHERE r.role = 'response'
  AND r.verdict_id = (
    SELECT max(latest.verdict_id) FROM result latest WHERE latest.experiment_id = r.experiment_id);

-- +goose Down
DROP VIEW current_response;
DELETE FROM result WHERE role = 'response';
ALTER TABLE result
    DROP CONSTRAINT result_response_complete,
    DROP COLUMN margin,
    DROP COLUMN expected,
    DROP COLUMN response_finding;
CREATE OR REPLACE VIEW current_result AS
SELECT r.*
FROM result r
WHERE r.verdict_id IS NOT DISTINCT FROM (
    SELECT max(latest.verdict_id) FROM result latest WHERE latest.experiment_id = r.experiment_id);
