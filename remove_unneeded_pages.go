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

	_, err = pool.Exec(ctx, `DELETE FROM public.pages WHERE route_url IN ('/dashboard/client-organizations', '/dashboard/document-vault')`)
	if err != nil {
		fmt.Println("Error deleting pages:", err)
		os.Exit(1)
	}

	fmt.Println("Successfully removed Organizations and Document Vault pages.")
}
