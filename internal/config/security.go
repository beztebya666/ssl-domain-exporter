package config

import (
	"fmt"
	"net"
	"net/mail"
	"net/url"
	"strconv"
	"strings"
	"time"
)

//nolint:gosec // Marker constant for masked secrets in API/UI responses; not a credential.
const RedactedSecret = "__REDACTED__"

type ValidationError struct {
	Issues []string `json:"issues"`
}

func (e *ValidationError) Error() string {
	if e == nil || len(e.Issues) == 0 {
		return "invalid configuration"
	}
	return strings.Join(e.Issues, "; ")
}

func (c *Config) RedactedSnapshot() *Config {
	snap := c.Snapshot()
	snap.RedactSecrets()
	snap.Warnings = snap.InsecureWarnings()
	return snap
}

func (c *Config) RedactSecrets() {
	if c == nil {
		return
	}
	c.Auth.Password = redactSecretValue(c.Auth.Password)
	c.Auth.APIKey = redactSecretValue(c.Auth.APIKey)
	c.Notifications.Webhook.URL = redactSecretValue(c.Notifications.Webhook.URL)
	c.Notifications.Telegram.BotToken = redactSecretValue(c.Notifications.Telegram.BotToken)
	c.Notifications.Email.Password = redactSecretValue(c.Notifications.Email.Password)
}

func (c *Config) RestoreRedactedSecrets(current *Config) {
	if c == nil || current == nil {
		return
	}
	if c.Auth.Password == RedactedSecret {
		c.Auth.Password = current.Auth.Password
	}
	if c.Auth.APIKey == RedactedSecret {
		c.Auth.APIKey = current.Auth.APIKey
	}
	if c.Notifications.Webhook.URL == RedactedSecret {
		c.Notifications.Webhook.URL = current.Notifications.Webhook.URL
	}
	if c.Notifications.Telegram.BotToken == RedactedSecret {
		c.Notifications.Telegram.BotToken = current.Notifications.Telegram.BotToken
	}
	if c.Notifications.Email.Password == RedactedSecret {
		c.Notifications.Email.Password = current.Notifications.Email.Password
	}
}

func (c *Config) InsecureWarnings() []string {
	if c == nil {
		return nil
	}

	warnings := make([]string, 0, 3)
	mode := strings.ToLower(strings.TrimSpace(c.Auth.Mode))
	if c.Auth.Enabled && (mode == "basic" || mode == "both") &&
		strings.TrimSpace(c.Auth.Username) == "admin" &&
		c.Auth.Password == "admin" {
		warnings = append(warnings, "Default legacy admin credentials admin/admin are still active.")
	}
	if c.Auth.Enabled && !c.Auth.CookieSecure {
		warnings = append(warnings, "Session cookies are not marked secure; enable auth.cookie_secure when serving the app over HTTPS.")
	}
	if c.Auth.Enabled && c.Auth.AnonymousReadOnlyEnabled() {
		warnings = append(warnings, "Anonymous read-only access is enabled because UI routes or read-only API routes are not fully protected.")
	}
	return warnings
}

