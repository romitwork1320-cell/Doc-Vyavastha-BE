-- name: CreateMagicLink :one
INSERT INTO magic_links (
    token, tenant_id, application_id, client_id, expires_at
) VALUES (
    $1, $2, $3, $4, $5
) RETURNING *;

-- name: GetMagicLinkByToken :one
SELECT * FROM magic_links
WHERE token = $1;

-- name: InvalidateMagicLinksForApplication :exec
UPDATE magic_links
SET is_used = TRUE
WHERE application_id = $1 AND tenant_id = $2;

-- name: MarkMagicLinkAsUsed :exec
UPDATE magic_links
SET is_used = TRUE
WHERE token = $1;
