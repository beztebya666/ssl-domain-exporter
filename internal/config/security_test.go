package config

import (
	"strings"
	"testing"
)

func TestRedactedSnapshotMasksSecretsAndIncludesWarnings(t *testing.T) {
	cfg := Default()
	cfg.Auth.APIKey = "top-secret"
	cfg.Notifications.Webhook.URL = "https://hooks.example.test/secret"
	cfg.Notifications.Telegram.BotToken = "bot-token"
	cfg.Notifications.Email.Password = "smtp-password"

	snap := cfg.RedactedSnapshot()

	if snap.Auth.Password != RedactedSecret {
		t.Fatalf("expected auth.password to be redacted, got %q", snap.Auth.Password)
	}
	if snap.Auth.APIKey != RedactedSecret {
		t.Fatalf("expected auth.api_key to be redacted, got %q", snap.Auth.APIKey)
	}
	if snap.Notifications.Webhook.URL != RedactedSecret {
		t.Fatalf("expected webhook url to be redacted, got %q", snap.Notifications.Webhook.URL)
	}
	if snap.Notifications.Telegram.BotToken != RedactedSecret {
		t.Fatalf("expected telegram bot token to be redacted, got %q", snap.Notifications.Telegram.BotToken)
	}
	if snap.Notifications.Email.Password != RedactedSecret {
		t.Fatalf("expected email password to be redacted, got %q", snap.Notifications.Email.Password)
	}
	if len(snap.Warnings) == 0 {
		t.Fatal("expected insecure default warnings to be present")
	}
	if cfg.Auth.Password != "admin" {
		t.Fatalf("original config should stay unchanged, got %q", cfg.Auth.Password)
	}
}

func TestRestoreRedactedSecretsPreservesCurrentValues(t *testing.T) {
	current := Default()
	current.Auth.APIKey = "keep-api-key"
	current.Notifications.Webhook.URL = "https://hooks.example.test/current"
	current.Notifications.Telegram.BotToken = "keep-bot-token"
	current.Notifications.Email.Password = "keep-email-password"

	next := current.Clone()
	next.Auth.Password = RedactedSecret
	next.Auth.APIKey = RedactedSecret
	next.Notifications.Webhook.URL = RedactedSecret
	next.Notifications.Telegram.BotToken = RedactedSecret
	next.Notifications.Email.Password = RedactedSecret

	next.RestoreRedactedSecrets(current)

	if next.Auth.Password != current.Auth.Password {
		t.Fatalf("expected auth.password to be preserved, got %q", next.Auth.Password)
	}
	if next.Auth.APIKey != current.Auth.APIKey {
		t.Fatalf("expected auth.api_key to be preserved, got %q", next.Auth.APIKey)
	}
	if next.Notifications.Webhook.URL != current.Notifications.Webhook.URL {
		t.Fatalf("expected webhook url to be preserved, got %q", next.Notifications.Webhook.URL)
	}
	if next.Notifications.Telegram.BotToken != current.Notifications.Telegram.BotToken {
		t.Fatalf("expected telegram bot token to be preserved, got %q", next.Notifications.Telegram.BotToken)
	}
	if next.Notifications.Email.Password != current.Notifications.Email.Password {
		t.Fatalf("expected email password to be preserved, got %q", next.Notifications.Email.Password)
	}
}

func TestValidateRejectsInvalidConfig(t *testing.T) {
	cfg := Default()
	cfg.Server.Port = "abc"
	cfg.Server.AllowedOrigins = []string{"ftp://ui.example.test"}
	cfg.Auth.SessionTTL = "later"
	cfg.Checker.Timeout = "-5s"
	cfg.DNS.Servers = []string{"resolver.internal:70000"}
	cfg.Notifications.Webhook.Enabled = true
	cfg.Notifications.Webhook.URL = "ftp://hooks.example.test"
	cfg.Notifications.Email.Enabled = true
	cfg.Notifications.Email.From = "invalid"
	cfg.Notifications.Email.To = []string{"ops@example.test", "broken"}
	cfg.Maintenance.AuditRetentionDays = -1

	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected validation error")
	}

	validationErr, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("expected ValidationError, got %T", err)
	}

	wantFragments := []string{
		"server.port",
		"server.allowed_origins",
		"auth.session_ttl",
		"checker.timeout",
		"dns.servers",
		"maintenance.audit_retention_days",
		"notifications.webhook.url",
		"notifications.email.from",
		"notifications.email.to",
	}
	for _, fragment := range wantFragments {
		if !strings.Contains(validationErr.Error(), fragment) {
			t.Fatalf("expected validation error to mention %q, got %q", fragment, validationErr.Error())
		}
	}
}

func TestValidateNotificationsOnlyAllowsDisabledEmailDefaults(t *testing.T) {
	cfg := Default()
	cfg.Notifications.Email = EmailConfig{}

	if err := cfg.ValidateNotificationsOnly(); err != nil {
		t.Fatalf("expected disabled email defaults to validate, got %v", err)
	}
}

func TestValidateRejectsLiteralRedactedSecrets(t *testing.T) {
	cfg := Default()
	cfg.Auth.Password = RedactedSecret
	cfg.Auth.APIKey = RedactedSecret
	cfg.Auth.Mode = "both"
	cfg.Notifications.Webhook.URL = RedactedSecret
	cfg.Notifications.Telegram.Enabled = true
	cfg.Notifications.Telegram.BotToken = RedactedSecret
	cfg.Notifications.Telegram.ChatID = "ops-room"
	cfg.Notifications.Email.Enabled = true
	cfg.Notifications.Email.Host = "smtp.example.test"
	cfg.Notifications.Email.From = "ops@example.test"
	cfg.Notifications.Email.To = []string{"ops@example.test"}
	cfg.Notifications.Email.Password = RedactedSecret

	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected validation error")
	}
	message := err.Error()
	for _, fragment := range []string{
		"auth.password",
		"auth.api_key",
		"notifications.webhook.url",
		"notifications.telegram.bot_token",
		"notifications.email.password",
	} {
		if !strings.Contains(message, fragment) {
			t.Fatalf("expected validation error to mention %q, got %q", fragment, message)
		}
	}
}
