package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

type ServerConfig struct {
	Host string `yaml:"host" json:"host"`
	Port string `yaml:"port" json:"port"`
}

type DatabaseConfig struct {
	Path string `yaml:"path" json:"path"`
}

type AuthConfig struct {
	Enabled        bool   `yaml:"enabled" json:"enabled"`
	Mode           string `yaml:"mode" json:"mode"`
	Username       string `yaml:"username" json:"username"`
	Password       string `yaml:"password" json:"password"`
	APIKey         string `yaml:"api_key" json:"api_key"`
	ProtectAPI     bool   `yaml:"protect_api" json:"protect_api"`
	ProtectMetrics bool   `yaml:"protect_metrics" json:"protect_metrics"`
	ProtectUI      bool   `yaml:"protect_ui" json:"protect_ui"`
}

type CheckerConfig struct {
	Interval         string `yaml:"interval" json:"interval"`
	Timeout          string `yaml:"timeout" json:"timeout"`
	ConcurrentChecks int    `yaml:"concurrent_checks" json:"concurrent_checks"`
	RetryCount       int    `yaml:"retry_count" json:"retry_count"`
}

type FeaturesConfig struct {
	HTTPCheck          bool `yaml:"http_check" json:"http_check"`
	CipherCheck        bool `yaml:"cipher_check" json:"cipher_check"`
	OCSPCheck          bool `yaml:"ocsp_check" json:"ocsp_check"`
	CRLCheck           bool `yaml:"crl_check" json:"crl_check"`
	CAACheck           bool `yaml:"caa_check" json:"caa_check"`
	Notifications      bool `yaml:"notifications" json:"notifications"`
	CSVExport          bool `yaml:"csv_export" json:"csv_export"`
	TimelineView       bool `yaml:"timeline_view" json:"timeline_view"`
	DashboardTagFilter bool `yaml:"dashboard_tag_filter" json:"dashboard_tag_filter"`
	StructuredLogs     bool `yaml:"structured_logs" json:"structured_logs"`
}

type AlertsConfig struct {
	DomainExpiryWarningDays  int `yaml:"domain_expiry_warning_days" json:"domain_expiry_warning_days"`
	DomainExpiryCriticalDays int `yaml:"domain_expiry_critical_days" json:"domain_expiry_critical_days"`
	SSLExpiryWarningDays     int `yaml:"ssl_expiry_warning_days" json:"ssl_expiry_warning_days"`
	SSLExpiryCriticalDays    int `yaml:"ssl_expiry_critical_days" json:"ssl_expiry_critical_days"`
}

type WebhookConfig struct {
	Enabled    bool   `yaml:"enabled" json:"enabled"`
	URL        string `yaml:"url" json:"url"`
	OnCritical bool   `yaml:"on_critical" json:"on_critical"`
	OnWarning  bool   `yaml:"on_warning" json:"on_warning"`
}

type TelegramConfig struct {
	Enabled    bool   `yaml:"enabled" json:"enabled"`
	BotToken   string `yaml:"bot_token" json:"bot_token"`
	ChatID     string `yaml:"chat_id" json:"chat_id"`
	OnCritical bool   `yaml:"on_critical" json:"on_critical"`
	OnWarning  bool   `yaml:"on_warning" json:"on_warning"`
}

type NotificationsConfig struct {
	Webhook  WebhookConfig  `yaml:"webhook" json:"webhook"`
	Telegram TelegramConfig `yaml:"telegram" json:"telegram"`
}

type DomainsConfig struct {
	SubdomainFallback bool   `yaml:"subdomain_fallback" json:"subdomain_fallback"`
	FallbackDepth     int    `yaml:"fallback_depth" json:"fallback_depth"`
	DefaultCheckMode  string `yaml:"default_check_mode" json:"default_check_mode"` // "full" or "ssl_only"
}

type DNSConfig struct {
	Servers      []string `yaml:"servers" json:"servers"`               // custom DNS servers, e.g. ["10.0.0.1:53"]
	UseSystemDNS bool     `yaml:"use_system_dns" json:"use_system_dns"` // allow OS DNS discovery and OS resolver fallback
	Timeout      string   `yaml:"timeout" json:"timeout"`               // DNS query timeout, e.g. "5s"
}

type PrometheusConfig struct {
	Enabled bool   `yaml:"enabled" json:"enabled"`
	Path    string `yaml:"path" json:"path"`
}

type LoggingConfig struct {
	JSON bool `yaml:"json" json:"json"`
}

