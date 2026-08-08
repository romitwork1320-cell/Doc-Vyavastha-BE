ALTER TABLE application_type_documents
DROP CONSTRAINT unique_application_document,
DROP COLUMN document_type_id,
ADD COLUMN document_name VARCHAR(255) NOT NULL DEFAULT 'Unknown',
ADD COLUMN description TEXT;

DROP TABLE document_types;
