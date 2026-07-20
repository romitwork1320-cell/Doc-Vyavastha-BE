-- name: ListStudentPayments :many
SELECT p.*, ft.name AS fee_type_name,
       COALESCE(NULLIF(TRIM(CONCAT(up.first_name, ' ', up.last_name)), ''), u.email, '')::text AS created_by_name
FROM student_payments p
LEFT JOIN student_fee_plans fp ON fp.id = p.student_fee_plan_id
LEFT JOIN fee_types ft ON ft.id = fp.fee_type_id
LEFT JOIN public.user_profiles up ON up.user_id = p.created_by
LEFT JOIN public.users u ON u.id = p.created_by
WHERE p.branch_id = sqlc.arg(branch_id) AND (sqlc.arg(filter)::text = '' OR p.payment_number ILIKE '%'||sqlc.arg(filter)||'%' OR p.receipt_number ILIKE '%'||sqlc.arg(filter)||'%')
ORDER BY p.payment_date DESC, p.created_at DESC
LIMIT sqlc.arg(lim) OFFSET sqlc.arg(off);

-- name: CountStudentPayments :one
SELECT count(*) FROM student_payments p
WHERE p.branch_id = sqlc.arg(branch_id) AND (sqlc.arg(filter)::text = '' OR p.payment_number ILIKE '%'||sqlc.arg(filter)||'%' OR p.receipt_number ILIKE '%'||sqlc.arg(filter)||'%');

-- name: GetStudentPayment :one
SELECT p.*, ft.name AS fee_type_name,
       COALESCE(NULLIF(TRIM(CONCAT(up.first_name, ' ', up.last_name)), ''), u.email, '')::text AS created_by_name
FROM student_payments p
LEFT JOIN student_fee_plans fp ON fp.id = p.student_fee_plan_id
LEFT JOIN fee_types ft ON ft.id = fp.fee_type_id
LEFT JOIN public.user_profiles up ON up.user_id = p.created_by
LEFT JOIN public.users u ON u.id = p.created_by
WHERE p.id = $1;

-- name: GetPaymentsByStudent :many
SELECT p.*, ft.name AS fee_type_name,
       COALESCE(NULLIF(TRIM(CONCAT(up.first_name, ' ', up.last_name)), ''), u.email, '')::text AS created_by_name
FROM student_payments p
LEFT JOIN student_fee_plans fp ON fp.id = p.student_fee_plan_id
LEFT JOIN fee_types ft ON ft.id = fp.fee_type_id
LEFT JOIN public.user_profiles up ON up.user_id = p.created_by
LEFT JOIN public.users u ON u.id = p.created_by
WHERE p.student_id = $1 AND p.branch_id = $2
ORDER BY p.payment_date DESC, p.created_at DESC;

-- name: GetPaymentsByFeePlan :many
SELECT p.*, ft.name AS fee_type_name,
       COALESCE(NULLIF(TRIM(CONCAT(up.first_name, ' ', up.last_name)), ''), u.email, '')::text AS created_by_name
FROM student_payments p
LEFT JOIN student_fee_plans fp ON fp.id = p.student_fee_plan_id
LEFT JOIN fee_types ft ON ft.id = fp.fee_type_id
LEFT JOIN public.user_profiles up ON up.user_id = p.created_by
LEFT JOIN public.users u ON u.id = p.created_by
WHERE p.student_fee_plan_id = $1 AND p.branch_id = $2
ORDER BY p.payment_date DESC, p.created_at DESC;

-- name: CreateStudentPayment :one
INSERT INTO student_payments
    (payment_number, receipt_number, student_id, branch_id, student_fee_plan_id, payment_date, amount, payment_method, reference_number, remarks, created_by)
VALUES
    (sqlc.arg(payment_number), sqlc.arg(receipt_number), sqlc.arg(student_id), sqlc.arg(branch_id), sqlc.arg(student_fee_plan_id),
     sqlc.arg(payment_date), sqlc.arg(amount), sqlc.arg(payment_method), sqlc.arg(reference_number), sqlc.arg(remarks), sqlc.arg(created_by))
RETURNING *;

-- name: UpdateStudentPayment :one
UPDATE student_payments SET
    payment_date = sqlc.arg(payment_date),
    amount = sqlc.arg(amount),
    payment_method = sqlc.arg(payment_method),
    reference_number = sqlc.arg(reference_number),
    remarks = sqlc.arg(remarks),
    updated_at = now()
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: DeleteStudentPayment :execrows
DELETE FROM student_payments WHERE id = $1;

-- name: NextPaymentSeq :one
SELECT nextval('payment_seq')::bigint AS seq;

-- name: NextReceiptSeq :one
SELECT nextval('receipt_seq')::bigint AS seq;
