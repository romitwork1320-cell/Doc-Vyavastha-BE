-- name: ListApplicationStatuses :many
SELECT * FROM application_statuses
WHERE (sqlc.arg(filter)::text = '' OR name ILIKE '%'||sqlc.arg(filter)||'%')
ORDER BY display_order ASC, name ASC
LIMIT sqlc.arg(lim) OFFSET sqlc.arg(off);

-- name: CountApplicationStatuses :one
SELECT count(*) FROM application_statuses
WHERE (sqlc.arg(filter)::text = '' OR name ILIKE '%'||sqlc.arg(filter)||'%');

-- name: GetApplicationStatus :one
SELECT * FROM application_statuses WHERE id = $1;

-- name: CreateApplicationStatus :one
INSERT INTO application_statuses (name, description, color_code, status, display_order)
VALUES (sqlc.arg(name), sqlc.arg(description), sqlc.arg(color_code), sqlc.arg(status), sqlc.arg(display_order))
RETURNING *;

-- name: UpdateApplicationStatus :one
UPDATE application_statuses
SET name = sqlc.arg(name),
    description = sqlc.arg(description),
    color_code = sqlc.arg(color_code),
    status = sqlc.arg(status),
    display_order = sqlc.arg(display_order),
    updated_at = now()
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: DeleteApplicationStatus :execrows
DELETE FROM application_statuses WHERE id = $1;

-- name: CheckApplicationStatusInUse :one
SELECT EXISTS (
  SELECT 1 FROM student_applications 
  WHERE application_status_id = $1 AND deleted_at IS NULL
);
