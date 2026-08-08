DROP TABLE IF EXISTS connection_permissions CASCADE;

ALTER TABLE client_connections DROP COLUMN IF EXISTS connected_by;
ALTER TABLE client_connections DROP COLUMN IF EXISTS accepted_at;
ALTER TABLE client_connections DROP COLUMN IF EXISTS accepted_by;
ALTER TABLE client_connections DROP COLUMN IF EXISTS removed_at;
ALTER TABLE client_connections DROP COLUMN IF EXISTS removed_by;
ALTER TABLE client_connections DROP COLUMN IF EXISTS status_reason;
