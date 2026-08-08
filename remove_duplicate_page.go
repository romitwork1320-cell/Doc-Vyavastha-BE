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

	// 1. Delete duplicate KYC Status (keep MIN id)
	_, err = pool.Exec(ctx, `
		DELETE FROM public.role_page_permissions WHERE page_id IN (
			SELECT page_id FROM public.pages WHERE route_url = '/dashboard/kyc-status' AND page_id > (
				SELECT MIN(page_id) FROM public.pages WHERE route_url = '/dashboard/kyc-status'
			)
		);
		DELETE FROM public.pages WHERE route_url = '/dashboard/kyc-status' AND page_id > (
			SELECT MIN(page_id) FROM public.pages WHERE route_url = '/dashboard/kyc-status'
		);
	`)
	if err != nil {
		fmt.Println("Error deleting duplicate pages:", err)
		os.Exit(1)
	}
	fmt.Println("Duplicate KYC Status deleted.")
}
