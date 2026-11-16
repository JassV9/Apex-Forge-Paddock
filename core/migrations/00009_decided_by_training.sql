-- A verdict a training program decides (docs/adr/0020-ai-training-program.md): keeping the
-- baseline after an unclear or a rejected change is the program's call, not the driver's, and
-- the ledger says who decided.
--
-- Down refuses (the CHECK cannot be added back) while such a verdict is in the ledger: the
-- ledger is never rewritten to make a rollback pass.

-- +goose Up
ALTER TABLE verdict DROP CONSTRAINT verdict_decided_by_check;
ALTER TABLE verdict
    ADD CONSTRAINT verdict_decided_by_check CHECK (decided_by IN ('engine', 'driver', 'training'));

-- +goose Down
ALTER TABLE verdict DROP CONSTRAINT verdict_decided_by_check;
ALTER TABLE verdict
    ADD CONSTRAINT verdict_decided_by_check CHECK (decided_by IN ('engine', 'driver'));