type Config struct {
	Server        ServerConfig        `yaml:"server" json:"server"`
	Database      DatabaseConfig      `yaml:"database" json:"database"`
	Auth          AuthConfig          `yaml:"auth" json:"auth"`
	Checker       CheckerConfig       `yaml:"checker" json:"checker"`
	Features      FeaturesConfig      `yaml:"features" json:"features"`
	Alerts        AlertsConfig        `yaml:"alerts" json:"alerts"`
	Notifications NotificationsConfig `yaml:"notifications" json:"notifications"`
	Domains       DomainsConfig       `yaml:"domains" json:"domains"`
	DNS           DNSConfig           `yaml:"dns" json:"dns"`
	Prometheus    PrometheusConfig    `yaml:"prometheus" json:"prometheus"`
	Logging       LoggingConfig       `yaml:"logging" json:"logging"`

	mu       sync.RWMutex `yaml:"-" json:"-"`
	filePath string       `yaml:"-" json:"-"`
}

func Default() *Config {
	cfg := &Config{}
	cfg.Server.Host = "0.0.0.0"
	cfg.Server.Port = "8080"
	cfg.Database.Path = "./data/checker.db"

	cfg.Auth.Enabled = true
	cfg.Auth.Mode = "basic"
	cfg.Auth.Username = "admin"
	cfg.Auth.Password = "admin"
	cfg.Auth.ProtectAPI = true
	cfg.Auth.ProtectMetrics = false
	cfg.Auth.ProtectUI = false

	cfg.Checker.Interval = "6h"
	cfg.Checker.Timeout = "30s"
	cfg.Checker.ConcurrentChecks = 5
	cfg.Checker.RetryCount = 2

	cfg.Alerts.DomainExpiryWarningDays = 30
	cfg.Alerts.DomainExpiryCriticalDays = 7
	cfg.Alerts.SSLExpiryWarningDays = 14
	cfg.Alerts.SSLExpiryCriticalDays = 3

	cfg.Notifications.Webhook.OnCritical = true
	cfg.Notifications.Telegram.OnCritical = true

	cfg.Domains.SubdomainFallback = true
	cfg.Domains.FallbackDepth = 5
	cfg.Domains.DefaultCheckMode = "full"

	cfg.DNS.Servers = []string{}
	cfg.DNS.UseSystemDNS = true
	cfg.DNS.Timeout = "5s"

	cfg.Prometheus.Enabled = true
	cfg.Prometheus.Path = "/metrics"
	cfg.Logging.JSON = false
	return cfg
}

func Load(path string) (*Config, error) {
	cfg := Default()
	cfg.filePath = path

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			applyEnvOverrides(cfg)
			cfg.normalize()
			if err2 := cfg.Save(); err2 != nil {
				return cfg, fmt.Errorf("create default config: %w", err2)
			}
			return cfg, nil
		}
		return cfg, err
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return cfg, fmt.Errorf("parse config: %w", err)
	}
	cfg.filePath = path
	cfg.normalize()
	applyEnvOverrides(cfg)
	cfg.normalize()
	return cfg, nil
}

func (c *Config) normalize() {
	if c.Server.Host == "" {
		c.Server.Host = "0.0.0.0"
	}
	if c.Server.Port == "" {
		c.Server.Port = "8080"
	}
	if c.Database.Path == "" {
		c.Database.Path = "./data/checker.db"
	}

	mode := strings.ToLower(strings.TrimSpace(c.Auth.Mode))
	switch mode {
	case "", "basic", "api_key", "both":
		if mode == "" {
			mode = "basic"
		}
	default:
		mode = "basic"
	}
	c.Auth.Mode = mode

	if (c.Auth.Mode == "basic" || c.Auth.Mode == "both") && c.Auth.Username == "" {
		c.Auth.Username = "admin"
	}
	if (c.Auth.Mode == "basic" || c.Auth.Mode == "both") && c.Auth.Password == "" {
		c.Auth.Password = "admin"
	}

	if c.Checker.Interval == "" {
		c.Checker.Interval = "6h"
	}
	if c.Checker.Timeout == "" {
		c.Checker.Timeout = "30s"
	}
	if c.Checker.ConcurrentChecks <= 0 {
		c.Checker.ConcurrentChecks = 5
	}
	if c.Checker.RetryCount < 0 {
		c.Checker.RetryCount = 0
	}

	if c.Alerts.DomainExpiryWarningDays <= 0 {
		c.Alerts.DomainExpiryWarningDays = 30
	}
	if c.Alerts.DomainExpiryCriticalDays <= 0 {
		c.Alerts.DomainExpiryCriticalDays = 7
	}
	if c.Alerts.SSLExpiryWarningDays <= 0 {
		c.Alerts.SSLExpiryWarningDays = 14
	}
	if c.Alerts.SSLExpiryCriticalDays <= 0 {
		c.Alerts.SSLExpiryCriticalDays = 3
	}

	if c.Domains.FallbackDepth <= 0 {
		c.Domains.FallbackDepth = 5
	}

	cm := strings.ToLower(strings.TrimSpace(c.Domains.DefaultCheckMode))
	if cm != "full" && cm != "ssl_only" {
		cm = "full"
	}
	c.Domains.DefaultCheckMode = cm

	if c.DNS.Servers == nil {
		c.DNS.Servers = []string{}
	}
	if c.DNS.Timeout == "" {
		c.DNS.Timeout = "5s"
	}

	if c.Prometheus.Path == "" {
		c.Prometheus.Path = "/metrics"
	}
}