func (c *Config) Validate() error {
	if c == nil {
		return &ValidationError{Issues: []string{"configuration payload is required"}}
	}

	issues := make([]string, 0, 16)

	validateStringPort(&issues, "server.port", c.Server.Port)
	for _, origin := range c.Server.AllowedOrigins {
		if err := validateAllowedOrigin(origin); err != nil {
			issues = append(issues, fmt.Sprintf("server.allowed_origins contains invalid entry %q: %v", origin, err))
		}
	}

	authMode := strings.ToLower(strings.TrimSpace(c.Auth.Mode))
	switch authMode {
	case "basic", "api_key", "both":
	default:
		issues = append(issues, "auth.mode must be one of basic, api_key, both")
	}
	if c.Auth.Enabled && (authMode == "basic" || authMode == "both") {
		if strings.TrimSpace(c.Auth.Username) == "" {
			issues = append(issues, "auth.username is required when basic authentication is enabled")
		}
		if strings.TrimSpace(c.Auth.Password) == "" {
			issues = append(issues, "auth.password is required when basic authentication is enabled")
		} else if isLiteralRedactedSecret(c.Auth.Password) {
			issues = append(issues, "auth.password cannot be set to the reserved redacted placeholder")
		}
	}
	if c.Auth.Enabled && (authMode == "api_key" || authMode == "both") {
		if strings.TrimSpace(c.Auth.APIKey) == "" {
			issues = append(issues, "auth.api_key is required when API key authentication is enabled")
		} else if isLiteralRedactedSecret(c.Auth.APIKey) {
			issues = append(issues, "auth.api_key cannot be set to the reserved redacted placeholder")
		}
	}
	validateRequiredDuration(&issues, "auth.session_ttl", c.Auth.SessionTTL)
	if strings.TrimSpace(c.Auth.CookieName) == "" {
		issues = append(issues, "auth.cookie_name is required")
	}

	validateRequiredDuration(&issues, "checker.interval", c.Checker.Interval)
	validateRequiredDuration(&issues, "checker.timeout", c.Checker.Timeout)
	if c.Checker.ConcurrentChecks <= 0 {
		issues = append(issues, "checker.concurrent_checks must be greater than 0")
	}
	if c.Checker.RetryCount < 0 {
		issues = append(issues, "checker.retry_count cannot be negative")
	}

	validatePositiveInt(&issues, "alerts.domain_expiry_warning_days", c.Alerts.DomainExpiryWarningDays)
	validatePositiveInt(&issues, "alerts.domain_expiry_critical_days", c.Alerts.DomainExpiryCriticalDays)
	validatePositiveInt(&issues, "alerts.ssl_expiry_warning_days", c.Alerts.SSLExpiryWarningDays)
	validatePositiveInt(&issues, "alerts.ssl_expiry_critical_days", c.Alerts.SSLExpiryCriticalDays)
	if c.Alerts.DomainExpiryCriticalDays > c.Alerts.DomainExpiryWarningDays {
		issues = append(issues, "alerts.domain_expiry_critical_days cannot exceed alerts.domain_expiry_warning_days")
	}
	if c.Alerts.SSLExpiryCriticalDays > c.Alerts.SSLExpiryWarningDays {
		issues = append(issues, "alerts.ssl_expiry_critical_days cannot exceed alerts.ssl_expiry_warning_days")
	}

	if strings.ToLower(strings.TrimSpace(c.Domains.DefaultCheckMode)) != ValidateCheckMode(c.Domains.DefaultCheckMode) {
		issues = append(issues, "domains.default_check_mode must be full or ssl_only")
	}
	if c.Domains.FallbackDepth <= 0 {
		issues = append(issues, "domains.fallback_depth must be greater than 0")
	}

	validateRequiredDuration(&issues, "dns.timeout", c.DNS.Timeout)
	for _, server := range c.DNS.Servers {
		if err := validateDNSServer(server); err != nil {
			issues = append(issues, fmt.Sprintf("dns.servers contains invalid entry %q: %v", server, err))
		}
	}

	validateRequiredDuration(&issues, "security.login_window", c.Security.LoginWindow)
	validateRequiredDuration(&issues, "security.admin_window", c.Security.AdminWindow)
	if c.Security.LoginRequests <= 0 {
		issues = append(issues, "security.login_requests must be greater than 0")
	}
	if c.Security.AdminWriteRequests <= 0 {
		issues = append(issues, "security.admin_write_requests must be greater than 0")
	}

	if strings.TrimSpace(c.Prometheus.Path) == "" {
		issues = append(issues, "prometheus.path is required")
	} else if !strings.HasPrefix(strings.TrimSpace(c.Prometheus.Path), "/") {
		issues = append(issues, "prometheus.path must start with /")
	}
	for _, key := range c.Prometheus.Labels.MetadataKeys {
		if strings.TrimSpace(key) == "" {
			issues = append(issues, "prometheus.labels.metadata_keys cannot contain empty values")
			break
		}
	}

	if strings.TrimSpace(c.Maintenance.BackupsDir) == "" {
		issues = append(issues, "maintenance.backups_dir is required")
	}
	validateRequiredDuration(&issues, "maintenance.retention_sweep_interval", c.Maintenance.RetentionSweepInterval)
	if c.Maintenance.CheckRetentionDays < 0 {
		issues = append(issues, "maintenance.check_retention_days cannot be negative")
	}
	if c.Maintenance.AuditRetentionDays < 0 {
		issues = append(issues, "maintenance.audit_retention_days cannot be negative")
	}

	validateNotificationSettings(&issues, c)

	if len(issues) > 0 {
		return &ValidationError{Issues: issues}
	}
	return nil
}

