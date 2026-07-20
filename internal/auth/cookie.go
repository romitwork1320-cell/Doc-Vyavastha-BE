package auth

import (
	"net/http"
	"strings"
	"time"

	"github.com/thinkparq/edconsultancy-be/internal/config"
)

// setRefreshCookie writes the httpOnly refresh cookie the FE relies on
// (it posts an empty body to /Auth/refresh-token and depends on withCredentials).
func setRefreshCookie(w http.ResponseWriter, cfg *config.Config, value string, ttl time.Duration) {
	http.SetCookie(w, &http.Cookie{
		Name:     cfg.CookieName,
		Value:    value,
		Path:     "/api/Auth",
		Domain:   cfg.CookieDomain,
		MaxAge:   int(ttl.Seconds()),
		Secure:   cfg.CookieSecure,
		HttpOnly: true,
		SameSite: sameSite(cfg.CookieSameSite),
	})
}

// clearRefreshCookie expires the refresh cookie.
func clearRefreshCookie(w http.ResponseWriter, cfg *config.Config) {
	http.SetCookie(w, &http.Cookie{
		Name:     cfg.CookieName,
		Value:    "",
		Path:     "/api/Auth",
		Domain:   cfg.CookieDomain,
		MaxAge:   -1,
		Secure:   cfg.CookieSecure,
		HttpOnly: true,
		SameSite: sameSite(cfg.CookieSameSite),
	})
}

// readRefreshCookie returns the raw refresh token from the request cookie.
func readRefreshCookie(r *http.Request, cfg *config.Config) string {
	c, err := r.Cookie(cfg.CookieName)
	if err != nil {
		return ""
	}
	return c.Value
}

func sameSite(s string) http.SameSite {
	switch strings.ToLower(s) {
	case "strict":
		return http.SameSiteStrictMode
	case "none":
		return http.SameSiteNoneMode
	default:
		return http.SameSiteLaxMode
	}
}
