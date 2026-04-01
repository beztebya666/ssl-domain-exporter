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

type TLSConfig struct {
	Enabled  bool   `yaml:"enabled" json:"enabled"`
	CertFile string `yaml:"cert_file" json:"cert_file"`
	KeyFile  string `yaml:"key_file" json:"key_file"`
}

type ServerConfig struct {
	Host           string    `yaml:"host" json:"host"`
	Port           string    `yaml:"port" json:"port"`
	AllowedOrigins []string  `yaml:"allowed_origins" json:"allowed_origins"`
	TLS            TLSConfig `yaml:"tls" json:"tls"`
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
	SessionTTL     string `yaml:"session_ttl" json:"session_ttl"`
	CookieName     string `yaml:"cookie_name" json:"cookie_name"`
	CookieSecure   bool   `yaml:"cookie_secure" json:"cookie_secure"`
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

type StatusPolicyConfig struct {
	BadgeOnInvalidChain      bool `yaml:"badge_on_invalid_chain" json:"badge_on_invalid_chain"`
	BadgeOnSelfSigned        bool `yaml:"badge_on_self_signed" json:"badge_on_self_signed"`
	BadgeOnHTTPProbeError    bool `yaml:"badge_on_http_probe_error" json:"badge_on_http_probe_error"`
	BadgeOnHTTPClientError   bool `yaml:"badge_on_http_client_error" json:"badge_on_http_client_error"`
	BadgeOnCipherWarning     bool `yaml:"badge_on_cipher_warning" json:"badge_on_cipher_warning"`
	BadgeOnOCSPUnknown       bool `yaml:"badge_on_ocsp_unknown" json:"badge_on_ocsp_unknown"`
	BadgeOnCRLUnknown        bool `yaml:"badge_on_crl_unknown" json:"badge_on_crl_unknown"`
	BadgeOnCAAMissing        bool `yaml:"badge_on_caa_missing" json:"badge_on_caa_missing"`
	BadgeOnDomainLookupError bool `yaml:"badge_on_domain_lookup_error" json:"badge_on_domain_lookup_error"`
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
	Timeout  string         `yaml:"timeout" json:"timeout"`
	Webhook  WebhookConfig  `yaml:"webhook" json:"webhook"`
	Telegram TelegramConfig `yaml:"telegram" json:"telegram"`
	Email    EmailConfig    `yaml:"email" json:"email"`
}

type EmailConfig struct {
	Enabled            bool     `yaml:"enabled" json:"enabled"`
	Host               string   `yaml:"host" json:"host"`
	Port               int      `yaml:"port" json:"port"`
	Username           string   `yaml:"username" json:"username"`
	Password           string   `yaml:"password" json:"password"`
	From               string   `yaml:"from" json:"from"`
	To                 []string `yaml:"to" json:"to"`
	Mode               string   `yaml:"mode" json:"mode"`
	OnCritical         bool     `yaml:"on_critical" json:"on_critical"`
	OnWarning          bool     `yaml:"on_warning" json:"on_warning"`
	SubjectPrefix      string   `yaml:"subject_prefix" json:"subject_prefix"`
	InsecureSkipVerify bool     `yaml:"insecure_skip_verify" json:"insecure_skip_verify"`
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

type SecurityConfig struct {
	CSRFEnabled        bool   `yaml:"csrf_enabled" json:"csrf_enabled"`
	RateLimitEnabled   bool   `yaml:"rate_limit_enabled" json:"rate_limit_enabled"`
	LoginRequests      int    `yaml:"login_requests" json:"login_requests"`
	LoginWindow        string `yaml:"login_window" json:"login_window"`
	AdminWriteRequests int    `yaml:"admin_write_requests" json:"admin_write_requests"`
	AdminWindow        string `yaml:"admin_window" json:"admin_window"`
}

type PrometheusLabelsConfig struct {
	ExportTags     bool     `yaml:"export_tags" json:"export_tags"`
	ExportMetadata bool     `yaml:"export_metadata" json:"export_metadata"`
	MetadataKeys   []string `yaml:"metadata_keys" json:"metadata_keys"`
}

type PrometheusConfig struct {
	Enabled bool                   `yaml:"enabled" json:"enabled"`
	Path    string                 `yaml:"path" json:"path"`
	Labels  PrometheusLabelsConfig `yaml:"labels" json:"labels"`
}

type SyslogConfig struct {
	Enabled  bool   `yaml:"enabled" json:"enabled"`
	Network  string `yaml:"network" json:"network"`   // "tcp" or "udp"
	Address  string `yaml:"address" json:"address"`   // host:port, e.g. "syslog.example.com:514"
	Tag      string `yaml:"tag" json:"tag"`           // syslog tag, default "ssl-domain-exporter"
	Facility string `yaml:"facility" json:"facility"` // syslog facility, default "local0"
}

type LoggingConfig struct {
	JSON   bool         `yaml:"json" json:"json"`
	Syslog SyslogConfig `yaml:"syslog" json:"syslog"`
}

type MaintenanceConfig struct {
	BackupsDir             string `yaml:"backups_dir" json:"backups_dir"`
	CheckRetentionDays     int    `yaml:"check_retention_days" json:"check_retention_days"`
	AuditRetentionDays     int    `yaml:"audit_retention_days" json:"audit_retention_days"`
	RetentionSweepInterval string `yaml:"retention_sweep_interval" json:"retention_sweep_interval"`
}

type K8SConfig struct {
	Enabled            bool   `yaml:"enabled" json:"enabled"`
	APIServer          string `yaml:"api_server" json:"api_server"`
	Token              string `yaml:"token" json:"token"`
	TokenFile          string `yaml:"token_file" json:"token_file"`
	Namespace          string `yaml:"namespace" json:"namespace"`
	LabelSelector      string `yaml:"label_selector" json:"label_selector"`
	InsecureSkipVerify bool   `yaml:"insecure_skip_verify" json:"insecure_skip_verify"`
	CACertFile         string `yaml:"ca_cert_file" json:"ca_cert_file"`
}

type F5Config struct {
	Enabled            bool   `yaml:"enabled" json:"enabled"`
	Host               string `yaml:"host" json:"host"`
	Username           string `yaml:"username" json:"username"`
	Password           string `yaml:"password" json:"password"`
	InsecureSkipVerify bool   `yaml:"insecure_skip_verify" json:"insecure_skip_verify"`
	Partition          string `yaml:"partition" json:"partition"`
}

type Config struct {
	Server        ServerConfig        `yaml:"server" json:"server"`
	Database      DatabaseConfig      `yaml:"database" json:"database"`
	Auth          AuthConfig          `yaml:"auth" json:"auth"`
	Checker       CheckerConfig       `yaml:"checker" json:"checker"`
	Features      FeaturesConfig      `yaml:"features" json:"features"`
	Alerts        AlertsConfig        `yaml:"alerts" json:"alerts"`
	StatusPolicy  StatusPolicyConfig  `yaml:"status_policy" json:"status_policy"`
	Notifications NotificationsConfig `yaml:"notifications" json:"notifications"`
	Domains       DomainsConfig       `yaml:"domains" json:"domains"`
	DNS           DNSConfig           `yaml:"dns" json:"dns"`
	Security      SecurityConfig      `yaml:"security" json:"security"`
	Prometheus    PrometheusConfig    `yaml:"prometheus" json:"prometheus"`
	Maintenance   MaintenanceConfig   `yaml:"maintenance" json:"maintenance"`
	Logging       LoggingConfig       `yaml:"logging" json:"logging"`
	Kubernetes    K8SConfig           `yaml:"kubernetes" json:"kubernetes"`
	F5            F5Config            `yaml:"f5" json:"f5"`
	Warnings      []string            `yaml:"-" json:"warnings,omitempty"`

	mu       sync.RWMutex `yaml:"-" json:"-"`
	filePath string       `yaml:"-" json:"-"`
}

func Default() *Config {
	cfg := &Config{}
	cfg.Server.Host = "0.0.0.0"
	cfg.Server.Port = "8080"
	cfg.Server.AllowedOrigins = []string{}
	cfg.Database.Path = "./data/checker.db"

	cfg.Auth.Enabled = true
	cfg.Auth.Mode = "basic"
	cfg.Auth.Username = "admin"
	cfg.Auth.Password = "admin"
	cfg.Auth.ProtectAPI = true
	cfg.Auth.ProtectMetrics = false
	cfg.Auth.ProtectUI = false
	cfg.Auth.SessionTTL = "24h"
	cfg.Auth.CookieName = "ssl_domain_exporter_session"
	cfg.Auth.CookieSecure = false

	cfg.Checker.Interval = "6h"
	cfg.Checker.Timeout = "30s"
	cfg.Checker.ConcurrentChecks = 5
	cfg.Checker.RetryCount = 2

	cfg.Alerts.DomainExpiryWarningDays = 30
	cfg.Alerts.DomainExpiryCriticalDays = 7
	cfg.Alerts.SSLExpiryWarningDays = 14
	cfg.Alerts.SSLExpiryCriticalDays = 3

	cfg.StatusPolicy.BadgeOnInvalidChain = true
	cfg.StatusPolicy.BadgeOnSelfSigned = true
	cfg.StatusPolicy.BadgeOnHTTPProbeError = true
	cfg.StatusPolicy.BadgeOnHTTPClientError = true
	cfg.StatusPolicy.BadgeOnCipherWarning = true
	cfg.StatusPolicy.BadgeOnOCSPUnknown = true
	cfg.StatusPolicy.BadgeOnCRLUnknown = true
	cfg.StatusPolicy.BadgeOnCAAMissing = true
	cfg.StatusPolicy.BadgeOnDomainLookupError = true

	cfg.Notifications.Webhook.OnCritical = true
	cfg.Notifications.Telegram.OnCritical = true
	cfg.Notifications.Timeout = "15s"
	cfg.Notifications.Email.Port = 587
	cfg.Notifications.Email.Mode = "starttls"
	cfg.Notifications.Email.SubjectPrefix = "[SSL Domain Exporter]"
	cfg.Notifications.Email.OnCritical = true

	cfg.Domains.SubdomainFallback = true
	cfg.Domains.FallbackDepth = 5
	cfg.Domains.DefaultCheckMode = "full"

	cfg.DNS.Servers = []string{}
	cfg.DNS.UseSystemDNS = true
	cfg.DNS.Timeout = "5s"

	cfg.Security.CSRFEnabled = true
	cfg.Security.RateLimitEnabled = true
	cfg.Security.LoginRequests = 10
	cfg.Security.LoginWindow = "5m"
	cfg.Security.AdminWriteRequests = 300
	cfg.Security.AdminWindow = "1m"

	cfg.Prometheus.Enabled = true
	cfg.Prometheus.Path = "/metrics"
	cfg.Prometheus.Labels.ExportTags = true
	cfg.Prometheus.Labels.ExportMetadata = true
	cfg.Prometheus.Labels.MetadataKeys = []string{}

	cfg.Maintenance.BackupsDir = filepath.Join(filepath.Dir(cfg.Database.Path), "backups")
	cfg.Maintenance.CheckRetentionDays = 0
	cfg.Maintenance.AuditRetentionDays = 0
	cfg.Maintenance.RetentionSweepInterval = "24h"

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
	if c.Server.AllowedOrigins == nil {
		c.Server.AllowedOrigins = []string{}
	}
	c.Server.AllowedOrigins = normalizeStringList(c.Server.AllowedOrigins)
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
	if c.Auth.SessionTTL == "" {
		c.Auth.SessionTTL = "24h"
	}
	if c.Auth.CookieName == "" {
		c.Auth.CookieName = "ssl_domain_exporter_session"
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
	if c.Security.LoginRequests <= 0 {
		c.Security.LoginRequests = 10
	}
	if c.Security.LoginWindow == "" {
		c.Security.LoginWindow = "5m"
	}
	if c.Security.AdminWriteRequests <= 0 {
		c.Security.AdminWriteRequests = 300
	}
	if c.Security.AdminWindow == "" {
		c.Security.AdminWindow = "1m"
	}

	if c.Notifications.Email.Port <= 0 {
		c.Notifications.Email.Port = 587
	}
	if strings.TrimSpace(c.Notifications.Timeout) == "" {
		c.Notifications.Timeout = "15s"
	}
	emailMode := strings.ToLower(strings.TrimSpace(c.Notifications.Email.Mode))
	switch emailMode {
	case "", "starttls", "tls", "none":
		if emailMode == "" {
			emailMode = "starttls"
		}
	default:
		emailMode = "starttls"
	}
	c.Notifications.Email.Mode = emailMode
	if c.Notifications.Email.To == nil {
		c.Notifications.Email.To = []string{}
	}
	if c.Notifications.Email.SubjectPrefix == "" {
		c.Notifications.Email.SubjectPrefix = "[SSL Domain Exporter]"
	}

	if c.Prometheus.Path == "" {
		c.Prometheus.Path = "/metrics"
	}
	if c.Prometheus.Labels.MetadataKeys == nil {
		c.Prometheus.Labels.MetadataKeys = []string{}
	}
	c.Prometheus.Labels.MetadataKeys = normalizeStringList(c.Prometheus.Labels.MetadataKeys)

	if c.Maintenance.BackupsDir == "" {
		c.Maintenance.BackupsDir = filepath.Join(filepath.Dir(c.Database.Path), "backups")
	}
	if c.Maintenance.AuditRetentionDays < 0 {
		c.Maintenance.AuditRetentionDays = 0
	}
	if c.Maintenance.RetentionSweepInterval == "" {
		c.Maintenance.RetentionSweepInterval = "24h"
	}

	// Syslog defaults
	network := strings.ToLower(strings.TrimSpace(c.Logging.Syslog.Network))
	if network != "tcp" && network != "udp" {
		network = "udp"
	}
	c.Logging.Syslog.Network = network
	if c.Logging.Syslog.Tag == "" {
		c.Logging.Syslog.Tag = "ssl-domain-exporter"
	}
	facility := strings.ToLower(strings.TrimSpace(c.Logging.Syslog.Facility))
	if facility == "" {
		facility = "local0"
	}
	c.Logging.Syslog.Facility = facility
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
	return os.WriteFile(path, data, 0o600)
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
	clone := &Config{
		Server:        c.Server,
		Database:      c.Database,
		Auth:          c.Auth,
		Checker:       c.Checker,
		Features:      c.Features,
		Alerts:        c.Alerts,
		StatusPolicy:  c.StatusPolicy,
		Notifications: c.Notifications,
		Domains:       c.Domains,
		DNS:           c.DNS,
		Security:      c.Security,
		Prometheus:    c.Prometheus,
		Maintenance:   c.Maintenance,
		Logging:       c.Logging,
		Kubernetes:    c.Kubernetes,
		F5:            c.F5,
		filePath:      c.filePath,
	}
	// Deep copy slices
	if c.DNS.Servers != nil {
		clone.DNS.Servers = append([]string(nil), c.DNS.Servers...)
	}
	if c.Server.AllowedOrigins != nil {
		clone.Server.AllowedOrigins = append([]string(nil), c.Server.AllowedOrigins...)
	}
	if c.Notifications.Email.To != nil {
		clone.Notifications.Email.To = append([]string(nil), c.Notifications.Email.To...)
	}
	if c.Prometheus.Labels.MetadataKeys != nil {
		clone.Prometheus.Labels.MetadataKeys = append([]string(nil), c.Prometheus.Labels.MetadataKeys...)
	}
	if c.Warnings != nil {
		clone.Warnings = append([]string(nil), c.Warnings...)
	}
	return clone
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
	c.StatusPolicy = next.StatusPolicy
	c.Notifications = next.Notifications
	c.Domains = next.Domains
	c.DNS = next.DNS
	c.Security = next.Security
	c.Prometheus = next.Prometheus
	c.Maintenance = next.Maintenance
	c.Logging = next.Logging
	c.Kubernetes = next.Kubernetes
	c.F5 = next.F5
	c.Warnings = nil
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
	if v := os.Getenv("SERVER_ALLOWED_ORIGINS"); v != "" {
		cfg.Server.AllowedOrigins = normalizeCommaList(v)
	}
	if v := os.Getenv("SERVER_TLS_ENABLED"); v != "" {
		cfg.Server.TLS.Enabled = ParseBool(v)
	}
	if v := os.Getenv("SERVER_TLS_CERT_FILE"); v != "" {
		cfg.Server.TLS.CertFile = v
	}
	if v := os.Getenv("SERVER_TLS_KEY_FILE"); v != "" {
		cfg.Server.TLS.KeyFile = v
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
		cfg.Auth.Enabled = ParseBool(v)
	}
	if v := os.Getenv("AUTH_MODE"); v != "" {
		cfg.Auth.Mode = v
	}
	if v := os.Getenv("AUTH_USERNAME"); v != "" {
		cfg.Auth.Username = v
	}
	if v := os.Getenv("AUTH_PASSWORD"); v != "" {
		cfg.Auth.Password = v
	} else if v, ok := lookupEnvFile("AUTH_PASSWORD_FILE"); ok {
		cfg.Auth.Password = v
	}
	if v := os.Getenv("AUTH_API_KEY"); v != "" {
		cfg.Auth.APIKey = v
	} else if v, ok := lookupEnvFile("AUTH_API_KEY_FILE"); ok {
		cfg.Auth.APIKey = v
	}
	if v := os.Getenv("AUTH_PROTECT_API"); v != "" {
		cfg.Auth.ProtectAPI = ParseBool(v)
	}
	if v := os.Getenv("AUTH_PROTECT_METRICS"); v != "" {
		cfg.Auth.ProtectMetrics = ParseBool(v)
	}
	if v := os.Getenv("AUTH_PROTECT_UI"); v != "" {
		cfg.Auth.ProtectUI = ParseBool(v)
	}
	if v := os.Getenv("AUTH_SESSION_TTL"); v != "" {
		cfg.Auth.SessionTTL = v
	}
	if v := os.Getenv("AUTH_COOKIE_NAME"); v != "" {
		cfg.Auth.CookieName = v
	}
	if v := os.Getenv("AUTH_COOKIE_SECURE"); v != "" {
		cfg.Auth.CookieSecure = ParseBool(v)
	}

	if v := os.Getenv("FEATURE_HTTP_CHECK"); v != "" {
		cfg.Features.HTTPCheck = ParseBool(v)
	}
	if v := os.Getenv("FEATURE_CIPHER_CHECK"); v != "" {
		cfg.Features.CipherCheck = ParseBool(v)
	}
	if v := os.Getenv("FEATURE_OCSP_CHECK"); v != "" {
		cfg.Features.OCSPCheck = ParseBool(v)
	}
	if v := os.Getenv("FEATURE_CRL_CHECK"); v != "" {
		cfg.Features.CRLCheck = ParseBool(v)
	}
	if v := os.Getenv("FEATURE_CAA_CHECK"); v != "" {
		cfg.Features.CAACheck = ParseBool(v)
	}
	if v := os.Getenv("FEATURE_NOTIFICATIONS"); v != "" {
		cfg.Features.Notifications = ParseBool(v)
	}
	if v := os.Getenv("FEATURE_CSV_EXPORT"); v != "" {
		cfg.Features.CSVExport = ParseBool(v)
	}
	if v := os.Getenv("FEATURE_TIMELINE_VIEW"); v != "" {
		cfg.Features.TimelineView = ParseBool(v)
	}
	if v := os.Getenv("FEATURE_DASHBOARD_TAG_FILTER"); v != "" {
		cfg.Features.DashboardTagFilter = ParseBool(v)
	}
	if v := os.Getenv("FEATURE_STRUCTURED_LOGS"); v != "" {
		cfg.Features.StructuredLogs = ParseBool(v)
	}

	// Syslog env overrides
	if v := os.Getenv("SYSLOG_ENABLED"); v != "" {
		cfg.Logging.Syslog.Enabled = ParseBool(v)
	}
	if v := os.Getenv("SYSLOG_NETWORK"); v != "" {
		cfg.Logging.Syslog.Network = v
	}
	if v := os.Getenv("SYSLOG_ADDRESS"); v != "" {
		cfg.Logging.Syslog.Address = v
	}
	if v := os.Getenv("SYSLOG_TAG"); v != "" {
		cfg.Logging.Syslog.Tag = v
	}
	if v := os.Getenv("SYSLOG_FACILITY"); v != "" {
		cfg.Logging.Syslog.Facility = v
	}

	// K8S env overrides
	if v := os.Getenv("K8S_ENABLED"); v != "" {
		cfg.Kubernetes.Enabled = ParseBool(v)
	}
	if v := os.Getenv("K8S_API_SERVER"); v != "" {
		cfg.Kubernetes.APIServer = v
	}
	if v := os.Getenv("K8S_TOKEN"); v != "" {
		cfg.Kubernetes.Token = v
	} else if v, ok := lookupEnvFile("K8S_TOKEN_FILE"); ok {
		cfg.Kubernetes.Token = v
	}
	if v := os.Getenv("K8S_TOKEN_FILE"); v != "" {
		cfg.Kubernetes.TokenFile = v
	}
	if v := os.Getenv("K8S_NAMESPACE"); v != "" {
		cfg.Kubernetes.Namespace = v
	}
	if v := os.Getenv("K8S_LABEL_SELECTOR"); v != "" {
		cfg.Kubernetes.LabelSelector = v
	}
	if v := os.Getenv("K8S_CA_CERT_FILE"); v != "" {
		cfg.Kubernetes.CACertFile = v
	}
	if v := os.Getenv("K8S_INSECURE_SKIP_VERIFY"); v != "" {
		cfg.Kubernetes.InsecureSkipVerify = ParseBool(v)
	}

	// F5 env overrides
	if v := os.Getenv("F5_ENABLED"); v != "" {
		cfg.F5.Enabled = ParseBool(v)
	}
	if v := os.Getenv("F5_HOST"); v != "" {
		cfg.F5.Host = v
	}
	if v := os.Getenv("F5_USERNAME"); v != "" {
		cfg.F5.Username = v
	}
	if v := os.Getenv("F5_PASSWORD"); v != "" {
		cfg.F5.Password = v
	} else if v, ok := lookupEnvFile("F5_PASSWORD_FILE"); ok {
		cfg.F5.Password = v
	}
	if v := os.Getenv("F5_PARTITION"); v != "" {
		cfg.F5.Partition = v
	}
	if v := os.Getenv("F5_INSECURE_SKIP_VERIFY"); v != "" {
		cfg.F5.InsecureSkipVerify = ParseBool(v)
	}

	if v := os.Getenv("WEBHOOK_ENABLED"); v != "" {
		cfg.Notifications.Webhook.Enabled = ParseBool(v)
	}
	if v := os.Getenv("WEBHOOK_URL"); v != "" {
		cfg.Notifications.Webhook.URL = v
	} else if v, ok := lookupEnvFile("WEBHOOK_URL_FILE"); ok {
		cfg.Notifications.Webhook.URL = v
	}
	if v := os.Getenv("WEBHOOK_ON_CRITICAL"); v != "" {
		cfg.Notifications.Webhook.OnCritical = ParseBool(v)
	}
	if v := os.Getenv("WEBHOOK_ON_WARNING"); v != "" {
		cfg.Notifications.Webhook.OnWarning = ParseBool(v)
	}
	if v := os.Getenv("NOTIFICATIONS_TIMEOUT"); v != "" {
		cfg.Notifications.Timeout = v
	}

	if v := os.Getenv("TELEGRAM_ENABLED"); v != "" {
		cfg.Notifications.Telegram.Enabled = ParseBool(v)
	}
	if v := os.Getenv("TELEGRAM_BOT_TOKEN"); v != "" {
		cfg.Notifications.Telegram.BotToken = v
	} else if v, ok := lookupEnvFile("TELEGRAM_BOT_TOKEN_FILE"); ok {
		cfg.Notifications.Telegram.BotToken = v
	}
	if v := os.Getenv("TELEGRAM_CHAT_ID"); v != "" {
		cfg.Notifications.Telegram.ChatID = v
	}
	if v := os.Getenv("TELEGRAM_ON_CRITICAL"); v != "" {
		cfg.Notifications.Telegram.OnCritical = ParseBool(v)
	}
	if v := os.Getenv("TELEGRAM_ON_WARNING"); v != "" {
		cfg.Notifications.Telegram.OnWarning = ParseBool(v)
	}
	if v := os.Getenv("EMAIL_ENABLED"); v != "" {
		cfg.Notifications.Email.Enabled = ParseBool(v)
	}
	if v := os.Getenv("EMAIL_HOST"); v != "" {
		cfg.Notifications.Email.Host = v
	}
	if v := os.Getenv("EMAIL_PORT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.Notifications.Email.Port = n
		}
	}
	if v := os.Getenv("EMAIL_USERNAME"); v != "" {
		cfg.Notifications.Email.Username = v
	}
	if v := os.Getenv("EMAIL_PASSWORD"); v != "" {
		cfg.Notifications.Email.Password = v
	} else if v, ok := lookupEnvFile("EMAIL_PASSWORD_FILE"); ok {
		cfg.Notifications.Email.Password = v
	}
	if v := os.Getenv("EMAIL_FROM"); v != "" {
		cfg.Notifications.Email.From = v
	}
	if v := os.Getenv("EMAIL_TO"); v != "" {
		recipients := []string{}
		for _, s := range strings.Split(v, ",") {
			s = strings.TrimSpace(s)
			if s != "" {
				recipients = append(recipients, s)
			}
		}
		cfg.Notifications.Email.To = recipients
	}
	if v := os.Getenv("EMAIL_MODE"); v != "" {
		cfg.Notifications.Email.Mode = v
	}
	if v := os.Getenv("EMAIL_ON_CRITICAL"); v != "" {
		cfg.Notifications.Email.OnCritical = ParseBool(v)
	}
	if v := os.Getenv("EMAIL_ON_WARNING"); v != "" {
		cfg.Notifications.Email.OnWarning = ParseBool(v)
	}
	if v := os.Getenv("EMAIL_SUBJECT_PREFIX"); v != "" {
		cfg.Notifications.Email.SubjectPrefix = v
	}
	if v := os.Getenv("EMAIL_INSECURE_SKIP_VERIFY"); v != "" {
		cfg.Notifications.Email.InsecureSkipVerify = ParseBool(v)
	}

	if v := os.Getenv("SUBDOMAIN_FALLBACK"); v != "" {
		cfg.Domains.SubdomainFallback = ParseBool(v)
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
		cfg.DNS.UseSystemDNS = ParseBool(v)
	}
	if v := os.Getenv("DNS_TIMEOUT"); v != "" {
		cfg.DNS.Timeout = v
	}

	if v := os.Getenv("SECURITY_CSRF_ENABLED"); v != "" {
		cfg.Security.CSRFEnabled = ParseBool(v)
	}
	if v := os.Getenv("SECURITY_RATE_LIMIT_ENABLED"); v != "" {
		cfg.Security.RateLimitEnabled = ParseBool(v)
	}
	if v := os.Getenv("SECURITY_LOGIN_REQUESTS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.Security.LoginRequests = n
		}
	}
	if v := os.Getenv("SECURITY_LOGIN_WINDOW"); v != "" {
		cfg.Security.LoginWindow = v
	}
	if v := os.Getenv("SECURITY_ADMIN_WRITE_REQUESTS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.Security.AdminWriteRequests = n
		}
	}
	if v := os.Getenv("SECURITY_ADMIN_WINDOW"); v != "" {
		cfg.Security.AdminWindow = v
	}

	if v := os.Getenv("PROMETHEUS_ENABLED"); v != "" {
		cfg.Prometheus.Enabled = ParseBool(v)
	}
	if v := os.Getenv("PROMETHEUS_PATH"); v != "" {
		cfg.Prometheus.Path = v
	}
	if v := os.Getenv("PROMETHEUS_EXPORT_TAGS"); v != "" {
		cfg.Prometheus.Labels.ExportTags = ParseBool(v)
	}
	if v := os.Getenv("PROMETHEUS_EXPORT_METADATA"); v != "" {
		cfg.Prometheus.Labels.ExportMetadata = ParseBool(v)
	}
	if v := os.Getenv("PROMETHEUS_METADATA_KEYS"); v != "" {
		cfg.Prometheus.Labels.MetadataKeys = normalizeCommaList(v)
	}
	if v := os.Getenv("MAINTENANCE_BACKUPS_DIR"); v != "" {
		cfg.Maintenance.BackupsDir = v
	}
	if v := os.Getenv("MAINTENANCE_CHECK_RETENTION_DAYS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.Maintenance.CheckRetentionDays = n
		}
	}
	if v := os.Getenv("MAINTENANCE_AUDIT_RETENTION_DAYS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.Maintenance.AuditRetentionDays = n
		}
	}
	if v := os.Getenv("MAINTENANCE_RETENTION_SWEEP_INTERVAL"); v != "" {
		cfg.Maintenance.RetentionSweepInterval = v
	}
	if v := os.Getenv("LOG_JSON"); v != "" {
		cfg.Logging.JSON = ParseBool(v)
	}
}

func ParseBool(v string) bool {
	v = strings.ToLower(strings.TrimSpace(v))
	return v == "1" || v == "true" || v == "yes" || v == "on"
}

func (a AuthConfig) AnonymousReadOnlyEnabled() bool {
	return !a.ProtectUI || !a.ProtectAPI
}

// ValidateCheckMode normalizes and validates a check mode string.
func ValidateCheckMode(mode string) string {
	m := strings.ToLower(strings.TrimSpace(mode))
	if m == "ssl_only" {
		return "ssl_only"
	}
	return "full"
}

func lookupEnvFile(name string) (string, bool) {
	path := strings.TrimSpace(os.Getenv(name))
	if path == "" {
		return "", false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	return strings.TrimSpace(string(data)), true
}

func normalizeCommaList(raw string) []string {
	parts := strings.Split(raw, ",")
	return normalizeStringList(parts)
}

func normalizeStringList(values []string) []string {
	if len(values) == 0 {
		return []string{}
	}
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, value)
	}
	return out
}
