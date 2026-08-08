package main

import (
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
	issuer := token.NewIssuer(cfg.JWTAccessSecret, "", 0, 0)
	
	tokenStr, _ := issuer.IssueAccess(user.ID, 1, "Admin", nil)
	
	req, _ := http.NewRequest("GET", "http://localhost:8081/api/Permissions", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)
	
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
