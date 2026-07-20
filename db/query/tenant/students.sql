-- name: ListStudents :many
SELECT * FROM students
WHERE deleted_at IS NULL
  AND students.branch_id = sqlc.arg(branch_id)
  AND (sqlc.arg(filter)::text = ''
       OR student_code ILIKE '%'||sqlc.arg(filter)||'%'
       OR full_name ILIKE '%'||sqlc.arg(filter)||'%'
       OR primary_mobile ILIKE '%'||sqlc.arg(filter)||'%'
       OR city ILIKE '%'||sqlc.arg(filter)||'%'
       OR EXISTS (
           SELECT 1 FROM student_assigned_codes sac
           WHERE sac.student_id = students.id
             AND sac.student_code ILIKE '%'||sqlc.arg(filter)||'%'
       ))
  AND (sqlc.narg(application_type_id)::uuid IS NULL 
       OR EXISTS (
           SELECT 1 FROM student_applications sa 
           WHERE sa.student_id = students.id 
             AND sa.application_type_id = sqlc.narg(application_type_id)::uuid 
             AND sa.deleted_at IS NULL
       ))
ORDER BY created_at DESC
LIMIT sqlc.arg(lim) OFFSET sqlc.arg(off);

-- name: CountStudents :one
SELECT count(*) FROM students
WHERE deleted_at IS NULL
  AND students.branch_id = sqlc.arg(branch_id)
  AND (sqlc.arg(filter)::text = ''
       OR student_code ILIKE '%'||sqlc.arg(filter)||'%'
       OR full_name ILIKE '%'||sqlc.arg(filter)||'%'
       OR primary_mobile ILIKE '%'||sqlc.arg(filter)||'%'
       OR city ILIKE '%'||sqlc.arg(filter)||'%'
       OR EXISTS (
           SELECT 1 FROM student_assigned_codes sac
           WHERE sac.student_id = students.id
             AND sac.student_code ILIKE '%'||sqlc.arg(filter)||'%'
       ))
  AND (sqlc.narg(application_type_id)::uuid IS NULL 
       OR EXISTS (
           SELECT 1 FROM student_applications sa 
           WHERE sa.student_id = students.id 
             AND sa.application_type_id = sqlc.narg(application_type_id)::uuid 
             AND sa.deleted_at IS NULL
       ));

-- name: GetStudent :one
SELECT * FROM students WHERE id = $1 AND deleted_at IS NULL;

-- name: GetStudentByCode :one
SELECT * FROM students WHERE student_code = $1 AND deleted_at IS NULL;

-- name: CreateStudent :one
INSERT INTO students (
    student_code, category_id, caste_id, branch_id, year_config_id,
    full_name, father_name, mother_name, gender, email,
    primary_mobile, secondary_mobile, whatsapp_mobile,
    home_address, city, state, pincode,
    school_name, passing_board, tenth_passing_year, twelfth_passing_year,
    scholarship_uid, scholarship_password, profile_photo_url,
    status, remarks, created_by, updated_by
) VALUES (
    sqlc.arg(student_code), sqlc.arg(category_id), sqlc.narg(caste_id), sqlc.arg(branch_id), sqlc.arg(year_config_id),
    sqlc.arg(full_name), sqlc.arg(father_name), sqlc.arg(mother_name), sqlc.arg(gender), sqlc.arg(email),
    sqlc.arg(primary_mobile), sqlc.arg(secondary_mobile), sqlc.arg(whatsapp_mobile),
    sqlc.arg(home_address), sqlc.arg(city), sqlc.arg(state), sqlc.arg(pincode),
    sqlc.arg(school_name), sqlc.arg(passing_board), sqlc.arg(tenth_passing_year), sqlc.arg(twelfth_passing_year),
    sqlc.arg(scholarship_uid), sqlc.arg(scholarship_password), sqlc.arg(profile_photo_url),
    sqlc.arg(status), sqlc.arg(remarks), sqlc.arg(created_by), sqlc.arg(updated_by)
)
RETURNING *;

-- name: UpdateStudent :one
UPDATE students SET
    category_id = sqlc.arg(category_id),
    year_config_id = sqlc.arg(year_config_id),
    student_code = sqlc.arg(student_code),
    caste_id = sqlc.narg(caste_id),
    branch_id = sqlc.arg(branch_id),
    full_name = sqlc.arg(full_name),
    father_name = sqlc.arg(father_name),
    mother_name = sqlc.arg(mother_name),
    gender = sqlc.arg(gender),
    email = sqlc.arg(email),
    primary_mobile = sqlc.arg(primary_mobile),
    secondary_mobile = sqlc.arg(secondary_mobile),
    whatsapp_mobile = sqlc.arg(whatsapp_mobile),
    home_address = sqlc.arg(home_address),
    city = sqlc.arg(city),
    state = sqlc.arg(state),
    pincode = sqlc.arg(pincode),
    school_name = sqlc.arg(school_name),
    passing_board = sqlc.arg(passing_board),
    tenth_passing_year = sqlc.arg(tenth_passing_year),
    twelfth_passing_year = sqlc.arg(twelfth_passing_year),
    scholarship_uid = sqlc.arg(scholarship_uid),
    scholarship_password = sqlc.arg(scholarship_password),
    profile_photo_url = sqlc.arg(profile_photo_url),
    status = sqlc.arg(status),
    remarks = sqlc.arg(remarks),
    updated_by = sqlc.arg(updated_by),
    updated_at = now()
WHERE id = sqlc.arg(id) AND deleted_at IS NULL
RETURNING *;

-- name: SoftDeleteStudent :execrows
UPDATE students SET deleted_at = now() WHERE id = $1 AND deleted_at IS NULL;

-- name: ExportStudents :many
SELECT 
    s.*,
    c.name AS category_name,
    ca.name AS caste_name
FROM students s
LEFT JOIN student_categories c ON c.id = s.category_id
LEFT JOIN student_castes ca ON ca.id = s.caste_id
WHERE s.deleted_at IS NULL
  AND s.branch_id = sqlc.arg(branch_id)
  AND (sqlc.arg(filter)::text = ''
       OR s.student_code ILIKE '%'||sqlc.arg(filter)||'%'
       OR s.full_name ILIKE '%'||sqlc.arg(filter)||'%'
       OR s.primary_mobile ILIKE '%'||sqlc.arg(filter)||'%'
       OR s.city ILIKE '%'||sqlc.arg(filter)||'%'
       OR EXISTS (
           SELECT 1 FROM student_assigned_codes sac
           WHERE sac.student_id = s.id
             AND sac.student_code ILIKE '%'||sqlc.arg(filter)||'%'
       ))
  AND (sqlc.narg(application_type_id)::uuid IS NULL 
       OR EXISTS (
           SELECT 1 FROM student_applications sa 
           WHERE sa.student_id = s.id 
             AND sa.application_type_id = sqlc.narg(application_type_id)::uuid 
             AND sa.deleted_at IS NULL
       ))
ORDER BY s.created_at DESC;
