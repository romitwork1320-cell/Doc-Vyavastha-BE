-- name: CreateClientDocument :one
INSERT INTO client_documents (
    client_id, document_type_id, file_name, original_file_name, file_size, mime_type, sha256_hash, storage_path, uploaded_by, source
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10
) RETURNING *;

-- name: GetClientDocument :one
SELECT * FROM client_documents
WHERE id = $1;

-- name: ListClientVaultFolders :many
SELECT 
    dt.id AS document_type_id, 
    dt.name AS document_type_name,
    COUNT(cd.id)::int AS file_count
FROM document_types dt
JOIN client_documents cd ON cd.document_type_id = dt.id
WHERE cd.client_id = $1
GROUP BY dt.id, dt.name
ORDER BY dt.name ASC;

-- name: ListClientVaultFiles :many
SELECT cd.*,
    dt.name AS document_type_name,
    COALESCE(u.first_name || ' ' || u.last_name, '')::varchar AS uploaded_by_name
FROM client_documents cd
JOIN document_types dt ON dt.id = cd.document_type_id
LEFT JOIN user_profiles u ON u.user_id = cd.uploaded_by
WHERE cd.client_id = $1 AND cd.document_type_id = $2
ORDER BY cd.created_at DESC;

-- name: CreateDocumentAccessGrant :one
INSERT INTO document_access_grants (document_id, connection_id)
VALUES ($1, $2)
RETURNING *;

-- name: GetDocumentAccessGrants :many
SELECT 
    dag.id, 
    dag.document_id, 
    dag.connection_id, 
    dag.granted_at,
    t.tenant_name AS organization_name
FROM document_access_grants dag
JOIN client_connections cc ON cc.id = dag.connection_id
JOIN tenants t ON t.tenant_id = cc.tenant_id
WHERE dag.document_id = $1
ORDER BY dag.granted_at DESC;

-- name: DeleteDocumentAccessGrant :exec
DELETE FROM document_access_grants
WHERE document_id = $1 AND connection_id = $2;

-- name: DeleteAllDocumentAccessGrantsForConnection :exec
DELETE FROM document_access_grants
WHERE connection_id = $1;

-- name: UpdateClientDocumentName :one
UPDATE client_documents
SET file_name = $2, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: CheckDocumentAccess :one
SELECT * FROM document_access_grants
WHERE document_id = $1 AND connection_id = $2;

-- name: DeleteClientDocument :exec
DELETE FROM client_documents
WHERE id = $1;
