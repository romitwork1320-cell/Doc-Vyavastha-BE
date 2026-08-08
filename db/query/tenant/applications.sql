-- name: CreateApplication :one
INSERT INTO applications (
    client_id, title, status
) VALUES (
    $1, $2, 'OPEN'
) RETURNING *;

-- name: UpdateApplicationStatus :one
UPDATE applications
SET status = $2, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: SetMagicLinkToken :one
UPDATE applications
SET magic_link_token = $2, magic_link_expires_at = $3, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: GetApplicationByID :one
SELECT * FROM applications
WHERE id = $1;

-- name: ListApplicationsByClient :many
SELECT * FROM applications
WHERE client_id = $1 AND deleted_at IS NULL
ORDER BY created_at DESC;

-- name: ListApplications :many
SELECT * FROM applications
WHERE deleted_at IS NULL
ORDER BY created_at DESC;

-- name: CreateApplicationRequirement :one
INSERT INTO application_requirements (
    application_id, document_type_id, document_name, description, status, display_order, is_required
) VALUES (
    $1, $2, $3, $4, 'PENDING', $5, $6
) RETURNING *;

-- name: GetApplicationRequirements :many
SELECT * FROM application_requirements
WHERE application_id = $1
ORDER BY display_order ASC;

-- name: GetApplicationRequirementByID :one
SELECT * FROM application_requirements
WHERE id = $1;

-- name: UpdateRequirementStatus :one
UPDATE application_requirements
SET status = $2, rejection_reason = $3, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: CreateApplicationDocumentVersion :one
INSERT INTO application_document_versions (
    requirement_id, version_number, uploaded_by_client
) VALUES (
    $1, $2, $3
) RETURNING *;

-- name: CreateRequirementVersionFile :one
INSERT INTO application_document_version_files (
    version_id, file_url, document_name, client_document_id
) VALUES (
    $1, $2, $3, $4
) RETURNING *;

-- name: GetDocumentVersionsByRequirement :many
SELECT * FROM application_document_versions
WHERE requirement_id = $1
ORDER BY created_at DESC;

-- name: UpdateVersionFileClientDocumentID :exec
UPDATE application_document_version_files
SET client_document_id = $2
WHERE id = $1;

-- name: GetDocumentVersionByID :one
SELECT * FROM application_document_versions
WHERE id = $1;

-- name: GetVersionFilesByVersionID :many
SELECT * FROM application_document_version_files
WHERE version_id = $1
ORDER BY created_at ASC;

-- name: GetVersionFileByID :one
SELECT * FROM application_document_version_files
WHERE id = $1;

-- name: GetLatestDocumentVersionNumber :one
SELECT COALESCE(MAX(version_number), 0)::int FROM application_document_versions
WHERE requirement_id = $1;

-- name: SetFinalDeliverable :one
UPDATE applications
SET final_deliverable_id = $2, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: ArchiveApplication :one
UPDATE applications
SET archived_at = now(), archived_by = $2, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: RestoreApplication :one
UPDATE applications
SET archived_at = NULL, archived_by = NULL, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: SoftDeleteApplication :one
UPDATE applications
SET deleted_at = now(), deleted_by = $2, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: AddApplicationDeliverable :one
INSERT INTO application_deliverables (
  application_id, client_document_id, uploaded_by_user_id
) VALUES (
  $1, $2, $3
) RETURNING *;

-- name: GetApplicationDeliverables :many
SELECT * FROM application_deliverables
WHERE application_id = $1
ORDER BY created_at DESC;

-- name: GetApplicationDeliverableByID :one
SELECT * FROM application_deliverables
WHERE id = $1;

-- name: UpdateApplicationTitle :one
UPDATE applications
SET title = $2, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: CreateTimelineEvent :one
INSERT INTO application_timeline (
    application_id, event_type, actor_type, actor_id, description, metadata
) VALUES (
    $1, $2, $3, $4, $5, $6
) RETURNING *;

-- name: ListTimelineEvents :many
SELECT * FROM application_timeline
WHERE application_id = $1
ORDER BY created_at DESC;

-- name: CreateRequirementReview :one
INSERT INTO application_requirement_reviews (
    requirement_id, reviewer_id, reviewer_type, reviewer_name, status, comment
) VALUES (
    $1, $2, $3, $4, $5, $6
) RETURNING *;

-- name: GetRequirementReviews :many
SELECT * FROM application_requirement_reviews
WHERE requirement_id = $1
ORDER BY created_at DESC;