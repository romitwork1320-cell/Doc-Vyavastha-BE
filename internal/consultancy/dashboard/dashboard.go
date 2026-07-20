package dashboard

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/go-chi/chi/v5"

	"github.com/thinkparq/edconsultancy-be/internal/apiresp"
	"github.com/thinkparq/edconsultancy-be/internal/db/tenant"
	"github.com/thinkparq/edconsultancy-be/internal/reqctx"
	"github.com/thinkparq/edconsultancy-be/internal/tenancy"
	"github.com/thinkparq/edconsultancy-be/internal/middleware"
)

type Handler struct {
	tm     *tenancy.Manager
	logger *slog.Logger
	mw     *middleware.Auth
}

func New(tm *tenancy.Manager, logger *slog.Logger, mw *middleware.Auth) *Handler {
	return &Handler{tm: tm, logger: logger, mw: mw}
}

func (h *Handler) Mount(r chi.Router) {
	r.Route("/Dashboard", func(r chi.Router) {
		r.Get("/kpis", h.getKpis)
		r.Get("/student-analytics", h.getStudentAnalytics)
		r.Get("/application-analytics", h.getApplicationAnalytics)
		r.Get("/revenue-analytics", h.getRevenueAnalytics)
		r.Get("/upcoming-deadlines", h.getUpcomingDeadlines)
		r.Get("/action-center", h.getActionCenter)
		r.Get("/recent-activity", h.getRecentActivity)
	})
}

// ── Helpers ─────────────────────────────────────────────────────────────────

func parseDates(r *http.Request) (time.Time, time.Time) {
	fromStr := r.URL.Query().Get("fromDate")
	toStr := r.URL.Query().Get("toDate")

	var from, to time.Time
	if fromStr != "" {
		if parsed, err := time.Parse(time.RFC3339, fromStr); err == nil {
			from = parsed
		}
	}
	if toStr != "" {
		if parsed, err := time.Parse(time.RFC3339, toStr); err == nil {
			to = parsed
		}
	}

	if from.IsZero() {
		from = time.Now().AddDate(-10, 0, 0) // Default 10 years ago if empty
	}
	if to.IsZero() {
		to = time.Now().AddDate(0, 0, 1) // Default tomorrow if empty
	}

	return from, to
}

// ── Handlers ────────────────────────────────────────────────────────────────

