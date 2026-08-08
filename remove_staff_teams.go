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

	_, err = pool.Exec(ctx, `UPDATE public.role_page_permissions SET can_view = FALSE, can_add = FALSE, can_edit = FALSE, can_delete = FALSE WHERE role_id = (SELECT role_id FROM public.roles WHERE role_name = 'Staff') AND page_id = (SELECT page_id FROM public.pages WHERE route_url = '/teams')`)
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}
	fmt.Println("Done")
}
