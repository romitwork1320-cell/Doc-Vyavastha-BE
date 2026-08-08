CREATE TABLE magic_links (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    token UUID NOT NULL UNIQUE,
    tenant_id BIGINT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
    application_id BIGINT NOT NULL,
    client_id BIGINT NOT NULL REFERENCES client_profiles(id) ON DELETE CASCADE,
    expires_at TIMESTAMPTZ NOT NULL,
    is_used BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_magic_links_token ON magic_links(token);
