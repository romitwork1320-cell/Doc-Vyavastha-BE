-- name: GetUserProfile :one
SELECT * FROM user_profiles WHERE user_id = $1;

-- name: UpsertUserProfile :one
INSERT INTO user_profiles (user_id, first_name, last_name, job_title, contact_number)
VALUES (sqlc.arg(user_id), sqlc.arg(first_name), sqlc.arg(last_name), sqlc.arg(job_title), sqlc.arg(contact_number))
ON CONFLICT (user_id) DO UPDATE SET
    first_name = EXCLUDED.first_name,
    last_name = EXCLUDED.last_name,
    job_title = EXCLUDED.job_title,
    contact_number = EXCLUDED.contact_number,
    updated_at = now()
RETURNING *;

-- name: GetCompanyProfile :one
SELECT * FROM company_profiles WHERE tenant_id = $1;

-- name: UpsertCompanyProfile :one
INSERT INTO company_profiles (
    tenant_id, company_name, company_logo_url, contact_email, contact_phone, support_phone,
    website, address_line1, address_line2, city, state, country, postal_code, gstin, pan, cin_no, msme_no
) VALUES (
    sqlc.arg(tenant_id), sqlc.arg(company_name), sqlc.arg(company_logo_url), sqlc.arg(contact_email), sqlc.arg(contact_phone), sqlc.arg(support_phone),
    sqlc.arg(website), sqlc.arg(address_line1), sqlc.arg(address_line2), sqlc.arg(city), sqlc.arg(state), sqlc.arg(country), sqlc.arg(postal_code), sqlc.arg(gstin), sqlc.arg(pan), sqlc.arg(cin_no), sqlc.arg(msme_no)
)
ON CONFLICT (tenant_id) DO UPDATE SET
    company_name = EXCLUDED.company_name,
    company_logo_url = EXCLUDED.company_logo_url,
    contact_email = EXCLUDED.contact_email,
    contact_phone = EXCLUDED.contact_phone,
    support_phone = EXCLUDED.support_phone,
    website = EXCLUDED.website,
    address_line1 = EXCLUDED.address_line1,
    address_line2 = EXCLUDED.address_line2,
    city = EXCLUDED.city,
    state = EXCLUDED.state,
    country = EXCLUDED.country,
    postal_code = EXCLUDED.postal_code,
    gstin = EXCLUDED.gstin,
    pan = EXCLUDED.pan,
    cin_no = EXCLUDED.cin_no,
    msme_no = EXCLUDED.msme_no,
    updated_at = now()
RETURNING *;

-- name: GetPublicTenantLogo :one
SELECT COALESCE(cp.company_logo_url, t.logo_url) AS logo_url, COALESCE(cp.company_name, t.company_name, t.tenant_name) AS company_name
FROM tenants t
LEFT JOIN company_profiles cp ON cp.tenant_id = t.tenant_id
WHERE t.tenant_id = $1;
