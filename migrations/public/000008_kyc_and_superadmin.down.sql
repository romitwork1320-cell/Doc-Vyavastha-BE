
DROP TABLE IF EXISTS tenant_documents;

ALTER TABLE tenants DROP COLUMN IF EXISTS kyc_rejection_reason;
ALTER TABLE tenants DROP COLUMN IF EXISTS kyc_rejected_at;
ALTER TABLE tenants DROP COLUMN IF EXISTS kyc_rejected_by;
ALTER TABLE tenants DROP COLUMN IF EXISTS kyc_verified_at;
ALTER TABLE tenants DROP COLUMN IF EXISTS kyc_verified_by;
ALTER TABLE tenants DROP COLUMN IF EXISTS kyc_status;
ALTER TABLE tenants DROP COLUMN IF EXISTS org_type;

ALTER TABLE users DROP COLUMN IF EXISTS is_superadmin;
