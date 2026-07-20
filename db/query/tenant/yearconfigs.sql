-- name: ListStudentCodeConfigs :many
SELECT * FROM student_code_year_configs
WHERE (sqlc.arg(filter)::text = '' OR prefix ILIKE '%'||sqlc.arg(filter)||'%')
ORDER BY business_year DESC, prefix ASC
LIMIT sqlc.arg(lim) OFFSET sqlc.arg(off);

-- name: CountStudentCodeConfigs :one
SELECT count(*) FROM student_code_year_configs
WHERE (sqlc.arg(filter)::text = '' OR prefix ILIKE '%'||sqlc.arg(filter)||'%');

-- name: GetStudentCodeConfig :one
SELECT * FROM student_code_year_configs WHERE id = $1;

-- name: GetActiveConfigByCategory :one
SELECT * FROM student_code_year_configs
WHERE category_id = $1 AND is_active = TRUE
ORDER BY business_year DESC
LIMIT 1;

-- name: GetActiveConfigByCategoryYear :one
SELECT * FROM student_code_year_configs
WHERE category_id = $1 AND business_year = $2 AND is_active = TRUE
LIMIT 1;

-- name: CreateStudentCodeConfig :one
INSERT INTO student_code_year_configs
    (category_id, business_year, prefix, separator, padding_length, reset_sequence, is_active)
VALUES
    (sqlc.arg(category_id), sqlc.arg(business_year), sqlc.arg(prefix), sqlc.arg(separator),
     sqlc.arg(padding_length), sqlc.arg(reset_sequence), sqlc.arg(is_active))
RETURNING *;

-- name: UpdateStudentCodeConfig :one
UPDATE student_code_year_configs
SET category_id = sqlc.arg(category_id),
    business_year = sqlc.arg(business_year),
    prefix = sqlc.arg(prefix),
    separator = sqlc.arg(separator),
    padding_length = sqlc.arg(padding_length),
    reset_sequence = sqlc.arg(reset_sequence),
    is_active = sqlc.arg(is_active),
    updated_at = now()
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: DeleteStudentCodeConfig :execrows
DELETE FROM student_code_year_configs WHERE id = $1;

-- name: CheckYearConfigInUse :one
SELECT EXISTS(SELECT 1 FROM students WHERE year_config_id = $1::uuid);
