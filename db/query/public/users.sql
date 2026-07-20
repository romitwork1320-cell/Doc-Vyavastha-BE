-- name: GetUserByEmail :one
SELECT * FROM users WHERE lower(email) = lower($1) AND deleted_at IS NULL;

-- name: GetUserByID :one
SELECT * FROM users WHERE id = $1 AND deleted_at IS NULL;

-- name: GetUserByGoogleID :one
SELECT * FROM users WHERE google_id = $1 AND deleted_at IS NULL;

-- name: CreateUser :one
INSERT INTO users (email, password_hash, google_id, is_active, is_verified)
VALUES (lower(sqlc.arg(email)), sqlc.arg(password_hash), sqlc.arg(google_id), sqlc.arg(is_active), sqlc.arg(is_verified))
RETURNING *;

-- name: SetUserPassword :exec
UPDATE users SET password_hash = sqlc.arg(password_hash), is_verified = TRUE, updated_at = now()
WHERE id = sqlc.arg(id);

-- name: MarkUserVerified :exec
UPDATE users SET is_verified = TRUE, updated_at = now() WHERE id = $1;

-- name: TouchLastLogin :exec
UPDATE users SET last_login_at = now() WHERE id = $1;

-- name: SetUserGoogleID :exec
UPDATE users SET google_id = sqlc.arg(google_id), updated_at = now() WHERE id = sqlc.arg(id);

-- name: UpdateUserActive :exec
UPDATE users SET is_active = sqlc.arg(is_active), updated_at = now() WHERE id = sqlc.arg(id);

-- name: SoftDeleteUser :execrows
UPDATE users SET deleted_at = now() WHERE id = $1 AND deleted_at IS NULL;

-- name: ListTenantUsers :many
SELECT u.id, u.email AS username, COALESCE(r.role_name, '') AS role,
       u.is_active, ut.tenant_id, ut.status AS membership_status, ut.is_owner,
       up.first_name, up.last_name, up.contact_number
FROM users u
JOIN user_tenants ut ON ut.user_id = u.id
LEFT JOIN roles r ON r.role_id = ut.role_id
LEFT JOIN user_profiles up ON up.user_id = u.id
WHERE ut.tenant_id = sqlc.arg(tenant_id)
  AND u.deleted_at IS NULL
  AND (sqlc.arg(filter)::text = '' OR u.email ILIKE '%'||sqlc.arg(filter)||'%' OR up.first_name ILIKE '%'||sqlc.arg(filter)||'%' OR up.last_name ILIKE '%'||sqlc.arg(filter)||'%')
ORDER BY u.email ASC
LIMIT sqlc.arg(lim) OFFSET sqlc.arg(off);

-- name: CountTenantUsers :one
SELECT count(*)
FROM users u
JOIN user_tenants ut ON ut.user_id = u.id
LEFT JOIN user_profiles up ON up.user_id = u.id
WHERE ut.tenant_id = sqlc.arg(tenant_id)
  AND u.deleted_at IS NULL
  AND (sqlc.arg(filter)::text = '' OR u.email ILIKE '%'||sqlc.arg(filter)||'%' OR up.first_name ILIKE '%'||sqlc.arg(filter)||'%' OR up.last_name ILIKE '%'||sqlc.arg(filter)||'%');

-- name: GetTenantUser :one
SELECT u.id, u.email AS username, COALESCE(r.role_name, '') AS role,
       u.is_active, ut.tenant_id, ut.status AS membership_status, ut.is_owner,
       up.first_name, up.last_name, up.contact_number
FROM users u
JOIN user_tenants ut ON ut.user_id = u.id
LEFT JOIN roles r ON r.role_id = ut.role_id
LEFT JOIN user_profiles up ON up.user_id = u.id
WHERE u.id = sqlc.arg(user_id) AND ut.tenant_id = sqlc.arg(tenant_id) AND u.deleted_at IS NULL;

-- name: ListTenantUserLookup :many
SELECT u.id AS user_id, COALESCE(NULLIF(trim(coalesce(p.first_name,'')||' '||coalesce(p.last_name,'')), ''), u.email) AS display_name,
       COALESCE(r.role_name, '') AS role_name
FROM users u
JOIN user_tenants ut ON ut.user_id = u.id
LEFT JOIN roles r ON r.role_id = ut.role_id
LEFT JOIN user_profiles p ON p.user_id = u.id
WHERE ut.tenant_id = $1 AND u.deleted_at IS NULL AND u.is_active = TRUE
ORDER BY display_name ASC;
