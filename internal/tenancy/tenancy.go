// Package tenancy resolves tenants to their PostgreSQL schema and runs tenant
// business queries with the correct search_path. This is the heart of the
// schema-per-tenant isolation model: the FE only ever knows an integer
// tenantId; the schema name (tenant_0001, ...) is resolved internally here and
// never leaves the backend.
package tenancy

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/thinkparq/edconsultancy-be/internal/db"
	"github.com/thinkparq/edconsultancy-be/internal/db/public"
	"github.com/thinkparq/edconsultancy-be/internal/db/tenant"
	"github.com/thinkparq/edconsultancy-be/internal/reqctx"
)

// Manager resolves tenant schemas and executes tenant-scoped transactions.
type Manager struct {
	pool *pgxpool.Pool
	pub  *public.Queries

	mu    sync.RWMutex
	cache map[int64]string // tenantID -> schema_name
}

// NewManager builds a tenancy Manager over the shared pool.
func NewManager(pool *pgxpool.Pool) *Manager {
	return &Manager{pool: pool, pub: public.New(pool), cache: make(map[int64]string)}
}

// Schema resolves a tenant id to its (validated) schema name, caching results.
func (m *Manager) Schema(ctx context.Context, tenantID int64) (string, error) {
	m.mu.RLock()
	s, ok := m.cache[tenantID]
	m.mu.RUnlock()
	if ok {
		return s, nil
	}

	t, err := m.pub.GetTenant(ctx, tenantID)
	if err != nil {
		return "", fmt.Errorf("resolve tenant %d: %w", tenantID, err)
	}
	if !db.ValidSchemaName(t.SchemaName) {
		return "", fmt.Errorf("tenant %d has invalid schema name %q", tenantID, t.SchemaName)
	}

	m.mu.Lock()
	m.cache[tenantID] = t.SchemaName
	m.mu.Unlock()
	return t.SchemaName, nil
}

// InTenantTx runs fn inside a transaction whose search_path is scoped to the
// tenant schema (plus public for shared types). SET LOCAL resets automatically
// when the tx ends, so pooled connections never leak a search_path.
func (m *Manager) InTenantTx(ctx context.Context, schema string, fn func(q *tenant.Queries) error) error {
	if !db.ValidSchemaName(schema) {
		return fmt.Errorf("invalid schema name %q", schema)
	}
	conn, err := m.pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()

	tx, err := conn.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op after a successful Commit

	ident := pgx.Identifier{schema}.Sanitize()
	if _, err := tx.Exec(ctx, "SET LOCAL search_path TO "+ident+", public"); err != nil {
		return fmt.Errorf("set search_path: %w", err)
	}

	if id, ok := reqctx.Get(ctx); ok && id.UserID != 0 {
		// Ignore errors for SET LOCAL; if they fail, audit logging simply falls back to defaults.
		_, _ = tx.Exec(ctx, fmt.Sprintf("SET LOCAL app.user_id = '%d'", id.UserID))
		_, _ = tx.Exec(ctx, fmt.Sprintf("SET LOCAL app.tenant_id = '%d'", id.TenantID))
		
		if id.BranchID != "" {
			_, _ = tx.Exec(ctx, fmt.Sprintf("SET LOCAL app.current_branch_id = '%s'", id.BranchID))
		}
		if id.RequestURL != "" {
			_, _ = tx.Exec(ctx, fmt.Sprintf("SET LOCAL app.request_url = '%s'", id.RequestURL))
		}
		if id.RequestMethod != "" {
			_, _ = tx.Exec(ctx, fmt.Sprintf("SET LOCAL app.request_method = '%s'", id.RequestMethod))
		}
		if id.IPAddress != "" {
			_, _ = tx.Exec(ctx, fmt.Sprintf("SET LOCAL app.ip_address = '%s'", id.IPAddress))
		}
	}

	if err := fn(tenant.New(tx)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// InTenantTxByID resolves the tenant's schema and runs fn within it.
func (m *Manager) InTenantTxByID(ctx context.Context, tenantID int64, fn func(q *tenant.Queries) error) error {
	schema, err := m.Schema(ctx, tenantID)
	if err != nil {
		return err
	}
	return m.InTenantTx(ctx, schema, fn)
}

// Pub returns the public-schema query set bound to the pool.
func (m *Manager) Pub() *public.Queries { return m.pub }

// NewTenant describes a tenant to provision.
type NewTenant struct {
	TenantName   string
	CompanyName  string
	ContactPhone string
	ContactEmail string
	CreatedBy    *int64
}

// ProvisionNewTenant inserts the tenant row (with a canonical tenant_NNNN
// schema name derived from its id) and materializes its dedicated schema with
// the tenant table template. Membership/roles are assigned by the caller.
func (m *Manager) ProvisionNewTenant(ctx context.Context, in NewTenant) (public.Tenant, error) {
	tx, err := m.pool.Begin(ctx)
	if err != nil {
		return public.Tenant{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	q := public.New(tx)
	tmpSchema := "tmp_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	t, err := q.CreateTenant(ctx, public.CreateTenantParams{
		TenantName:   in.TenantName,
		CompanyName:  strPtr(in.CompanyName),
		SchemaName:   tmpSchema,
		TenantCode:   nil,
		ContactPhone: strPtr(in.ContactPhone),
		ContactEmail: strPtr(in.ContactEmail),
		CreatedBy:    in.CreatedBy,
	})
	if err != nil {
		return public.Tenant{}, fmt.Errorf("create tenant: %w", err)
	}

	schema := db.SchemaNameForTenant(t.TenantID)
	if err := q.SetTenantSchemaName(ctx, public.SetTenantSchemaNameParams{SchemaName: schema, TenantID: t.TenantID}); err != nil {
		return public.Tenant{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return public.Tenant{}, err
	}

	if err := db.ProvisionTenantSchema(ctx, m.pool, schema); err != nil {
		return public.Tenant{}, fmt.Errorf("provision schema %s: %w", schema, err)
	}

	t.SchemaName = schema
	m.mu.Lock()
	m.cache[t.TenantID] = schema
	m.mu.Unlock()
	return t, nil
}

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
