package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"ssl-domain-exporter/internal/config"
)

func TestRouterCORSAllowsConfiguredOrigin(t *testing.T) {
	cfg := config.Default()
	cfg.Server.AllowedOrigins = []string{"https://ui.example.test"}

	router := NewRouter(cfg, nil, nil, nil, nil)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodOptions, "/api/me", nil)
	req.Header.Set("Origin", "https://ui.example.test")
	req.Header.Set("Access-Control-Request-Method", http.MethodGet)

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected preflight success, got %d", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://ui.example.test" {
		t.Fatalf("expected allowed origin header, got %q", got)
	}
}

func TestRouterCORSRejectsUnknownOrigin(t *testing.T) {
	cfg := config.Default()
	cfg.Server.AllowedOrigins = []string{"https://ui.example.test"}

	router := NewRouter(cfg, nil, nil, nil, nil)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodOptions, "/api/me", nil)
	req.Header.Set("Origin", "https://evil.example.test")
	req.Header.Set("Access-Control-Request-Method", http.MethodGet)

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected forbidden preflight, got %d", rec.Code)
	}
}
