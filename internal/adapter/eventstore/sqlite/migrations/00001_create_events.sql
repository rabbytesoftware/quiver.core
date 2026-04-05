-- +goose Up
CREATE TABLE IF NOT EXISTS events (
    aggregate_id TEXT    NOT NULL,
    version      INTEGER NOT NULL,
    data         BLOB    NOT NULL,
    PRIMARY KEY (aggregate_id, version)
);

-- +goose Down
DROP TABLE events;
