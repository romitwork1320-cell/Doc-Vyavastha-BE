-- name: CreateOrganizationType :one
INSERT INTO organization_types (
    name, description
) VALUES (
    $1, $2
) RETURNING *;

-- name: GetOrganizationType :one
SELECT * FROM organization_types
WHERE id = $1 LIMIT 1;

-- name: ListOrganizationTypes :many
SELECT * FROM organization_types
WHERE is_active = TRUE
ORDER BY name ASC;

-- name: UpdateOrganizationType :one
UPDATE organization_types
SET name = $2,
    description = $3,
    updated_at = now()
WHERE id = $1
RETURNING *;

-- name: DeleteOrganizationType :exec
UPDATE organization_types
SET is_active = FALSE
WHERE id = $1;