func (c *Config) ValidateNotificationsOnly() error {
	if c == nil {
		return &ValidationError{Issues: []string{"configuration payload is required"}}
	}
	issues := make([]string, 0, 8)
	validateNotificationSettings(&issues, c)
	if len(issues) > 0 {
		return &ValidationError{Issues: issues}
	}
	return nil
}

func redactSecretValue(value string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	return RedactedSecret
}

func validateRequiredDuration(issues *[]string, field, raw string) {
	if strings.TrimSpace(raw) == "" {
		*issues = append(*issues, fmt.Sprintf("%s is required", field))
		return
	}
	d, err := time.ParseDuration(strings.TrimSpace(raw))
	if err != nil {
		*issues = append(*issues, fmt.Sprintf("%s must be a valid duration", field))
		return
	}
	if d <= 0 {
		*issues = append(*issues, fmt.Sprintf("%s must be greater than 0", field))
	}
}

func validatePositiveInt(issues *[]string, field string, value int) {
	if value <= 0 {
		*issues = append(*issues, fmt.Sprintf("%s must be greater than 0", field))
	}
}

func validateStringPort(issues *[]string, field, raw string) {
	value := strings.TrimSpace(raw)
	if value == "" {
		*issues = append(*issues, fmt.Sprintf("%s is required", field))
		return
	}
	port, err := strconv.Atoi(value)
	if err != nil || port < 1 || port > 65535 {
		*issues = append(*issues, fmt.Sprintf("%s must be a valid TCP port between 1 and 65535", field))
	}
}

func validateWebhookConfig(issues *[]string, cfg WebhookConfig) {
	value := strings.TrimSpace(cfg.URL)
	if cfg.Enabled && value == "" {
		*issues = append(*issues, "notifications.webhook.url is required when the webhook channel is enabled")
		return
	}
	if value == "" {
		return
	}
	if isLiteralRedactedSecret(value) {
		*issues = append(*issues, "notifications.webhook.url cannot be set to the reserved redacted placeholder")
		return
	}
	parsed, err := url.ParseRequestURI(value)
	if err != nil {
		*issues = append(*issues, "notifications.webhook.url must be a valid URL")
		return
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		*issues = append(*issues, "notifications.webhook.url must use http or https")
	}
}

func validateTelegramConfig(issues *[]string, cfg TelegramConfig) {
	if !cfg.Enabled {
		if isLiteralRedactedSecret(cfg.BotToken) {
			*issues = append(*issues, "notifications.telegram.bot_token cannot be set to the reserved redacted placeholder")
		}
		return
	}
	if strings.TrimSpace(cfg.BotToken) == "" {
		*issues = append(*issues, "notifications.telegram.bot_token is required when Telegram notifications are enabled")
	} else if isLiteralRedactedSecret(cfg.BotToken) {
		*issues = append(*issues, "notifications.telegram.bot_token cannot be set to the reserved redacted placeholder")
	}
	if strings.TrimSpace(cfg.ChatID) == "" {
		*issues = append(*issues, "notifications.telegram.chat_id is required when Telegram notifications are enabled")
	}
}

