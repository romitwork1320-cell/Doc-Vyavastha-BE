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

	rows, err := conn.Query(context.Background(), "SELECT tenant_id, company_name, contact_email, contact_phone, address_line1, city FROM company_profiles")
	if err != nil {
		log.Fatal(err)
	}
	defer rows.Close()

	for rows.Next() {
		var tenant_id int64
		var company_name, contact_email, contact_phone, address_line1, city *string
		if err := rows.Scan(&tenant_id, &company_name, &contact_email, &contact_phone, &address_line1, &city); err != nil {
			log.Fatal(err)
		}
		fmt.Printf("Tenant: %d, Email: %v, Phone: %v, Addr: %v, City: %v\n", tenant_id, strPtr(contact_email), strPtr(contact_phone), strPtr(address_line1), strPtr(city))
	}
}

func strPtr(s *string) string {
	if s == nil {
		return "<nil>"
	}
	return *s
}
