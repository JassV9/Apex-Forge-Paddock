-- AI training programs (docs/adr/0020-ai-training-program.md): a queue of experiments that AC's
-- AI drives on a car one after another, each judged when its laps are in, the next one taken
-- from the ranking that the last verdict changed.

-- +goose Up
CREATE TABLE training_program (
    id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    session_id BIGINT NOT NULL REFERENCES session (id),
    car_id     BIGINT NOT NULL REFERENCES car (id),
    track_id   BIGINT NOT NULL REFERENCES track (id),
    target     INTEGER NOT NULL CHECK (target BETWEEN 1 AND 20),
    state      TEXT NOT NULL DEFAULT 'running'
        CHECK (state IN ('running', 'waiting', 'done', 'stopped')),
    message    TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- one program at a time per car: two would fight over the car
CREATE UNIQUE INDEX training_program_one_open_per_car
    ON training_program (car_id) WHERE state IN ('running', 'waiting');

CREATE TABLE training_step (
    program_id    BIGINT NOT NULL REFERENCES training_program (id),
    experiment_id BIGINT NOT NULL REFERENCES experiment (id),
    added_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (program_id, experiment_id)
);

-- +goose Down
DROP TABLE training_step;
DROP TABLE training_program;
