// Package config loads and validates runtime configuration from the
// environment (and an optional .env file). It is the single source of truth
// for every tunable in the service.
package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/caarlos0/env/v11"
	"github.com/joho/godotenv"
)

// Config is the fully-parsed application configuration.
type Config struct {
	AppEnv   string `env:"APP_ENV" envDefault:"development"`
	HTTPPort string `env:"HTTP_PORT" envDefault:"8080"`
	LogLevel string `env:"LOG_LEVEL" envDefault:"info"`

	DatabaseURL string `env:"DATABASE_URL,required"`
	DBMaxConns  int32  `env:"DB_MAX_CONNS" envDefault:"20"`
	DBMinConns  int32  `env:"DB_MIN_CONNS" envDefault:"2"`

	// AutoMigrate runs public + all-tenant migrations on startup (handy for
	// docker-compose / local dev). SeedDemo inserts idempotent demo data.
	AutoMigrate bool `env:"AUTO_MIGRATE" envDefault:"false"`
	SeedDemo    bool `env:"SEED_DEMO" envDefault:"false"`

	JWTIssuer       string        `env:"JWT_ISSUER" envDefault:"edconsultancy"`
	JWTAccessSecret string        `env:"JWT_ACCESS_SECRET,required"`
	JWTAccessTTL    time.Duration `env:"JWT_ACCESS_TTL" envDefault:"15m"`
	JWTRefreshTTL   time.Duration `env:"JWT_REFRESH_TTL" envDefault:"720h"`
	JWTTempTTL      time.Duration `env:"JWT_TEMP_TTL" envDefault:"10m"`

	CookieName     string `env:"COOKIE_NAME" envDefault:"edc_refresh"`
	CookieDomain   string `env:"COOKIE_DOMAIN"`
	CookieSecure   bool   `env:"COOKIE_SECURE" envDefault:"false"`
	CookieSameSite string `env:"COOKIE_SAMESITE" envDefault:"lax"`

	CORSAllowedOrigins []string `env:"CORS_ALLOWED_ORIGINS" envSeparator:"," envDefault:"http://localhost:4200"`
	ClientURL          string   `env:"CLIENT_URL" envDefault:"http://localhost:4200"`

	GoogleClientID string `env:"GOOGLE_CLIENT_ID"`

	EmailSender   string `env:"EMAIL_SENDER" envDefault:"log"`
	SMTPHost      string `env:"SMTP_HOST"`
	SMTPPort      int    `env:"SMTP_PORT" envDefault:"587"`
	SMTPUsername  string `env:"SMTP_USERNAME"`
	SMTPPassword  string `env:"SMTP_PASSWORD"`
	EmailFrom     string `env:"EMAIL_FROM" envDefault:"no-reply@edconsultancy.local"`
	EmailFromName string `env:"EMAIL_FROM_NAME" envDefault:"EDConsultancy"`

	OTPTTL    time.Duration `env:"OTP_TTL" envDefault:"5m"`
	OTPLength int           `env:"OTP_LENGTH" envDefault:"6"`

	LinkEncryptionKey string `env:"LINK_ENCRYPTION_KEY" envDefault:"dreamworldi-9999"`
}

// IsProduction reports whether the service is running in production mode.
func (c *Config) IsProduction() bool { return strings.EqualFold(c.AppEnv, "production") }

// Load reads configuration from the environment. If a .env file is present in
// the working directory it is loaded first (existing env vars take precedence).
func Load() (*Config, error) {
	// Best-effort: a missing .env is fine (prod injects real env vars).
	_ = godotenv.Load()

	var cfg Config
	if err := env.Parse(&cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	return &cfg, nil
}