func (h *Handler) getKpis(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	branchIDStr := reqctx.BranchID(ctx)
	branchUUID, _ := uuid.Parse(branchIDStr)

	startDate, endDate := parseDates(r)

	var res struct {
		TotalStudents        int64   `json:"totalStudents"`
		StudentGrowthPct     float64 `json:"studentGrowthPct"`
		NewStudentsThisMonth int64   `json:"newStudentsThisMonth"`
		TotalApplications    int64   `json:"totalApplications"`
		PendingApplications  int64   `json:"pendingApplications"`
		TotalFeePlanned      float64 `json:"totalFeePlanned"`
		TotalFeeCollected    float64 `json:"totalFeeCollected"`
		PendingFeeCollection float64 `json:"pendingFeeCollection"`
		UpcomingDeadlines    int64   `json:"upcomingDeadlines"`
	}

	err := h.tm.InTenantTx(ctx, reqctx.Schema(ctx), func(q *tenant.Queries) error {
		totalSt, _ := q.GetTotalStudents(ctx, tenant.GetTotalStudentsParams{BranchID: branchUUID, StartDate: startDate, EndDate: endDate})
		res.TotalStudents = totalSt

		totalApp, _ := q.GetTotalApplications(ctx, tenant.GetTotalApplicationsParams{BranchID: branchUUID, StartDate: startDate, EndDate: endDate})
		res.TotalApplications = totalApp

		pendingApp, _ := q.GetPendingApplications(ctx, tenant.GetPendingApplicationsParams{BranchID: branchUUID, StartDate: startDate, EndDate: endDate})
		res.PendingApplications = pendingApp

		feePlanned, _ := q.GetTotalFeePlanned(ctx, tenant.GetTotalFeePlannedParams{BranchID: branchUUID, StartDate: startDate, EndDate: endDate})
		if feePlanned.Valid {
			if f, err := feePlanned.Float64Value(); err == nil {
				res.TotalFeePlanned = f.Float64
			}
		}

		feeCollected, _ := q.GetTotalFeeCollected(ctx, tenant.GetTotalFeeCollectedParams{BranchID: branchUUID, StartDate: startDate, EndDate: endDate})
		if feeCollected.Valid {
			if f, err := feeCollected.Float64Value(); err == nil {
				res.TotalFeeCollected = f.Float64
			}
		}

		feeDiscount, _ := q.GetTotalFeeDiscount(ctx, tenant.GetTotalFeeDiscountParams{BranchID: branchUUID, StartDate: startDate, EndDate: endDate})
		var discount float64
		if feeDiscount.Valid {
			if f, err := feeDiscount.Float64Value(); err == nil {
				discount = f.Float64
			}
		}
		
		res.PendingFeeCollection = res.TotalFeePlanned - res.TotalFeeCollected - discount
		if res.PendingFeeCollection < 0 {
			res.PendingFeeCollection = 0
		}
		
		// Optional: calculate new students this month exactly
		now := time.Now()
		startOfMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
		newSt, _ := q.GetTotalStudents(ctx, tenant.GetTotalStudentsParams{
			BranchID:  branchUUID,
			StartDate: startOfMonth,
			EndDate:   now,
		})
		res.NewStudentsThisMonth = newSt
		
		deadlines, _ := q.GetUpcomingDeadlines(ctx, branchUUID)
		res.UpcomingDeadlines = int64(len(deadlines))

		return nil
	})

	if err != nil {
		h.logger.Error("failed to get KPIs", "err", err)
		apiresp.ServerError(w, "Failed to load KPIs")
		return
	}

	apiresp.OK(w, res, "KPIs loaded")
}

func (h *Handler) getStudentAnalytics(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	branchIDStr := reqctx.BranchID(ctx)
	branchUUID, _ := uuid.Parse(branchIDStr)

	startDate, endDate := parseDates(r)

	type distItem struct {
		Category string `json:"category"`
		Count    int64  `json:"count"`
	}
	type trendItem struct {
		Month string `json:"month"`
		Count int64  `json:"count"`
	}
	var res struct {
		DistributionByCategory []distItem  `json:"distributionByCategory"`
		AdmissionTrend         []trendItem `json:"admissionTrend"`
	}
	res.DistributionByCategory = []distItem{}
	res.AdmissionTrend = []trendItem{}

	err := h.tm.InTenantTx(ctx, reqctx.Schema(ctx), func(q *tenant.Queries) error {
		distRows, err := q.GetStudentDistributionByCategory(ctx, tenant.GetStudentDistributionByCategoryParams{BranchID: branchUUID, StartDate: startDate, EndDate: endDate})
		if err == nil {
			for _, r := range distRows {
				res.DistributionByCategory = append(res.DistributionByCategory, distItem{
					Category: r.CategoryName,
					Count:    r.Count,
				})
			}
		}
		trendRows, err := q.GetAdmissionTrend(ctx, tenant.GetAdmissionTrendParams{BranchID: branchUUID, StartDate: startDate, EndDate: endDate})
		if err == nil {
			for _, r := range trendRows {
				res.AdmissionTrend = append(res.AdmissionTrend, trendItem{
					Month: r.Month,
					Count: r.Count,
				})
			}
		}
		return nil
	})

	if err != nil {
		h.logger.Error("failed to get student analytics", "err", err)
		apiresp.ServerError(w, "Failed to load student analytics")
		return
	}

	apiresp.OK(w, res, "Student analytics loaded")
}

