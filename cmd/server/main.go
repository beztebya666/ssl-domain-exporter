package main

import (
	"context"
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"domain-ssl-checker/internal/api"
	"domain-ssl-checker/internal/checker"
	"domain-ssl-checker/internal/config"
	"domain-ssl-checker/internal/db"
	"domain-ssl-checker/internal/metrics"
)

var (
	AppVersion = "v1.1.0"
	UIVersion  = "v1.1.0"
	BuildTime  = "unknown"
	GitCommit  = "unknown"
)

func main() {
	configPath := flag.String("config", "", "path to config file (default: ./config.yaml or $CONFIG_PATH)")
	configDir := flag.String("config-dir", "", "directory for config/data defaults (optional)")
	flag.Parse()

	if *configDir == "" {
		if v := os.Getenv("CONFIG_DIR"); v != "" {
			*configDir = v
		}
	}

	if *configPath == "" {
		if v := os.Getenv("CONFIG_PATH"); v != "" {
			*configPath = v
		} else if *configDir != "" {
			*configPath = filepath.Join(*configDir, "config.yaml")
		} else {
			*configPath = "config.yaml"
		}
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Printf("Warning: config load error (%v), using defaults", err)
	}
	log.Printf("Config: %s", cfg.FilePath())

	if *configDir != "" {
		defaultDB := "./data/checker.db"
		if cfg.Database.Path == "" || cfg.Database.Path == defaultDB {
			cfg.Database.Path = filepath.Join(*configDir, "checker.db")
			if saveErr := cfg.Save(); saveErr != nil {
				log.Printf("Warning: failed to save config with config-dir defaults: %v", saveErr)
			}
		}
	}

	if cfg.Logging.JSON || cfg.Features.StructuredLogs {
		log.SetFlags(0)
		log.SetOutput(jsonLogWriter{})
		log.Println("structured logging enabled")
	}

	log.Printf("Version: app=%s ui=%s commit=%s build_time=%s", AppVersion, UIVersion, GitCommit, BuildTime)

	database, err := db.New(cfg.Database.Path)
	if err != nil {
		log.Fatalf("DB init failed: %v", err)
	}
	defer database.Close()

	if err := database.Migrate(); err != nil {
		log.Fatalf("DB migrate failed: %v", err)
	}

	m := metrics.New()
	notifier := checker.NewNotifier(cfg)
	chk := checker.New(cfg, database, m, notifier)
	sched := checker.NewScheduler(cfg, database, chk, m)
	sched.Start()
	defer sched.Stop()

	router := api.NewRouter(cfg, database, chk, sched, m)

	srv := &http.Server{
		Addr:         cfg.Server.Host + ":" + cfg.Server.Port,
		Handler:      router,
		ReadTimeout:  60 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	go func() {
		log.Printf("Server starting on http://%s:%s", cfg.Server.Host, cfg.Server.Port)
		if cfg.Prometheus.Enabled {
			log.Printf("Prometheus metrics at http://%s:%s%s", cfg.Server.Host, cfg.Server.Port, cfg.Prometheus.Path)
		}
		if cfg.Auth.Enabled {
			log.Printf("Auth enabled (mode=%s user=%s)", cfg.Auth.Mode, cfg.Auth.Username)
		}
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down...")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
	log.Println("Bye")
}

type jsonLogWriter struct{}

func (w jsonLogWriter) Write(p []byte) (int, error) {
	msg := strings.TrimSpace(string(p))
	if msg == "" {
		return len(p), nil
	}
	entry := map[string]string{
		"ts":    time.Now().UTC().Format(time.RFC3339Nano),
		"level": "info",
		"msg":   msg,
	}
	b, err := json.Marshal(entry)
	if err != nil {
		return os.Stdout.Write(p)
	}
	_, err = os.Stdout.Write(append(b, '\n'))
	return len(p), err
}
