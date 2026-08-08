-- Add document_type_id column as nullable initially
ALTER TABLE application_requirements 
ADD COLUMN document_type_id BIGINT REFERENCES public.document_types(id) ON DELETE RESTRICT;

-- Backfill legacy records by looking up the document_types table based on the document_name
UPDATE application_requirements ar
SET document_type_id = (
    SELECT id FROM public.document_types dt 
    WHERE dt.name = ar.document_name 
    LIMIT 1
);

-- Set any NULLs to the "Other" document type id if it exists.
UPDATE application_requirements
SET document_type_id = (SELECT id FROM public.document_types WHERE name = 'Other' LIMIT 1)
WHERE document_type_id IS NULL;

-- If there are still NULLs (e.g. "Other" doesn't exist), delete them to satisfy NOT NULL.
DELETE FROM application_requirements WHERE document_type_id IS NULL;

-- Now we can safely make it NOT NULL
ALTER TABLE application_requirements
ALTER COLUMN document_type_id SET NOT NULL;
