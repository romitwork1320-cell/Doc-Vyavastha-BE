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

	rows, err := pool.Query(ctx, `SELECT p.page_name, p.route_url, r.role_name, rpp.can_view 
	FROM public.role_page_permissions rpp 
	JOIN public.pages p ON rpp.page_id = p.page_id 
	JOIN public.roles r ON r.role_id = rpp.role_id
	WHERE r.role_name = 'Staff'`)
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}
	for rows.Next() {
		var name, route, role string
		var view bool
		rows.Scan(&name, &route, &role, &view)
		fmt.Printf("%s (%s): %v\n", name, route, view)
	}
}
