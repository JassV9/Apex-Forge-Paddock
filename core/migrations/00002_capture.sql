-- Phase 1: what the agent captures. Cars and tracks are created the first time a session
-- names them; a corner pass is the unit every later measurement is built from.

-- +goose Up
CREATE TABLE car (
    id                BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    ac_id             TEXT  NOT NULL UNIQUE,
    name              TEXT  NOT NULL,
    drivetrain        TEXT,
    mass_kg           DOUBLE PRECISION,
    weight_dist_front DOUBLE PRECISION,
    aero_class        TEXT,
    tyre_model        TEXT,
    features          JSONB NOT NULL DEFAULT '{}'
);

-- A new corner_map_version is a new row: corner numbers are only comparable within one map.
CREATE TABLE track (
    id                 BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    ac_id              TEXT    NOT NULL,
    layout             TEXT    NOT NULL DEFAULT '',
    name               TEXT    NOT NULL,
    length_m           DOUBLE PRECISION NOT NULL CHECK (length_m > 0),
    corner_map         JSONB   NOT NULL,
    corner_map_version INTEGER NOT NULL,
    UNIQUE (ac_id, layout, corner_map_version)
);

CREATE TABLE corner (
    id          BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    track_id    BIGINT  NOT NULL REFERENCES track (id) ON DELETE CASCADE,
    number      INTEGER NOT NULL CHECK (number >= 1),
    apex_s      DOUBLE PRECISION NOT NULL,  -- metres from the start line
    radius_m    DOUBLE PRECISION NOT NULL CHECK (radius_m > 0),
    speed_class TEXT,                       -- set by the engine once speeds are known
    UNIQUE (track_id, number)
);

CREATE TABLE session (
    id            BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    spool_id      TEXT        NOT NULL UNIQUE,  -- the agent's session id
    car_id        BIGINT      NOT NULL REFERENCES car (id),
    track_id      BIGINT      NOT NULL REFERENCES track (id),
    started_at    TIMESTAMPTZ NOT NULL,
    source        TEXT        NOT NULL CHECK (source IN ('human', 'ai', 'ghost', 'demo')),
    agent_version TEXT        NOT NULL,
    weather       JSONB       NOT NULL DEFAULT '{}',
    parquet_uri   TEXT        NOT NULL          -- directory of Parquet parts with raw samples
);

CREATE TABLE corner_pass (
    id             BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    session_id     BIGINT      NOT NULL REFERENCES session (id) ON DELETE CASCADE,
    corner_id      BIGINT      NOT NULL REFERENCES corner (id),
    seq            INTEGER     NOT NULL,
    t_start        TIMESTAMPTZ NOT NULL,
    duration_s     DOUBLE PRECISION NOT NULL CHECK (duration_s > 0),
    valid          BOOLEAN     NOT NULL DEFAULT TRUE,
    invalid_reason TEXT,
    fuel_l         DOUBLE PRECISION,
    tyre_core_c    DOUBLE PRECISION[],  -- FL, FR, RL, RR
    tyre_surface_c DOUBLE PRECISION[],
    tyre_press     DOUBLE PRECISION[],
    wear           DOUBLE PRECISION[],
    grip           DOUBLE PRECISION,
    air_c          DOUBLE PRECISION,
    road_c         DOUBLE PRECISION,
    lap_in_session INTEGER,
    setup_hash     TEXT,
    n_samples      INTEGER     NOT NULL DEFAULT 0,
    features       JSONB       NOT NULL DEFAULT '{}',
    -- ingest is idempotent on this key: replaying a spool file inserts nothing twice
    UNIQUE (session_id, corner_id, t_start)
);

CREATE INDEX corner_pass_session_idx ON corner_pass (session_id, seq);

-- Which spool files have been ingested, so the spool itself can stay read-only.
CREATE TABLE spool_chunk (
    spool_id    TEXT        NOT NULL,
    file_name   TEXT        NOT NULL,
    lines       INTEGER     NOT NULL,
    rejected    INTEGER     NOT NULL,
    ingested_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (spool_id, file_name)
);

-- +goose Down
DROP TABLE spool_chunk;
DROP TABLE corner_pass;
DROP TABLE session;
DROP TABLE corner;
DROP TABLE track;
DROP TABLE car;
