package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"ssl-domain-exporter/internal/config"
)

func TestLoginSessionSetsCookieForLocalUser(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := config.Default()
	cfg.Auth.CookieName = "test_session"

	database := newHandlerTestDB(t)
	defer database.Close()

	hash, err := hashPassword("secret-123")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	if _, err := database.CreateUser("viewer01", hash, "viewer", true); err != nil {
		t.Fatalf("create user: %v", err)
	}

	handler := NewHandler(cfg, database, nil, nil, nil)
	body := []byte(`{"username":"viewer01","password":"secret-123"}`)
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/session/login", bytes.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")

	handler.LoginSession(ctx)

	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status code: got %d body=%s", rec.Code, rec.Body.String())
	}

	cookieHeader := rec.Header().Get("Set-Cookie")
	if !strings.Contains(cookieHeader, "test_session=") {
		t.Fatalf("expected session cookie to be set, got %q", cookieHeader)
	}
}

func TestUpdateUserRejectsDemotingLastEnabledAdmin(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := config.Default()
	database := newHandlerTestDB(t)
	defer database.Close()

	hash, err := hashPassword("secret-123")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	admin, err := database.CreateUser("admin01", hash, "admin", true)
	if err != nil {
		t.Fatalf("create admin: %v", err)
	}

	handler := NewHandler(cfg, database, nil, nil, nil)
	payload := map[string]any{
		"role":    "viewer",
		"enabled": true,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodPut, "/api/users/"+strconv.FormatInt(admin.ID, 10), bytes.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Params = gin.Params{{Key: "id", Value: strconv.FormatInt(admin.ID, 10)}}

	handler.UpdateUser(ctx)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("unexpected status code: got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(strings.ToLower(rec.Body.String()), "last enabled admin") {
		t.Fatalf("expected last admin error, got %s", rec.Body.String())
	}
}

func TestDeleteUserRejectsDeletingLastEnabledAdmin(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := config.Default()
	database := newHandlerTestDB(t)
	defer database.Close()

	hash, err := hashPassword("secret-123")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	admin, err := database.CreateUser("admin01", hash, "admin", true)
	if err != nil {
		t.Fatalf("create admin: %v", err)
	}

	handler := NewHandler(cfg, database, nil, nil, nil)
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodDelete, "/api/users/"+strconv.FormatInt(admin.ID, 10), nil)
	ctx.Params = gin.Params{{Key: "id", Value: strconv.FormatInt(admin.ID, 10)}}

	handler.DeleteUser(ctx)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("unexpected status code: got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(strings.ToLower(rec.Body.String()), "last enabled admin") {
		t.Fatalf("expected last admin error, got %s", rec.Body.String())
	}
}

func TestGetBootstrapIncludesAnonymousReadOnlyFlag(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := config.Default()
	cfg.Auth.Enabled = true
	cfg.Auth.ProtectAPI = true
	cfg.Auth.ProtectUI = false

	database := newHandlerTestDB(t)
	defer database.Close()

	handler := NewHandler(cfg, database, nil, nil, nil)
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/bootstrap", nil)

	handler.GetBootstrap(ctx)

	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status code: got %d body=%s", rec.Code, rec.Body.String())
	}

	var resp bootstrapResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode bootstrap response: %v", err)
	}
	if !resp.Auth.PublicUI || !resp.Auth.AnonymousReadOnly {
		t.Fatalf("expected anonymous read-only bootstrap flags to be true, got %+v", resp.Auth)
	}
}
