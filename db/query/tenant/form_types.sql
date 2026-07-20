-- name: ListFormTypes :many
SELECT * FROM form_types
WHERE (sqlc.arg(filter)::text = '' OR name ILIKE '%'||sqlc.arg(filter)||'%')
ORDER BY display_order ASC, name ASC
LIMIT sqlc.arg(lim) OFFSET sqlc.arg(off);

-- name: CountFormTypes :one
SELECT count(*) FROM form_types
WHERE (sqlc.arg(filter)::text = '' OR name ILIKE '%'||sqlc.arg(filter)||'%');

-- name: GetFormType :one
SELECT * FROM form_types WHERE id = $1;

-- name: CreateFormType :one
INSERT INTO form_types (
    name, description, status, display_order
) VALUES (
    sqlc.arg(name), sqlc.arg(description), sqlc.arg(status), sqlc.arg(display_order)
) RETURNING *;

-- name: UpdateFormType :one
UPDATE form_types SET
    name = sqlc.arg(name),
    description = sqlc.arg(description),
    status = sqlc.arg(status),
    display_order = sqlc.arg(display_order),
    updated_at = now()
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: DeleteFormType :exec
DELETE FROM form_types WHERE id = $1;
