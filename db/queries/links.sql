-- name: CreateLink :one
INSERT INTO links (original_url, short_name)
VALUES ($1, $2)
RETURNING id, original_url, short_name, created_at, updated_at;

-- name: GetLink :one
SELECT id, original_url, short_name, created_at, updated_at
FROM links
WHERE id = $1;

-- name: GetLinkByShortName :one
SELECT id, original_url, short_name, created_at, updated_at
FROM links
WHERE short_name = $1;

-- name: ListLinks :many
SELECT id, original_url, short_name, created_at, updated_at
FROM links
ORDER BY id;

-- name: ListLinksPage :many
SELECT id, original_url, short_name, created_at, updated_at
FROM links
ORDER BY id
LIMIT sqlc.arg(page_limit)::int
OFFSET sqlc.arg(page_offset)::int;

-- name: CountLinks :one
SELECT COUNT(*)
FROM links;

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

-- name: CreateLinkVisit :one
INSERT INTO link_visits (link_id, ip, user_agent, referer, status)
VALUES ($1, $2, $3, $4, $5)
RETURNING id, link_id, created_at, ip, user_agent, referer, status;

-- name: ListLinkVisits :many
SELECT id, link_id, created_at, ip, user_agent, referer, status
FROM link_visits
ORDER BY id;

-- name: ListLinkVisitsPage :many
SELECT id, link_id, created_at, ip, user_agent, referer, status
FROM link_visits
ORDER BY id
LIMIT sqlc.arg(page_limit)::int
OFFSET sqlc.arg(page_offset)::int;

-- name: CountLinkVisits :one
SELECT COUNT(*)
FROM link_visits;
