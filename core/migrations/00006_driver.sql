-- Who drives an experiment's laps: the human, or AC's AI on the player's car (Ghost Driver,
-- docs/adr/0014-ghost-driver.md). Existing experiments were all driven by the human.

-- +goose Up
ALTER TABLE experiment
    ADD COLUMN driver TEXT NOT NULL DEFAULT 'human' CHECK (driver IN ('human', 'ai'));

-- +goose Down
ALTER TABLE experiment DROP COLUMN driver;