func (c *Config) Save() error {
	path := c.FilePath()
	if path == "" {
		return fmt.Errorf("no config file path set")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	// Take a consistent snapshot under read lock, then marshal outside the lock.
	snap := c.Snapshot()
	snap.normalize()
	data, err := yaml.Marshal(snap)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func (c *Config) FilePath() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.filePath
}

func (c *Config) SetFilePath(path string) {
	c.mu.Lock()
	c.filePath = path
	c.mu.Unlock()
}

// Snapshot returns a read-only deep copy of the config, safe to use without locks.
func (c *Config) Snapshot() *Config {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.cloneInternal()
}

// Clone returns a deep copy (public, no locking - use when you already hold the lock or own the value).
func (c *Config) Clone() *Config {
	return c.cloneInternal()
}

func (c *Config) cloneInternal() *Config {
	clone := *c
	clone.mu = sync.RWMutex{}
	// Deep copy slices
	if c.DNS.Servers != nil {
		clone.DNS.Servers = make([]string, len(c.DNS.Servers))
		copy(clone.DNS.Servers, c.DNS.Servers)
	}
	return &clone
}

// ApplyFrom replaces the config contents under write lock.
func (c *Config) ApplyFrom(in *Config) {
	if in == nil {
		return
	}
	next := in.Snapshot()

	c.mu.Lock()
	filePath := c.filePath
	c.Server = next.Server
	c.Database = next.Database
	c.Auth = next.Auth
	c.Checker = next.Checker
	c.Features = next.Features
	c.Alerts = next.Alerts
	c.Notifications = next.Notifications
	c.Domains = next.Domains
	c.DNS = next.DNS
	c.Prometheus = next.Prometheus
	c.Logging = next.Logging
	c.normalize()
	c.filePath = filePath
	c.mu.Unlock()
}

func applyEnvOverrides(cfg *Config) {
	if v := os.Getenv("SERVER_HOST"); v != "" {
		cfg.Server.Host = v
	}
	if v := os.Getenv("SERVER_PORT"); v != "" {
		cfg.Server.Port = v
	}
	if v := os.Getenv("DATABASE_PATH"); v != "" {
		cfg.Database.Path = v
	}
	if v := os.Getenv("CHECKER_INTERVAL"); v != "" {
		cfg.Checker.Interval = v
	}
	if v := os.Getenv("CHECKER_TIMEOUT"); v != "" {
		cfg.Checker.Timeout = v
	}
	if v := os.Getenv("CONCURRENT_CHECKS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.Checker.ConcurrentChecks = n
		}
	}

	if v := os.Getenv("AUTH_ENABLED"); v != "" {
		cfg.Auth.Enabled = parseBool(v)
	}
	if v := os.Getenv("AUTH_MODE"); v != "" {
		cfg.Auth.Mode = v
	}
	if v := os.Getenv("AUTH_USERNAME"); v != "" {
		cfg.Auth.Username = v
	}
	if v := os.Getenv("AUTH_PASSWORD"); v != "" {
		cfg.Auth.Password = v
	}
	if v := os.Getenv("AUTH_API_KEY"); v != "" {
		cfg.Auth.APIKey = v
	}
	if v := os.Getenv("AUTH_PROTECT_API"); v != "" {
		cfg.Auth.ProtectAPI = parseBool(v)
	}
	if v := os.Getenv("AUTH_PROTECT_METRICS"); v != "" {
		cfg.Auth.ProtectMetrics = parseBool(v)
	}
	if v := os.Getenv("AUTH_PROTECT_UI"); v != "" {
		cfg.Auth.ProtectUI = parseBool(v)
	}

	if v := os.Getenv("FEATURE_HTTP_CHECK"); v != "" {
		cfg.Features.HTTPCheck = parseBool(v)
	}
	if v := os.Getenv("FEATURE_CIPHER_CHECK"); v != "" {
		cfg.Features.CipherCheck = parseBool(v)
	}
	if v := os.Getenv("FEATURE_OCSP_CHECK"); v != "" {
		cfg.Features.OCSPCheck = parseBool(v)
	}
	if v := os.Getenv("FEATURE_CRL_CHECK"); v != "" {
		cfg.Features.CRLCheck = parseBool(v)
	}
	if v := os.Getenv("FEATURE_CAA_CHECK"); v != "" {
		cfg.Features.CAACheck = parseBool(v)
	}
	if v := os.Getenv("FEATURE_NOTIFICATIONS"); v != "" {
		cfg.Features.Notifications = parseBool(v)
	}
	if v := os.Getenv("FEATURE_CSV_EXPORT"); v != "" {
		cfg.Features.CSVExport = parseBool(v)
	}
	if v := os.Getenv("FEATURE_TIMELINE_VIEW"); v != "" {
		cfg.Features.TimelineView = parseBool(v)
	}
	if v := os.Getenv("FEATURE_DASHBOARD_TAG_FILTER"); v != "" {
		cfg.Features.DashboardTagFilter = parseBool(v)
	}
	if v := os.Getenv("FEATURE_STRUCTURED_LOGS"); v != "" {
		cfg.Features.StructuredLogs = parseBool(v)
	}

	if v := os.Getenv("WEBHOOK_ENABLED"); v != "" {
		cfg.Notifications.Webhook.Enabled = parseBool(v)
	}
	if v := os.Getenv("WEBHOOK_URL"); v != "" {
		cfg.Notifications.Webhook.URL = v
	}
	if v := os.Getenv("WEBHOOK_ON_CRITICAL"); v != "" {
		cfg.Notifications.Webhook.OnCritical = parseBool(v)
	}
	if v := os.Getenv("WEBHOOK_ON_WARNING"); v != "" {
		cfg.Notifications.Webhook.OnWarning = parseBool(v)
	}

	if v := os.Getenv("TELEGRAM_ENABLED"); v != "" {
		cfg.Notifications.Telegram.Enabled = parseBool(v)
	}
	if v := os.Getenv("TELEGRAM_BOT_TOKEN"); v != "" {
		cfg.Notifications.Telegram.BotToken = v
	}
	if v := os.Getenv("TELEGRAM_CHAT_ID"); v != "" {
		cfg.Notifications.Telegram.ChatID = v
	}
	if v := os.Getenv("TELEGRAM_ON_CRITICAL"); v != "" {
		cfg.Notifications.Telegram.OnCritical = parseBool(v)
	}
	if v := os.Getenv("TELEGRAM_ON_WARNING"); v != "" {
		cfg.Notifications.Telegram.OnWarning = parseBool(v)
	}

	if v := os.Getenv("SUBDOMAIN_FALLBACK"); v != "" {
		cfg.Domains.SubdomainFallback = parseBool(v)
	}
	if v := os.Getenv("SUBDOMAIN_FALLBACK_DEPTH"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.Domains.FallbackDepth = n
		}
	}
	if v := os.Getenv("DEFAULT_CHECK_MODE"); v != "" {
		cfg.Domains.DefaultCheckMode = v
	}

	// DNS env overrides
	if v := os.Getenv("DNS_SERVERS"); v != "" {
		servers := []string{}
		for _, s := range strings.Split(v, ",") {
			s = strings.TrimSpace(s)
			if s != "" {
				servers = append(servers, s)
			}
		}
		if len(servers) > 0 {
			cfg.DNS.Servers = servers
		}
	}
	if v := os.Getenv("DNS_USE_SYSTEM_DNS"); v != "" {
		cfg.DNS.UseSystemDNS = parseBool(v)
	}
	if v := os.Getenv("DNS_TIMEOUT"); v != "" {
		cfg.DNS.Timeout = v
	}

	if v := os.Getenv("PROMETHEUS_ENABLED"); v != "" {
		cfg.Prometheus.Enabled = parseBool(v)
	}
	if v := os.Getenv("PROMETHEUS_PATH"); v != "" {
		cfg.Prometheus.Path = v
	}
	if v := os.Getenv("LOG_JSON"); v != "" {
		cfg.Logging.JSON = parseBool(v)
	}
}

func parseBool(v string) bool {
	v = strings.ToLower(strings.TrimSpace(v))
	return v == "1" || v == "true" || v == "yes" || v == "on"
}

// ValidateCheckMode normalizes and validates a check mode string.
func ValidateCheckMode(mode string) string {
	m := strings.ToLower(strings.TrimSpace(mode))
	if m == "ssl_only" {
		return "ssl_only"
	}
	return "full"
}
