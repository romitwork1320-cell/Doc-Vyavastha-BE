package main

import (
	"context"
	"fmt"
	"os"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/thinkparq/edconsultancy-be/internal/config"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Println("Error loading config:", err)
		os.Exit(1)
	}

	pool, err := pgxpool.New(context.Background(), cfg.DatabaseURL)
	if err != nil {
		fmt.Println("Error connecting to DB:", err)
		os.Exit(1)
	}
	defer pool.Close()

	ctx := context.Background()

	// 1. Delete old pages
	_, err = pool.Exec(ctx, `DELETE FROM public.pages WHERE route_url IN ('/fee-plans', '/payments', '/daily-collections')`)
	if err != nil {
		fmt.Println("Error deleting old pages:", err)
		os.Exit(1)
	}

	// 2. Insert new pages
	_, err = pool.Exec(ctx, `
		INSERT INTO public.pages (page_name, route_url, display_order)
		SELECT v.page_name, v.route_url, CAST(v.display_order AS INT)
		FROM (VALUES
			('Clients', '/dashboard/clients', 2),
			('Organizations', '/dashboard/client-organizations', 3),
			('Applications', '/dashboard/applications', 4),
			('Document Vault', '/dashboard/document-vault', 5)
		) AS v(page_name, route_url, display_order)
		WHERE NOT EXISTS (
			SELECT 1 FROM public.pages p WHERE p.route_url = v.route_url
		)
	`)
	if err != nil {
		fmt.Println("Error inserting new pages:", err)
		os.Exit(1)
	}

	// 3. Grant Admin access
	_, err = pool.Exec(ctx, `
		INSERT INTO public.role_page_permissions (role_id, page_id, can_view, can_add, can_edit, can_delete)
		SELECT r.role_id, p.page_id, true, true, true, true
		FROM public.roles r
		CROSS JOIN public.pages p
		WHERE r.role_name = 'Admin' AND p.route_url IN (
			'/dashboard/clients', 
			'/dashboard/client-organizations',
			'/dashboard/applications',
			'/dashboard/document-vault'
		)
		ON CONFLICT (role_id, page_id) DO NOTHING
	`)
	if err != nil {
		fmt.Println("Error granting Admin access:", err)
		os.Exit(1)
	}

	// 4. Grant Client access
	_, err = pool.Exec(ctx, `
		INSERT INTO public.role_page_permissions (role_id, page_id, can_view, can_add, can_edit, can_delete)
		SELECT r.role_id, p.page_id, true, true, true, false
		FROM public.roles r
		CROSS JOIN public.pages p
		WHERE r.role_name = 'Client' AND p.route_url IN (
			'/dashboard/client-organizations',
			'/dashboard/document-vault'
		)
		ON CONFLICT (role_id, page_id) DO NOTHING
	`)
	if err != nil {
		fmt.Println("Error granting Client access:", err)
		os.Exit(1)
	}

	// 5. Fix existing client_connections status
	res, err := pool.Exec(ctx, "UPDATE public.client_connections SET status = 'ACTIVE' WHERE status = 'ACCEPTED'")
	if err == nil {
		fmt.Printf("Updated %d rows from ACCEPTED to ACTIVE\n", res.RowsAffected())
	}
	res2, err := pool.Exec(ctx, "UPDATE public.client_connections SET status = 'IN_ACTIVE' WHERE status IN ('REJECTED', 'DELETED')")
	if err == nil {
		fmt.Printf("Updated %d rows to IN_ACTIVE\n", res2.RowsAffected())
	}

	fmt.Println("Successfully ran fix_pages.go!")
}
