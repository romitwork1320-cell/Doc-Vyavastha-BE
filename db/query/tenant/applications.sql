-- name: ListStudentApplications :many
SELECT a.* FROM student_applications a
WHERE a.deleted_at IS NULL
  AND a.branch_id = sqlc.arg(branch_id)
  AND (sqlc.arg(filter)::text = ''
       OR a.application_name ILIKE '%'||sqlc.arg(filter)||'%'
       OR a.application_number ILIKE '%'||sqlc.arg(filter)||'%'
       OR EXISTS (
           SELECT 1 FROM student_application_colleges sac 
           JOIN colleges c ON sac.college_id = c.id
           WHERE sac.application_id = a.id AND c.name ILIKE '%'||sqlc.arg(filter)||'%'
       ))
ORDER BY a.created_at DESC
LIMIT sqlc.arg(lim) OFFSET sqlc.arg(off);

-- name: CountStudentApplications :one
SELECT count(*) FROM student_applications a
WHERE a.deleted_at IS NULL
  AND a.branch_id = sqlc.arg(branch_id)
  AND (sqlc.arg(filter)::text = ''
       OR a.application_name ILIKE '%'||sqlc.arg(filter)||'%'
       OR a.application_number ILIKE '%'||sqlc.arg(filter)||'%'
       OR EXISTS (
           SELECT 1 FROM student_application_colleges sac 
           JOIN colleges c ON sac.college_id = c.id
           WHERE sac.application_id = a.id AND c.name ILIKE '%'||sqlc.arg(filter)||'%'
       ));

-- name: GetStudentApplication :one
SELECT * FROM student_applications WHERE id = $1 AND deleted_at IS NULL;

-- name: GetApplicationsByStudent :many
SELECT * FROM student_applications
WHERE student_id = $1 AND branch_id = $2 AND deleted_at IS NULL
ORDER BY created_at DESC;

-- name: CreateStudentApplication :one
INSERT INTO student_applications (
    student_id, branch_id, application_type_id, application_status_id,
    application_number, application_name, last_date, applied_date, submitted_date,
    form_type_id, portal_username, portal_password, remarks, created_by, updated_by
) VALUES (
    sqlc.arg(student_id), sqlc.arg(branch_id), sqlc.arg(application_type_id), sqlc.arg(application_status_id),
    sqlc.arg(application_number), sqlc.arg(application_name), sqlc.arg(last_date), sqlc.arg(applied_date), sqlc.arg(submitted_date),
    sqlc.arg(form_type_id), sqlc.arg(portal_username), sqlc.arg(portal_password), sqlc.arg(remarks), sqlc.arg(created_by), sqlc.arg(updated_by)
)
RETURNING *;

-- name: UpdateStudentApplication :one
UPDATE student_applications SET
    application_type_id = sqlc.arg(application_type_id),
    application_status_id = sqlc.arg(application_status_id),
    application_number = sqlc.arg(application_number),
    application_name = sqlc.arg(application_name),
    last_date = sqlc.arg(last_date),
    applied_date = sqlc.arg(applied_date),
    submitted_date = sqlc.arg(submitted_date),
    form_type_id = sqlc.arg(form_type_id),
    portal_username = sqlc.arg(portal_username),
    portal_password = sqlc.arg(portal_password),
    remarks = sqlc.arg(remarks),
    updated_by = sqlc.arg(updated_by),
    updated_at = now()
WHERE id = sqlc.arg(id) AND deleted_at IS NULL
RETURNING *;

-- name: SoftDeleteStudentApplication :execrows
UPDATE student_applications SET deleted_at = now(), deleted_by = sqlc.arg(deleted_by) WHERE id = sqlc.arg(id) AND deleted_at IS NULL;
-- name: CountApplicationsByStatus :many
SELECT s.name AS status_name, count(a.id) AS total
FROM application_statuses s
LEFT JOIN student_applications a
       ON a.application_status_id = s.id AND a.deleted_at IS NULL
GROUP BY s.name
ORDER BY s.name;

-- name: NextAppSequence :one
SELECT nextval('app_seq')::bigint AS seq;


-- name: ExportStudentApplications :many
SELECT 
    a.*,
    s.student_code,
    s.full_name AS student_name,
    t.name AS application_type_name,
    st.name AS application_status_name,
    f.name AS form_type_name
FROM student_applications a
JOIN students s ON s.id = a.student_id
LEFT JOIN application_types t ON t.id = a.application_type_id
LEFT JOIN application_statuses st ON st.id = a.application_status_id
LEFT JOIN form_types f ON f.id = a.form_type_id
WHERE a.deleted_at IS NULL
  AND (sqlc.arg(filter)::text = ''
       OR a.application_name ILIKE '%'||sqlc.arg(filter)||'%'
       OR a.application_number ILIKE '%'||sqlc.arg(filter)||'%'
       OR EXISTS (
           SELECT 1 FROM student_application_colleges sac 
           JOIN colleges c ON sac.college_id = c.id
           WHERE sac.application_id = a.id AND c.name ILIKE '%'||sqlc.arg(filter)||'%'
       ))
ORDER BY a.created_at DESC;

-- name: ClearStudentApplicationColleges :exec
DELETE FROM student_application_colleges WHERE application_id = $1;

-- name: AddStudentApplicationCollege :exec
INSERT INTO student_application_colleges (application_id, college_id) VALUES ($1, $2);

-- name: GetCollegesForApplication :many
SELECT c.* FROM colleges c
JOIN student_application_colleges sac ON sac.college_id = c.id
WHERE sac.application_id = $1
ORDER BY c.name;

-- name: GetCollegesForApplications :many
SELECT sac.application_id, c.id, c.name 
FROM colleges c
JOIN student_application_colleges sac ON sac.college_id = c.id
WHERE sac.application_id = ANY(sqlc.arg(application_ids)::uuid[])
ORDER BY c.name;

-- name: GetApplicationsByStudentsAndType :many
SELECT * FROM student_applications 
WHERE application_type_id = $1 AND student_id = ANY(sqlc.arg(student_ids)::uuid[]) AND deleted_at IS NULL;