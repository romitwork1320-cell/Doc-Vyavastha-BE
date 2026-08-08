DROP TABLE IF EXISTS application_document_versions;
DROP TABLE IF EXISTS application_requirements;

ALTER TABLE applications DROP COLUMN magic_link_token;
ALTER TABLE applications DROP COLUMN magic_link_expires_at;
ALTER TABLE applications DROP COLUMN final_deliverable_id;
