-- +goose Up
CREATE TABLE IF NOT EXISTS arrows (
    namespace TEXT    PRIMARY KEY,
    manifest  TEXT    NOT NULL,
    removed   INTEGER NOT NULL DEFAULT 0
);

-- +goose Down
DROP TABLE arrows;
