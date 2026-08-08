-- Add archive and soft delete to applications
ALTER TABLE applications ADD COLUMN archived_at TIMESTAMPTZ;
ALTER TABLE applications ADD COLUMN archived_by BIGINT REFERENCES public.users(id) ON DELETE SET NULL;
ALTER TABLE applications ADD COLUMN deleted_at TIMESTAMPTZ;
ALTER TABLE applications ADD COLUMN deleted_by BIGINT REFERENCES public.users(id) ON DELETE SET NULL;

-- Enhance application requirements
ALTER TABLE application_requirements ADD COLUMN display_order INT NOT NULL DEFAULT 0;
ALTER TABLE application_requirements ADD COLUMN is_required BOOLEAN NOT NULL DEFAULT TRUE;

-- Create Application Timeline table
CREATE TABLE application_timeline (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    application_id BIGINT NOT NULL REFERENCES applications(id) ON DELETE CASCADE,
    event_type VARCHAR(50) NOT NULL,
    actor_type VARCHAR(50) NOT NULL, -- e.g., 'SYSTEM', 'ORGANIZATION', 'CLIENT'
    actor_id BIGINT,                 -- References user_id or client_id depending on actor_type
    description TEXT NOT NULL,
    metadata JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_app_timeline_appid ON application_timeline(application_id);
CREATE INDEX idx_app_timeline_created ON application_timeline(created_at);
