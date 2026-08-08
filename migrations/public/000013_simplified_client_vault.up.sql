-- Drop the old table completely. This is acceptable as we are redesigning the vault.
DROP TABLE IF EXISTS document_access_grants CASCADE;
DROP TABLE IF EXISTS client_documents CASCADE;

CREATE TABLE client_documents (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    client_id BIGINT NOT NULL REFERENCES client_profiles(id) ON DELETE CASCADE,
    document_type_id BIGINT NOT NULL REFERENCES document_types(id) ON DELETE RESTRICT,
    file_name TEXT NOT NULL,
    original_file_name TEXT NOT NULL,
    file_size BIGINT NOT NULL,
    mime_type VARCHAR(100) NOT NULL,
    sha256_hash VARCHAR(64),
    storage_path TEXT NOT NULL,
    uploaded_by BIGINT REFERENCES users(id) ON DELETE SET NULL,
    source VARCHAR(50) NOT NULL DEFAULT 'MANUAL_UPLOAD',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_client_documents_client ON client_documents (client_id);
CREATE INDEX idx_client_documents_type ON client_documents (document_type_id);

CREATE TABLE document_access_grants (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    document_id BIGINT NOT NULL REFERENCES client_documents(id) ON DELETE CASCADE,
    connection_id BIGINT NOT NULL REFERENCES client_connections(id) ON DELETE CASCADE,
    granted_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX uq_document_access_grants ON document_access_grants (document_id, connection_id);
