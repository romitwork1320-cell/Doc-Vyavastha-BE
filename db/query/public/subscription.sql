-- name: ListActivePlans :many
SELECT * FROM subscription_plans WHERE is_active = TRUE ORDER BY price ASC;

-- name: ListAllPlans :many
SELECT * FROM subscription_plans ORDER BY price ASC;

-- name: GetPlan :one
SELECT * FROM subscription_plans WHERE plan_id = $1;

-- name: GetCurrentSubscription :one
SELECT * FROM tenant_subscriptions
WHERE tenant_id = $1
ORDER BY end_date DESC
LIMIT 1;

-- name: CreateTenantSubscription :one
INSERT INTO tenant_subscriptions (tenant_id, plan_id, start_date, end_date, status)
VALUES (sqlc.arg(tenant_id), sqlc.arg(plan_id), sqlc.arg(start_date), sqlc.arg(end_date), sqlc.arg(status))
RETURNING *;

-- name: GetPromoByCode :one
SELECT * FROM promo_codes WHERE upper(code) = upper($1);

-- name: IncrementPromoUse :exec
UPDATE promo_codes SET used_count = used_count + 1 WHERE promo_id = $1;

-- name: CreatePromo :one
INSERT INTO promo_codes (code, discount_type, discount_value, extension_days, max_uses, valid_from, valid_to, is_active)
VALUES (upper(sqlc.arg(code)), sqlc.arg(discount_type), sqlc.arg(discount_value), sqlc.arg(extension_days), sqlc.arg(max_uses), sqlc.arg(valid_from), sqlc.arg(valid_to), sqlc.arg(is_active))
RETURNING *;

-- name: CreateTenantPayment :one
INSERT INTO tenant_payments (tenant_id, plan_id, promo_code_id, original_amount, discount_amount, final_amount, payment_mode, reference_number)
VALUES (sqlc.arg(tenant_id), sqlc.arg(plan_id), sqlc.arg(promo_code_id), sqlc.arg(original_amount), sqlc.arg(discount_amount), sqlc.arg(final_amount), sqlc.arg(payment_mode), sqlc.arg(reference_number))
RETURNING *;
