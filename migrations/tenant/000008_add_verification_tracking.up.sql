ALTER TABLE application_fee_collections ADD COLUMN verified_by BIGINT;
ALTER TABLE application_fee_collections ADD COLUMN verified_at TIMESTAMPTZ;
