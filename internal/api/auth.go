package api

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"ssl-domain-exporter/internal/config"
	"ssl-domain-exporter/internal/db"
)

const principalContextKey = "principal"

type Principal struct {
	Authenticated bool   `json:"authenticated"`
	UserID        int64  `json:"user_id,omitempty"`
	Username      string `json:"username,omitempty"`
	Role          string `json:"role"`
	Source        string `json:"source"`
}

type PrincipalResponse struct {
	Authenticated bool   `json:"authenticated"`
	Username      string `json:"username,omitempty"`
	Role          string `json:"role"`
	Source        string `json:"source"`
	CanView       bool   `json:"can_view"`
	CanEdit       bool   `json:"can_edit"`
	CanAdmin      bool   `json:"can_admin"`
	PublicUI      bool   `json:"public_ui"`
}

func anonymousPrincipal() Principal {
	return Principal{Role: "anonymous", Source: "anonymous"}
}

func (p Principal) CanView(cfg *config.Config) bool {
	if cfg == nil || !cfg.Auth.Enabled {
		return true
	}
	if p.Authenticated {
		return true
	}
	return cfg.Auth.AnonymousReadOnlyEnabled()
}

func (p Principal) CanEdit() bool {
	return p.Authenticated && (p.Role == "editor" || p.Role == "admin")
}

func (p Principal) CanAdmin() bool {
	return p.Authenticated && p.Role == "admin"
}

func principalToResponse(p Principal, cfg *config.Config) PrincipalResponse {
	publicUI := cfg != nil && cfg.Auth.AnonymousReadOnlyEnabled()
	return PrincipalResponse{
		Authenticated: p.Authenticated,
		Username:      p.Username,
		Role:          p.Role,
		Source:        p.Source,
		CanView:       p.CanView(cfg),
		CanEdit:       p.CanEdit(),
		CanAdmin:      p.CanAdmin(),
		PublicUI:      publicUI,
	}
}

func AuthMiddleware(cfg *config.Config, database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		snap := cfg.Snapshot()
		principal := resolvePrincipal(snap, database, c)
		c.Set(principalContextKey, principal)

		if c.Request.Method == http.MethodOptions {
			c.Next()
			return
		}

		if snap.Prometheus.Enabled && c.Request.URL.Path == snap.Prometheus.Path && snap.Auth.Enabled && snap.Auth.ProtectMetrics && !principal.CanAdmin() {
			unauthorized(c)
			return
		}

		c.Next()
	}
}

func resolvePrincipal(cfg *config.Config, database *db.DB, c *gin.Context) Principal {
	if cfg == nil {
		return anonymousPrincipal()
	}
	if !cfg.Auth.Enabled {
		return Principal{
			Authenticated: true,
			Username:      "local-admin",
			Role:          "admin",
			Source:        "auth_disabled",
		}
	}

	if principal, ok := resolveSessionPrincipal(cfg, database, c); ok {
		return principal
	}
	if principal, ok := resolveLegacyPrincipal(cfg, c); ok {
		return principal
	}
	return anonymousPrincipal()
}

func resolveSessionPrincipal(cfg *config.Config, database *db.DB, c *gin.Context) (Principal, bool) {
	if database == nil {
		return Principal{}, false
	}
	cookieName := strings.TrimSpace(cfg.Auth.CookieName)
	if cookieName == "" {
		cookieName = "ssl_domain_exporter_session"
	}
	rawToken, err := c.Cookie(cookieName)
	if err != nil || strings.TrimSpace(rawToken) == "" {
		return Principal{}, false
	}

	user, session, err := database.GetUserBySessionTokenHash(hashSessionToken(rawToken))
	if err != nil || user == nil || session == nil {
		clearSessionCookie(c, cfg)
		return Principal{}, false
	}
	if !user.Enabled || session.ExpiresAt.Before(time.Now()) {
		_ = database.DeleteSession(hashSessionToken(rawToken))
		clearSessionCookie(c, cfg)
		return Principal{}, false
	}

	_ = database.TouchSession(session.TokenHash, time.Now())
	return Principal{
		Authenticated: true,
		UserID:        user.ID,
		Username:      user.Username,
		Role:          db.NormalizeUserRole(user.Role),
		Source:        "session",
	}, true
}

