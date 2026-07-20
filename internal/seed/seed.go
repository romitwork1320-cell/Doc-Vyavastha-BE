// Package seed inserts idempotent demo data: subscription plans, the page
// catalog, an Admin role, a demo tenant (with its schema + lookup data) and an
// admin login. Every step is individually guarded, so it is safe to re-run and
// a partially-completed prior run self-heals on the next boot (rather than
// crash-looping on a duplicate-key error).
package seed

import (
	"context"
	"errors"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"

	"github.com/thinkparq/edconsultancy-be/internal/db/public"
	"github.com/thinkparq/edconsultancy-be/internal/db/tenant"
	"github.com/thinkparq/edconsultancy-be/internal/tenancy"
)

const (
	demoEmail    = "admin@demo.local"
	demoPassword = "admin123"
	demoTenant   = "Demo Consultancy"
)

// Run seeds demo data idempotently.
func Run(ctx context.Context, pool *pgxpool.Pool, tm *tenancy.Manager, logger *slog.Logger) error {
	q := public.New(pool)

	// ── Admin user (get-or-create) ──
	u, err := q.GetUserByEmail(ctx, demoEmail)
	if errors.Is(err, pgx.ErrNoRows) {
		hash, herr := bcrypt.GenerateFromPassword([]byte(demoPassword), bcrypt.DefaultCost)
		if herr != nil {
			return herr
		}
		hs := string(hash)
		u, err = q.CreateUser(ctx, public.CreateUserParams{Email: demoEmail, PasswordHash: &hs, IsActive: true, IsVerified: true})
		if err == nil {
			logger.Info("seed: created demo admin", "email", demoEmail)
		}
	}
	if err != nil {
		return err
	}

	// ── Plans (only if none) ──
	if err := insertIfEmpty(ctx, pool, "subscription_plans", `
        INSERT INTO subscription_plans (plan_name, code, price, duration_months, duration_days, whatsapp_credits, is_trial, is_active)
        VALUES ('Free Trial','TRIAL',0,0,14,100,TRUE,TRUE),
               ('Standard','STD',4999,12,0,5000,FALSE,TRUE),
               ('Premium','PREM',9999,12,0,20000,FALSE,TRUE)`); err != nil {
		return err
	}

	// ⚡️ Page catalog (insert missing pages idempotently) ⚡️
	if _, err := pool.Exec(ctx, `
        INSERT INTO pages (page_name, route_url, display_order)
        SELECT v.page_name, v.route_url, CAST(v.display_order AS INT)
        FROM (VALUES
            ('Dashboard','/dashboard',1), ('Students','/students',2),
            ('Student Applications','/student-applications',3), ('Student Categories','/student-categories',4),
            ('Code Configurations','/student-code-configurations',5), ('Code Sequences','/student-code-sequences',6),
            ('Application Types','/application-types',7), ('Application Statuses','/application-statuses',8),
            ('Fee Types','/fee-types',9), ('Teams','/teams',10), ('Subscription','/subscription',11),
            ('Support','/support',12), ('My Profile','/my-profile',13), ('Settings','/settings',14),
            ('Fee Plans','/fee-plans',15), ('Payments','/payments',16)
        ) AS v(page_name, route_url, display_order)
        WHERE NOT EXISTS (
            SELECT 1 FROM pages p WHERE p.route_url = v.route_url
        )`); err != nil {
		return err
	}

	// ── Admin role + full permissions (idempotent) ──
	role, err := q.EnsureRole(ctx, "Admin")
	if err != nil {
		return err
	}
	if _, err := pool.Exec(ctx, `
        INSERT INTO role_page_permissions (role_id, page_id, can_view, can_add, can_edit, can_delete)
        SELECT $1, page_id, TRUE, TRUE, TRUE, TRUE FROM pages
        ON CONFLICT (role_id, page_id) DO NOTHING`, role.RoleID); err != nil {
		return err
	}

	roleStaff, err := q.EnsureRole(ctx, "Staff")
	if err != nil {
		return err
	}
	if _, err := pool.Exec(ctx, `
        INSERT INTO role_page_permissions (role_id, page_id, can_view, can_add, can_edit, can_delete)
        SELECT $1, page_id, TRUE, TRUE, TRUE, FALSE FROM pages
        ON CONFLICT (role_id, page_id) DO NOTHING`, roleStaff.RoleID); err != nil {
		return err
	}

	roleViewer, err := q.EnsureRole(ctx, "Viewer")
	if err != nil {
		return err
	}
	if _, err := pool.Exec(ctx, `
        INSERT INTO role_page_permissions (role_id, page_id, can_view, can_add, can_edit, can_delete)
        SELECT $1, page_id, TRUE, FALSE, FALSE, FALSE FROM pages
        ON CONFLICT (role_id, page_id) DO NOTHING`, roleViewer.RoleID); err != nil {
		return err
	}

	// ── Demo tenant (get-or-create) ──
	var tenantID int64
	var schemaName string
	err = pool.QueryRow(ctx,
		`SELECT tenant_id, schema_name FROM tenants WHERE tenant_name = $1 ORDER BY tenant_id LIMIT 1`,
		demoTenant).Scan(&tenantID, &schemaName)
	if errors.Is(err, pgx.ErrNoRows) {
		t, perr := tm.ProvisionNewTenant(ctx, tenancy.NewTenant{TenantName: demoTenant, CompanyName: demoTenant, ContactEmail: demoEmail})
		if perr != nil {
			return perr
		}
		tenantID, schemaName = t.TenantID, t.SchemaName
		logger.Info("seed: provisioned demo tenant", "tenant", tenantID, "schema", schemaName)
	} else if err != nil {
		return err
	}

	// ── Membership (upsert) ──
	if _, err := q.AddUserToTenant(ctx, public.AddUserToTenantParams{
		UserID: u.ID, TenantID: tenantID, RoleID: &role.RoleID, Status: "Active", IsOwner: true,
	}); err != nil {
		return err
	}

	// ── Tenant-schema lookup data (only if not already seeded) ──
	if err := tm.InTenantTx(ctx, schemaName, func(tq *tenant.Queries) error {
		n, err := tq.CountStudentCategories(ctx, "")
		if err != nil {
			return err
		}
		if n > 0 {
			return nil // already seeded
		}
		var first tenant.StudentCategory
		for i, name := range []string{"A Group", "Commerce", "Diploma", "Scholarship Only"} {
			c, err := tq.CreateStudentCategory(ctx, tenant.CreateStudentCategoryParams{Name: name, Status: "Active"})
			if err != nil {
				return err
			}
			if i == 0 {
				first = c
			}
		}
		if _, err := tq.CreateStudentCodeConfig(ctx, tenant.CreateStudentCodeConfigParams{
			CategoryID: first.ID, BusinessYear: 2026, Prefix: "A", Separator: "-", PaddingLength: 4, ResetSequence: true, IsActive: true,
		}); err != nil {
			return err
		}
		for _, name := range []string{"ACPC", "GUJCET", "JEE", "NEET", "Scholarship"} {
			if _, err := tq.CreateApplicationType(ctx, tenant.CreateApplicationTypeParams{Name: name, Status: "Active"}); err != nil {
				return err
			}
		}
		statuses := []struct {
			name  string
			order int32
			color string
		}{{"Pending", 1, "#f59e0b"}, {"In Progress", 2, "#3b82f6"}, {"Completed", 3, "#10b981"}, {"Rejected", 4, "#ef4444"}, {"Hold", 5, "#6b7280"}}
		for _, s := range statuses {
			color := s.color
			if _, err := tq.CreateApplicationStatus(ctx, tenant.CreateApplicationStatusParams{
				Name: s.name, ColorCode: &color, Status: "Active", DisplayOrder: s.order,
			}); err != nil {
				return err
			}
		}
		var zeroAmount pgtype.Numeric
		if err := zeroAmount.Scan("0"); err != nil {
			return err
		}
		for _, name := range []string{"Tuition Fee", "Exam Fee", "Hostel Fee"} {
			if _, err := tq.CreateFeeType(ctx, tenant.CreateFeeTypeParams{Name: name, Amount: zeroAmount, Status: "Active"}); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return err
	}

	logger.Info("seed: ready", "login", demoEmail, "tenant", tenantID)
	return nil
}

// insertIfEmpty runs insertSQL only when `table` currently has no rows. `table`
// is always a trusted constant from the callers above (never user input).
func insertIfEmpty(ctx context.Context, pool *pgxpool.Pool, table, insertSQL string) error {
	var n int64
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM "+table).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	_, err := pool.Exec(ctx, insertSQL)
	return err
}
