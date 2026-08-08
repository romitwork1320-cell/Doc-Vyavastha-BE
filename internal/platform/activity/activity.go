package activity

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// LogActivity inserts a record into public.user_activities
func LogActivity(pool *pgxpool.Pool, userID int64, tenantID int64, description string) {
	q := `INSERT INTO public.user_activities (user_id, user_name, tenant_id, url, method, ip_address, description, created_on, branch_id)
		  VALUES ($1, $2, $3, $4, $5, $6, $7, NOW(), $8)`
	
	// Fire and forget since it's just logging
	go func() {
		// Use a background context so it doesn't get cancelled if the request finishes
		bgCtx := context.Background()
		_, _ = pool.Exec(bgCtx, q, fmt.Sprintf("%d", userID), "System", tenantID, "", "", "", description, nil)
	}()
}
