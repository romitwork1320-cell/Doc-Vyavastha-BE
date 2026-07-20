-- name: ListStudentCodeSequences :many
SELECT * FROM student_code_sequences
ORDER BY updated_at DESC
LIMIT sqlc.arg(lim) OFFSET sqlc.arg(off);

-- name: CountStudentCodeSequences :one
SELECT count(*) FROM student_code_sequences;

-- name: GetStudentCodeSequence :one
SELECT * FROM student_code_sequences WHERE id = $1;

-- name: GetSequenceByYearConfig :one
SELECT * FROM student_code_sequences WHERE year_config_id = $1;

-- name: EnsureSequenceExists :exec
INSERT INTO student_code_sequences (year_config_id, current_number)
VALUES ($1, 0)
ON CONFLICT (year_config_id) DO NOTHING;

-- name: GetSequenceByYearConfigForUpdate :one
SELECT * FROM student_code_sequences
WHERE year_config_id = $1
FOR UPDATE;

-- name: SetSequenceValue :one
UPDATE student_code_sequences
SET current_number = sqlc.arg(current_number),
    last_generated_code = sqlc.arg(last_generated_code),
    updated_at = now()
WHERE year_config_id = sqlc.arg(year_config_id)
RETURNING *;

-- name: CreateStudentCodeSequence :one
INSERT INTO student_code_sequences (year_config_id, current_number, last_generated_code)
VALUES (sqlc.arg(year_config_id), sqlc.arg(current_number), sqlc.arg(last_generated_code))
RETURNING *;

-- name: UpdateStudentCodeSequence :one
UPDATE student_code_sequences
SET current_number = sqlc.arg(current_number),
    last_generated_code = sqlc.arg(last_generated_code),
    updated_at = now()
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: DeleteStudentCodeSequence :execrows
DELETE FROM student_code_sequences WHERE id = $1;
