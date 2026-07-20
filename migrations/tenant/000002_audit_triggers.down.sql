-- 000002_audit_triggers.down.sql

DROP TRIGGER IF EXISTS trg_applications_audit ON student_applications;
DROP TRIGGER IF EXISTS trg_students_audit ON students;
