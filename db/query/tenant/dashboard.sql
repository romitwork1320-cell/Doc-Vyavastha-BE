-- name: GetTotalStudents :one
SELECT count(*) FROM students WHERE deleted_at IS NULL AND branch_id = sqlc.arg(branch_id) AND branch_id = sqlc.arg(branch_id) AND created_at >= sqlc.arg(start_date) AND created_at <= sqlc.arg(end_date);

-- name: GetTotalApplications :one
SELECT count(*) FROM student_applications WHERE deleted_at IS NULL AND branch_id = sqlc.arg(branch_id) AND branch_id = sqlc.arg(branch_id) AND created_at >= sqlc.arg(start_date) AND created_at <= sqlc.arg(end_date);

-- name: GetPendingApplications :one
SELECT count(*) FROM student_applications a 
JOIN application_statuses s ON a.application_status_id = s.id 
WHERE a.deleted_at IS NULL AND a.branch_id = sqlc.arg(branch_id) AND a.branch_id = sqlc.arg(branch_id) AND s.name ILIKE '%pending%' AND a.created_at >= sqlc.arg(start_date) AND a.created_at <= sqlc.arg(end_date);

-- name: GetTotalFeePlanned :one
SELECT COALESCE(sum(total_amount), 0)::numeric FROM student_fee_plans WHERE branch_id = sqlc.arg(branch_id) AND created_at >= sqlc.arg(start_date) AND created_at <= sqlc.arg(end_date);

-- name: GetTotalFeeCollected :one
SELECT COALESCE(sum(amount), 0)::numeric FROM student_payments WHERE branch_id = sqlc.arg(branch_id) AND payment_date >= sqlc.arg(start_date) AND payment_date <= sqlc.arg(end_date);

-- name: GetTotalFeeDiscount :one
SELECT COALESCE(sum(discount_amount), 0)::numeric FROM student_fee_plans WHERE branch_id = sqlc.arg(branch_id) AND created_at >= sqlc.arg(start_date) AND created_at <= sqlc.arg(end_date);

-- name: GetStudentDistributionByCategory :many
SELECT c.name AS category_name, count(s.id) AS count
FROM student_categories c
LEFT JOIN students s ON s.category_id = c.id AND s.deleted_at IS NULL AND s.branch_id = sqlc.arg(branch_id) AND s.branch_id = sqlc.arg(branch_id) AND s.created_at >= sqlc.arg(start_date) AND s.created_at <= sqlc.arg(end_date)
GROUP BY c.name
ORDER BY count DESC;

-- name: GetAdmissionTrend :many
WITH months AS (
    SELECT generate_series(
        date_trunc('month', sqlc.arg(start_date)::timestamp),
        date_trunc('month', sqlc.arg(end_date)::timestamp),
        '1 month'::interval
    ) AS month_date
)
SELECT 
    to_char(m.month_date, 'Mon YYYY') AS month,
    count(s.id) AS count
FROM months m
LEFT JOIN students s ON date_trunc('month', s.created_at) = m.month_date 
    AND s.created_at >= sqlc.arg(start_date) 
    AND s.created_at <= sqlc.arg(end_date)
    AND s.deleted_at IS NULL AND s.branch_id = sqlc.arg(branch_id) AND s.branch_id = sqlc.arg(branch_id)
GROUP BY m.month_date
ORDER BY m.month_date;

-- name: GetApplicationStatusDistribution :many
SELECT s.name AS status_name, count(a.id) AS count
FROM application_statuses s
LEFT JOIN student_applications a ON a.application_status_id = s.id AND a.deleted_at IS NULL AND a.branch_id = sqlc.arg(branch_id) AND a.branch_id = sqlc.arg(branch_id) AND a.created_at >= sqlc.arg(start_date) AND a.created_at <= sqlc.arg(end_date)
GROUP BY s.name
ORDER BY count DESC;

-- name: GetApplicationTypesDistribution :many
SELECT t.name AS type_name, count(a.id) AS count
FROM application_types t
LEFT JOIN student_applications a ON a.application_type_id = t.id AND a.deleted_at IS NULL AND a.branch_id = sqlc.arg(branch_id) AND a.branch_id = sqlc.arg(branch_id) AND a.created_at >= sqlc.arg(start_date) AND a.created_at <= sqlc.arg(end_date)
GROUP BY t.name
ORDER BY count DESC;

-- name: GetRevenueCollectionTrend :many
WITH months AS (
    SELECT generate_series(
        date_trunc('month', sqlc.arg(start_date)::timestamp),
        date_trunc('month', sqlc.arg(end_date)::timestamp),
        '1 month'::interval
    ) AS month_date
)
SELECT 
    to_char(m.month_date, 'Mon YYYY') AS month,
    COALESCE(sum(p.amount), 0)::numeric AS amount
FROM months m
LEFT JOIN student_payments p ON date_trunc('month', p.payment_date) = m.month_date 
    AND p.payment_date >= sqlc.arg(start_date) 
    AND p.payment_date <= sqlc.arg(end_date)
    AND p.branch_id = sqlc.arg(branch_id)
GROUP BY m.month_date
ORDER BY m.month_date;

-- name: GetPaymentMethodDistribution :many
SELECT payment_method AS method, COALESCE(sum(amount), 0)::numeric AS amount
FROM student_payments
WHERE branch_id = sqlc.arg(branch_id) AND payment_date >= sqlc.arg(start_date) AND payment_date <= sqlc.arg(end_date)
GROUP BY payment_method
ORDER BY amount DESC;

-- name: GetUpcomingDeadlines :many
SELECT a.id, s.student_code, s.full_name AS student_name, 
       a.application_name, a.last_date, 
       EXTRACT(DAY FROM a.last_date - now())::int AS remaining_days
FROM student_applications a
JOIN students s ON a.student_id = s.id
WHERE a.deleted_at IS NULL AND a.branch_id = sqlc.arg(branch_id) AND a.branch_id = sqlc.arg(branch_id) AND s.deleted_at IS NULL AND s.branch_id = sqlc.arg(branch_id) AND s.branch_id = sqlc.arg(branch_id) 
  AND a.last_date >= now() 
  AND a.last_date <= now() + interval '30 days'
ORDER BY a.last_date ASC
LIMIT 10;

-- name: GetActionCenterItems :many
SELECT a.id, s.student_code, s.full_name AS student_name, 
       'Application missing portal credentials'::text AS issue, 'High'::text AS priority
FROM student_applications a
JOIN students s ON a.student_id = s.id
WHERE a.deleted_at IS NULL AND a.branch_id = sqlc.arg(branch_id) AND a.branch_id = sqlc.arg(branch_id) AND s.deleted_at IS NULL AND s.branch_id = sqlc.arg(branch_id) AND s.branch_id = sqlc.arg(branch_id) 
  AND (a.portal_username = '' OR a.portal_username IS NULL)
  AND a.created_at >= sqlc.arg(start_date) AND a.created_at <= sqlc.arg(end_date)
LIMIT 10;
