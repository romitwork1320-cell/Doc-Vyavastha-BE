-- name: GetRoleByName :one
SELECT * FROM roles WHERE lower(role_name) = lower($1);

-- name: GetRoleByID :one
SELECT * FROM roles WHERE role_id = $1;

-- name: ListRoles :many
SELECT * FROM roles ORDER BY role_name ASC;

-- name: EnsureRole :one
INSERT INTO roles (role_name) VALUES (sqlc.arg(role_name))
ON CONFLICT (lower(role_name)) DO UPDATE SET updated_at = now()
RETURNING *;

-- name: ListPages :many
SELECT * FROM pages ORDER BY display_order ASC, page_name ASC;

-- name: ListRolePagePermissions :many
SELECT * FROM role_page_permissions;

-- name: UpsertRolePagePermission :exec
INSERT INTO role_page_permissions (role_id, page_id, can_view, can_add, can_edit, can_delete)
VALUES (sqlc.arg(role_id), sqlc.arg(page_id), sqlc.arg(can_view), sqlc.arg(can_add), sqlc.arg(can_edit), sqlc.arg(can_delete))
ON CONFLICT (role_id, page_id) DO UPDATE SET
    can_view = EXCLUDED.can_view,
    can_add = EXCLUDED.can_add,
    can_edit = EXCLUDED.can_edit,
    can_delete = EXCLUDED.can_delete;

-- name: GrantUserPagePermission :exec
INSERT INTO user_permissions (user_id, tenant_id, page_id, can_view)
VALUES (sqlc.arg(user_id), sqlc.arg(tenant_id), sqlc.arg(page_id), sqlc.arg(can_view))
ON CONFLICT (user_id, tenant_id, page_id) DO UPDATE SET can_view = EXCLUDED.can_view;

-- name: GetUserAllowedPageUrls :many
SELECT DISTINCT p.route_url
FROM pages p
WHERE p.page_id IN (
    -- role-derived view grants for the user's role in this tenant
    SELECT rpp.page_id
    FROM role_page_permissions rpp
    JOIN user_tenants ut ON ut.role_id = rpp.role_id
    WHERE ut.user_id = sqlc.arg(user_id) AND ut.tenant_id = sqlc.arg(tenant_id) AND rpp.can_view
    UNION
    -- per-user overrides
    SELECT up.page_id
    FROM user_permissions up
    WHERE up.user_id = sqlc.arg(user_id) AND up.tenant_id = sqlc.arg(tenant_id) AND up.can_view
)
ORDER BY p.route_url;

-- name: GetUserPagePermissions :many
SELECT 
    p.route_url,
    COALESCE(rpp.can_view, false) AS can_view,
    COALESCE(rpp.can_add, false) AS can_add,
    COALESCE(rpp.can_edit, false) AS can_edit,
    COALESCE(rpp.can_delete, false) AS can_delete
FROM pages p
JOIN role_page_permissions rpp ON rpp.page_id = p.page_id
JOIN user_tenants ut ON ut.role_id = rpp.role_id
WHERE ut.user_id = sqlc.arg(user_id) AND ut.tenant_id = sqlc.arg(tenant_id);
