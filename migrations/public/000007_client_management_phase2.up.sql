-- 1. Expand client_connections with auditing and status fields
ALTER TABLE client_connections ADD COLUMN connected_by BIGINT REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE client_connections ADD COLUMN accepted_at TIMESTAMPTZ;
ALTER TABLE client_connections ADD COLUMN accepted_by BIGINT REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE client_connections ADD COLUMN removed_at TIMESTAMPTZ;
ALTER TABLE client_connections ADD COLUMN removed_by BIGINT REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE client_connections ADD COLUMN status_reason TEXT;

-- 2. Create connection_permissions table
CREATE TABLE connection_permissions (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    connection_id BIGINT NOT NULL REFERENCES client_connections(id) ON DELETE CASCADE UNIQUE,
    view_profile BOOLEAN NOT NULL DEFAULT TRUE,
    view_documents BOOLEAN NOT NULL DEFAULT FALSE,
    upload_documents BOOLEAN NOT NULL DEFAULT FALSE,
    create_applications BOOLEAN NOT NULL DEFAULT FALSE,
    view_applications BOOLEAN NOT NULL DEFAULT FALSE,
    approve_applications BOOLEAN NOT NULL DEFAULT FALSE,
    manage_connection BOOLEAN NOT NULL DEFAULT FALSE,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);


