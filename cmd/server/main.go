package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"ssl-domain-exporter/internal/api"
	"ssl-domain-exporter/internal/checker"
	"ssl-domain-exporter/internal/config"
	"ssl-domain-exporter/internal/db"
	"ssl-domain-exporter/internal/metrics"
)

var (
	AppVersion = "v1.4.0"
	UIVersion  = "v1.4.0"
	BuildTime  = "unknown"
	GitCommit  = "unknown"
)

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))

	configPath := flag.String("config", "", "path to config file (default: ./config.yaml or $CONFIG_PATH)")
	configDir := flag.String("config-dir", "", "directory for config/data defaults (optional)")
	backupPath := flag.String("backup", "", "create a sqlite backup at the provided path and exit")
	restorePath := flag.String("restore", "", "restore the sqlite database from the provided backup path and exit")
	showVersion := flag.Bool("version", false, "print version information and exit")
	flag.Parse()
	if *showVersion {
		printVersion(os.Stdout)
		return
	}

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

	// Ensure config directory exists and is writable before loading
	if dir := filepath.Dir(*configPath); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			slog.Warn("Cannot create config directory", "path", dir, "error", err)
		}
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		slog.Warn("Config load error, using defaults", "error", err)
	}
	configureLogger(cfg)
	slog.Info("Config loaded", "path", cfg.FilePath())

	if *configDir != "" {
		defaultDB := "./data/checker.db"
		if cfg.Database.Path == "" || cfg.Database.Path == defaultDB {
			cfg.Database.Path = filepath.Join(*configDir, "checker.db")
			if saveErr := cfg.Save(); saveErr != nil {
				slog.Warn("Failed to save config with config-dir defaults", "error", saveErr)
			}
		}
	}
	if *restorePath != "" {
		if err := db.RestoreSQLiteFile(*restorePath, cfg.Database.Path); err != nil {
			fatal("Restore failed", "error", err)
		}
		slog.Info("Database restored", "source", *restorePath, "destination", cfg.Database.Path)
		return
	}
	for _, warning := range cfg.InsecureWarnings() {
		slog.Warn("Security warning", "warning", warning)
	}

	slog.Info("Version info", "app", AppVersion, "ui", UIVersion, "commit", GitCommit, "build_time", BuildTime)

	database, err := db.New(cfg.Database.Path)
	if err != nil {
		fatal("DB init failed", "error", err)
	}
	defer database.Close()

	if err := database.Migrate(); err != nil {
		fatal("DB migrate failed", "error", err)
	}
	if *backupPath != "" {
		if err := database.BackupTo(*backupPath); err != nil {
			fatal("Backup failed", "error", err)
		}
		slog.Info("Database backup created", "path", *backupPath)
		return
	}

	m := metrics.New(cfg)
	if domains, err := database.GetDomains(); err == nil {
		m.SyncDomains(domains)
		m.SetTotalDomains(len(domains))
	} else {
		slog.Warn("Failed to preload domain metrics", "error", err)
	}
	notifier := checker.NewNotifier(cfg)
	chk := checker.New(cfg, database, m, notifier)
	sched := checker.NewScheduler(cfg, database, chk, m)
	sched.Start()

	router := api.NewRouter(cfg, database, chk, sched, m)

	srv := &http.Server{
		Addr:         cfg.Server.Host + ":" + cfg.Server.Port,
		Handler:      router,
		ReadTimeout:  60 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	serverErrCh := make(chan error, 1)
	go func() {
		scheme := "http"
		if cfg.Server.TLS.Enabled {
			scheme = "https"
		}
		slog.Info("Server starting", "url", scheme+"://"+cfg.Server.Host+":"+cfg.Server.Port)
		if cfg.Prometheus.Enabled {
			slog.Info("Prometheus metrics exposed", "url", scheme+"://"+cfg.Server.Host+":"+cfg.Server.Port+cfg.Prometheus.Path)
		}
		if cfg.Auth.Enabled {
			slog.Info("Authentication enabled", "mode", cfg.Auth.Mode, "user", cfg.Auth.Username)
		}
		var listenErr error
		if cfg.Server.TLS.Enabled {
			slog.Info("TLS enabled", "cert", cfg.Server.TLS.CertFile, "key", cfg.Server.TLS.KeyFile)
			listenErr = srv.ListenAndServeTLS(cfg.Server.TLS.CertFile, cfg.Server.TLS.KeyFile)
		} else {
			listenErr = srv.ListenAndServe()
		}
		if listenErr != nil && listenErr != http.ErrServerClosed {
			serverErrCh <- listenErr
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(quit)

	exitCode := 0
	select {
	case sig := <-quit:
		slog.Info("Shutting down", "signal", sig.String())
	case err := <-serverErrCh:
		exitCode = 1
		slog.Error("Server error", "error", err)
	}

	httpCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := srv.Shutdown(httpCtx); err != nil {
		slog.Error("HTTP shutdown failed", "error", err)
	}
	sched.Stop()
	notifierCtx, notifierCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer notifierCancel()
	if err := notifier.Stop(notifierCtx); err != nil {
		slog.Warn("Notifier drain timed out", "error", err)
	}
	slog.Info("Shutdown complete")
	if exitCode != 0 {
		os.Exit(exitCode)
	}
}

func configureLogger(cfg *config.Config) {
	handlerOpts := &slog.HandlerOptions{Level: slog.LevelInfo}
	useJSON := cfg != nil && (cfg.Logging.JSON || cfg.Features.StructuredLogs)

	var writers []io.Writer
	writers = append(writers, os.Stdout)

	// Set up syslog writer if configured
	if cfg != nil && cfg.Logging.Syslog.Enabled {
		syslogWriter, err := config.NewSyslogWriter(cfg.Logging.Syslog)
		if err != nil {
			slog.Warn("Failed to initialize syslog writer", "error", err)
		} else {
			writers = append(writers, syslogWriter)
			slog.Info("Syslog forwarding enabled", "address", cfg.Logging.Syslog.Address, "network", cfg.Logging.Syslog.Network)
		}
	}

	var output io.Writer
	if len(writers) > 1 {
		output = io.MultiWriter(writers...)
	} else {
		output = writers[0]
	}

	var handler slog.Handler
	if useJSON {
		handler = slog.NewJSONHandler(output, handlerOpts)
	} else {
		handler = slog.NewTextHandler(output, handlerOpts)
	}
	logger := slog.New(handler)
	slog.SetDefault(logger)
	log.SetFlags(0)
	log.SetOutput(io.Discard)
}

func fatal(msg string, args ...any) {
	slog.Error(msg, args...)
	os.Exit(1)
}

func printVersion(w io.Writer) {
	_, _ = fmt.Fprintf(w, "ssl-domain-exporter app=%s ui=%s commit=%s build_time=%s\n", AppVersion, UIVersion, GitCommit, BuildTime)
}
