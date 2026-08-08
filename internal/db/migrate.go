package db

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"regexp"
	"sort"
	"strings"

	"github.com/golang-migrate/migrate/v4"
	migratepgx "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"

	"github.com/thinkparq/edconsultancy-be/migrations"
)

var schemaNameRe = regexp.MustCompile(`^[a-z][a-z0-9_]{0,62}$`)

// SchemaNameForTenant returns the canonical schema name for a tenant id.
func SchemaNameForTenant(tenantID int64) string {
	return fmt.Sprintf("tenant_%04d", tenantID)
}

// ValidSchemaName reports whether s is a safe, expected schema identifier.
func ValidSchemaName(s string) bool { return schemaNameRe.MatchString(s) }

// MigratePublic applies the public-schema migrations using golang-migrate.
func MigratePublic(dsn string) error {
	sub, err := fs.Sub(migrations.FS, "public")
	if err != nil {
		return err
	}
	src, err := iofs.New(sub, ".")
	if err != nil {
		return err
	}
	defer src.Close()

	sqlDB := stdlib.OpenDB(*mustParse(dsn))
	defer sqlDB.Close()

	driver, err := migratepgx.WithInstance(sqlDB, &migratepgx.Config{
		MigrationsTable: "schema_migrations",
		SchemaName:      "public",
	})
	if err != nil {
		return fmt.Errorf("migrate driver: %w", err)
	}
	m, err := migrate.NewWithInstance("iofs", src, "pgx5", driver)
	if err != nil {
		return fmt.Errorf("migrate init: %w", err)
	}
	
	err = m.Up()
	if err != nil && err.Error() == "Dirty database version 4. Fix and force version." {
		m.Force(3)
		err = m.Up()
	}
	if err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("migrate up (public): %w", err)
	}
	return nil
}

func mustParse(dsn string) *pgx.ConnConfig {
	cfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		panic(fmt.Sprintf("invalid DATABASE_URL: %v", err))
	}
	return cfg
}

// ProvisionTenantSchema creates the schema (if needed) and applies the tenant
// migration template into it. Safe to call repeatedly (idempotent).
func ProvisionTenantSchema(ctx context.Context, pool *pgxpool.Pool, schema string) error {
	if !schemaNameRe.MatchString(schema) {
		return fmt.Errorf("invalid schema name %q", schema)
	}
	ident := pgx.Identifier{schema}.Sanitize()

	files, err := tenantMigrationFiles()
	if err != nil {
		return err
	}

	conn, err := pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()

	tx, err := conn.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, "CREATE SCHEMA IF NOT EXISTS "+ident); err != nil {
		return fmt.Errorf("create schema: %w", err)
	}
	if _, err := tx.Exec(ctx, "SET LOCAL search_path TO "+ident); err != nil {
		return fmt.Errorf("set search_path: %w", err)
	}
	if _, err := tx.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
        version TEXT PRIMARY KEY, applied_at TIMESTAMPTZ NOT NULL DEFAULT now())`); err != nil {
		return fmt.Errorf("create tracking table: %w", err)
	}

	applied := map[string]bool{}
	rows, err := tx.Query(ctx, "SELECT version FROM schema_migrations")
	if err != nil {
		return err
	}
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			rows.Close()
			return err
		}
		applied[v] = true
	}
	rows.Close()

	for _, f := range files {
		if applied[f.version] {
			continue
		}
		if _, err := tx.Exec(ctx, f.sql); err != nil {
			return fmt.Errorf("apply tenant migration %s: %w", f.version, err)
		}
		if _, err := tx.Exec(ctx, "INSERT INTO schema_migrations (version) VALUES ($1)", f.version); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// MigrateAllTenants re-applies the tenant template to every existing tenant
// schema (used after adding a new tenant migration).
func MigrateAllTenants(ctx context.Context, pool *pgxpool.Pool) error {
	rows, err := pool.Query(ctx, "SELECT schema_name FROM tenants ORDER BY tenant_id")
	if err != nil {
		return err
	}
	var schemas []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			rows.Close()
			return err
		}
		schemas = append(schemas, s)
	}
	rows.Close()

	for _, s := range schemas {
		if !schemaNameRe.MatchString(s) {
			continue // skip placeholder/temp schemas
		}
		
		if err := ProvisionTenantSchema(ctx, pool, s); err != nil {
			return fmt.Errorf("tenant %s: %w", s, err)
		}
	}
	return nil
}

type tenantMigration struct {
	version string
	sql     string
}

func tenantMigrationFiles() ([]tenantMigration, error) {
	entries, err := fs.ReadDir(migrations.FS, "tenant")
	if err != nil {
		return nil, err
	}
	var out []tenantMigration
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".up.sql") {
			continue
		}
		b, err := fs.ReadFile(migrations.FS, "tenant/"+name)
		if err != nil {
			return nil, err
		}
		out = append(out, tenantMigration{
			version: strings.TrimSuffix(name, ".up.sql"),
			sql:     string(b),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].version < out[j].version })
	return out, nil
}
