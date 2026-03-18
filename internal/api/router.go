package api

import (
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"domain-ssl-checker/internal/checker"
	"domain-ssl-checker/internal/config"
	"domain-ssl-checker/internal/db"
	"domain-ssl-checker/internal/metrics"
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

	r.Use(AuthMiddleware(cfg))

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
		api.GET("/summary", h.GetSummary)

		api.GET("/config", h.GetConfig)
		api.PUT("/config", h.UpdateConfig)
		api.GET("/settings", h.GetSettings)
		api.PUT("/settings", h.UpdateSettings)

		api.GET("/domains", h.ListDomains)
		api.POST("/domains", h.CreateDomain)
		api.POST("/domains/reorder", h.ReorderDomains)
		api.GET("/domains/export.csv", h.ExportDomainsCSV)
		api.GET("/domains/:id", h.GetDomain)
		api.PUT("/domains/:id", h.UpdateDomain)
		api.DELETE("/domains/:id", h.DeleteDomain)
		api.POST("/domains/:id/check", h.TriggerCheck)
		api.GET("/domains/:id/history", h.GetHistory)

		api.GET("/folders", h.ListFolders)
		api.POST("/folders", h.CreateFolder)
		api.PUT("/folders/:id", h.UpdateFolder)
		api.DELETE("/folders/:id", h.DeleteFolder)
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
