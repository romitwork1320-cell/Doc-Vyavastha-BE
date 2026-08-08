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

	// 1. Give Staff permission to Clients and Applications
	_, err = pool.Exec(ctx, `
		INSERT INTO public.role_page_permissions (role_id, page_id, can_view, can_add, can_edit, can_delete)
		SELECT r.role_id, p.page_id, true, true, true, false
		FROM public.roles r
		CROSS JOIN public.pages p
		WHERE r.role_name = 'Staff' AND p.route_url IN ('/dashboard/clients', '/dashboard/applications')
		ON CONFLICT (role_id, page_id) DO UPDATE SET
			can_view = true,
			can_add = true,
			can_edit = true;
	`)
	if err != nil {
		fmt.Println("Error granting Staff permissions:", err)
		os.Exit(1)
	}
	fmt.Println("Staff permissions granted.")

	// 2. Add KYC Status to pages
	_, err = pool.Exec(ctx, `
		INSERT INTO public.pages (page_name, route_url, display_order)
		VALUES ('KYC Status', '/dashboard/kyc-status', 10)
		ON CONFLICT DO NOTHING;
	`)
	if err != nil {
		fmt.Println("Error inserting KYC Status page:", err)
		os.Exit(1)
	}
	fmt.Println("KYC Status page inserted.")

	// 3. Grant Admin permission to KYC Status
	_, err = pool.Exec(ctx, `
		INSERT INTO public.role_page_permissions (role_id, page_id, can_view, can_add, can_edit, can_delete)
		SELECT r.role_id, p.page_id, true, true, true, true
		FROM public.roles r
		CROSS JOIN public.pages p
		WHERE r.role_name = 'Admin' AND p.route_url = '/dashboard/kyc-status'
		ON CONFLICT (role_id, page_id) DO UPDATE SET
			can_view = true,
			can_add = true,
			can_edit = true,
			can_delete = true;
	`)
	if err != nil {
		fmt.Println("Error granting Admin permission for KYC:", err)
		os.Exit(1)
	}
	fmt.Println("Admin permission granted for KYC Status.")
}
