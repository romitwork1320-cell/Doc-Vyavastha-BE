package main

import (
	"context"
	"fmt"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	dbURL := "postgres://postgres:password@localhost:5432/docvyavastha?sslmode=disable"
	ctx := context.Background()
	
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		fmt.Println("Unable to connect to database:", err)
		os.Exit(1)
	}
	defer pool.Close()

	// Update ACCEPTED to ACTIVE
	res1, err := pool.Exec(ctx, "UPDATE public.client_connections SET status = 'ACTIVE' WHERE status = 'ACCEPTED'")
	if err != nil {
		fmt.Println("Error updating ACCEPTED to ACTIVE:", err)
	} else {
		fmt.Printf("Updated %d rows from ACCEPTED to ACTIVE\n", res1.RowsAffected())
	}
	
	// Update others to IN_ACTIVE if there are any that were deleted/rejected
	res2, err := pool.Exec(ctx, "UPDATE public.client_connections SET status = 'IN_ACTIVE' WHERE status IN ('REJECTED', 'DELETED')")
	if err != nil {
		fmt.Println("Error updating others to IN_ACTIVE:", err)
	} else {
		fmt.Printf("Updated %d rows to IN_ACTIVE\n", res2.RowsAffected())
	}
}
