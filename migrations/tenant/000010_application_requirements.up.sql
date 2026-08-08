-- Add new fields to applications
ALTER TABLE applications ADD COLUMN magic_link_token UUID UNIQUE;
ALTER TABLE applications ADD COLUMN magic_link_expires_at TIMESTAMPTZ;
ALTER TABLE applications ADD COLUMN final_deliverable_id BIGINT REFERENCES public.client_documents(id) ON DELETE SET NULL;
ALTER TABLE applications ALTER COLUMN status SET DEFAULT 'DRAFT';

-- Application Requirements
CREATE TABLE application_requirements (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    application_id BIGINT NOT NULL REFERENCES applications(id) ON DELETE CASCADE,
    document_name VARCHAR(255) NOT NULL,
    description TEXT,
    status VARCHAR(50) NOT NULL DEFAULT 'PENDING',
    rejection_reason TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_application_reqs_app ON application_requirements(application_id);

-- Application Document Versions
CREATE TABLE application_document_versions (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    requirement_id BIGINT NOT NULL REFERENCES application_requirements(id) ON DELETE CASCADE,
    version_number INT NOT NULL,
    file_url TEXT NOT NULL,
    uploaded_by_client BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_application_doc_versions_req ON application_document_versions(requirement_id);
