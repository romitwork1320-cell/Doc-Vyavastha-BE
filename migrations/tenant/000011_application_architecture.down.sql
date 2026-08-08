DROP TABLE IF EXISTS application_timeline;

ALTER TABLE application_requirements DROP COLUMN IF EXISTS display_order;
ALTER TABLE application_requirements DROP COLUMN IF EXISTS is_required;

ALTER TABLE applications DROP COLUMN IF EXISTS deleted_by;
ALTER TABLE applications DROP COLUMN IF EXISTS deleted_at;
ALTER TABLE applications DROP COLUMN IF EXISTS archived_by;
ALTER TABLE applications DROP COLUMN IF EXISTS archived_at;
