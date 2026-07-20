-- 1. Add branch_id to child operational tables
ALTER TABLE student_applications ADD COLUMN branch_id UUID REFERENCES branches(id);
ALTER TABLE student_fee_plans ADD COLUMN branch_id UUID REFERENCES branches(id);
ALTER TABLE student_payments ADD COLUMN branch_id UUID REFERENCES branches(id);

-- 2. Data Migration: Assign existing students and operational data to a default branch
DO $$
DECLARE
    default_branch_id UUID;
BEGIN
    -- Try to find an existing branch
    SELECT id INTO default_branch_id FROM branches ORDER BY created_at ASC LIMIT 1;
    
    -- If no branch exists, create one (safety measure for existing setups)
    IF default_branch_id IS NULL THEN
        INSERT INTO branches (name, code, address, status) 
        VALUES ('Main Branch', 'MAIN', 'Default Branch Address', 'Active')
        RETURNING id INTO default_branch_id;
    END IF;

    -- Update existing students
    UPDATE students SET branch_id = default_branch_id WHERE branch_id IS NULL;
    
    -- Update existing child records (applications, fee plans, payments)
    UPDATE student_applications SET branch_id = default_branch_id WHERE branch_id IS NULL;
    UPDATE student_fee_plans SET branch_id = default_branch_id WHERE branch_id IS NULL;
    UPDATE student_payments SET branch_id = default_branch_id WHERE branch_id IS NULL;
END $$;

-- Make branch_id NOT NULL on students and child tables now that migration is done
ALTER TABLE students ALTER COLUMN branch_id SET NOT NULL;
ALTER TABLE student_applications ALTER COLUMN branch_id SET NOT NULL;
ALTER TABLE student_fee_plans ALTER COLUMN branch_id SET NOT NULL;
ALTER TABLE student_payments ALTER COLUMN branch_id SET NOT NULL;
