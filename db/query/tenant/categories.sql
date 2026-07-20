-- name: ListStudentCategories :many
SELECT * FROM student_categories
WHERE (sqlc.arg(filter)::text = '' OR name ILIKE '%'||sqlc.arg(filter)||'%' OR description ILIKE '%'||sqlc.arg(filter)||'%')
ORDER BY name ASC
LIMIT sqlc.arg(lim) OFFSET sqlc.arg(off);

-- name: CountStudentCategories :one
SELECT count(*) FROM student_categories
WHERE (sqlc.arg(filter)::text = '' OR name ILIKE '%'||sqlc.arg(filter)||'%' OR description ILIKE '%'||sqlc.arg(filter)||'%');

-- name: GetStudentCategory :one
SELECT * FROM student_categories WHERE id = $1;

-- name: CreateStudentCategory :one
INSERT INTO student_categories (name, description, status)
VALUES (sqlc.arg(name), sqlc.arg(description), sqlc.arg(status))
RETURNING *;

-- name: UpdateStudentCategory :one
UPDATE student_categories
SET name = sqlc.arg(name),
    description = sqlc.arg(description),
    status = sqlc.arg(status),
    updated_at = now()
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: DeleteStudentCategory :execrows
DELETE FROM student_categories WHERE id = $1;
