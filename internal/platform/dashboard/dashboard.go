package dashboard

import (
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/thinkparq/edconsultancy-be/internal/apiresp"
	"github.com/thinkparq/edconsultancy-be/internal/db/public"
	"github.com/thinkparq/edconsultancy-be/internal/reqctx"
)

type Handler struct {
	q      *public.Queries
	logger *slog.Logger
	pool   *pgxpool.Pool
}

func New(pool *pgxpool.Pool, logger *slog.Logger) *Handler {
	return &Handler{
		q:      public.New(pool),
		logger: logger,
		pool:   pool,
	}
}

func (h *Handler) Mount(r chi.Router) {
	r.Route("/dashboard", func(r chi.Router) {
		r.Get("/", h.getDashboard)
	})
}

type activityRecord struct {
	Description string `json:"description"`
	CreatedOn   string `json:"createdOn"`
}

type statusCount struct {
	Status string `json:"status"`
	Count  int    `json:"count"`
}

type dashboardResponse struct {
	WorkspaceName string `json:"workspaceName"`
	
	// Organization KPIs
	TotalClients           int `json:"totalClients,omitempty"`
	OpenApplications       int `json:"openApplications,omitempty"`
	InProgressApplications int `json:"inProgressApplications,omitempty"`
	CompletedApplications  int `json:"completedApplications,omitempty"`
	
	// Client KPIs
	PendingDocumentRequests int `json:"pendingDocumentRequests,omitempty"`
	ConnectedOrganizations  int `json:"connectedOrganizations,omitempty"`
	PendingApps             int `json:"pendingApps,omitempty"`
	CompletedApps           int `json:"completedApps,omitempty"`
	
	// Shared
	RecentActivity []activityRecord `json:"recentActivity"`
	
	// Chart Data (Org)
	ApplicationsByStatus []statusCount `json:"applicationsByStatus,omitempty"`
}

func (h *Handler) getDashboard(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tenantID := reqctx.TenantID(ctx)
	userID := reqctx.MustUserID(ctx)

	var res dashboardResponse
	res.RecentActivity = []activityRecord{} // initialize to empty slice
	res.ApplicationsByStatus = []statusCount{}

	if tenantID > 0 {
		// Organization Dashboard
		// 1. Get Workspace Name
		var workspaceName string
		err := h.pool.QueryRow(ctx, "SELECT company_name FROM public.tenants WHERE tenant_id = $1", tenantID).Scan(&workspaceName)
		if err == nil {
			res.WorkspaceName = workspaceName
		}

		// 2. Client Metrics
		h.pool.QueryRow(ctx, "SELECT COUNT(*) FROM public.client_connections WHERE tenant_id = $1 AND status = 'ACTIVE'", tenantID).Scan(&res.TotalClients)

		// 3. Application Metrics (from tenant schema)
		schema := reqctx.Schema(ctx)
		if schema != "" {
			// Open Applications
			h.pool.QueryRow(ctx, "SELECT COUNT(*) FROM "+schema+".applications WHERE status = 'OPEN' AND deleted_at IS NULL AND archived_at IS NULL").Scan(&res.OpenApplications)
			
			// In Progress Applications
			h.pool.QueryRow(ctx, "SELECT COUNT(*) FROM "+schema+".applications WHERE status = 'IN_PROGRESS' AND deleted_at IS NULL AND archived_at IS NULL").Scan(&res.InProgressApplications)
			
			// Completed Applications (All Time)
			h.pool.QueryRow(ctx, "SELECT COUNT(*) FROM "+schema+".applications WHERE status = 'COMPLETED' AND deleted_at IS NULL AND archived_at IS NULL").Scan(&res.CompletedApplications)
			
			// Chart Data: Applications By Status
			rows, _ := h.pool.Query(ctx, "SELECT status, COUNT(*) FROM "+schema+".applications WHERE deleted_at IS NULL AND archived_at IS NULL GROUP BY status")
			for rows.Next() {
				var sc statusCount
				if err := rows.Scan(&sc.Status, &sc.Count); err == nil {
					res.ApplicationsByStatus = append(res.ApplicationsByStatus, sc)
				}
			}
			rows.Close()
		}
		
		// 4. Recent Activity
		rows, _ := h.pool.Query(ctx, "SELECT description, created_on::text FROM public.user_activities WHERE tenant_id = $1 ORDER BY created_on DESC LIMIT 10", tenantID)
		for rows.Next() {
			var a activityRecord
			if err := rows.Scan(&a.Description, &a.CreatedOn); err == nil {
				res.RecentActivity = append(res.RecentActivity, a)
			}
		}
		rows.Close()

	} else {
		// Personal Dashboard (Client)
		res.WorkspaceName = "Personal Workspace"
		
		clientProfile, err := h.q.GetClientProfileByUserId(ctx, userID)
		if err == nil {
			// 1. Connected Organizations
			h.pool.QueryRow(ctx, "SELECT COUNT(*) FROM public.client_connections WHERE client_id = $1 AND status = 'ACTIVE'", clientProfile.ID).Scan(&res.ConnectedOrganizations)
			
			// In Doc Vyavastha V1, applications live in the tenant schemas, so it's hard to get a global count
			// across all connected organizations without querying each tenant schema.
			// For now, we will leave pendingApps and completedApps as 0 or query across known schemas if possible.
			// To keep it simple and performant, we'll just populate the orgs count and activity.
			
			// 2. Recent Activity (for this user)
			userIDStr := strconv.FormatInt(userID, 10)
			rows, err := h.pool.Query(ctx, "SELECT description, created_on::text FROM public.user_activities WHERE user_id = $1 ORDER BY created_on DESC LIMIT 10", userIDStr)
			if err == nil {
				for rows.Next() {
					var a activityRecord
					if err := rows.Scan(&a.Description, &a.CreatedOn); err == nil {
						res.RecentActivity = append(res.RecentActivity, a)
					}
				}
				rows.Close()
			}
		}
	}

	apiresp.OK(w, res, "")
}
