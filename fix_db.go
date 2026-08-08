package main

import (
	"context"
	"fmt"
	"log"
	"github.com/jackc/pgx/v5"
)

func main() {
	conn, err := pgx.Connect(context.Background(), "postgres://postgres:postgres@localhost:5432/docvyavastha?sslmode=disable")
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close(context.Background())

	tenants := []string{"tenant_1", "tenant_2"}
	
	for _, t := range tenants {
		fmt.Println("Fixing", t)
		
		_, err = conn.Exec(context.Background(), "SET search_path TO " + t)
		if err != nil {
			log.Println("skip", t, err)
			continue
		}

		_, err = conn.Exec(context.Background(), "DROP TABLE IF EXISTS application_requirements CASCADE")
		if err != nil {
			log.Println(err)
		}
		_, err = conn.Exec(context.Background(), "DROP TABLE IF EXISTS application_document_versions CASCADE")
		if err != nil {
			log.Println(err)
		}
		
		_, err = conn.Exec(context.Background(), "DELETE FROM schema_migrations WHERE version = '000010_application_requirements'")
		if err != nil {
			log.Println(err)
		}
		
		// Remove columns from applications so 000010 can re-add them
		conn.Exec(context.Background(), "ALTER TABLE applications DROP COLUMN IF EXISTS magic_link_token CASCADE")
		conn.Exec(context.Background(), "ALTER TABLE applications DROP COLUMN IF EXISTS magic_link_expires_at CASCADE")
		conn.Exec(context.Background(), "ALTER TABLE applications DROP COLUMN IF EXISTS final_deliverable_id CASCADE")
		
		fmt.Println(t, "fixed")
	}
}
