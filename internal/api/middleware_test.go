package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"ssl-domain-exporter/internal/config"
)

func TestRequestIDMiddlewareGeneratesAndPreservesHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(RequestIDMiddleware())
	router.GET("/ping", func(c *gin.Context) {
		c.String(http.StatusOK, GetRequestID(c))
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status code: got %d", rec.Code)
	}
	generated := strings.TrimSpace(rec.Header().Get(requestIDHeaderName))
	if generated == "" {
		t.Fatal("expected generated request id header")
	}
	if body := strings.TrimSpace(rec.Body.String()); body != generated {
		t.Fatalf("expected request id in body to match header, got body=%q header=%q", body, generated)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.Header.Set(requestIDHeaderName, "req-fixed")
	router.ServeHTTP(rec, req)

	if got := rec.Header().Get(requestIDHeaderName); got != "req-fixed" {
		t.Fatalf("expected request id header to be preserved, got %q", got)
	}
}

func TestCSRFMiddlewareRejectsSessionWriteWithoutToken(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := config.Default()
	router := gin.New()
	router.Use(CSRFMiddleware(cfg))
	router.POST("/api/session/login", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/session/login", nil)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 without csrf token, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "csrf") {
		t.Fatalf("expected csrf error body, got %s", rec.Body.String())
	}
	if setCookie := rec.Header().Get("Set-Cookie"); !strings.Contains(setCookie, "_csrf=") {
		t.Fatalf("expected csrf cookie to be issued, got %q", setCookie)
	}
}

func TestCSRFMiddlewareAcceptsDoubleSubmitToken(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := config.Default()
	router := gin.New()
	router.Use(CSRFMiddleware(cfg))
	router.GET("/bootstrap", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})
	router.POST("/api/session/login", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	bootstrapRec := httptest.NewRecorder()
	bootstrapReq := httptest.NewRequest(http.MethodGet, "/bootstrap", nil)
	router.ServeHTTP(bootstrapRec, bootstrapReq)
	if bootstrapRec.Code != http.StatusNoContent {
		t.Fatalf("expected bootstrap request to pass, got %d", bootstrapRec.Code)
	}

	var csrfCookie *http.Cookie
	for _, cookie := range bootstrapRec.Result().Cookies() {
		if cookie.Name == csrfCookieName(cfg) {
			csrfCookie = cookie
			break
		}
	}
	if csrfCookie == nil || strings.TrimSpace(csrfCookie.Value) == "" {
		t.Fatal("expected bootstrap request to issue a csrf cookie")
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/session/login", nil)
	req.AddCookie(csrfCookie)
	req.Header.Set(csrfHeaderName, csrfCookie.Value)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected valid double-submit csrf request to pass, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestCSRFMiddlewareSkipsTokenAuthWrites(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := config.Default()
	cfg.Auth.Mode = "api_key"
	cfg.Auth.APIKey = "top-secret"

	router := gin.New()
	router.Use(CSRFMiddleware(cfg))
	router.POST("/api/domains", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/domains", nil)
	req.Header.Set("X-API-Key", "top-secret")
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected api key auth to skip csrf, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestLoginRateLimitMiddlewareBlocksAfterConfiguredBurst(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := config.Default()
	cfg.Security.LoginRequests = 2
	cfg.Security.LoginWindow = "1h"

	router := gin.New()
	router.POST("/api/session/login", LoginRateLimitMiddleware(cfg, NewRateLimiter()), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	for i := 0; i < 2; i++ {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/session/login", nil)
		req.RemoteAddr = "198.51.100.25:1234"
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusNoContent {
			t.Fatalf("expected request %d to pass, got %d body=%s", i+1, rec.Code, rec.Body.String())
		}
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/session/login", nil)
	req.RemoteAddr = "198.51.100.25:1234"
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected third request to be rate limited, got %d body=%s", rec.Code, rec.Body.String())
	}
	if retryAfter := rec.Header().Get("Retry-After"); retryAfter == "" {
		t.Fatal("expected Retry-After header on rate-limited response")
	}
}

func TestRateLimiterSweepsIdleKeys(t *testing.T) {
	clock := time.Date(2026, time.March, 26, 12, 0, 0, 0, time.UTC)
	limiter := NewRateLimiter()
	limiter.sweepInterval = time.Minute
	limiter.now = func() time.Time {
		return clock
	}

	allowed, _ := limiter.Allow("login:198.51.100.10", 2, time.Minute)
	if !allowed {
		t.Fatal("expected first key to be allowed")
	}
	if len(limiter.entries) != 1 {
		t.Fatalf("expected one limiter entry, got %d", len(limiter.entries))
	}

	clock = clock.Add(2 * time.Minute)
	allowed, _ = limiter.Allow("login:198.51.100.11", 2, time.Minute)
	if !allowed {
		t.Fatal("expected second key to be allowed after sweep")
	}

	if len(limiter.entries) != 1 {
		t.Fatalf("expected expired key to be swept, got %d entries", len(limiter.entries))
	}
	if _, ok := limiter.entries["login:198.51.100.10"]; ok {
		t.Fatal("expected idle key to be swept from limiter state")
	}
}
