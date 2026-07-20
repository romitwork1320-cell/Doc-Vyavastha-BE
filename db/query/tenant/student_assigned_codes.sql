-- name: GetAssignedCodesByStudent :many
SELECT * FROM student_assigned_codes
WHERE student_id = $1
ORDER BY created_at ASC;

-- name: GetAssignedCodeByCategory :one
SELECT * FROM student_assigned_codes
WHERE student_id = $1 AND category_id = $2;

-- name: CreateAssignedCode :one
INSERT INTO student_assigned_codes (
    student_id, category_id, year_config_id, student_code
) VALUES (
    $1, $2, $3, $4
)
RETURNING *;

-- name: GetAssignedCodesByStudentIDs :many
SELECT * FROM student_assigned_codes
WHERE student_id = ANY(sqlc.arg(student_ids)::uuid[])
ORDER BY created_at ASC;
