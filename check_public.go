package main

import (
	"context"
	"fmt"
	"log"
	"github.com/jackc/pgx/v5"
)

func main() {
	conn, err := pgx.Connect(context.Background(), "postgres://postgres:root@localhost:5432/doc_vyavastha?sslmode=disable")
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close(context.Background())

	rows, err := conn.Query(context.Background(), "SELECT table_name FROM information_schema.tables WHERE table_schema = 'public'")
	if err != nil {
		log.Fatal(err)
	}
	defer rows.Close()

	fmt.Println("Tables in public schema:")
	for rows.Next() {
		var tbl string
		if err := rows.Scan(&tbl); err != nil {
			log.Fatal(err)
		}
		fmt.Println("-", tbl)
	}
}
