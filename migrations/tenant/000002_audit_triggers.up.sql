-- 000002_audit_triggers.up.sql

CREATE TRIGGER trg_students_audit
AFTER INSERT OR UPDATE OR DELETE ON students
FOR EACH ROW EXECUTE FUNCTION public.log_audit_activity();

CREATE TRIGGER trg_applications_audit
AFTER INSERT OR UPDATE OR DELETE ON student_applications
FOR EACH ROW EXECUTE FUNCTION public.log_audit_activity();
