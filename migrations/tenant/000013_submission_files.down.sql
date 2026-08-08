ALTER TABLE application_document_versions ADD COLUMN file_url TEXT;

UPDATE application_document_versions v
SET file_url = (
    SELECT f.file_url 
    FROM application_document_version_files f 
    WHERE f.version_id = v.id 
    ORDER BY f.created_at ASC 
    LIMIT 1
);

ALTER TABLE application_document_versions ALTER COLUMN file_url SET NOT NULL;

DROP TABLE IF EXISTS application_document_version_files;
