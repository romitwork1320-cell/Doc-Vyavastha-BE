package main

import (
	"context"
	"fmt"
	"github.com/thinkparq/edconsultancy-be/internal/config"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	cfg, _ := config.Load()
	pool, _ := pgxpool.New(context.Background(), cfg.DatabaseURL)
	defer pool.Close()
	rows, _ := pool.Query(context.Background(), "SELECT role_id, role_name FROM roles")
	for rows.Next() {
		var id int
		var name string
		rows.Scan(&id, &name)
		fmt.Printf("Role %d: %s\n", id, name)
	}
}
