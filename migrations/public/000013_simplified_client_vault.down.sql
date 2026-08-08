DROP TABLE IF EXISTS document_access_grants;
DROP TABLE IF EXISTS client_documents;

-- Recreate old simple table
CREATE TABLE client_documents (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    client_id BIGINT NOT NULL REFERENCES client_profiles(id) ON DELETE CASCADE,
    document_name VARCHAR(200) NOT NULL,
    document_type VARCHAR(50) NOT NULL,
    file_url TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE document_access_grants (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    connection_id BIGINT NOT NULL REFERENCES client_connections(id) ON DELETE CASCADE,
    document_id BIGINT NOT NULL REFERENCES client_documents(id) ON DELETE CASCADE,
    granted_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
