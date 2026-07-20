-- name: CreateApplicationFeeCollection :one
INSERT INTO application_fee_collections (
    application_id, staff_id, college_fee_amount, payment_to_college_method, student_reimbursement_method, student_transaction_ref, staff_remarks, admin_verification_status, admin_remarks, branch_id, created_by, updated_by
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12
) RETURNING *;

-- name: UpdateApplicationFeeCollection :one
UPDATE application_fee_collections SET
    college_fee_amount = COALESCE(sqlc.narg(college_fee_amount), college_fee_amount),
    payment_to_college_method = COALESCE(sqlc.narg(payment_to_college_method), payment_to_college_method),
    student_reimbursement_method = COALESCE(sqlc.narg(student_reimbursement_method), student_reimbursement_method),
    student_transaction_ref = COALESCE(sqlc.narg(student_transaction_ref), student_transaction_ref),
    staff_remarks = COALESCE(sqlc.narg(staff_remarks), staff_remarks),
    admin_verification_status = COALESCE(sqlc.narg(admin_verification_status), admin_verification_status),
    admin_remarks = COALESCE(sqlc.narg(admin_remarks), admin_remarks),
    verified_by = COALESCE(sqlc.narg(verified_by), verified_by),
    verified_at = COALESCE(sqlc.narg(verified_at), verified_at),
    updated_by = sqlc.arg(updated_by),
    updated_at = NOW()
WHERE id = sqlc.arg(id) AND deleted_at IS NULL
RETURNING *;

-- name: GetApplicationFeeCollection :one
SELECT * FROM application_fee_collections
WHERE id = $1 AND deleted_at IS NULL;

-- name: ListApplicationFeeCollections :many
SELECT c.*, 
       a.application_name, a.student_id,
       up.first_name AS staff_first_name, up.last_name AS staff_last_name,
       s.full_name AS student_first_name, NULL::text AS student_last_name,
       u.email AS staff_email,
       s.email AS student_email,
       vu.first_name AS verified_by_first_name, vu.last_name AS verified_by_last_name
FROM application_fee_collections c
JOIN student_applications a ON a.id = c.application_id
LEFT JOIN public.users u ON u.id = c.staff_id
LEFT JOIN public.user_profiles up ON up.user_id = u.id
LEFT JOIN public.user_profiles vu ON vu.user_id = c.verified_by
LEFT JOIN students s ON s.id = a.student_id
WHERE c.branch_id = sqlc.arg(branch_id)
AND c.deleted_at IS NULL
AND (sqlc.narg(staff_id)::bigint IS NULL OR c.staff_id = sqlc.narg(staff_id))
AND (sqlc.narg(status)::text IS NULL OR c.admin_verification_status = sqlc.narg(status))
AND (sqlc.narg(start_date)::timestamptz IS NULL OR c.created_at >= sqlc.narg(start_date))
AND (sqlc.narg(end_date)::timestamptz IS NULL OR c.created_at <= sqlc.narg(end_date))
ORDER BY c.created_at DESC;

-- name: DeleteApplicationFeeCollection :exec
UPDATE application_fee_collections SET deleted_at = NOW(), deleted_by = $1 WHERE id = $2 AND deleted_at IS NULL;
