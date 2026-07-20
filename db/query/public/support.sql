-- name: CreateTicket :one
INSERT INTO support_tickets (tenant_id, user_id, subject, category, priority, description, attachment_url, status)
VALUES (sqlc.arg(tenant_id), sqlc.arg(user_id), sqlc.arg(subject), sqlc.arg(category), sqlc.arg(priority), sqlc.arg(description), sqlc.arg(attachment_url), 'Open')
RETURNING *;

-- name: AddTicketReply :one
INSERT INTO ticket_messages (ticket_id, sender_user_id, is_admin_reply, message, attachment_url)
VALUES (sqlc.arg(ticket_id), sqlc.arg(sender_user_id), sqlc.arg(is_admin_reply), sqlc.arg(message), sqlc.arg(attachment_url))
RETURNING *;

-- name: TouchTicket :exec
UPDATE support_tickets SET updated_at = now() WHERE ticket_id = $1;

-- name: ListTickets :many
SELECT * FROM support_tickets
WHERE (sqlc.arg(tenant_id)::bigint = 0 OR tenant_id = sqlc.arg(tenant_id))
  AND (sqlc.arg(status)::text = '' OR status = sqlc.arg(status))
ORDER BY updated_at DESC;

-- name: GetTicket :one
SELECT * FROM support_tickets WHERE ticket_id = $1;

-- name: ListTicketMessages :many
SELECT * FROM ticket_messages WHERE ticket_id = $1 ORDER BY created_on ASC;

-- name: UpdateTicketStatus :exec
UPDATE support_tickets SET status = sqlc.arg(status), updated_at = now() WHERE ticket_id = sqlc.arg(ticket_id);
