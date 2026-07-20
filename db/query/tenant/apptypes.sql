-- name: ListApplicationTypes :many
SELECT * FROM application_types
WHERE (sqlc.arg(filter)::text = '' OR name ILIKE '%'||sqlc.arg(filter)||'%' OR description ILIKE '%'||sqlc.arg(filter)||'%')
ORDER BY name ASC
LIMIT sqlc.arg(lim) OFFSET sqlc.arg(off);

-- name: CountApplicationTypes :one
SELECT count(*) FROM application_types
WHERE (sqlc.arg(filter)::text = '' OR name ILIKE '%'||sqlc.arg(filter)||'%' OR description ILIKE '%'||sqlc.arg(filter)||'%');

-- name: GetApplicationType :one
SELECT * FROM application_types WHERE id = $1;

-- name: CreateApplicationType :one
INSERT INTO application_types (name, description, status)
VALUES (sqlc.arg(name), sqlc.arg(description), sqlc.arg(status))
RETURNING *;

-- name: UpdateApplicationType :one
UPDATE application_types
SET name = sqlc.arg(name), description = sqlc.arg(description), status = sqlc.arg(status), updated_at = now()
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: DeleteApplicationType :execrows
DELETE FROM application_types WHERE id = $1;

-- name: CheckApplicationTypeInUse :one
SELECT EXISTS (
  SELECT 1 FROM student_applications 
  WHERE application_type_id = $1 AND deleted_at IS NULL
);
