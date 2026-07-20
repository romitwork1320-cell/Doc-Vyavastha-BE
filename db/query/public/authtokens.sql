-- ── Refresh tokens ──────────────────────────────────────────────────────
-- name: InsertRefreshToken :one
INSERT INTO refresh_tokens (user_id, tenant_id, token_hash, user_agent, ip_address, expires_at)
VALUES (sqlc.arg(user_id), sqlc.arg(tenant_id), sqlc.arg(token_hash), sqlc.arg(user_agent), sqlc.arg(ip_address), sqlc.arg(expires_at))
RETURNING *;

-- name: GetRefreshTokenByHash :one
SELECT * FROM refresh_tokens
WHERE token_hash = $1 AND revoked_at IS NULL AND expires_at > now();

-- name: RevokeRefreshToken :exec
UPDATE refresh_tokens SET revoked_at = now() WHERE token_hash = $1;

-- name: RevokeAllUserRefreshTokens :exec
UPDATE refresh_tokens SET revoked_at = now() WHERE user_id = $1 AND revoked_at IS NULL;

-- name: DeleteExpiredRefreshTokens :execrows
DELETE FROM refresh_tokens WHERE expires_at < now();

-- ── OTP codes ───────────────────────────────────────────────────────────
-- name: InsertOTP :one
INSERT INTO otp_codes (email, code_hash, purpose, expires_at)
VALUES (lower(sqlc.arg(email)), sqlc.arg(code_hash), sqlc.arg(purpose), sqlc.arg(expires_at))
RETURNING *;

-- name: GetLatestOTP :one
SELECT * FROM otp_codes
WHERE lower(email) = lower(sqlc.arg(email)) AND purpose = sqlc.arg(purpose) AND consumed_at IS NULL
ORDER BY created_at DESC
LIMIT 1;

-- name: ConsumeOTP :exec
UPDATE otp_codes SET consumed_at = now() WHERE id = $1;

-- name: IncrementOTPAttempts :exec
UPDATE otp_codes SET attempts = attempts + 1 WHERE id = $1;

-- ── Email-verification / set-password tokens ────────────────────────────
-- name: InsertAuthToken :one
INSERT INTO auth_tokens (user_id, token_hash, purpose, expires_at)
VALUES (sqlc.arg(user_id), sqlc.arg(token_hash), sqlc.arg(purpose), sqlc.arg(expires_at))
RETURNING *;

-- name: GetAuthTokenByHash :one
SELECT * FROM auth_tokens
WHERE token_hash = sqlc.arg(token_hash) AND purpose = sqlc.arg(purpose)
  AND consumed_at IS NULL AND expires_at > now();

-- name: ConsumeAuthToken :exec
UPDATE auth_tokens SET consumed_at = now() WHERE id = $1;
