DROP TABLE IF EXISTS document_access_grants CASCADE;
DROP TABLE IF EXISTS client_connections CASCADE;
DROP TABLE IF EXISTS connection_requests CASCADE;
DROP TABLE IF EXISTS client_documents CASCADE;
DROP TABLE IF EXISTS client_profiles CASCADE;

ALTER TABLE users DROP COLUMN IF EXISTS user_type;
