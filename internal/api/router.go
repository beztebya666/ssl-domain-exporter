package api

import (
	"log"
	"net/http"
	"os"
	"path/filepath"

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

	r.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"*"},
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization", "X-API-Key"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
	}))

	r.Use(AuthMiddleware(cfg, database))

	h := NewHandler(cfg, database, chk, sched, m)
	distDir := resolveFrontendDistDir()
	log.Printf("UI dist directory: %s", distDir)

	if cfg.Prometheus.Enabled {
		r.GET(cfg.Prometheus.Path, gin.WrapH(promhttp.Handler()))
	}

	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	api := r.Group("/api")
	{
		api.GET("/bootstrap", h.GetBootstrap)
		api.GET("/me", h.GetMe)
		api.POST("/session/login", h.LoginSession)
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
	{
		admin.GET("/config", h.GetConfig)
		admin.PUT("/config", h.UpdateConfig)
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
