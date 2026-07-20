-- name: ListFeeTypes :many
SELECT * FROM fee_types
WHERE (sqlc.arg(filter)::text = '' OR name ILIKE '%'||sqlc.arg(filter)||'%' OR description ILIKE '%'||sqlc.arg(filter)||'%')
ORDER BY name ASC
LIMIT sqlc.arg(lim) OFFSET sqlc.arg(off);

-- name: CountFeeTypes :one
SELECT count(*) FROM fee_types
WHERE (sqlc.arg(filter)::text = '' OR name ILIKE '%'||sqlc.arg(filter)||'%' OR description ILIKE '%'||sqlc.arg(filter)||'%');

-- name: GetFeeType :one
SELECT * FROM fee_types WHERE id = $1;

-- name: CreateFeeType :one
INSERT INTO fee_types (name, description, amount, status)
VALUES (sqlc.arg(name), sqlc.arg(description), sqlc.arg(amount), sqlc.arg(status))
RETURNING *;

-- name: UpdateFeeType :one
UPDATE fee_types
SET name = sqlc.arg(name), description = sqlc.arg(description), amount = sqlc.arg(amount), status = sqlc.arg(status), updated_at = now()
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: DeleteFeeType :execrows
DELETE FROM fee_types WHERE id = $1;

-- name: AssignCategoryToFeeType :exec
INSERT INTO fee_type_categories (fee_type_id, category_id) VALUES ($1, $2);


-- name: ClearCategoriesForFeeType :exec
DELETE FROM fee_type_categories WHERE fee_type_id = $1;

-- name: GetCategoriesForFeeType :many
SELECT category_id FROM fee_type_categories WHERE fee_type_id = $1;

-- name: GetAllFeeTypeCategories :many
SELECT fee_type_id, category_id FROM fee_type_categories;

-- name: GetFeeTypeByCategory :one
SELECT ft.* FROM fee_types ft
JOIN fee_type_categories ftc ON ft.id = ftc.fee_type_id
WHERE ftc.category_id = $1 LIMIT 1;
