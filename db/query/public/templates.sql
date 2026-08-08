-- name: CreateDocumentType :one
INSERT INTO document_types (
    name, description
) VALUES (
    $1, $2
) RETURNING *;

-- name: GetDocumentType :one
SELECT * FROM document_types
WHERE id = $1 LIMIT 1;

-- name: ListDocumentTypes :many
SELECT * FROM document_types
WHERE is_active = TRUE
ORDER BY name ASC;

-- name: GetDocumentTypeByName :one
SELECT * FROM document_types
WHERE name = $1;

-- name: UpdateDocumentType :one
UPDATE document_types
SET name = $2,
    description = $3,
    updated_at = now()
WHERE id = $1
RETURNING *;

-- name: DeleteDocumentType :exec
UPDATE document_types
SET is_active = FALSE
WHERE id = $1;

-- name: CreateApplicationType :one
INSERT INTO application_types (
    name, description
) VALUES (
    $1, $2
) RETURNING *;

-- name: GetApplicationType :one
SELECT * FROM application_types
WHERE id = $1 LIMIT 1;

-- name: ListApplicationTypes :many
SELECT * FROM application_types
ORDER BY name ASC;

-- name: UpdateApplicationType :one
UPDATE application_types
SET name = $2,
    description = $3,
    updated_at = now()
WHERE id = $1
RETURNING *;

-- name: DeleteApplicationType :exec
DELETE FROM application_types
WHERE id = $1;

-- name: AddDocumentToApplicationType :exec
INSERT INTO application_type_documents (
    application_type_id, document_type_id, display_order, is_required
) VALUES (
    $1, $2, $3, $4
);

-- name: RemoveDocumentsFromApplicationType :exec
DELETE FROM application_type_documents
WHERE application_type_id = $1;

-- name: GetDocumentsForApplicationType :many
SELECT d.*, ad.display_order, ad.is_required
FROM document_types d
JOIN application_type_documents ad ON d.id = ad.document_type_id
WHERE ad.application_type_id = $1
ORDER BY ad.display_order ASC;
