-- +goose Up
CREATE TABLE links (
    id BIGSERIAL PRIMARY KEY,
    original_url TEXT NOT NULL,
    short_name TEXT NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- +goose Down
DROP TABLE links;
