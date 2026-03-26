package api

import (
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"ssl-domain-exporter/internal/checker"
	"ssl-domain-exporter/internal/config"
	"ssl-domain-exporter/internal/db"
	"ssl-domain-exporter/internal/metrics"
)

func NewRouter(cfg *config.Config, database *db.DB, chk *checker.Checker, sched *checker.Scheduler, m *metrics.Metrics) http.Handler {
	if cfg.Server.Host != "" {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.Default()

	loginLimiter := NewRateLimiter()
	adminLimiter := NewRateLimiter()

	r.Use(RequestIDMiddleware())
	r.Use(cors.New(cors.Config{
		AllowOriginFunc:  corsOriginAllowed(cfg),
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization", "X-API-Key", csrfHeaderName, requestIDHeaderName},
		ExposeHeaders:    []string{"Content-Length", requestIDHeaderName},
		AllowCredentials: true,
	}))

	r.Use(AuthMiddleware(cfg, database))
	r.Use(CSRFMiddleware(cfg))

	h := NewHandler(cfg, database, chk, sched, m)
	distDir := resolveFrontendDistDir()
	slog.Info("Resolved UI dist directory", "path", distDir)

	if cfg.Prometheus.Enabled {
		r.GET(cfg.Prometheus.Path, gin.WrapH(promhttp.Handler()))
	}

	r.GET("/health", h.Health)
	r.GET("/ready", h.Readiness)

	api := r.Group("/api")
	{
		api.GET("/bootstrap", h.GetBootstrap)
		api.GET("/me", h.GetMe)
		api.POST("/session/login", LoginRateLimitMiddleware(cfg, loginLimiter), h.LoginSession)
		api.POST("/session/logout", h.LogoutSession)
	}

	view := api.Group("/")
	view.Use(RequireView(cfg))
	{
		view.GET("/summary", h.GetSummary)
		view.GET("/timeline", h.GetTimeline)
		view.GET("/domains", h.ListDomains)
		view.GET("/domains/search", h.SearchDomains)
		view.GET("/domains/export.csv", h.ExportDomainsCSV)
		view.GET("/domains/:id", h.GetDomain)
		view.GET("/domains/:id/history", h.GetHistory)
		view.GET("/folders", h.ListFolders)
		view.GET("/tags", h.ListTags)
		view.GET("/custom-fields", h.ListCustomFields)
	}

	editor := api.Group("/")
	editor.Use(RequireEditor(cfg))
	{
		editor.POST("/domains", h.CreateDomain)
		editor.POST("/domains/import", h.ImportDomains)
		editor.POST("/domains/reorder", h.ReorderDomains)
		editor.PUT("/domains/:id", h.UpdateDomain)
		editor.DELETE("/domains/:id", h.DeleteDomain)
		editor.POST("/domains/:id/check", h.TriggerCheck)

		editor.POST("/folders", h.CreateFolder)
		editor.PUT("/folders/:id", h.UpdateFolder)
		editor.DELETE("/folders/:id", h.DeleteFolder)
	}

	admin := api.Group("/")
	admin.Use(RequireAdmin(cfg))
	admin.Use(AdminWriteRateLimitMiddleware(cfg, adminLimiter))
	{
		admin.GET("/config", h.GetConfig)
		admin.PUT("/config", h.UpdateConfig)
		admin.GET("/audit-logs", h.GetAuditLogs)
		admin.GET("/maintenance/backups", h.ListBackups)
		admin.POST("/maintenance/backup", h.CreateBackup)
		admin.POST("/maintenance/prune", h.PruneChecks)
		admin.POST("/custom-fields", h.CreateCustomField)
		admin.PUT("/custom-fields/:id", h.UpdateCustomField)
		admin.DELETE("/custom-fields/:id", h.DeleteCustomField)
		admin.GET("/notifications/status", h.GetNotificationStatus)
		admin.POST("/notifications/test", h.TestNotifications)
		admin.GET("/settings", h.GetSettings)
		admin.PUT("/settings", h.UpdateSettings)
		admin.GET("/users", h.ListUsers)
		admin.POST("/users", h.CreateUser)
		admin.PUT("/users/:id", h.UpdateUser)
		admin.DELETE("/users/:id", h.DeleteUser)
	}

	r.Static("/assets", filepath.Join(distDir, "assets"))
	r.StaticFile("/favicon.ico", filepath.Join(distDir, "favicon.ico"))
	r.NoRoute(func(c *gin.Context) {
		c.Header("Cache-Control", "no-store, no-cache, must-revalidate")
		c.File(filepath.Join(distDir, "index.html"))
	})

	return r
}

func resolveFrontendDistDir() string {
	candidates := []string{
		filepath.Clean("./frontend/dist"),
	}

	if exePath, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exePath)
		candidates = append(candidates,
			filepath.Join(exeDir, "frontend", "dist"),
			filepath.Join(exeDir, "..", "frontend", "dist"),
		)
	}

	for _, dir := range candidates {
		indexPath := filepath.Join(dir, "index.html")
		if st, err := os.Stat(indexPath); err == nil && !st.IsDir() {
			if abs, err := filepath.Abs(dir); err == nil {
				return abs
			}
			return dir
		}
	}

	return filepath.Clean("./frontend/dist")
}

func corsOriginAllowed(cfg *config.Config) func(string) bool {
	return func(origin string) bool {
		normalized, ok := normalizeAllowedOrigin(origin)
		if !ok {
			return false
		}
		snap := cfg.Snapshot()
		for _, candidate := range snap.Server.AllowedOrigins {
			if allowed, valid := normalizeAllowedOrigin(candidate); valid && allowed == normalized {
				return true
			}
		}
		return false
	}
}

func normalizeAllowedOrigin(origin string) (string, bool) {
	value := strings.TrimSpace(origin)
	if value == "" || value == "null" {
		return "", false
	}
	parsed, err := url.ParseRequestURI(value)
	if err != nil {
		return "", false
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return "", false
	}
	if parsed.Host == "" {
		return "", false
	}
	if parsed.User != nil {
		return "", false
	}
	if parsed.Path != "" && parsed.Path != "/" {
		return "", false
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", false
	}
	return scheme + "://" + strings.ToLower(parsed.Host), true
}
