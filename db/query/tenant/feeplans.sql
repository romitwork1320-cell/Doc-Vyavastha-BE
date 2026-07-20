-- name: ListStudentFeePlans :many
SELECT fp.*, ft.name AS fee_type_name,
       COALESCE(NULLIF(TRIM(CONCAT(up.first_name, ' ', up.last_name)), ''), u.email, '')::text AS created_by_name
FROM student_fee_plans fp
LEFT JOIN fee_types ft ON ft.id = fp.fee_type_id
LEFT JOIN public.user_profiles up ON up.user_id = fp.created_by
LEFT JOIN public.users u ON u.id = fp.created_by
WHERE fp.branch_id = sqlc.arg(branch_id) AND (sqlc.arg(filter)::text = '' OR fp.fee_name ILIKE '%'||sqlc.arg(filter)||'%')
ORDER BY fp.created_at DESC
LIMIT sqlc.arg(lim) OFFSET sqlc.arg(off);

-- name: CountStudentFeePlans :one
SELECT count(*) FROM student_fee_plans fp
WHERE fp.branch_id = sqlc.arg(branch_id) AND (sqlc.arg(filter)::text = '' OR fp.fee_name ILIKE '%'||sqlc.arg(filter)||'%');

-- name: GetStudentFeePlan :one
SELECT fp.*, ft.name AS fee_type_name,
       COALESCE(NULLIF(TRIM(CONCAT(up.first_name, ' ', up.last_name)), ''), u.email, '')::text AS created_by_name
FROM student_fee_plans fp
LEFT JOIN fee_types ft ON ft.id = fp.fee_type_id
LEFT JOIN public.user_profiles up ON up.user_id = fp.created_by
LEFT JOIN public.users u ON u.id = fp.created_by
WHERE fp.id = $1;

-- name: GetFeePlansByStudent :many
SELECT fp.*, ft.name AS fee_type_name,
       COALESCE(NULLIF(TRIM(CONCAT(up.first_name, ' ', up.last_name)), ''), u.email, '')::text AS created_by_name
FROM student_fee_plans fp
LEFT JOIN fee_types ft ON ft.id = fp.fee_type_id
LEFT JOIN public.user_profiles up ON up.user_id = fp.created_by
LEFT JOIN public.users u ON u.id = fp.created_by
WHERE fp.student_id = $1 AND fp.branch_id = $2
ORDER BY fp.created_at DESC;

-- name: CreateStudentFeePlan :one
INSERT INTO student_fee_plans
    (student_id, branch_id, fee_type_id, fee_name, total_amount, discount_amount, discount_reason, remarks, created_by)
VALUES
    (sqlc.arg(student_id), sqlc.arg(branch_id), sqlc.arg(fee_type_id), sqlc.arg(fee_name), sqlc.arg(total_amount),
     sqlc.arg(discount_amount), sqlc.arg(discount_reason), sqlc.arg(remarks), sqlc.arg(created_by))
RETURNING *;

-- name: UpdateStudentFeePlan :one
UPDATE student_fee_plans SET
    fee_type_id = sqlc.arg(fee_type_id),
    fee_name = sqlc.arg(fee_name),
    total_amount = sqlc.arg(total_amount),
    discount_amount = sqlc.arg(discount_amount),
    discount_reason = sqlc.arg(discount_reason),
    remarks = sqlc.arg(remarks),
    updated_at = now()
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: DeleteStudentFeePlan :execrows
DELETE FROM student_fee_plans WHERE id = $1;

-- name: SumCollectedForFeePlan :one
SELECT COALESCE(sum(amount), 0)::numeric AS collected
FROM student_payments WHERE student_fee_plan_id = $1;
