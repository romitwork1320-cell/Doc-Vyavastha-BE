-- name: ListTenants :many
SELECT t.*,
    COALESCE(to_char(s.end_date, 'YYYY-MM-DD'), '')::text AS subscription_end_date,
    COALESCE(s.status, '')::text AS subscription_status
FROM tenants t
LEFT JOIN (
    SELECT DISTINCT ON (tenant_id) tenant_id, end_date, status
    FROM tenant_subscriptions
    ORDER BY tenant_id, end_date DESC
) s ON s.tenant_id = t.tenant_id
WHERE (sqlc.arg(filter)::text = ''
       OR t.tenant_name ILIKE '%'||sqlc.arg(filter)||'%'
       OR t.company_name ILIKE '%'||sqlc.arg(filter)||'%')
ORDER BY t.tenant_id ASC
LIMIT sqlc.arg(lim) OFFSET sqlc.arg(off);

-- name: CountTenants :one
SELECT count(*) FROM tenants t
WHERE (sqlc.arg(filter)::text = ''
       OR t.tenant_name ILIKE '%'||sqlc.arg(filter)||'%'
       OR t.company_name ILIKE '%'||sqlc.arg(filter)||'%');

-- name: GetTenant :one
SELECT * FROM tenants WHERE tenant_id = $1;

-- name: GetTenantBySchema :one
SELECT * FROM tenants WHERE schema_name = $1;

-- name: CreateTenant :one
INSERT INTO tenants (tenant_name, company_name, schema_name, tenant_code, contact_phone, contact_email, org_type, organization_type_id, created_by)
VALUES (sqlc.arg(tenant_name), sqlc.arg(company_name), sqlc.arg(schema_name), sqlc.arg(tenant_code), sqlc.arg(contact_phone), sqlc.arg(contact_email), sqlc.arg(org_type), sqlc.arg(organization_type_id), sqlc.arg(created_by))
RETURNING *;

-- name: UpdateTenantStatus :exec
UPDATE tenants SET is_active = sqlc.arg(is_active),
    status = CASE WHEN sqlc.arg(is_active)::bool THEN 'Active' ELSE 'Suspended' END,
    updated_at = now()
WHERE tenant_id = sqlc.arg(tenant_id);

-- name: AddTenantWhatsappBalance :one
UPDATE tenants
SET whatsapp_balance = COALESCE(whatsapp_balance, 0) + sqlc.arg(amount_to_add), updated_at = now()
WHERE tenant_id = sqlc.arg(tenant_id)
RETURNING *;

-- name: GetTenantStats :one
SELECT count(*)::bigint AS total_tenants,
       count(*) FILTER (WHERE is_active)::bigint AS active_tenants,
       COALESCE(sum(whatsapp_balance), 0)::bigint AS total_balance
FROM tenants;

-- name: SetTenantSchemaName :exec
UPDATE tenants SET schema_name = sqlc.arg(schema_name) WHERE tenant_id = sqlc.arg(tenant_id);

-- name: ListAllTenantSchemas :many
SELECT tenant_id, schema_name FROM tenants ORDER BY tenant_id;

-- name: MaxTenantID :one
SELECT COALESCE(max(tenant_id), 0)::bigint FROM tenants;
