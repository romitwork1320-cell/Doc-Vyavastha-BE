ALTER TABLE student_applications ADD COLUMN college_name VARCHAR(255) NOT NULL DEFAULT '';
DROP TABLE IF EXISTS student_application_colleges;
DROP TABLE IF EXISTS colleges;


DROP TABLE student_assigned_codes;


ALTER TABLE students ADD COLUMN first_name VARCHAR(100);
ALTER TABLE students ADD COLUMN middle_name VARCHAR(100);
ALTER TABLE students ADD COLUMN last_name VARCHAR(100);

-- Try to split full_name back (best effort)
UPDATE students SET 
    first_name = SPLIT_PART(full_name, ' ', 1),
    last_name = SUBSTRING(full_name FROM POSITION(' ' IN full_name) + 1);

-- Make required fields NOT NULL
UPDATE students SET first_name = 'Unknown' WHERE first_name IS NULL OR first_name = '';
UPDATE students SET last_name = 'Unknown' WHERE last_name IS NULL OR last_name = '';
ALTER TABLE students ALTER COLUMN first_name SET NOT NULL;
ALTER TABLE students ALTER COLUMN last_name SET NOT NULL;

ALTER TABLE students DROP COLUMN full_name;
ALTER TABLE students DROP COLUMN caste_id;

DROP TABLE student_castes;


DROP TABLE IF EXISTS fee_type_categories;


ALTER TABLE student_applications ADD COLUMN form_type VARCHAR(50);

UPDATE student_applications sa
SET form_type = ft.name
FROM form_types ft
WHERE sa.form_type_id = ft.id;

ALTER TABLE student_applications DROP COLUMN form_type_id;

DROP TABLE form_types;


ALTER TABLE student_applications DROP COLUMN IF EXISTS deleted_by;
DROP INDEX IF EXISTS uq_application_number;
DROP SEQUENCE IF EXISTS app_seq;


DROP INDEX IF EXISTS uq_student_mobile;


DROP INDEX IF EXISTS uq_year_config_cat_active;
DROP INDEX IF EXISTS uq_prefix;

DROP INDEX IF EXISTS uq_app_type_name;

DROP INDEX IF EXISTS uq_app_status_name;

CREATE UNIQUE INDEX uq_year_config_cat_year ON student_code_year_configs (category_id, business_year);

ALTER TABLE fee_types DROP COLUMN IF EXISTS amount;


DROP SEQUENCE IF EXISTS receipt_seq;
DROP SEQUENCE IF EXISTS payment_seq;
DROP TABLE IF EXISTS student_payments;
DROP TABLE IF EXISTS student_fee_plans;
DROP TABLE IF EXISTS fee_types;
DROP TABLE IF EXISTS student_applications;
DROP TABLE IF EXISTS application_statuses;
DROP TABLE IF EXISTS application_types;
DROP TABLE IF EXISTS students;
DROP TABLE IF EXISTS student_code_sequences;
DROP TABLE IF EXISTS student_code_year_configs;
DROP TABLE IF EXISTS student_categories;


