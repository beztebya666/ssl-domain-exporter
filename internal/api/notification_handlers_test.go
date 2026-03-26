package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"ssl-domain-exporter/internal/checker"
	"ssl-domain-exporter/internal/config"
)

func TestTestNotificationsTargetsSelectedChannelWithOverrides(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := config.Default()
	cfg.Auth.Password = "current-password"
	cfg.Notifications.Email.Password = "smtp-password"

	notifier := checker.NewNotifier(cfg)
	t.Cleanup(func() {
		_ = notifier.Stop(context.Background())
	})
	chk := checker.New(cfg, nil, nil, notifier)
	handler := NewHandler(cfg, nil, chk, nil, nil)

	payload := notificationTestRequest{
		Channel: "email",
		Features: &notificationFeatureOverride{
			Notifications: false,
		},
		Notifications: &config.NotificationsConfig{
			Timeout: "15s",
			Email: config.EmailConfig{
				Enabled:       true,
				Host:          "smtp.example.test",
				Port:          587,
				Username:      "ops",
				Password:      config.RedactedSecret,
				From:          "ops@example.test",
				To:            []string{"ops@example.test"},
				Mode:          "starttls",
				SubjectPrefix: "[SSL Domain Exporter]",
			},
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/notifications/test", bytes.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")

	handler.TestNotifications(ctx)

	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status code: got %d body=%s", rec.Code, rec.Body.String())
	}

	var results []checker.TestResult
	if err := json.Unmarshal(rec.Body.Bytes(), &results); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected one test result, got %d", len(results))
	}
	if results[0].Channel != "email" {
		t.Fatalf("expected email test result, got %+v", results[0])
	}
	if !results[0].Enabled {
		t.Fatalf("expected selected channel to be testable with override config, got %+v", results[0])
	}
}

func TestTestNotificationsRejectsUnknownChannel(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := config.Default()
	notifier := checker.NewNotifier(cfg)
	t.Cleanup(func() {
		_ = notifier.Stop(context.Background())
	})
	chk := checker.New(cfg, nil, nil, notifier)
	handler := NewHandler(cfg, nil, chk, nil, nil)

	body := []byte(`{"channel":"pagerduty"}`)
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/notifications/test", bytes.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")

	handler.TestNotifications(ctx)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for unknown channel, got %d body=%s", rec.Code, rec.Body.String())
	}
}
