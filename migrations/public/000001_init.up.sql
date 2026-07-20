-- Public schema: centralized identity & auth. Integer PKs (the FE decodes
-- UserId/TenantId as integers from the JWT).

CREATE TABLE users (
    id              BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    email           VARCHAR(255) NOT NULL,
    password_hash   TEXT,
    google_id       VARCHAR(255),
    is_active       BOOLEAN NOT NULL DEFAULT TRUE,
    is_verified     BOOLEAN NOT NULL DEFAULT FALSE,
    last_login_at   TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at      TIMESTAMPTZ
);
-- Case-insensitive unique login identity (email doubles as username).
CREATE UNIQUE INDEX uq_users_email_lower ON users (lower(email)) WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX uq_users_google_id ON users (google_id) WHERE google_id IS NOT NULL;

-- Per-user personal profile (UserProfileDto).
CREATE TABLE user_profiles (
    user_id         BIGINT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    first_name      VARCHAR(150),
    last_name       VARCHAR(150),
    job_title       VARCHAR(150),
    contact_number  VARCHAR(30),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Hashed refresh tokens (delivered to the FE via httpOnly cookie).
CREATE TABLE refresh_tokens (
    id          BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id     BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    tenant_id   BIGINT,
    token_hash  TEXT NOT NULL,
    user_agent  TEXT,
    ip_address  VARCHAR(100),
    expires_at  TIMESTAMPTZ NOT NULL,
    revoked_at  TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_refresh_tokens_user ON refresh_tokens (user_id);
CREATE UNIQUE INDEX uq_refresh_tokens_hash ON refresh_tokens (token_hash);

-- One-time OTP codes for passwordless / 2-step login (hashed).
CREATE TABLE otp_codes (
    id          BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    email       VARCHAR(255) NOT NULL,
    code_hash   TEXT NOT NULL,
    purpose     VARCHAR(30) NOT NULL DEFAULT 'login',
    attempts    INT NOT NULL DEFAULT 0,
    expires_at  TIMESTAMPTZ NOT NULL,
    consumed_at TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_otp_email ON otp_codes (lower(email), purpose);

-- Email verification & password-set tokens (hashed).
CREATE TABLE auth_tokens (
    id          BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id     BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash  TEXT NOT NULL,
    purpose     VARCHAR(30) NOT NULL, -- 'verify_email' | 'set_password'
    expires_at  TIMESTAMPTZ NOT NULL,
    consumed_at TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX uq_auth_tokens_hash ON auth_tokens (token_hash);


-- Tenant registry, user<->tenant membership, RBAC, and per-tenant branding.

CREATE TABLE tenants (
    tenant_id           BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    tenant_name         VARCHAR(255) NOT NULL,
    company_name        VARCHAR(255),
    tenant_code         VARCHAR(50),
    schema_name         VARCHAR(100) NOT NULL,   -- e.g. tenant_0001 (never exposed to FE)
    subdomain           VARCHAR(100),
    status              VARCHAR(30) NOT NULL DEFAULT 'Active',
    contact_email       VARCHAR(255),
    contact_phone       VARCHAR(30),
    logo_url            TEXT,
    whatsapp_balance    INTEGER,
    whatsapp_sent_count BIGINT NOT NULL DEFAULT 0,
    is_active           BOOLEAN NOT NULL DEFAULT TRUE,
    created_by          BIGINT,
    updated_by          BIGINT,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ
);
CREATE UNIQUE INDEX uq_tenants_schema_name ON tenants (schema_name);
CREATE UNIQUE INDEX uq_tenants_code ON tenants (tenant_code) WHERE tenant_code IS NOT NULL;

CREATE TABLE roles (
    role_id     BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    role_name   VARCHAR(100) NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX uq_roles_name ON roles (lower(role_name));

-- Many-to-many membership; a user can belong to several tenants with a role each.
CREATE TABLE user_tenants (
    id          BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id     BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    tenant_id   BIGINT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
    role_id     BIGINT REFERENCES roles(role_id),
    status      VARCHAR(30) NOT NULL DEFAULT 'Active', -- Active | Inactive | Invited
    is_owner    BOOLEAN NOT NULL DEFAULT FALSE,
    joined_at   TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX uq_user_tenants ON user_tenants (user_id, tenant_id);
CREATE INDEX idx_user_tenants_tenant ON user_tenants (tenant_id);

-- Application pages used for permission management (route_url is what the FE
-- compares against; the permission string[] endpoint returns these urls).
CREATE TABLE pages (
    page_id        BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    page_name      VARCHAR(150) NOT NULL,
    route_url      VARCHAR(200) NOT NULL,
    permission_key VARCHAR(150),
    display_order  INT NOT NULL DEFAULT 0,
    parent_page_id BIGINT REFERENCES pages(page_id)
);

CREATE TABLE role_page_permissions (
    id          BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    role_id     BIGINT NOT NULL REFERENCES roles(role_id) ON DELETE CASCADE,
    page_id     BIGINT NOT NULL REFERENCES pages(page_id) ON DELETE CASCADE,
    can_view    BOOLEAN NOT NULL DEFAULT FALSE,
    can_add     BOOLEAN NOT NULL DEFAULT FALSE,
    can_edit    BOOLEAN NOT NULL DEFAULT FALSE,
    can_delete  BOOLEAN NOT NULL DEFAULT FALSE
);
CREATE UNIQUE INDEX uq_role_page ON role_page_permissions (role_id, page_id);

-- Optional per-user grant overrides (grant-permission endpoint).
CREATE TABLE user_permissions (
    id          BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id     BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    tenant_id   BIGINT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
    page_id     BIGINT NOT NULL REFERENCES pages(page_id) ON DELETE CASCADE,
    can_view    BOOLEAN NOT NULL DEFAULT FALSE
);
CREATE UNIQUE INDEX uq_user_page ON user_permissions (user_id, tenant_id, page_id);

-- Per-tenant company branding/details (CompanyProfileDto).
CREATE TABLE company_profiles (
    tenant_id       BIGINT PRIMARY KEY REFERENCES tenants(tenant_id) ON DELETE CASCADE,
    company_name    VARCHAR(255),
    company_logo_url TEXT,
    contact_email   VARCHAR(255),
    contact_phone   VARCHAR(30),
    support_phone   VARCHAR(30),
    website         VARCHAR(255),
    address_line1   VARCHAR(255),
    address_line2   VARCHAR(255),
    city            VARCHAR(120),
    state           VARCHAR(120),
    country         VARCHAR(120),
    postal_code     VARCHAR(20),
    gstin           VARCHAR(30),
    pan             VARCHAR(20),
    cin_no          VARCHAR(40),
    msme_no         VARCHAR(40),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);


-- SaaS subscription plans, tenant subscriptions, promo codes and payments.

CREATE TABLE subscription_plans (
    plan_id          BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    plan_name        VARCHAR(100) NOT NULL,
    code             VARCHAR(50),
    price            NUMERIC(12,2) NOT NULL DEFAULT 0,
    duration_months  INT NOT NULL DEFAULT 0,
    duration_days    INT NOT NULL DEFAULT 0,
    max_users        INT,
    max_students     INT,
    whatsapp_credits INT NOT NULL DEFAULT 0,
    features_json    JSONB,
    is_trial         BOOLEAN NOT NULL DEFAULT FALSE,
    is_active        BOOLEAN NOT NULL DEFAULT TRUE,
    status           VARCHAR(30) NOT NULL DEFAULT 'Active',
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX uq_plan_code ON subscription_plans (code) WHERE code IS NOT NULL;

CREATE TABLE tenant_subscriptions (
    subscription_id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    tenant_id       BIGINT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
    plan_id         BIGINT NOT NULL REFERENCES subscription_plans(plan_id),
    start_date      DATE NOT NULL,
    end_date        DATE NOT NULL,
    status          VARCHAR(30) NOT NULL DEFAULT 'Active', -- Active | Expired | Cancelled
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_tenant_subs_tenant ON tenant_subscriptions (tenant_id);

CREATE TABLE promo_codes (
    promo_id       BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    code           VARCHAR(50) NOT NULL,
    discount_type  VARCHAR(20) NOT NULL DEFAULT 'percent', -- percent | flat
    discount_value NUMERIC(12,2) NOT NULL DEFAULT 0,
    extension_days INT NOT NULL DEFAULT 0,
    max_uses       INT,
    used_count     INT NOT NULL DEFAULT 0,
    valid_from     DATE,
    valid_to       DATE,
    is_active      BOOLEAN NOT NULL DEFAULT TRUE,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX uq_promo_code ON promo_codes (upper(code));

-- Manual / recorded subscription payments (billing trail).
CREATE TABLE tenant_payments (
    payment_id      BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    tenant_id       BIGINT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
    plan_id         BIGINT REFERENCES subscription_plans(plan_id),
    promo_code_id   BIGINT REFERENCES promo_codes(promo_id),
    original_amount NUMERIC(12,2) NOT NULL DEFAULT 0,
    discount_amount NUMERIC(12,2) NOT NULL DEFAULT 0,
    final_amount    NUMERIC(12,2) NOT NULL DEFAULT 0,
    payment_mode    VARCHAR(50),
    reference_number VARCHAR(100),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_tenant_payments_tenant ON tenant_payments (tenant_id);


-- Audit logs, support tickets, and in-app notifications.

CREATE TABLE system_logs (
    log_id      BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    log_date    TIMESTAMPTZ NOT NULL DEFAULT now(),
    log_level   VARCHAR(20) NOT NULL DEFAULT 'Info',
    source      VARCHAR(200),
    message     TEXT,
    stack_trace TEXT,
    tenant_name VARCHAR(255),
    user_id     VARCHAR(100),
    request_url TEXT,
    ip_address  VARCHAR(100)
);
CREATE INDEX idx_system_logs_date ON system_logs (log_date DESC);

-- Per-request audit trail (UserActivity).
CREATE TABLE user_activities (
    id          BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id     VARCHAR(100) NOT NULL,
    user_name   VARCHAR(255),
    tenant_id   BIGINT,
    url         TEXT NOT NULL,
    method      VARCHAR(10) NOT NULL,
    ip_address  VARCHAR(100),
    description TEXT,
    changes     TEXT,
    created_on  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_user_activities_created ON user_activities (created_on DESC);

CREATE TABLE support_tickets (
    ticket_id      BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    tenant_id      BIGINT NOT NULL,
    user_id        BIGINT NOT NULL,
    subject        VARCHAR(255) NOT NULL,
    category       VARCHAR(100),
    priority       VARCHAR(50),
    description    TEXT,
    attachment_url TEXT,
    status         VARCHAR(50) NOT NULL DEFAULT 'Open',
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_support_tickets_tenant ON support_tickets (tenant_id);

CREATE TABLE ticket_messages (
    id             BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    ticket_id      BIGINT NOT NULL REFERENCES support_tickets(ticket_id) ON DELETE CASCADE,
    sender_user_id BIGINT NOT NULL,
    is_admin_reply BOOLEAN NOT NULL DEFAULT FALSE,
    message        TEXT NOT NULL,
    attachment_url TEXT,
    created_on     TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_ticket_messages_ticket ON ticket_messages (ticket_id);

CREATE TABLE notifications (
    id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id    BIGINT NOT NULL,
    tenant_id  BIGINT,
    title      VARCHAR(255),
    body       TEXT,
    type       VARCHAR(50),
    link       TEXT,
    is_read    BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_notifications_user ON notifications (user_id, is_read);



