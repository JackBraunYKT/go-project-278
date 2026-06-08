-- name: CreateLink :one
INSERT INTO links (original_url, short_name)
VALUES ($1, $2)
RETURNING id, original_url, short_name, created_at, updated_at;

-- name: GetLink :one
SELECT id, original_url, short_name, created_at, updated_at
FROM links
WHERE id = $1;

-- name: ListLinks :many
SELECT id, original_url, short_name, created_at, updated_at
FROM links
ORDER BY id;

-- name: UpdateLink :one
UPDATE links
SET original_url = $2,
    short_name = $3,
    updated_at = NOW()
WHERE id = $1
RETURNING id, original_url, short_name, created_at, updated_at;

-- name: DeleteLink :execrows
DELETE FROM links
WHERE id = $1;
