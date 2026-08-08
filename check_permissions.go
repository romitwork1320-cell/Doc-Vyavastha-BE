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

	rows, err := pool.Query(ctx, `
		SELECT r.role_name, p.page_name, rpp.can_view, rpp.can_add, rpp.can_edit, rpp.can_delete
		FROM public.role_page_permissions rpp
		JOIN public.roles r ON r.role_id = rpp.role_id
		JOIN public.pages p ON p.page_id = rpp.page_id
		ORDER BY r.role_name, p.page_name
	`)
	if err != nil {
		fmt.Println("Error querying:", err)
		os.Exit(1)
	}
	defer rows.Close()

	for rows.Next() {
		var roleName, pageName string
		var canView, canAdd, canEdit, canDelete bool
		if err := rows.Scan(&roleName, &pageName, &canView, &canAdd, &canEdit, &canDelete); err != nil {
			fmt.Println("Error scanning:", err)
			continue
		}
		fmt.Printf("%s - %s: View=%t Add=%t Edit=%t Del=%t\n", roleName, pageName, canView, canAdd, canEdit, canDelete)
	}
}
