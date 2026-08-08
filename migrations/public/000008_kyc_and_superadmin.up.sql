
ALTER TABLE users ADD COLUMN is_superadmin BOOLEAN NOT NULL DEFAULT FALSE;

ALTER TABLE tenants ADD COLUMN org_type VARCHAR(100);
ALTER TABLE tenants ADD COLUMN kyc_status VARCHAR(50) NOT NULL DEFAULT 'PENDING_SUBMISSION';
ALTER TABLE tenants ADD COLUMN kyc_verified_by BIGINT REFERENCES users(id);
ALTER TABLE tenants ADD COLUMN kyc_verified_at TIMESTAMPTZ;
ALTER TABLE tenants ADD COLUMN kyc_rejected_by BIGINT REFERENCES users(id);
ALTER TABLE tenants ADD COLUMN kyc_rejected_at TIMESTAMPTZ;
ALTER TABLE tenants ADD COLUMN kyc_rejection_reason TEXT;

CREATE TABLE tenant_documents (
    id              BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    tenant_id       BIGINT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
    document_type   VARCHAR(50) NOT NULL,
    original_name   VARCHAR(255) NOT NULL,
    stored_name     VARCHAR(255) NOT NULL,
    mime_type       VARCHAR(100) NOT NULL,
    storage_path    TEXT NOT NULL,
    uploaded_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    uploaded_by     BIGINT NOT NULL REFERENCES users(id)
);
CREATE INDEX idx_tenant_documents_tenant ON tenant_documents(tenant_id);