func resolveLegacyPrincipal(cfg *config.Config, c *gin.Context) (Principal, bool) {
	if validateBasic(cfg, c) {
		return Principal{
			Authenticated: true,
			Username:      cfg.Auth.Username,
			Role:          "admin",
			Source:        "basic",
		}, true
	}
	if validateAPIKey(cfg, c) {
		return Principal{
			Authenticated: true,
			Username:      "api-key",
			Role:          "admin",
			Source:        "api_key",
		}, true
	}
	return Principal{}, false
}

func RequireView(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		principal := GetPrincipal(c)
		if principal.CanView(cfg.Snapshot()) {
			c.Next()
			return
		}
		unauthorized(c)
	}
}

func RequireEditor(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		snap := cfg.Snapshot()
		principal := GetPrincipal(c)
		if !snap.Auth.Enabled {
			c.Next()
			return
		}
		if !principal.Authenticated {
			unauthorized(c)
			return
		}
		if principal.CanEdit() || principal.CanAdmin() {
			c.Next()
			return
		}
		forbidden(c)
	}
}

func RequireAdmin(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		snap := cfg.Snapshot()
		principal := GetPrincipal(c)
		if !snap.Auth.Enabled {
			c.Next()
			return
		}
		if !principal.Authenticated {
			unauthorized(c)
			return
		}
		if principal.CanAdmin() {
			c.Next()
			return
		}
		forbidden(c)
	}
}

func GetPrincipal(c *gin.Context) Principal {
	if c == nil {
		return anonymousPrincipal()
	}
	if value, ok := c.Get(principalContextKey); ok {
		if principal, ok := value.(Principal); ok {
			return principal
		}
	}
	return anonymousPrincipal()
}

func validateBasic(cfg *config.Config, c *gin.Context) bool {
	if cfg == nil || cfg.Auth.Username == "" || cfg.Auth.Password == "" {
		return false
	}
	mode := strings.ToLower(strings.TrimSpace(cfg.Auth.Mode))
	if mode == "api_key" {
		return false
	}
	username, password, ok := c.Request.BasicAuth()
	if !ok {
		return false
	}
	userOK := subtle.ConstantTimeCompare([]byte(username), []byte(cfg.Auth.Username)) == 1
	passOK := subtle.ConstantTimeCompare([]byte(password), []byte(cfg.Auth.Password)) == 1
	return userOK && passOK
}

func validateAPIKey(cfg *config.Config, c *gin.Context) bool {
	if cfg == nil || cfg.Auth.APIKey == "" {
		return false
	}
	mode := strings.ToLower(strings.TrimSpace(cfg.Auth.Mode))
	if mode == "basic" {
		return false
	}
	candidate := strings.TrimSpace(c.GetHeader("X-API-Key"))
	if candidate == "" {
		authz := strings.TrimSpace(c.GetHeader("Authorization"))
		if strings.HasPrefix(strings.ToLower(authz), "bearer ") {
			candidate = strings.TrimSpace(authz[7:])
		}
	}
	if candidate == "" {
		candidate = strings.TrimSpace(c.Query("api_key"))
	}
	return subtle.ConstantTimeCompare([]byte(candidate), []byte(cfg.Auth.APIKey)) == 1
}

func hashSessionToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func clearSessionCookie(c *gin.Context, cfg *config.Config) {
	c.SetCookie(cookieName(cfg), "", -1, "/", "", cfg.Auth.CookieSecure, true)
}

func cookieName(cfg *config.Config) string {
	if cfg != nil && strings.TrimSpace(cfg.Auth.CookieName) != "" {
		return cfg.Auth.CookieName
	}
	return "ssl_domain_exporter_session"
}

func unauthorized(c *gin.Context) {
	c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
}

func forbidden(c *gin.Context) {
	c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "forbidden"})
}
