-- +goose Up
CREATE TABLE IF NOT EXISTS port_allocations (
    port      INTEGER PRIMARY KEY,
    protocol  TEXT    NOT NULL,
    owner_key TEXT    NOT NULL,
    forwarded INTEGER NOT NULL DEFAULT 0
);

-- +goose Down
DROP TABLE port_allocations;
