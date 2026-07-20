// Package e2e runs a full-stack smoke test against a real (embedded) Postgres:
// migrate -> seed -> boot router -> exercise auth + a tenant-schema round-trip
// including student-code generation. Run with: go test ./internal/e2e/...
package e2e

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	embeddedpostgres "github.com/fergusstrange/embedded-postgres"

	"github.com/thinkparq/edconsultancy-be/internal/config"
	"github.com/thinkparq/edconsultancy-be/internal/db"
	"github.com/thinkparq/edconsultancy-be/internal/db/public"
	"github.com/thinkparq/edconsultancy-be/internal/seed"
	"github.com/thinkparq/edconsultancy-be/internal/server"
	"github.com/thinkparq/edconsultancy-be/internal/tenancy"
)

const (
	pgPort = 54329
	dsn    = "postgres://edc:edc@localhost:54329/edconsultancy?sslmode=disable"
)

type envelope struct {
	Data       json.RawMessage `json:"data"`
	Success    bool            `json:"success"`
	Message    string          `json:"message"`
	StatusCode int             `json:"statusCode"`
	TotalCount *int64          `json:"totalCount"`
}

func TestEndToEnd(t *testing.T) {
	pg := embeddedpostgres.NewDatabase(embeddedpostgres.DefaultConfig().
		Username("edc").Password("edc").Database("edconsultancy").
		Port(pgPort).
		RuntimePath(t.TempDir()))
	if err := pg.Start(); err != nil {
		t.Skipf("could not start embedded postgres (likely no network to fetch binary): %v", err)
	}
	defer func() { _ = pg.Stop() }()

	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))

	if err := db.MigratePublic(dsn); err != nil {
		t.Fatalf("migrate public: %v", err)
	}
	pool, err := db.NewPool(ctx, dsn, 10, 2)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	defer pool.Close()

	tm := tenancy.NewManager(pool)
	if err := seed.Run(ctx, pool, tm, logger); err != nil {
		t.Fatalf("seed: %v", err)
	}

	cfg := &config.Config{
		AppEnv: "test", JWTIssuer: "edconsultancy", JWTAccessSecret: "test-secret-please-change",
		JWTAccessTTL: 15 * time.Minute, JWTRefreshTTL: 720 * time.Hour, JWTTempTTL: 10 * time.Minute,
		CookieName: "edc_refresh", CookieSameSite: "lax",
		CORSAllowedOrigins: []string{"http://localhost:4200"}, ClientURL: "http://localhost:4200",
		OTPLength: 6, OTPTTL: 5 * time.Minute, EmailSender: "log",
	}
	ts := httptest.NewServer(server.New(cfg, pool, logger).Router())
	defer ts.Close()

	// 1. Health.
	if env := do(t, ts, http.MethodGet, "/health", "", nil); !env.Success {
		t.Fatalf("health not successful: %+v", env)
	}

	// 2. Login via one-time code (email OTP -> final token for the single seeded
	//    tenant). Seed the OTP directly since the test uses the "log" email sender.
	const otpCode = "123456"
	sum := sha256.Sum256([]byte(otpCode))
	if _, err := public.New(pool).InsertOTP(ctx, public.InsertOTPParams{
		Email: "admin@demo.local", CodeHash: hex.EncodeToString(sum[:]),
		Purpose: "login", ExpiresAt: time.Now().Add(5 * time.Minute),
	}); err != nil {
		t.Fatalf("seed otp: %v", err)
	}
	loginEnv := do(t, ts, http.MethodPost, "/api/Auth/verify-otp", "",
		map[string]any{"email": "admin@demo.local", "otpCode": otpCode})
	if !loginEnv.Success {
		t.Fatalf("otp login failed: %s", loginEnv.Message)
	}
	var login struct {
		IsProfileComplete bool `json:"isProfileComplete"`
		TenantData        struct {
			RequiresSelection bool   `json:"requiresSelection"`
			Token             string `json:"token"`
		} `json:"tenantData"`
	}
	mustUnmarshal(t, loginEnv.Data, &login)
	if !login.IsProfileComplete || login.TenantData.RequiresSelection || login.TenantData.Token == "" {
		t.Fatalf("expected a final token, got %+v", login)
	}
	bearer := login.TenantData.Token

	// 3. List seeded categories (tenant-schema read via search_path).
	catEnv := do(t, ts, http.MethodGet, "/api/StudentCategories", bearer, nil)
	if !catEnv.Success || catEnv.TotalCount == nil || *catEnv.TotalCount < 1 {
		t.Fatalf("categories list bad: %+v", catEnv)
	}
	var cats []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	mustUnmarshal(t, catEnv.Data, &cats)
	var aGroupID string
	for _, c := range cats {
		if c.Name == "A Group" {
			aGroupID = c.ID
		}
	}
	if aGroupID == "" {
		t.Fatalf("seeded 'A Group' category not found in %+v", cats)
	}

	// 4. Create a student -> expect generated code "A-0001".
	stuEnv := do(t, ts, http.MethodPost, "/api/Students", bearer, map[string]any{
		"categoryId": aGroupID, "firstName": "Asha", "lastName": "Patel", "primaryMobile": "9999999999",
	})
	if !stuEnv.Success || stuEnv.StatusCode != http.StatusCreated {
		t.Fatalf("create student failed: %+v", stuEnv)
	}
	var stu struct {
		ID          string `json:"id"`
		StudentCode string `json:"studentCode"`
	}
	mustUnmarshal(t, stuEnv.Data, &stu)
	if stu.StudentCode != "A-0001" {
		t.Fatalf("expected generated code A-0001, got %q", stu.StudentCode)
	}

	// 5. Second student -> A-0002 (sequence advanced atomically).
	stuEnv2 := do(t, ts, http.MethodPost, "/api/Students", bearer, map[string]any{
		"categoryId": aGroupID, "firstName": "Ben", "lastName": "Shah", "primaryMobile": "8888888888",
	})
	var stu2 struct {
		StudentCode string `json:"studentCode"`
	}
	mustUnmarshal(t, stuEnv2.Data, &stu2)
	if stu2.StudentCode != "A-0002" {
		t.Fatalf("expected A-0002, got %q", stu2.StudentCode)
	}

	// 5b. Breadth smoke-test across the fanned-out modules (runtime correctness:
	// routes resolve, queries run, tenant scoping works).
	requireOK(t, do(t, ts, http.MethodGet, "/api/Subscription/plans", bearer, nil), "subscription plans")
	requireOK(t, do(t, ts, http.MethodGet, "/api/Tenants/stats", bearer, nil), "tenant stats")
	requireOK(t, do(t, ts, http.MethodGet, "/api/Tenants", bearer, nil), "tenants list")
	requireOK(t, do(t, ts, http.MethodGet, "/api/Users", bearer, nil), "users list")
	requireOK(t, do(t, ts, http.MethodGet, "/api/Users/pages", bearer, nil), "users pages")
	requireOK(t, do(t, ts, http.MethodGet, "/api/Permissions", bearer, nil), "permissions matrix")
	requireOK(t, do(t, ts, http.MethodGet, "/api/Profile", bearer, nil), "profile")
	requireOK(t, do(t, ts, http.MethodGet, "/api/UserActivity/recent", bearer, nil), "user activity")
	requireOK(t, do(t, ts, http.MethodGet, "/api/FeeTypes", bearer, nil), "fee types")

	atEnv := do(t, ts, http.MethodGet, "/api/ApplicationTypes", bearer, nil)
	requireOK(t, atEnv, "application types")
	asEnv := do(t, ts, http.MethodGet, "/api/ApplicationStatuses", bearer, nil)
	requireOK(t, asEnv, "application statuses")
	atID := firstID(t, atEnv.Data)
	asID := firstID(t, asEnv.Data)

	// Fee plan + payment round-trip (number generation).
	fpEnv := do(t, ts, http.MethodPost, "/api/StudentFeePlans", bearer, map[string]any{
		"studentId": stu.ID, "feeName": "Tuition Fee", "totalAmount": 50000, "remarks": "year 1",
	})
	requireOK(t, fpEnv, "create fee plan")
	var fp struct {
		ID string `json:"id"`
	}
	mustUnmarshal(t, fpEnv.Data, &fp)

	payEnv := do(t, ts, http.MethodPost, "/api/StudentPayments", bearer, map[string]any{
		"studentId": stu.ID, "studentFeePlanId": fp.ID, "amount": 10000,
		"paymentDate": "2026-01-15", "paymentMethod": "Cash", "referenceNumber": "RCP-1", "remarks": "installment 1",
	})
	requireOK(t, payEnv, "create payment")
	var pay struct {
		PaymentNumber string `json:"paymentNumber"`
	}
	mustUnmarshal(t, payEnv.Data, &pay)
	if pay.PaymentNumber != "PAY0001" {
		t.Fatalf("expected payment number PAY0001, got %q", pay.PaymentNumber)
	}

	// Student application round-trip + by-student lookup.
	appEnv := do(t, ts, http.MethodPost, "/api/StudentApplications", bearer, map[string]any{
		"studentId": stu.ID, "applicationTypeId": atID, "applicationStatusId": asID,
		"applicationName": "ACPC 2026", "lastDate": "2026-06-30",
	})
	requireOK(t, appEnv, "create application")
	byStu := do(t, ts, http.MethodGet, "/api/StudentApplications/student/"+stu.ID, bearer, nil)
	requireOK(t, byStu, "applications by student")
	var apps []map[string]any
	mustUnmarshal(t, byStu.Data, &apps)
	if len(apps) != 1 {
		t.Fatalf("expected 1 application for student, got %d", len(apps))
	}

	// 6. Unauthorized request must yield a real HTTP 401 (drives FE refresh).
	resp, err := http.Get(ts.URL + "/api/Students")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 without token, got %d", resp.StatusCode)
	}

	t.Logf("E2E OK: login + tenant search_path + code-gen (A-0001, A-0002) verified")
}

func do(t *testing.T, ts *httptest.Server, method, path, bearer string, body any) envelope {
	t.Helper()
	var rdr io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, ts.URL+path, rdr)
	if err != nil {
		t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var env envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("%s %s: non-JSON response (%d): %s", method, path, resp.StatusCode, string(raw))
	}
	return env
}

func requireOK(t *testing.T, env envelope, what string) {
	t.Helper()
	if !env.Success {
		t.Fatalf("%s: not successful (status %d): %s", what, env.StatusCode, env.Message)
	}
}

// firstID returns the "id" of the first element of a JSON array of objects.
func firstID(t *testing.T, raw json.RawMessage) string {
	t.Helper()
	var arr []map[string]any
	mustUnmarshal(t, raw, &arr)
	if len(arr) == 0 {
		t.Fatalf("expected non-empty list, got %s", string(raw))
	}
	id, _ := arr[0]["id"].(string)
	if id == "" {
		t.Fatalf("first element has no string id: %v", arr[0])
	}
	return id
}

func mustUnmarshal(t *testing.T, raw json.RawMessage, dst any) {
	t.Helper()
	if err := json.Unmarshal(raw, dst); err != nil {
		t.Fatalf("unmarshal %s: %v", string(raw), err)
	}
}
