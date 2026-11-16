-- A base setup is judged as its own experiment: the baseline against several measured
-- changes at once (docs/adr/0018-base-setup.md). It needs its own kind.
--
-- Down refuses (the CHECK cannot be added back) while a base experiment is in the ledger:
-- failed experiments are kept, so nothing is deleted to make the rollback pass.

-- +goose Up
ALTER TABLE experiment DROP CONSTRAINT experiment_kind_check;
ALTER TABLE experiment
    ADD CONSTRAINT experiment_kind_check CHECK (kind IN ('ofat', 'factorial', 'recheck', 'base'));

-- +goose Down
ALTER TABLE experiment DROP CONSTRAINT experiment_kind_check;
ALTER TABLE experiment
    ADD CONSTRAINT experiment_kind_check CHECK (kind IN ('ofat', 'factorial', 'recheck'));
