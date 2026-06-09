-- name: ListLinks :many
SELECT * FROM links ORDER BY id LIMIT $1 OFFSET $2;
-- name: ListAllLinks :many
SELECT * FROM links ORDER BY id;
-- name: LinkByShortName :one
SELECT * FROM links WHERE short_name = $1;
-- name: CountLinks :one
SELECT COUNT(*) FROM links;
-- name: ShortNameExists :one
SELECT EXISTS (
  SELECT 1
  FROM links
  WHERE short_name = $1
);
-- name: GetLink :one
SELECT * FROM links WHERE id = $1;
-- name: CreateLink :one
INSERT INTO links (original_url, short_name, short_url) VALUES ($1, $2, $3) RETURNING *;
-- name: UpdateLink :execrows
UPDATE links SET original_url = $2, short_name = $3, short_url = $4 WHERE id = $1 RETURNING *;
-- name: DeleteLink :execrows
DELETE FROM links WHERE id = $1;

