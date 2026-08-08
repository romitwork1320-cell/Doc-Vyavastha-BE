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

	rows, err := pool.Query(ctx, `SELECT page_id, page_name, route_url FROM public.pages`)
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}
	for rows.Next() {
		var id int64
		var name, route string
		rows.Scan(&id, &name, &route)
		fmt.Printf("%d: %s (%s)\n", id, name, route)
	}
}
