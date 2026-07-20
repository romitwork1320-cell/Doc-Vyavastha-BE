package main

import (
	"context"
	"fmt"
	"log"

	"github.com/jackc/pgx/v5"
)

func main() {
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, "postgres://postgres:root@localhost:5432/edconsultancy?sslmode=disable")
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close(ctx)

	rows, err := conn.Query(ctx, "SELECT id, changes::json->'new'->>'branch_id' FROM public.user_activities WHERE branch_id IS NULL AND changes IS NOT NULL LIMIT 5;")
	if err != nil {
		log.Fatal(err)
	}
	defer rows.Close()

	for rows.Next() {
		var id int64
		var branchID *string
		err := rows.Scan(&id, &branchID)
		if err != nil {
			log.Fatal(err)
		}
		
		fmt.Printf("ID: %d, extracted_branch_id: %v\n", id, strOrNil(branchID))
	}
}

func strOrNil(s *string) string {
	if s == nil {
		return "<nil>"
	}
	return *s
}
