-- Tenant business schema template. Applied into each tenant's dedicated schema
-- (tenant_0001, ...). Tables are UNQUALIFIED so they materialize in whatever
-- schema is current (set via search_path at provision/migrate time). UUID PKs
-- match the FE's string IDs for the student domain. No tenant_id column needed
-- — isolation is physical (schema-per-tenant).

CREATE TABLE student_categories (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        VARCHAR(255) NOT NULL,
    description TEXT,
    status      VARCHAR(30) NOT NULL DEFAULT 'Active',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Year-wise student-code configuration (the doc's centerpiece).
CREATE TABLE student_code_year_configs (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    category_id    UUID NOT NULL REFERENCES student_categories(id) ON DELETE CASCADE,
    business_year  INT NOT NULL,
    prefix         VARCHAR(20) NOT NULL,
    separator      VARCHAR(10) NOT NULL DEFAULT '-',
    padding_length INT NOT NULL DEFAULT 4,
    reset_sequence BOOLEAN NOT NULL DEFAULT TRUE,
    is_active      BOOLEAN NOT NULL DEFAULT TRUE,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX uq_year_config_cat_year ON student_code_year_configs (category_id, business_year);

-- Running sequence per year-config (never derived from COUNT()).
CREATE TABLE student_code_sequences (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    year_config_id      UUID NOT NULL REFERENCES student_code_year_configs(id) ON DELETE CASCADE,
    current_number      BIGINT NOT NULL DEFAULT 0,
    last_generated_code VARCHAR(50),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX uq_sequence_year_config ON student_code_sequences (year_config_id);

CREATE TABLE students (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    student_code        VARCHAR(50) NOT NULL,
    category_id         UUID NOT NULL REFERENCES student_categories(id),
    year_config_id      UUID REFERENCES student_code_year_configs(id),
    first_name          VARCHAR(100) NOT NULL,
    middle_name         VARCHAR(100),
    last_name           VARCHAR(100) NOT NULL,
    father_name         VARCHAR(255),
    mother_name         VARCHAR(255),
    gender              VARCHAR(20),
    email               VARCHAR(255),
    primary_mobile      VARCHAR(20) NOT NULL,
    secondary_mobile    VARCHAR(20),
    whatsapp_mobile     VARCHAR(20),
    home_address        TEXT,
    city                VARCHAR(100),
    state               VARCHAR(100),
    pincode             VARCHAR(20),
    school_name         VARCHAR(255),
    passing_board       VARCHAR(100),
    tenth_passing_year  INT,
    twelfth_passing_year INT,
    scholarship_uid     VARCHAR(255),
    scholarship_password TEXT,
    profile_photo_url   TEXT,
    status              VARCHAR(30) NOT NULL DEFAULT 'Active',
    remarks             TEXT,
    created_by          BIGINT,
    updated_by          BIGINT,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at          TIMESTAMPTZ
);
CREATE UNIQUE INDEX uq_students_code ON students (student_code) WHERE deleted_at IS NULL;
CREATE INDEX idx_students_category ON students (category_id);

CREATE TABLE application_types (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        VARCHAR(255) NOT NULL,
    description TEXT,
    status      VARCHAR(30) NOT NULL DEFAULT 'Active',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE application_statuses (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name          VARCHAR(100) NOT NULL,
    description   TEXT,
    color_code    VARCHAR(20),
    status        VARCHAR(30) NOT NULL DEFAULT 'Active',
    display_order INT NOT NULL DEFAULT 0,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE student_applications (
    id                    UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    student_id            UUID NOT NULL REFERENCES students(id) ON DELETE CASCADE,
    application_type_id   UUID NOT NULL REFERENCES application_types(id),
    application_status_id UUID NOT NULL REFERENCES application_statuses(id),
    application_number    VARCHAR(100),
    application_name      VARCHAR(255),
    last_date             DATE,
    applied_date          DATE,
    submitted_date        DATE,
    form_type             VARCHAR(100),
    college_name          VARCHAR(255),
    portal_username       VARCHAR(255),   -- FE field: userId (application-portal login)
    portal_password       TEXT,           -- FE field: password
    remarks               TEXT,
    created_by            BIGINT,
    updated_by            BIGINT,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at            TIMESTAMPTZ
);
CREATE INDEX idx_student_apps_student ON student_applications (student_id);

CREATE TABLE fee_types (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        VARCHAR(255) NOT NULL,
    description TEXT,
    status      VARCHAR(30) NOT NULL DEFAULT 'Active',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE student_fee_plans (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    student_id      UUID NOT NULL REFERENCES students(id) ON DELETE CASCADE,
    fee_type_id     UUID REFERENCES fee_types(id),
    fee_name        VARCHAR(255),
    total_amount    NUMERIC(12,2) NOT NULL DEFAULT 0,
    discount_amount NUMERIC(12,2) NOT NULL DEFAULT 0,
    discount_reason VARCHAR(255),
    remarks         TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_fee_plans_student ON student_fee_plans (student_id);

CREATE TABLE student_payments (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    payment_number     VARCHAR(50) NOT NULL,
    receipt_number     VARCHAR(50),
    student_id         UUID NOT NULL REFERENCES students(id) ON DELETE CASCADE,
    student_fee_plan_id UUID NOT NULL REFERENCES student_fee_plans(id) ON DELETE CASCADE,
    payment_date       DATE NOT NULL,
    amount             NUMERIC(12,2) NOT NULL DEFAULT 0,
    payment_method     VARCHAR(50),
    reference_number   VARCHAR(100),
    remarks            TEXT,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_payments_student ON student_payments (student_id);
CREATE INDEX idx_payments_plan ON student_payments (student_fee_plan_id);

-- Per-tenant counters for human-friendly payment/receipt numbers.
CREATE SEQUENCE payment_seq START 1;
CREATE SEQUENCE receipt_seq START 1;


DROP INDEX IF EXISTS uq_year_config_cat_year;

DROP INDEX IF EXISTS uq_year_config_cat_active;
CREATE UNIQUE INDEX uq_year_config_cat_active ON student_code_year_configs (category_id) WHERE is_active = true;

DROP INDEX IF EXISTS uq_prefix;
CREATE UNIQUE INDEX uq_prefix ON student_code_year_configs (UPPER(TRIM(prefix)));

DROP INDEX IF EXISTS uq_app_type_name;
CREATE UNIQUE INDEX uq_app_type_name ON application_types (UPPER(TRIM(name)));

DROP INDEX IF EXISTS uq_app_status_name;
CREATE UNIQUE INDEX uq_app_status_name ON application_statuses (UPPER(TRIM(name)));

ALTER TABLE fee_types ADD COLUMN IF NOT EXISTS amount NUMERIC(12,2) NOT NULL DEFAULT 0;


CREATE UNIQUE INDEX uq_student_mobile ON students(primary_mobile) WHERE deleted_at IS NULL;


CREATE SEQUENCE app_seq START 1;
CREATE UNIQUE INDEX uq_application_number ON student_applications (application_number) WHERE deleted_at IS NULL;
ALTER TABLE student_applications ADD COLUMN deleted_by BIGINT;


CREATE TABLE form_types (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name          VARCHAR(255) NOT NULL,
    description   TEXT,
    status        VARCHAR(30) NOT NULL DEFAULT 'Active',
    display_order INT NOT NULL DEFAULT 0,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

INSERT INTO form_types (id, name, display_order) VALUES 
(gen_random_uuid(), 'Engineering', 1),
(gen_random_uuid(), 'Medical', 2),
(gen_random_uuid(), 'Agriculture', 3),
(gen_random_uuid(), 'Science', 4),
(gen_random_uuid(), 'Commerce', 5),
(gen_random_uuid(), 'Arts', 6),
(gen_random_uuid(), 'Other', 7);

ALTER TABLE student_applications RENAME COLUMN form_type TO old_form_type;
ALTER TABLE student_applications ADD COLUMN form_type_id UUID REFERENCES form_types(id);

UPDATE student_applications sa
SET form_type_id = ft.id
FROM form_types ft
WHERE sa.old_form_type = ft.name;

ALTER TABLE student_applications DROP COLUMN old_form_type;


CREATE TABLE fee_type_categories (
    fee_type_id UUID NOT NULL REFERENCES fee_types(id) ON DELETE CASCADE,
    category_id UUID NOT NULL REFERENCES student_categories(id) ON DELETE CASCADE,
    PRIMARY KEY (fee_type_id, category_id),
    CONSTRAINT uq_category_fee_type UNIQUE (category_id)
);


CREATE TABLE student_castes (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        VARCHAR(255) NOT NULL,
    description TEXT,
    status      VARCHAR(30) NOT NULL DEFAULT 'Active',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX uq_caste_name ON student_castes (UPPER(TRIM(name)));


ALTER TABLE students ADD COLUMN full_name VARCHAR(255);
ALTER TABLE students ADD COLUMN caste_id UUID REFERENCES student_castes(id);

UPDATE students SET full_name = TRIM(CONCAT_WS(' ', first_name, middle_name, last_name));

ALTER TABLE students ALTER COLUMN full_name SET NOT NULL;

ALTER TABLE students DROP COLUMN first_name;
ALTER TABLE students DROP COLUMN middle_name;
ALTER TABLE students DROP COLUMN last_name;


CREATE TABLE student_assigned_codes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    student_id UUID NOT NULL REFERENCES students(id),
    category_id UUID NOT NULL REFERENCES student_categories(id),
    year_config_id UUID NOT NULL REFERENCES student_code_year_configs(id),
    student_code VARCHAR(100) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(student_id, category_id)
);

-- Backfill data from existing students
INSERT INTO student_assigned_codes (student_id, category_id, year_config_id, student_code, created_at)
SELECT id, category_id, year_config_id, student_code, created_at
FROM students
WHERE deleted_at IS NULL;


CREATE TABLE colleges (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id BIGINT NOT NULL,
    name VARCHAR(255) NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT now(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT now(),
    deleted_at TIMESTAMP WITH TIME ZONE,
    created_by BIGINT,
    updated_by BIGINT,
    CONSTRAINT uq_tenant_college_name UNIQUE (tenant_id, name)
);

CREATE TABLE student_application_colleges (
    application_id UUID NOT NULL REFERENCES student_applications(id) ON DELETE CASCADE,
    college_id UUID NOT NULL REFERENCES colleges(id) ON DELETE CASCADE,
    PRIMARY KEY (application_id, college_id)
);

-- Drop the old column
ALTER TABLE student_applications DROP COLUMN IF EXISTS college_name;


