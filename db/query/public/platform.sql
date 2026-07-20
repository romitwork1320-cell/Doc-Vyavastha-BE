-- ── System logs ─────────────────────────────────────────────────────────
-- name: ListSystemLogs :many
SELECT * FROM system_logs
WHERE (sqlc.arg(filter)::text = '' OR message ILIKE '%'||sqlc.arg(filter)||'%' OR source ILIKE '%'||sqlc.arg(filter)||'%' OR log_level ILIKE '%'||sqlc.arg(filter)||'%')
  AND (sqlc.narg(from_date)::timestamptz IS NULL OR log_date >= sqlc.narg(from_date))
  AND (sqlc.narg(to_date)::timestamptz IS NULL OR log_date <= sqlc.narg(to_date))
ORDER BY log_date DESC
LIMIT sqlc.arg(lim) OFFSET sqlc.arg(off);

-- name: CountSystemLogs :one
SELECT count(*) FROM system_logs
WHERE (sqlc.arg(filter)::text = '' OR message ILIKE '%'||sqlc.arg(filter)||'%' OR source ILIKE '%'||sqlc.arg(filter)||'%' OR log_level ILIKE '%'||sqlc.arg(filter)||'%')
  AND (sqlc.narg(from_date)::timestamptz IS NULL OR log_date >= sqlc.narg(from_date))
  AND (sqlc.narg(to_date)::timestamptz IS NULL OR log_date <= sqlc.narg(to_date));

-- name: GetSystemLog :one
SELECT * FROM system_logs WHERE log_id = $1;

-- name: DeleteSystemLog :execrows
DELETE FROM system_logs WHERE log_id = $1;

-- name: CleanupSystemLogs :execrows
DELETE FROM system_logs WHERE log_date < (now() - make_interval(days => sqlc.arg(days)::int));

-- name: InsertSystemLog :exec
INSERT INTO system_logs (log_level, source, message, stack_trace, tenant_name, user_id, request_url, ip_address)
VALUES (sqlc.arg(log_level), sqlc.arg(source), sqlc.arg(message), sqlc.arg(stack_trace), sqlc.arg(tenant_name), sqlc.arg(user_id), sqlc.arg(request_url), sqlc.arg(ip_address));

-- ── User activity audit ─────────────────────────────────────────────────
-- name: ListRecentUserActivities :many
SELECT * FROM user_activities
WHERE tenant_id = sqlc.arg(tenant_id) AND branch_id = sqlc.arg(branch_id)
ORDER BY created_on DESC
LIMIT sqlc.arg(lim) OFFSET sqlc.arg(off);

-- name: CountUserActivities :one
SELECT count(*) FROM user_activities
WHERE tenant_id = sqlc.arg(tenant_id) AND branch_id = sqlc.arg(branch_id);

-- name: InsertUserActivity :exec
INSERT INTO user_activities (user_id, user_name, tenant_id, url, method, ip_address, description, changes)
VALUES (sqlc.arg(user_id), sqlc.arg(user_name), sqlc.arg(tenant_id), sqlc.arg(url), sqlc.arg(method), sqlc.arg(ip_address), sqlc.arg(description), sqlc.arg(changes));

-- ── Notifications ───────────────────────────────────────────────────────
-- name: ListUnreadNotifications :many
SELECT * FROM notifications WHERE user_id = $1 AND is_read = FALSE ORDER BY created_at DESC;

-- name: CountUnreadNotifications :one
SELECT count(*) FROM notifications WHERE user_id = $1 AND is_read = FALSE;

-- name: MarkNotificationRead :exec
UPDATE notifications SET is_read = TRUE WHERE id = sqlc.arg(id) AND user_id = sqlc.arg(user_id);

-- name: MarkAllNotificationsRead :exec
UPDATE notifications SET is_read = TRUE WHERE user_id = $1;

-- name: CreateNotification :one
INSERT INTO notifications (user_id, tenant_id, title, body, type, link)
VALUES (sqlc.arg(user_id), sqlc.arg(tenant_id), sqlc.arg(title), sqlc.arg(body), sqlc.arg(type), sqlc.arg(link))
RETURNING *;
