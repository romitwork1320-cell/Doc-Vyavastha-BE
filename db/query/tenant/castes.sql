-- name: ListCastes :many
SELECT * FROM student_castes
WHERE (sqlc.arg(filter)::text = '' OR name ILIKE '%'||sqlc.arg(filter)||'%')
ORDER BY created_at DESC
LIMIT sqlc.arg(lim) OFFSET sqlc.arg(off);

-- name: CountCastes :one
SELECT count(*) FROM student_castes
WHERE (sqlc.arg(filter)::text = '' OR name ILIKE '%'||sqlc.arg(filter)||'%');

-- name: GetCaste :one
SELECT * FROM student_castes WHERE id = $1;

-- name: CreateCaste :one
INSERT INTO student_castes (
    name, description, status
) VALUES (
    $1, $2, $3
)
RETURNING *;

-- name: UpdateCaste :one
UPDATE student_castes SET
    name = $2,
    description = $3,
    status = $4,
    updated_at = now()
WHERE id = $1
RETURNING *;

-- name: DeleteCaste :execrows
DELETE FROM student_castes WHERE id = $1;
