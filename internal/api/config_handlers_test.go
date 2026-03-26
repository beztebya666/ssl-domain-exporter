package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"ssl-domain-exporter/internal/config"
)

func TestGetConfigRedactsSecretsAndIncludesWarnings(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := config.Default()
	cfg.Auth.APIKey = "api-key-secret"
	cfg.Notifications.Webhook.URL = "https://hooks.example.test/secret"
	cfg.Notifications.Telegram.BotToken = "bot-token-secret"
	cfg.Notifications.Email.Password = "smtp-secret"

	handler := NewHandler(cfg, nil, nil, nil, nil)
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/config", nil)

	handler.GetConfig(ctx)

	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status code: got %d body=%s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "api-key-secret") || strings.Contains(rec.Body.String(), "smtp-secret") {
		t.Fatalf("response leaked a secret: %s", rec.Body.String())
	}

	var resp config.Config
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode config response: %v", err)
	}
	if resp.Auth.Password != config.RedactedSecret {
		t.Fatalf("expected auth.password to be redacted, got %q", resp.Auth.Password)
	}
	if resp.Auth.APIKey != config.RedactedSecret {
		t.Fatalf("expected auth.api_key to be redacted, got %q", resp.Auth.APIKey)
	}
	if resp.Notifications.Webhook.URL != config.RedactedSecret {
		t.Fatalf("expected webhook url to be redacted, got %q", resp.Notifications.Webhook.URL)
	}
	if len(resp.Warnings) == 0 {
		t.Fatal("expected insecure deployment warnings to be returned")
	}
}

func TestUpdateConfigPreservesRedactedSecrets(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := config.Default()
	cfg.SetFilePath(filepath.Join(t.TempDir(), "config.yaml"))
	cfg.Auth.APIKey = "existing-api-key"
	cfg.Notifications.Webhook.URL = "https://hooks.example.test/current"
	cfg.Notifications.Telegram.BotToken = "existing-bot-token"
	cfg.Notifications.Email.Password = "existing-email-password"

	next := cfg.RedactedSnapshot()
	next.Auth.SessionTTL = "48h"
	next.Notifications.Timeout = "25s"

	body, err := json.Marshal(next)
	if err != nil {
		t.Fatalf("marshal update payload: %v", err)
	}

	handler := NewHandler(cfg, nil, nil, nil, nil)
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodPut, "/api/config", bytes.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")

	handler.UpdateConfig(ctx)

	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status code: got %d body=%s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "existing-api-key") || strings.Contains(rec.Body.String(), "existing-email-password") {
		t.Fatalf("update response leaked a secret: %s", rec.Body.String())
	}

	snap := cfg.Snapshot()
	if snap.Auth.APIKey != "existing-api-key" {
		t.Fatalf("expected auth.api_key to be preserved, got %q", snap.Auth.APIKey)
	}
	if snap.Notifications.Webhook.URL != "https://hooks.example.test/current" {
		t.Fatalf("expected webhook url to be preserved, got %q", snap.Notifications.Webhook.URL)
	}
	if snap.Notifications.Telegram.BotToken != "existing-bot-token" {
		t.Fatalf("expected telegram bot token to be preserved, got %q", snap.Notifications.Telegram.BotToken)
	}
	if snap.Notifications.Email.Password != "existing-email-password" {
		t.Fatalf("expected email password to be preserved, got %q", snap.Notifications.Email.Password)
	}
	if snap.Auth.SessionTTL != "48h" {
		t.Fatalf("expected session TTL to be updated, got %q", snap.Auth.SessionTTL)
	}
	if snap.Notifications.Timeout != "25s" {
		t.Fatalf("expected notifications timeout to be updated, got %q", snap.Notifications.Timeout)
	}
}

func TestUpdateConfigRejectsInvalidValues(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := config.Default()
	next := cfg.Snapshot()
	next.Checker.Timeout = "definitely-not-a-duration"

	body, err := json.Marshal(next)
	if err != nil {
		t.Fatalf("marshal update payload: %v", err)
	}

	handler := NewHandler(cfg, nil, nil, nil, nil)
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodPut, "/api/config", bytes.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")

	handler.UpdateConfig(ctx)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("unexpected status code: got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "checker.timeout") {
		t.Fatalf("expected validation error mentioning checker.timeout, got %s", rec.Body.String())
	}
}
