package main

import (
	"context"
	"fmt"
	"log"

	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	pool, err := pgxpool.New(context.Background(), "postgresql://postgres:postgres@localhost:5432/docvyavastha?sslmode=disable")
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()

	userID := int64(1)
	tenantID := int64(1)
	description := "Test Activity from Backend"

	q := `INSERT INTO public.user_activities (user_id, user_name, tenant_id, url, method, ip_address, description, created_on)
		  VALUES ($1, $2, $3, $4, $5, $6, $7, NOW())`
	
	_, err = pool.Exec(context.Background(), q, fmt.Sprintf("%d", userID), "System", tenantID, "", "", "", description)
	if err != nil {
		fmt.Println("Error:", err)
	} else {
		fmt.Println("Success!")
	}
}
