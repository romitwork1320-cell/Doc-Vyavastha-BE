CREATE TABLE organization_types (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name VARCHAR(255) NOT NULL UNIQUE,
    description TEXT,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Seed initial data
INSERT INTO organization_types (name) VALUES 
('Advocate'),
('Chartered Accountant'),
('Notary'),
('Property Consultant'),
('Aadhaar Kendra'),
('Insurance Consultant'),
('Tax Consultant'),
('Other');

-- Add foreign key to tenants table
ALTER TABLE tenants
ADD COLUMN organization_type_id BIGINT REFERENCES organization_types(id) ON DELETE SET NULL;