func validateEmailConfig(issues *[]string, cfg EmailConfig) {
	if cfg.Enabled {
		if cfg.Port < 1 || cfg.Port > 65535 {
			*issues = append(*issues, "notifications.email.port must be a valid TCP port between 1 and 65535")
		}
	} else if cfg.Port != 0 && (cfg.Port < 1 || cfg.Port > 65535) {
		*issues = append(*issues, "notifications.email.port must be a valid TCP port between 1 and 65535")
	}

	mode := strings.ToLower(strings.TrimSpace(cfg.Mode))
	if cfg.Enabled || mode != "" {
		switch mode {
		case "starttls", "tls", "none":
		default:
			*issues = append(*issues, "notifications.email.mode must be one of starttls, tls, none")
		}
	}

	if !cfg.Enabled {
		if isLiteralRedactedSecret(cfg.Password) {
			*issues = append(*issues, "notifications.email.password cannot be set to the reserved redacted placeholder")
		}
		validateOptionalEmailAddresses(issues, cfg)
		return
	}

	if strings.TrimSpace(cfg.Host) == "" {
		*issues = append(*issues, "notifications.email.host is required when email notifications are enabled")
	}
	if strings.TrimSpace(cfg.From) == "" {
		*issues = append(*issues, "notifications.email.from is required when email notifications are enabled")
	}
	if len(cfg.To) == 0 {
		*issues = append(*issues, "notifications.email.to must contain at least one recipient when email notifications are enabled")
	}
	if isLiteralRedactedSecret(cfg.Password) {
		*issues = append(*issues, "notifications.email.password cannot be set to the reserved redacted placeholder")
	}
	validateOptionalEmailAddresses(issues, cfg)
}

func validateOptionalEmailAddresses(issues *[]string, cfg EmailConfig) {
	if from := strings.TrimSpace(cfg.From); from != "" {
		if _, err := mail.ParseAddress(from); err != nil {
			*issues = append(*issues, "notifications.email.from must be a valid email address")
		}
	}
	for _, recipient := range cfg.To {
		if strings.TrimSpace(recipient) == "" {
			*issues = append(*issues, "notifications.email.to cannot contain empty recipients")
			continue
		}
		if _, err := mail.ParseAddress(recipient); err != nil {
			*issues = append(*issues, fmt.Sprintf("notifications.email.to contains invalid email address %q", recipient))
		}
	}
}

func validateDNSServer(server string) error {
	value := strings.TrimSpace(server)
	host, port, err := net.SplitHostPort(value)
	if err != nil {
		host = value
		port = "53"
	}
	if strings.TrimSpace(host) == "" {
		return fmt.Errorf("host is required")
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 {
		return fmt.Errorf("port must be between 1 and 65535")
	}
	return nil
}

func validateNotificationSettings(issues *[]string, cfg *Config) {
	validateRequiredDuration(issues, "notifications.timeout", cfg.Notifications.Timeout)
	validateWebhookConfig(issues, cfg.Notifications.Webhook)
	validateTelegramConfig(issues, cfg.Notifications.Telegram)
	validateEmailConfig(issues, cfg.Notifications.Email)
}

func validateAllowedOrigin(origin string) error {
	value := strings.TrimSpace(origin)
	if value == "" {
		return fmt.Errorf("origin is required")
	}
	parsed, err := url.ParseRequestURI(value)
	if err != nil {
		return fmt.Errorf("origin must be a valid URL")
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return fmt.Errorf("origin must use http or https")
	}
	if parsed.Host == "" {
		return fmt.Errorf("origin host is required")
	}
	if parsed.User != nil {
		return fmt.Errorf("origin must not include user info")
	}
	if parsed.Path != "" && parsed.Path != "/" {
		return fmt.Errorf("origin must not include a path")
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return fmt.Errorf("origin must not include query or fragment components")
	}
	return nil
}

func isLiteralRedactedSecret(value string) bool {
	return strings.TrimSpace(value) == RedactedSecret
}
