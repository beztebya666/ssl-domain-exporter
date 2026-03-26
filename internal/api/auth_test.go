package api

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"ssl-domain-exporter/internal/config"
)

func TestValidateAPIKeyAcceptsHeaderAndRejectsQueryString(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := config.Default()
	cfg.Auth.Mode = "api_key"
	cfg.Auth.APIKey = "top-secret"

	headerRec := httptest.NewRecorder()
	headerCtx, _ := gin.CreateTestContext(headerRec)
	headerCtx.Request = httptest.NewRequest("GET", "/api/me", nil)
	headerCtx.Request.Header.Set("X-API-Key", "top-secret")
	if !validateAPIKey(cfg, headerCtx) {
		t.Fatal("expected X-API-Key header to authenticate")
	}

	queryRec := httptest.NewRecorder()
	queryCtx, _ := gin.CreateTestContext(queryRec)
	queryCtx.Request = httptest.NewRequest("GET", "/api/me?api_key=top-secret", nil)
	if validateAPIKey(cfg, queryCtx) {
		t.Fatal("expected api_key query parameter to be rejected")
	}
}

func TestHashSessionTokenUsesConfigBoundKey(t *testing.T) {
	cfgA := config.Default()
	cfgA.Auth.Password = "password-a"
	cfgB := config.Default()
	cfgB.Auth.Password = "password-b"

	hashA := hashSessionToken(cfgA, "session-token")
	hashAAgain := hashSessionToken(cfgA, "session-token")
	hashB := hashSessionToken(cfgB, "session-token")

	if hashA == "" {
		t.Fatal("expected non-empty token hash")
	}
	if hashA != hashAAgain {
		t.Fatal("expected session token hashing to be deterministic for the same config")
	}
	if hashA == hashB {
		t.Fatal("expected different config secrets to produce different token hashes")
	}
}
