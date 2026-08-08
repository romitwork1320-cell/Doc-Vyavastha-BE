
-- name: UpdateTenantKYCStatus :exec
UPDATE tenants
SET kyc_status = sqlc.arg(status)
WHERE tenant_id = sqlc.arg(tenant_id);

-- name: VerifyTenantKYC :exec
UPDATE tenants
SET kyc_status = 'VERIFIED',
    kyc_verified_by = sqlc.arg(verified_by),
    kyc_verified_at = now()
WHERE tenant_id = sqlc.arg(tenant_id);

-- name: RejectTenantKYC :exec
UPDATE tenants
SET kyc_status = 'REJECTED',
    kyc_rejected_by = sqlc.arg(rejected_by),
    kyc_rejected_at = now(),
    kyc_rejection_reason = sqlc.arg(reason)
WHERE tenant_id = sqlc.arg(tenant_id);

-- name: InsertTenantDocument :one
INSERT INTO tenant_documents (tenant_id, document_type, original_name, stored_name, mime_type, storage_path, uploaded_by)
VALUES (sqlc.arg(tenant_id), sqlc.arg(document_type), sqlc.arg(original_name), sqlc.arg(stored_name), sqlc.arg(mime_type), sqlc.arg(storage_path), sqlc.arg(uploaded_by))
RETURNING *;

-- name: GetTenantDocuments :many
SELECT * FROM tenant_documents
WHERE tenant_id = sqlc.arg(tenant_id)
ORDER BY uploaded_at DESC;

-- name: DeleteTenantDocumentsByType :exec
DELETE FROM tenant_documents
WHERE tenant_id = sqlc.arg(tenant_id) AND document_type = sqlc.arg(document_type);
