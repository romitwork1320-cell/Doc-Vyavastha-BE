-- name: AddUserToTenant :one
INSERT INTO user_tenants (user_id, tenant_id, role_id, status, is_owner, joined_at)
VALUES (sqlc.arg(user_id), sqlc.arg(tenant_id), sqlc.arg(role_id), sqlc.arg(status), sqlc.arg(is_owner), now())
ON CONFLICT (user_id, tenant_id) DO UPDATE
   SET role_id = EXCLUDED.role_id, status = EXCLUDED.status, is_owner = EXCLUDED.is_owner
RETURNING *;

-- name: UpdateMembership :exec
UPDATE user_tenants
SET role_id = sqlc.arg(role_id), status = sqlc.arg(status)
WHERE user_id = sqlc.arg(user_id) AND tenant_id = sqlc.arg(tenant_id);

-- name: RemoveUserFromTenant :execrows
DELETE FROM user_tenants WHERE user_id = sqlc.arg(user_id) AND tenant_id = sqlc.arg(tenant_id);

-- name: GetMembership :one
SELECT ut.*, COALESCE(r.role_name, '') AS role_name
FROM user_tenants ut
LEFT JOIN roles r ON r.role_id = ut.role_id
WHERE ut.user_id = sqlc.arg(user_id) AND ut.tenant_id = sqlc.arg(tenant_id);

-- name: ListUserTenantsForSelection :many
SELECT t.tenant_id, t.tenant_name, COALESCE(r.role_name, '') AS role,
       (ut.status = 'Active' AND t.is_active) AS is_active
FROM user_tenants ut
JOIN tenants t ON t.tenant_id = ut.tenant_id
LEFT JOIN roles r ON r.role_id = ut.role_id
WHERE ut.user_id = $1
ORDER BY t.tenant_name ASC;
