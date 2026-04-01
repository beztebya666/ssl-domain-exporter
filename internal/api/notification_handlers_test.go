package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/gin-gonic/gin"

	"ssl-domain-exporter/internal/checker"
	"ssl-domain-exporter/internal/config"
	"ssl-domain-exporter/internal/db"
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

func TestParseOptionalStringListJSONSupportsStringAndArray(t *testing.T) {
	t.Run("string", func(t *testing.T) {
		values, set, err := parseOptionalStringListJSON(json.RawMessage(`"ops@example.test"`), "email_to")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !set {
			t.Fatal("expected string input to be treated as set")
		}
		if len(values) != 1 || values[0] != "ops@example.test" {
			t.Fatalf("unexpected values: %#v", values)
		}
	})

	t.Run("array", func(t *testing.T) {
		values, set, err := parseOptionalStringListJSON(json.RawMessage(`["ops@example.test"," noc@example.test "]`), "email_to")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !set {
			t.Fatal("expected array input to be treated as set")
		}
		if len(values) != 2 || values[0] != "ops@example.test" || values[1] != "noc@example.test" {
			t.Fatalf("unexpected values: %#v", values)
		}
	})
}

func TestSendAdHocNotificationSupportsWebhookOverride(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := config.Default()
	cfg.Features.Notifications = true

	database := newHandlerTestDB(t)
	defer database.Close()

	domain, err := database.CreateDomain("example.internal", nil, nil, db.DomainSourceManual, nil, "", "full", "", 3600, 443, nil)
	if err != nil {
		t.Fatalf("create domain: %v", err)
	}

	requests := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		var payload map[string]string
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		requests <- payload["text"]
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	body := []byte(`{"message":"manual notice","webhook_url":"` + server.URL + `"}`)
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/domains/1/notify", bytes.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Params = gin.Params{{Key: "id", Value: strconv.FormatInt(domain.ID, 10)}}

	handler := NewHandler(cfg, database, nil, nil, nil)
	handler.SendAdHocNotification(ctx)

	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status code: got %d body=%s", rec.Code, rec.Body.String())
	}

	select {
	case text := <-requests:
		if text != "manual notice" {
			t.Fatalf("unexpected webhook body: %q", text)
		}
	default:
		t.Fatal("expected webhook override request to be sent")
	}

	var resp struct {
		Domain  string `json:"domain"`
		Results []struct {
			Channel string `json:"channel"`
			Success bool   `json:"success"`
		} `json:"results"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Domain != domain.Name {
		t.Fatalf("unexpected domain in response: %q", resp.Domain)
	}
	if len(resp.Results) != 1 || resp.Results[0].Channel != "webhook" || !resp.Results[0].Success {
		t.Fatalf("unexpected results: %+v", resp.Results)
	}
}

func TestSendAdHocNotificationRejectsUnknownChannel(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := config.Default()
	cfg.Features.Notifications = true

	database := newHandlerTestDB(t)
	defer database.Close()

	domain, err := database.CreateDomain("example.internal", nil, nil, db.DomainSourceManual, nil, "", "full", "", 3600, 443, nil)
	if err != nil {
		t.Fatalf("create domain: %v", err)
	}

	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/domains/1/notify", bytes.NewReader([]byte(`{"channels":["pagerduty"]}`)))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Params = gin.Params{{Key: "id", Value: strconv.FormatInt(domain.ID, 10)}}

	handler := NewHandler(cfg, database, nil, nil, nil)
	handler.SendAdHocNotification(ctx)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for unknown channel, got %d body=%s", rec.Code, rec.Body.String())
	}
}
