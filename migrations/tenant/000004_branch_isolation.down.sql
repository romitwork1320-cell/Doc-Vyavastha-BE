-- Remove NOT NULL constraints first
ALTER TABLE students ALTER COLUMN branch_id DROP NOT NULL;
ALTER TABLE student_applications ALTER COLUMN branch_id DROP NOT NULL;
ALTER TABLE student_fee_plans ALTER COLUMN branch_id DROP NOT NULL;
ALTER TABLE student_payments ALTER COLUMN branch_id DROP NOT NULL;

-- Remove columns
ALTER TABLE student_applications DROP COLUMN IF EXISTS branch_id;
ALTER TABLE student_fee_plans DROP COLUMN IF EXISTS branch_id;
ALTER TABLE student_payments DROP COLUMN IF EXISTS branch_id;
