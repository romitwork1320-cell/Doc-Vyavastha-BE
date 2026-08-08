-- name: GetClientConnections :many
SELECT cc.*, cp.connection_code, cp.full_name, cp.user_id, u.email, up.contact_number as phone, up.first_name, up.last_name
FROM client_connections cc
JOIN client_profiles cp ON cc.client_id = cp.id
JOIN users u ON cp.user_id = u.id
LEFT JOIN user_profiles up ON u.id = up.user_id
WHERE cc.tenant_id = $1
AND (NULLIF($2::text, '') IS NULL OR cc.status = $2)
AND (NULLIF($5::text, '') IS NULL OR cp.full_name ILIKE '%' || $5 || '%' OR cp.connection_code ILIKE '%' || $5 || '%' OR u.email ILIKE '%' || $5 || '%' OR up.first_name ILIKE '%' || $5 || '%' OR up.last_name ILIKE '%' || $5 || '%')
ORDER BY cc.created_at DESC
LIMIT $4 OFFSET $3;

-- name: GetConnectionRequestsForTenant :many
SELECT cr.*, cp.connection_code, cp.full_name, cp.user_id, u.email, up.contact_number as phone
FROM connection_requests cr
JOIN client_profiles cp ON cr.client_id = cp.id
JOIN users u ON cp.user_id = u.id
LEFT JOIN user_profiles up ON u.id = up.user_id
WHERE cr.tenant_id = $1
AND (NULLIF($2::text, '') IS NULL OR cr.status = $2)
ORDER BY cr.created_at DESC
LIMIT $4 OFFSET $3;

-- name: GetConnectionRequestsForClient :many
SELECT cr.*, t.company_name as tenant_name
FROM connection_requests cr
JOIN tenants t ON cr.tenant_id = t.tenant_id
WHERE cr.client_id = $1
AND (NULLIF($2::text, '') IS NULL OR cr.status = $2)
ORDER BY cr.created_at DESC
LIMIT $4 OFFSET $3;

-- name: GetClientConnectionDetails :one
SELECT cc.*, cp.connection_code, cp.full_name, cp.user_id, cp.created_at as profile_created_at, u.email, up.contact_number as phone, up.first_name, up.last_name,
       cprms.view_profile, cprms.view_documents, cprms.upload_documents, cprms.create_applications, cprms.view_applications, cprms.approve_applications, cprms.manage_connection
FROM client_connections cc
JOIN client_profiles cp ON cc.client_id = cp.id
JOIN users u ON cp.user_id = u.id
LEFT JOIN user_profiles up ON u.id = up.user_id
LEFT JOIN connection_permissions cprms ON cc.id = cprms.connection_id
WHERE cc.tenant_id = $1 AND cc.client_id = $2;

-- name: GetConnectedOrganizationsForClient :many
SELECT cc.*, 
       t.company_name as tenant_name,
       cp.contact_email,
       cp.contact_phone,
       cp.address_line1,
       cp.address_line2,
       cp.city,
       cp.state,
       cp.country,
       cp.postal_code
FROM client_connections cc
JOIN tenants t ON cc.tenant_id = t.tenant_id
LEFT JOIN company_profiles cp ON t.tenant_id = cp.tenant_id
WHERE cc.client_id = $1
ORDER BY cc.created_at DESC;

-- name: CreateConnectionRequest :one
INSERT INTO connection_requests (tenant_id, client_id, status)
VALUES ($1, $2, 'PENDING')
RETURNING *;

-- name: GetConnectionRequest :one
SELECT * FROM connection_requests WHERE id = $1;

-- name: UpdateConnectionRequestStatus :one
UPDATE connection_requests
SET status = $2
WHERE id = $1
RETURNING *;

-- name: CreateClientConnection :one
INSERT INTO client_connections (tenant_id, client_id, status, connected_by, accepted_at, accepted_by)
VALUES ($1, $2, 'ACTIVE', $3, $4, $5)
RETURNING *;

-- name: UpdateClientConnectionStatus :one
UPDATE client_connections
SET status = $2, removed_at = $3, removed_by = $4, status_reason = $5
WHERE id = $1
RETURNING *;

-- name: UpsertConnectionPermissions :one
INSERT INTO connection_permissions (connection_id, view_profile, view_documents, upload_documents, create_applications, view_applications, approve_applications, manage_connection)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
ON CONFLICT (connection_id)
DO UPDATE SET
    view_profile = EXCLUDED.view_profile,
    view_documents = EXCLUDED.view_documents,
    upload_documents = EXCLUDED.upload_documents,
    create_applications = EXCLUDED.create_applications,
    view_applications = EXCLUDED.view_applications,
    approve_applications = EXCLUDED.approve_applications,
    manage_connection = EXCLUDED.manage_connection,
    updated_at = now()
RETURNING *;









-- name: GetClientConnectionByTenantAndClient :one
SELECT * FROM client_connections
WHERE tenant_id = $1 AND client_id = $2;

-- name: GetClientProfileByUserId :one
SELECT * FROM client_profiles
WHERE user_id = $1 LIMIT 1;

-- name: GetClientProfileByCode :one
SELECT * FROM client_profiles
WHERE connection_code = $1 LIMIT 1;

-- name: CreateClientProfile :one
INSERT INTO client_profiles (user_id, connection_code, full_name)
VALUES ($1, $2, $3)
RETURNING *;



-- name: GetClientConnectionById :one
SELECT * FROM client_connections WHERE id = $1;




-- name: DeleteClientConnection :exec
DELETE FROM client_connections WHERE id = $1;
