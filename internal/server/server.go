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

	"github.com/thinkparq/edconsultancy-be/internal/platform/applications"
	"github.com/thinkparq/edconsultancy-be/internal/platform/clients"
	"github.com/thinkparq/edconsultancy-be/internal/platform/connectionpermissions"
	"github.com/thinkparq/edconsultancy-be/internal/platform/dashboard"
	"github.com/thinkparq/edconsultancy-be/internal/platform/document_types"
	"github.com/thinkparq/edconsultancy-be/internal/platform/kyc"
	"github.com/thinkparq/edconsultancy-be/internal/platform/magiclink"
	"github.com/thinkparq/edconsultancy-be/internal/platform/notifications"
	"github.com/thinkparq/edconsultancy-be/internal/platform/organization_types"
	"github.com/thinkparq/edconsultancy-be/internal/platform/permissions"
	"github.com/thinkparq/edconsultancy-be/internal/platform/profile"
	"github.com/thinkparq/edconsultancy-be/internal/platform/subscription"
	"github.com/thinkparq/edconsultancy-be/internal/platform/superadmin"
	"github.com/thinkparq/edconsultancy-be/internal/platform/support"
	"github.com/thinkparq/edconsultancy-be/internal/platform/systemlog"
	"github.com/thinkparq/edconsultancy-be/internal/platform/templates"
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
	
	magiclinkHandler *magiclink.Handler
	templatesHandler *templates.Handler

	authenticated   []Module // mounted behind Require
	protected       []Module // mounted behind RequireTenant
	branchProtected []Module // mounted behind RequireBranch
}

// New constructs a Server and all feature modules.
func New(cfg *config.Config, pool *pgxpool.Pool, logger *slog.Logger) *Server {
	tm := tenancy.NewManager(pool)
	issuer := token.NewIssuer(cfg.JWTAccessSecret, cfg.JWTIssuer, cfg.JWTAccessTTL, cfg.JWTTempTTL)
	mailer := email.New(cfg, logger)

	authSvc := auth.NewService(pool, issuer, mailer, cfg, tm, logger)

	// --- New Phase 2 Modules ---
	clientsHandler := clients.New(pool, logger)
	connPermsHandler := connectionpermissions.New(pool, logger)
	dashboardHandler := dashboard.New(pool, logger)

	s := &Server{
		cfg:            cfg,
		pool:           pool,
		logger:         logger,
		authMW:         mw.NewAuth(issuer, tm, logger, pool),
		authHandler:    auth.NewHandler(authSvc, cfg, logger),
		profileHandler: profile.New(pool, logger),
		templatesHandler: templates.New(pool, logger),
	}

	// Phase 3 Modules
	applicationsHandler := applications.New(pool, logger)
	s.magiclinkHandler = magiclink.NewHandler(pool, logger, applicationsHandler.Service())

	s.authenticated = []Module{
		notifications.New(pool, logger),
		s.profileHandler, // protected GET/PUT /Profile
		clientsHandler,
		connPermsHandler,
		dashboardHandler,
		kyc.New(pool, logger, s.authMW), // moved from protected
		superadmin.New(pool, logger),
		users.New(pool, logger, s.authMW),       // moved from protected
		permissions.New(pool, logger, s.authMW), // moved from protected
		document_types.New(pool, logger),
		organization_types.New(pool, logger),
		applicationsHandler,
	}

	s.protected = []Module{
		// Platform (public schema)
		tenants.New(pool, tm, logger, s.authMW),
		subscription.New(pool, logger),
		support.New(pool, logger),
		systemlog.New(pool, logger),
		useractivity.New(pool, logger),
		// Magic Link generation is protected (org uses it)
		s.magiclinkHandler,
	}

	s.branchProtected = []Module{}

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
		s.magiclinkHandler.MountPublic(api) // /MagicLink/*

		// ── Authenticated (no tenant required) ──
		api.Group(func(ar chi.Router) {
			ar.Use(s.authMW.Require)
			for _, m := range s.authenticated {
				m.Mount(ar)
			}
			s.templatesHandler.MountPublic(ar)
			s.templatesHandler.MountAdmin(ar)
		})

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
