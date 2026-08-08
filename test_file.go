package main

import (
	"context"
	"fmt"
	"os"
	"github.com/thinkparq/edconsultancy-be/internal/config"
	"github.com/thinkparq/edconsultancy-be/internal/db"
	"github.com/thinkparq/edconsultancy-be/internal/platform/applications"
	"github.com/thinkparq/edconsultancy-be/internal/platform/tenancy"
)

func main() {
	os.Setenv("DATABASE_URL", "postgresql://postgres:postgres@localhost:5432/docvyavastha?sslmode=disable")
	cfg, err := config.Load()
	if err != nil {
		fmt.Println("Config error:", err)
		return
	}
	dbPool, err := db.NewPool(context.Background(), cfg.DatabaseURL, cfg.DBMaxConns, cfg.DBMinConns)
	if err != nil {
		fmt.Println("DB error:", err)
		return
	}
	defer dbPool.Close()
	
	svc := applications.NewService(tenancy.NewManager(dbPool))
	ctx := context.Background() // note: reqctx.TenantID(ctx) will be 0 here!
	
	// Simulate tenantID=2, fileID=36
	tenantID := int64(2)
	fileID := int64(36)
	
	filePath, filename, err := svc.GetDocumentVersionFile(ctx, tenantID, fileID)
	fmt.Printf("Result:\nFilePath: %s\nFilename: %s\nError: %v\n", filePath, filename, err)
}
