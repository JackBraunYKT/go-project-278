-- name: ListLinkVisits :many
SELECT * FROM link_visits ORDER BY id LIMIT $1 OFFSET $2;
-- name: ListAllLinkVisits :many
SELECT * FROM link_visits ORDER BY id;
-- name: CountLinkVisits :one
SELECT COUNT(*) FROM link_visits;
-- name: CreateLinkVisit :one
INSERT INTO link_visits (link_id, ip, user_agent, status, referer) VALUES ($1, $2, $3, $4, $5) RETURNING *;
-- name: GetLastLinkVisitByLinkID :one
SELECT * FROM link_visits WHERE link_id = $1 ORDER BY created_at DESC LIMIT 1;
