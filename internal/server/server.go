// Package server wires the HTTP stack: router, global middleware, CORS, and
// the route groups for every module.
package server

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/thinkparq/edconsultancy-be/internal/apiresp"
	"github.com/thinkparq/edconsultancy-be/internal/auth"
	"github.com/thinkparq/edconsultancy-be/internal/config"
	"github.com/thinkparq/edconsultancy-be/internal/email"
	mw "github.com/thinkparq/edconsultancy-be/internal/middleware"
	"github.com/thinkparq/edconsultancy-be/internal/tenancy"
	"github.com/thinkparq/edconsultancy-be/internal/token"

	"github.com/thinkparq/edconsultancy-be/internal/consultancy/applications"
	"github.com/thinkparq/edconsultancy-be/internal/consultancy/appstatuses"
	"github.com/thinkparq/edconsultancy-be/internal/consultancy/apptypes"
	"github.com/thinkparq/edconsultancy-be/internal/consultancy/branches"
	"github.com/thinkparq/edconsultancy-be/internal/consultancy/castes"
	"github.com/thinkparq/edconsultancy-be/internal/consultancy/categories"
	"github.com/thinkparq/edconsultancy-be/internal/consultancy/codeconfig"
	"github.com/thinkparq/edconsultancy-be/internal/consultancy/codesequence"
	"github.com/thinkparq/edconsultancy-be/internal/consultancy/colleges"
	"github.com/thinkparq/edconsultancy-be/internal/consultancy/dashboard"
	"github.com/thinkparq/edconsultancy-be/internal/consultancy/feecollections"
	"github.com/thinkparq/edconsultancy-be/internal/consultancy/feeplans"
	"github.com/thinkparq/edconsultancy-be/internal/consultancy/feetypes"
	"github.com/thinkparq/edconsultancy-be/internal/consultancy/formtypes"
	"github.com/thinkparq/edconsultancy-be/internal/consultancy/payments"
	"github.com/thinkparq/edconsultancy-be/internal/consultancy/students"

	"github.com/thinkparq/edconsultancy-be/internal/platform/notifications"
	"github.com/thinkparq/edconsultancy-be/internal/platform/permissions"
	"github.com/thinkparq/edconsultancy-be/internal/platform/profile"
	"github.com/thinkparq/edconsultancy-be/internal/platform/subscription"
	"github.com/thinkparq/edconsultancy-be/internal/platform/support"
	"github.com/thinkparq/edconsultancy-be/internal/platform/systemlog"
	"github.com/thinkparq/edconsultancy-be/internal/platform/tenants"
	"github.com/thinkparq/edconsultancy-be/internal/platform/useractivity"
	"github.com/thinkparq/edconsultancy-be/internal/platform/users"
)

// Module is anything that can register its routes on a chi.Router.
type Module interface{ Mount(chi.Router) }

// Server holds shared dependencies for the HTTP layer.
type Server struct {
	cfg    *config.Config
	pool   *pgxpool.Pool
	logger *slog.Logger

	authMW *mw.Auth

	authHandler    *auth.Handler
	profileHandler *profile.Handler

	protected       []Module // mounted behind RequireTenant
	branchProtected []Module // mounted behind RequireBranch
}

// New constructs a Server and all feature modules.
func New(cfg *config.Config, pool *pgxpool.Pool, logger *slog.Logger) *Server {
	tm := tenancy.NewManager(pool)
	issuer := token.NewIssuer(cfg.JWTAccessSecret, cfg.JWTIssuer, cfg.JWTAccessTTL, cfg.JWTTempTTL)
	mailer := email.New(cfg, logger)

	authSvc := auth.NewService(pool, issuer, mailer, cfg, tm, logger)

	s := &Server{
		cfg:            cfg,
		pool:           pool,
		logger:         logger,
		authMW:         mw.NewAuth(issuer, tm, logger, pool),
		authHandler:    auth.NewHandler(authSvc, cfg, logger),
		profileHandler: profile.New(pool, logger),
	}

	s.protected = []Module{
		// Platform (public schema)
		users.New(pool, logger, s.authMW),
		permissions.New(pool, logger, s.authMW),
		tenants.New(pool, tm, logger, s.authMW),
		subscription.New(pool, logger),
		support.New(pool, logger),
		systemlog.New(pool, logger),
		useractivity.New(pool, logger),
		notifications.New(pool, logger),
		s.profileHandler, // protected GET/PUT /Profile

		categories.New(tm, logger, s.authMW),
		castes.New(tm, logger, s.authMW),
		codeconfig.New(tm, logger, s.authMW),
		apptypes.New(tm, logger, s.authMW),
		appstatuses.New(tm, logger, s.authMW),
		feetypes.New(tm, logger, s.authMW),
		codesequence.New(tm, logger, s.authMW),
		formtypes.New(tm, logger, s.authMW),
		colleges.New(tm, logger, s.authMW),
		branches.New(tm, logger, s.authMW),
	}

	s.branchProtected = []Module{
		students.New(tm, logger, s.authMW),
		applications.New(tm, logger, s.authMW),
		feeplans.New(tm, logger, s.authMW),
		feecollections.New(tm, logger, s.authMW),
		payments.New(tm, logger, s.authMW),
		dashboard.New(tm, logger, s.authMW),
	}

	return s
}

// Router builds the root HTTP handler.
func (s *Server) Router() http.Handler {
	r := chi.NewRouter()

	r.Use(chimw.RequestID)
	r.Use(chimw.RealIP)
	r.Use(s.requestLogger)
	r.Use(s.recoverer)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   s.cfg.CORSAllowedOrigins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-Requested-With", "X-Branch-ID"},
		ExposedHeaders:   []string{"Content-Disposition"},
		AllowCredentials: true, // FE sends withCredentials:true for the refresh cookie
		MaxAge:           300,
	}))
	r.Use(chimw.Timeout(60 * time.Second))

	// Health is served at the root (FE nginx proxies "/health" -> api:8080/health).
	r.Get("/health", s.health)

	r.Route("/api", func(api chi.Router) {
		api.Get("/ping", func(w http.ResponseWriter, _ *http.Request) {
			apiresp.OK(w, map[string]string{"pong": "ok"}, "pong")
		})

		// ── Public (no auth) ──
		s.authHandler.Mount(api)          // /Auth/*
		s.profileHandler.MountPublic(api) // GET /Profile/public-logo/{tenantId}

		// ── Protected (require a selected tenant) ──
		api.Group(func(pr chi.Router) {
			pr.Use(s.authMW.RequireTenant)
			for _, m := range s.protected {
				m.Mount(pr)
			}
			pr.Group(func(br chi.Router) {
				br.Use(s.authMW.RequireBranch)
				for _, m := range s.branchProtected {
					m.Mount(br)
				}
			})
		})
	})

	return r
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	dbOK := s.pool.Ping(ctx) == nil
	status := "healthy"
	if !dbOK {
		status = "degraded"
	}
	apiresp.OK(w, map[string]any{"status": status, "database": dbOK}, status)
}