func (h *Handler) getApplicationAnalytics(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	branchIDStr := reqctx.BranchID(ctx)
	branchUUID, _ := uuid.Parse(branchIDStr)

	startDate, endDate := parseDates(r)

	type statItem struct {
		Status string `json:"status"`
		Count  int64  `json:"count"`
	}
	type typeItem struct {
		Type  string `json:"type"`
		Count int64  `json:"count"`
	}
	var res struct {
		StatusDistribution []statItem `json:"statusDistribution"`
		TypesDistribution  []typeItem `json:"typesDistribution"`
	}
	res.StatusDistribution = []statItem{}
	res.TypesDistribution = []typeItem{}

	err := h.tm.InTenantTx(ctx, reqctx.Schema(ctx), func(q *tenant.Queries) error {

		statRows, err := q.GetApplicationStatusDistribution(ctx, tenant.GetApplicationStatusDistributionParams{BranchID: branchUUID, StartDate: startDate, EndDate: endDate})
		if err == nil {
			for _, r := range statRows {
				res.StatusDistribution = append(res.StatusDistribution, statItem{
					Status: r.StatusName,
					Count:  r.Count,
				})
			}
		}
		typeRows, err := q.GetApplicationTypesDistribution(ctx, tenant.GetApplicationTypesDistributionParams{BranchID: branchUUID, StartDate: startDate, EndDate: endDate})
		if err == nil {
			for _, r := range typeRows {
				res.TypesDistribution = append(res.TypesDistribution, typeItem{
					Type:  r.TypeName,
					Count: r.Count,
				})
			}
		}
		return nil
	})

	if err != nil {
		h.logger.Error("failed to get application analytics", "err", err)
		apiresp.ServerError(w, "Failed to load application analytics")
		return
	}

	apiresp.OK(w, res, "Application analytics loaded")
}

func (h *Handler) getRevenueAnalytics(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	branchIDStr := reqctx.BranchID(ctx)
	branchUUID, _ := uuid.Parse(branchIDStr)

	startDate, endDate := parseDates(r)

	type trendItem struct {
		Month  string  `json:"month"`
		Amount float64 `json:"amount"`
	}
	type methodItem struct {
		Method string  `json:"method"`
		Amount float64 `json:"amount"`
	}
	var res struct {
		TotalPlanned              float64      `json:"totalPlanned"`
		TotalDiscounts            float64      `json:"totalDiscounts"`
		NetRevenue                float64      `json:"netRevenue"`
		TotalCollected            float64      `json:"totalCollected"`
		TotalPending              float64      `json:"totalPending"`
		CollectionTrend           []trendItem  `json:"collectionTrend"`
		PaymentMethodDistribution []methodItem `json:"paymentMethodDistribution"`
	}
	res.CollectionTrend = []trendItem{}
	res.PaymentMethodDistribution = []methodItem{}

	err := h.tm.InTenantTx(ctx, reqctx.Schema(ctx), func(q *tenant.Queries) error {
		feePlanned, _ := q.GetTotalFeePlanned(ctx, tenant.GetTotalFeePlannedParams{BranchID: branchUUID, StartDate: startDate, EndDate: endDate})
		if feePlanned.Valid {
			if f, err := feePlanned.Float64Value(); err == nil {
				res.TotalPlanned = f.Float64
			}
		}

		feeDiscount, _ := q.GetTotalFeeDiscount(ctx, tenant.GetTotalFeeDiscountParams{BranchID: branchUUID, StartDate: startDate, EndDate: endDate})
		if feeDiscount.Valid {
			if f, err := feeDiscount.Float64Value(); err == nil {
				res.TotalDiscounts = f.Float64
			}
		}
		
		feeCollected, _ := q.GetTotalFeeCollected(ctx, tenant.GetTotalFeeCollectedParams{BranchID: branchUUID, StartDate: startDate, EndDate: endDate})
		if feeCollected.Valid {
			if f, err := feeCollected.Float64Value(); err == nil {
				res.TotalCollected = f.Float64
			}
		}
		
		res.NetRevenue = res.TotalPlanned - res.TotalDiscounts
		res.TotalPending = res.NetRevenue - res.TotalCollected
		if res.TotalPending < 0 {
			res.TotalPending = 0
		}

		trendRows, err := q.GetRevenueCollectionTrend(ctx, tenant.GetRevenueCollectionTrendParams{BranchID: branchUUID, StartDate: startDate, EndDate: endDate})
		if err == nil {
			for _, r := range trendRows {
				if amt, err := r.Amount.Float64Value(); err == nil {
					res.CollectionTrend = append(res.CollectionTrend, trendItem{
						Month:  r.Month,
						Amount: amt.Float64,
					})
				}
			}
		}
		
		methodRows, err := q.GetPaymentMethodDistribution(ctx, tenant.GetPaymentMethodDistributionParams{BranchID: branchUUID, StartDate: startDate, EndDate: endDate})
		if err == nil {
			for _, r := range methodRows {
				if amt, err := r.Amount.Float64Value(); err == nil {
					method := ""
					if r.Method != nil {
						method = *r.Method
					}
					res.PaymentMethodDistribution = append(res.PaymentMethodDistribution, methodItem{
						Method: method,
						Amount: amt.Float64,
					})
				}
			}
		}
		return nil
	})

	if err != nil {
		h.logger.Error("failed to get revenue analytics", "err", err)
		apiresp.ServerError(w, "Failed to load revenue analytics")
		return
	}

	apiresp.OK(w, res, "Revenue analytics loaded")
}

