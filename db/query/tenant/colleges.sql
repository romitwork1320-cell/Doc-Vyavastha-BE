-- name: ListColleges :many
SELECT * FROM colleges
WHERE deleted_at IS NULL
ORDER BY name;

-- name: GetCollege :one
SELECT * FROM colleges WHERE id = $1 AND deleted_at IS NULL;

-- name: CreateCollege :one
INSERT INTO colleges (tenant_id, name, created_by, updated_by)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: UpdateCollege :one
UPDATE colleges SET
    name = $2,
    updated_by = $3,
    updated_at = now()
WHERE id = $1 AND deleted_at IS NULL
RETURNING *;

-- name: SoftDeleteCollege :execrows
UPDATE colleges SET deleted_at = now(), updated_by = $2 WHERE id = $1 AND deleted_at IS NULL;
