ALTER TABLE user_activities ADD COLUMN branch_id UUID;

CREATE OR REPLACE FUNCTION public.log_audit_activity()
RETURNS TRIGGER AS $$
DECLARE
    v_user_id VARCHAR(100);
    v_user_name VARCHAR(255);
    v_ip VARCHAR(100);
    v_url TEXT;
    v_method VARCHAR(10);
    v_tenant_id BIGINT;
    v_branch_id UUID;
    v_changes TEXT;
    v_description TEXT;
    v_record JSON;
    v_record_id TEXT;
BEGIN
    -- Read session variables (with fallback to defaults)
    v_user_id := current_setting('app.user_id', true);
    IF v_user_id IS NULL OR v_user_id = '' THEN
        v_user_id := 'system';
    END IF;

    v_user_name := current_setting('app.user_name', true);
    v_ip := current_setting('app.ip_address', true);
    v_url := current_setting('app.request_url', true);
    v_method := current_setting('app.request_method', true);
    
    BEGIN
        v_tenant_id := NULLIF(current_setting('app.tenant_id', true), '')::BIGINT;
    EXCEPTION WHEN OTHERS THEN
        v_tenant_id := NULL;
    END;

    BEGIN
        v_branch_id := NULLIF(current_setting('app.current_branch_id', true), '')::UUID;
    EXCEPTION WHEN OTHERS THEN
        v_branch_id := NULL;
    END;

    IF TG_OP = 'INSERT' THEN
        v_changes := row_to_json(NEW)::TEXT;
        v_description := 'Created record in ' || TG_TABLE_NAME;
        v_record := row_to_json(NEW);
    ELSIF TG_OP = 'UPDATE' THEN
        -- Store old and new row state in JSON to make it easy to parse.
        v_changes := json_build_object('old', row_to_json(OLD), 'new', row_to_json(NEW))::TEXT;
        v_description := 'Updated record in ' || TG_TABLE_NAME;
        v_record := row_to_json(NEW);
    ELSIF TG_OP = 'DELETE' THEN
        v_changes := row_to_json(OLD)::TEXT;
        v_description := 'Deleted record in ' || TG_TABLE_NAME;
        v_record := row_to_json(OLD);
    END IF;

    -- Try to extract a human-readable identifier to append to the description
    v_record_id := v_record->>'full_name';
    IF v_record_id IS NULL THEN
        v_record_id := v_record->>'name';
    END IF;
    IF v_record_id IS NULL THEN
        v_record_id := v_record->>'student_code';
    END IF;
    IF v_record_id IS NULL THEN
        v_record_id := v_record->>'application_number';
    END IF;
    
    IF v_record_id IS NOT NULL AND v_record_id != '' THEN
        v_description := v_description || ' (' || v_record_id || ')';
    END IF;

    -- url and method are required by the schema (NOT NULL), so provide defaults
    IF v_url IS NULL OR v_url = '' THEN v_url := 'Internal DB Trigger'; END IF;
    IF v_method IS NULL OR v_method = '' THEN v_method := 'TRIGGER'; END IF;

    -- Write to public.user_activities
    INSERT INTO public.user_activities (
        user_id, user_name, tenant_id, branch_id, url, method, ip_address, description, changes
    ) VALUES (
        v_user_id, v_user_name, v_tenant_id, v_branch_id, v_url, v_method, v_ip, v_description, v_changes
    );

    IF TG_OP = 'DELETE' THEN
        RETURN OLD;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
