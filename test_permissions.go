package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"

	"github.com/thinkparq/edconsultancy-be/internal/config"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/thinkparq/edconsultancy-be/internal/db/public"
	"github.com/thinkparq/edconsultancy-be/internal/token"
)

func main() {
	cfg, _ := config.Load()
	pool, _ := pgxpool.New(context.Background(), cfg.DatabaseURL)
	defer pool.Close()
	q := public.New(pool)
	user, _ := q.GetUserByEmail(context.Background(), "admin@demo.local")
	issuer := token.NewIssuer(cfg.JWT.AccessSecret)
	
	// Issue a valid access token for the admin user on tenant 1
	tokenStr, _ := issuer.IssueAccess(user.ID, 1, "Admin", nil)
	
	// Create payload
	payload := `[{"roleId":3,"pageId":11,"canView":true,"canAdd":false,"canEdit":false,"canDelete":false}]`
	req, _ := http.NewRequest("PUT", "http://localhost:8081/api/Permissions", bytes.NewBuffer([]byte(payload)))
	req.Header.Set("Authorization", "Bearer "+tokenStr)
	req.Header.Set("Content-Type", "application/json")
	
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Println("Error:", err)
		return
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	fmt.Printf("Status: %d\nBody: %s\n", resp.StatusCode, string(body))
}
