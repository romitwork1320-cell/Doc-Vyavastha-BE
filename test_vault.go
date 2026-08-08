package main

import (
	"context"
	"fmt"
	"log"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/thinkparq/edconsultancy-be/internal/db/public"
)

func main() {
	ctx := context.Background()
	dsn := "postgres://postgres:root@localhost:5432/doc_vyavastha?sslmode=disable"
	
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		log.Fatalf("failed to connect database: %v", err)
	}
	defer pool.Close()

	q := public.New(pool)
	
	// Let's get all client profiles
	profiles, err := pool.Query(ctx, "SELECT id, user_id FROM client_profiles")
	if err != nil {
		log.Fatalf("failed to get client profiles: %v", err)
	}
	defer profiles.Close()
	
	for profiles.Next() {
		var clientID, userID int64
		profiles.Scan(&clientID, &userID)
		
		fmt.Printf("Testing for ClientID: %d\n", clientID)
		
		// Get folders
		folders, err := q.ListClientVaultFolders(ctx, clientID)
		if err != nil {
			log.Fatalf("ListClientVaultFolders error: %v", err)
		}
		
		if len(folders) > 0 {
			fmt.Printf("Folders found: %+v\n", folders)
			typeID := folders[0].DocumentTypeID
			fmt.Printf("Testing ListClientVaultFiles for TypeID: %d\n", typeID)
			files, err := q.ListClientVaultFiles(ctx, public.ListClientVaultFilesParams{
				ClientID: clientID,
				DocumentTypeID: typeID,
			})
			if err != nil {
				log.Fatalf("ListClientVaultFiles error: %v", err)
			}
			fmt.Printf("Files count: %d\n", len(files))
			for _, f := range files {
				fmt.Printf("File: %+v\n", f)
			}
		}
	}
}
