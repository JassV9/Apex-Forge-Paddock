-- The hybrid battery on each corner pass (docs/adr/0024-ers-state-per-pass.md): charge when the
-- pass started and ended (0..1) and the mean deployment over it, as CSP reports them. NULL on a
-- car without KERS/ERS and on every pass recorded before the agent sent them.
--
-- Down drops the columns and the values in them; the spool files still hold those values.

-- +goose Up
ALTER TABLE corner_pass
    ADD COLUMN ers_charge_start DOUBLE PRECISION,
    ADD COLUMN ers_charge_end   DOUBLE PRECISION,
    ADD COLUMN ers_deploy_mean  DOUBLE PRECISION;

-- +goose Down
ALTER TABLE corner_pass
    DROP COLUMN ers_deploy_mean,
    DROP COLUMN ers_charge_end,
    DROP COLUMN ers_charge_start;
