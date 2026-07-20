CREATE TABLE application_fee_collections (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    application_id UUID NOT NULL REFERENCES student_applications(id) ON DELETE CASCADE,
    staff_id BIGINT NOT NULL,
    college_fee_amount DECIMAL(12,2) NOT NULL DEFAULT 0,
    payment_to_college_method VARCHAR(50) NOT NULL,
    student_reimbursement_method VARCHAR(50) NOT NULL,
    student_transaction_ref VARCHAR(100),
    admin_verification_status VARCHAR(50) NOT NULL DEFAULT 'Pending',
    admin_remarks TEXT,
    branch_id UUID NOT NULL REFERENCES branches(id) ON DELETE CASCADE,
    created_by BIGINT,
    updated_by BIGINT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ,
    deleted_by BIGINT
);

CREATE INDEX idx_application_fee_collections_branch_id ON application_fee_collections(branch_id);
CREATE INDEX idx_application_fee_collections_staff_id ON application_fee_collections(staff_id);
CREATE INDEX idx_application_fee_collections_application_id ON application_fee_collections(application_id);
