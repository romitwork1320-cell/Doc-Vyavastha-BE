# EDConsultancy — Backend

Go backend for the **EDConsultancy** multi-tenant consultancy-management SaaS. It is the server the existing Angular frontend (`EDConsultancy-FE`) was built against: it reproduces that frontend's exact wire contract so the FE needs little-to-no rework.

- **Language / stack:** Go 1.26, [chi](https://github.com/go-chi/chi) router, [pgx](https://github.com/jackc/pgx) + [sqlc](https://sqlc.dev) (type-safe SQL, no ORM), [golang-migrate](https://github.com/golang-migrate/migrate), [golang-jwt](https://github.com/golang-jwt/jwt), bcrypt, [go-playground/validator](https://github.com/go-playground/validator), [go-mail](https://github.com/wneessen/go-mail).
- **Database:** PostgreSQL, **schema-per-tenant** isolation.

---

## Architecture

Two-tier schema design (this is what the FE's data actually implies):

| Tier | Schema | PK type | Holds |
|------|--------|---------|-------|
| **Platform** | `public` | **integer** (`bigint`) | users, tenants, user_tenants, roles/pages/permissions, subscription plans + tenant subscriptions, promo codes + payments, company/user profiles, system logs, user-activity audit, support tickets, notifications, refresh/OTP/verification tokens |
| **Business** | `tenant_0001`, `tenant_0002`, … (one per tenant) | **UUID** | student_categories, student_code_year_configs, student_code_sequences, students, application_types, application_statuses, student_applications, fee_types, student_fee_plans, student_payments |

Why the split? The FE decodes **integer** `UserId`/`TenantId` from the JWT and passes integer tenant ids in URLs, but uses **string/GUID** ids for the entire student domain. The backend honours both rather than forcing the doc's "UUID everywhere".

**Tenant isolation** is physical: every tenant gets its own PostgreSQL schema. On each authenticated request the JWT's `TenantId` is resolved to a schema name (`tenant_0001`) and business queries run inside a transaction with `SET LOCAL search_path TO tenant_0001, public`. The schema name is **never** exposed to the FE (it only ever sees the integer tenant id). See `internal/tenancy`.

```
request → auth middleware (JWT → identity{userID, tenantID, schema})
        → handler → tenancy.InTenantTx(schema, fn)  [SET LOCAL search_path]
                  → sqlc tenant.Queries on the tx
```

### Wire contract (must match the FE)

- **Base path** `/api`, **PascalCase** resource routes (`/api/Auth/login`, `/api/Students`, `/api/StudentCodeSequences/...`).
- **Response envelope** — every response is `ApiResponse<T>`:
  ```json
  { "data": <T|null>, "success": true, "message": "…", "statusCode": 200 }
  ```
  List endpoints additionally return `totalCount` **and** `totalRecords` (different FE services read different names).
- **Status codes:** handled outcomes (including logical 201/400/404) are returned as **HTTP 200** with the real code in `statusCode` — matching the original ASP.NET backend and the FE mocks. Only the **JWT auth boundary** returns a real **HTTP 401** (so the FE interceptor fires its single-flight refresh). Unexpected faults return **500**.
- **Auth:** `Authorization: Bearer <jwt>`; claims `UserId`/`TenantId` (PascalCase **strings**), `role`/`permissions` (lowercase). Refresh token is an **httpOnly cookie**; the FE posts an empty body to `/api/Auth/refresh-token` with `withCredentials:true`.
- **CORS:** explicit origin + `Access-Control-Allow-Credentials: true`.

---

## Run it

### Docker (everything, seeded)

```bash
docker compose up --build
```

Brings up Postgres + the API (migrated + seeded) on **http://localhost:8080**. Health: `GET /health`.

Seeded demo login (tenant "Demo Consultancy"): **admin@demo.local / admin123**.

The FE's `nginx.conf` proxies `/api` → `api:8080`, so this compose service name (`api`) matches a combined deployment.

### Local (Go + your own Postgres)

```bash
cp .env.example .env          # edit DATABASE_URL etc.
make tools                    # installs sqlc + migrate (once)
make migrate-up               # public schema migrations
AUTO_MIGRATE=true SEED_DEMO=true make run   # or set in .env
```

`make help` lists all targets.

---

## Project layout

```
cmd/api/                 entrypoint (config, pool, migrate/seed, server)
internal/
  config/                env config
  apiresp/               ApiResponse envelope + render helpers
  web/                   request binding, pagination, param parsing
  conv/                  DTO<->DB value converters
  reqctx/                authenticated-identity context
  token/                 JWT issue/verify (FE claim contract)
  middleware/            auth (Bearer) middleware
  email/                 transactional email (log | smtp)
  tenancy/               schema resolution, search_path tx, tenant provisioning
  db/                    pgx pool, migrate runner, sqlc-generated public/ + tenant/
  auth/                  /Auth flows (password, tenant-select, refresh, OTP, Google, signup, verify)
  platform/<x>/          public-schema modules (users, tenants, subscription, …)
  consultancy/<x>/       tenant-schema modules (students, categories, applications, fees, …)
  seed/                  idempotent demo data
migrations/public/       public schema (golang-migrate)
migrations/tenant/       tenant schema template (applied per tenant)
db/query/{public,tenant} sqlc query sources
sqlc.yaml                sqlc config (two packages)
```

Regenerate query code after editing `db/query/**` or migrations: `make sqlc`.

---

## Student-code generation

The doc's centrepiece. On `POST /api/Students` the code is generated **atomically** inside the tenant transaction (never from `COUNT()`):

1. resolve the active `student_code_year_configs` row for the category (and year),
2. `SELECT … FOR UPDATE` the matching `student_code_sequences` row,
3. `next = current_number + 1`, `code = prefix + separator + zeroPad(next, padding)`,
4. persist the new sequence value and insert the student with `student_code = code`.

See `internal/consultancy/students`.

---

## Frontend wiring guide

The student-domain Angular services are currently **client-side mocks** (they read `src/assets/mock-data/*.json`). To go live, swap each mock body for the real `HttpClient` call — the response shapes already match this backend's `ApiResponse<T>`.

Relationship endpoints chosen by this backend (wire the corresponding service methods to these):

| FE service method | Endpoint |
|---|---|
| `StudentCodeConfigurationService.getByCategory(categoryId)` | `GET /api/StudentCodeConfigurations/category/{categoryId}` |
| `StudentCodeSequenceService.getByYearConfig(yearConfigId)` | `GET /api/StudentCodeSequences/year-config/{yearConfigId}` |
| `StudentApplicationService.getByStudentId(studentId)` | `GET /api/StudentApplications/student/{studentId}` |
| `StudentFeePlanService.getAll(studentId)` | `GET /api/StudentFeePlans/student/{studentId}` |
| `StudentPaymentService.getAll(studentId, feePlanId)` | `GET /api/StudentPayments/student/{studentId}` · `…/fee-plan/{feePlanId}` |

`student_applications.portal_username` / `portal_password` are exposed to the FE as JSON `userId` / `password` (the application-portal credentials), per the FE model.

---

## Not yet implemented / deferred (v1 scope = consultancy core)

- **`/api/supportHub` SignalR WebSocket** (real-time support) — the FE `@microsoft/signalr` channel. App functions without it; add a Go SignalR hub later.
- **`/swagger/`** OpenAPI UI (the FE nginx proxies it) — annotations + swaggo generation is a follow-up.
- **`/api/Settings`** — the FE settings service contract was not pinned down; implement once confirmed.
- Legacy CRM modules (leads, deals, contacts, activities, campaigns) were intentionally **out of scope** — they are leftover template code.
- List endpoints paginate + filter; server-side dynamic **sort column** is not wired (the FE doesn't rely on it). Easy to add per entity.
