package api

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"ssl-domain-exporter/internal/config"
)

func AuthMiddleware(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Take a thread-safe snapshot so hot-reloaded config is read safely.
		snap := cfg.Snapshot()

		if !snap.Auth.Enabled {
			c.Next()
			return
		}
		if c.Request.Method == http.MethodOptions {
			c.Next()
			return
		}
		if !shouldProtectPath(snap, c.Request.URL.Path) {
			c.Next()
			return
		}

		mode := strings.ToLower(strings.TrimSpace(snap.Auth.Mode))
		switch mode {
		case "api_key":
			if validateAPIKey(snap, c) {
				c.Next()
				return
			}
			unauthorized(c)
			return
		case "both":
			if validateBasic(snap, c) || validateAPIKey(snap, c) {
				c.Next()
				return
			}
			unauthorized(c)
			return
		default:
			if validateBasic(snap, c) {
				c.Next()
				return
			}
			unauthorized(c)
			return
		}
	}
}

func shouldProtectPath(cfg *config.Config, path string) bool {
	if path == "/health" {
		return false
	}
	if strings.HasPrefix(path, "/api") {
		return cfg.Auth.ProtectAPI
	}
	if cfg.Prometheus.Enabled && path == cfg.Prometheus.Path {
		return cfg.Auth.ProtectMetrics
	}
	return cfg.Auth.ProtectUI
}

func validateBasic(cfg *config.Config, c *gin.Context) bool {
	if cfg.Auth.Username == "" || cfg.Auth.Password == "" {
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
	if cfg.Auth.APIKey == "" {
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

func unauthorized(c *gin.Context) {
	c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
}
