CREATE TABLE document_types (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name VARCHAR(255) NOT NULL UNIQUE,
    description TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Wipe existing data to safely alter columns
TRUNCATE TABLE application_type_documents CASCADE;
TRUNCATE TABLE application_types CASCADE;

ALTER TABLE application_type_documents
DROP COLUMN document_name,
DROP COLUMN description,
ADD COLUMN document_type_id BIGINT NOT NULL REFERENCES document_types(id) ON DELETE CASCADE;

ALTER TABLE application_type_documents
ADD CONSTRAINT unique_application_document UNIQUE (application_type_id, document_type_id);
