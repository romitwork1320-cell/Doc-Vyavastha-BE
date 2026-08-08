package main
import (
	"context"
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"
)
func main() {
	dbURL := "postgres://postgres:password@localhost:5432/docvyavastha?sslmode=disable"
	ctx := context.Background()
	pool, _ := pgxpool.New(ctx, dbURL)
	defer pool.Close()
	rows, _ := pool.Query(ctx, "SELECT id, client_id, status FROM public.client_connections")
	for rows.Next() {
		var id, client_id int
		var status string
		rows.Scan(&id, &client_id, &status)
		fmt.Printf("id: %d, client_id: %d, status: %s\n", id, client_id, status)
	}
}