func (h *Handler) getUpcomingDeadlines(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	branchIDStr := reqctx.BranchID(ctx)
	branchUUID, _ := uuid.Parse(branchIDStr)

	
	type dlItem struct {
		ID            string `json:"id"`
		StudentCode   string `json:"studentCode"`
		StudentName   string `json:"studentName"`
		Application   string `json:"application"`
		DeadlineDate  string `json:"deadlineDate"`
		RemainingDays int32  `json:"remainingDays"`
	}
	var res []dlItem

	err := h.tm.InTenantTx(ctx, reqctx.Schema(ctx), func(q *tenant.Queries) error {
		rows, err := q.GetUpcomingDeadlines(ctx, branchUUID)
		if err == nil {
			for _, r := range rows {
				appName := ""
				if r.ApplicationName != nil {
					appName = *r.ApplicationName
				}
				lastDate := ""
				if r.LastDate != nil {
					lastDate = r.LastDate.Format(time.RFC3339)
				}
				res = append(res, dlItem{
					ID:            r.ID.String(),
					StudentCode:   r.StudentCode,
					StudentName:   r.StudentName,
					Application:   appName,
					DeadlineDate:  lastDate,
					RemainingDays: r.RemainingDays,
				})
			}
		}
		return nil
	})

	if err != nil {
		h.logger.Error("failed to get upcoming deadlines", "err", err)
		apiresp.ServerError(w, "Failed to load upcoming deadlines")
		return
	}

	if res == nil {
		res = []dlItem{}
	}
	apiresp.OK(w, res, "Upcoming deadlines loaded")
}

func (h *Handler) getActionCenter(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	branchIDStr := reqctx.BranchID(ctx)
	branchUUID, _ := uuid.Parse(branchIDStr)

	startDate, endDate := parseDates(r)

	type acItem struct {
		ID          string `json:"id"`
		StudentCode string `json:"studentCode"`
		StudentName string `json:"studentName"`
		Issue       string `json:"issue"`
		Priority    string `json:"priority"`
	}
	var res []acItem

	err := h.tm.InTenantTx(ctx, reqctx.Schema(ctx), func(q *tenant.Queries) error {
		rows, err := q.GetActionCenterItems(ctx, tenant.GetActionCenterItemsParams{BranchID: branchUUID, StartDate: startDate, EndDate: endDate})
		if err == nil {
			for _, r := range rows {
				res = append(res, acItem{
					ID:          r.ID.String(),
					StudentCode: r.StudentCode,
					StudentName: r.StudentName,
					Issue:       r.Issue,
					Priority:    r.Priority,
				})
			}
		}
		return nil
	})

	if err != nil {
		h.logger.Error("failed to get action center", "err", err)
		apiresp.ServerError(w, "Failed to load action center")
		return
	}

	if res == nil {
		res = []acItem{}
	}
	apiresp.OK(w, res, "Action center loaded")
}

func (h *Handler) getRecentActivity(w http.ResponseWriter, r *http.Request) {
	// Not implemented via SQL yet, return empty for now
	var res []interface{}
	if res == nil {
		res = []interface{}{}
	}
	apiresp.OK(w, res, "Recent activity loaded")
}
