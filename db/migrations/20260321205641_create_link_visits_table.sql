-- +goose Up
CREATE TABLE link_visits (
  id BIGSERIAL PRIMARY KEY,
  link_id BIGINT NOT NULL REFERENCES links(id),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  ip TEXT NOT NULL,
  user_agent TEXT NOT NULL,
  referer TEXT NOT NULL,
  status INTEGER NOT NULL
);

-- +goose Down
DROP TABLE link_visits;
