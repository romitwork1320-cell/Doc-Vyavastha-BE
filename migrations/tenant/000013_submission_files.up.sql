CREATE TABLE application_document_version_files (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    version_id BIGINT NOT NULL REFERENCES application_document_versions(id) ON DELETE CASCADE,
    file_url TEXT NOT NULL,
    document_name TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_application_doc_version_files_version ON application_document_version_files(version_id);

-- Migrate existing file_urls to the new table
INSERT INTO application_document_version_files (version_id, file_url, document_name)
SELECT id, file_url, 'Document' FROM application_document_versions;

-- Drop the old file_url column
ALTER TABLE application_document_versions DROP COLUMN file_url;
