-- name: CreateBranch :one
INSERT INTO branches (
    name, code, address, contact, status
) VALUES (
    $1, $2, $3, $4, $5
) RETURNING *;

-- name: UpdateBranch :one
UPDATE branches
SET
    name = $2,
    code = $3,
    address = $4,
    contact = $5,
    status = $6,
    updated_at = now()
WHERE id = $1 AND deleted_at IS NULL
RETURNING *;

-- name: GetBranch :one
SELECT * FROM branches WHERE id = $1 AND deleted_at IS NULL;

-- name: ListBranches :many
SELECT * FROM branches WHERE deleted_at IS NULL ORDER BY name;

-- name: DeleteBranch :exec
UPDATE branches
SET deleted_at = now(), updated_at = now()
WHERE id = $1 AND deleted_at IS NULL;

-- name: AssignUserToBranch :exec
INSERT INTO user_branches (user_id, branch_id)
VALUES ($1, $2)
ON CONFLICT (user_id, branch_id) DO NOTHING;

-- name: RemoveUserFromBranch :exec
DELETE FROM user_branches WHERE user_id = $1 AND branch_id = $2;

-- name: GetUserBranches :many
SELECT b.* FROM branches b
JOIN user_branches ub ON b.id = ub.branch_id
WHERE ub.user_id = $1 AND b.deleted_at IS NULL
ORDER BY b.name;
