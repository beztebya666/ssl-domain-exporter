package api

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"ssl-domain-exporter/internal/config"
)

const (
	requestIDContextKey = "request_id"
	csrfHeaderName      = "X-CSRF-Token"
	requestIDHeaderName = "X-Request-ID"
)

type RateLimiter struct {
	mu            sync.Mutex
	entries       map[string]rateLimitEntry
	sweepInterval time.Duration
	lastSweep     time.Time
	now           func() time.Time
}

type rateLimitEntry struct {
	timestamps []time.Time
	window     time.Duration
}

func NewRateLimiter() *RateLimiter {
	return &RateLimiter{
		entries:       make(map[string]rateLimitEntry),
		sweepInterval: time.Minute,
		now:           time.Now,
	}
}

func RequestIDMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		requestID := strings.TrimSpace(c.GetHeader(requestIDHeaderName))
		if requestID == "" {
			requestID = newRequestID()
		}
		c.Set(requestIDContextKey, requestID)
		c.Header(requestIDHeaderName, requestID)
		c.Next()
	}
}

func CSRFMiddleware(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		snap := cfg.Snapshot()
		if !snap.Security.CSRFEnabled || !snap.Auth.Enabled {
			c.Next()
			return
		}

		token := ensureCSRFCookie(c, snap)
		if isSafeMethod(c.Request.Method) {
			c.Next()
			return
		}
		if !requiresCSRF(c, snap) {
			c.Next()
			return
		}

		candidate := strings.TrimSpace(c.GetHeader(csrfHeaderName))
		if candidate == "" || subtle.ConstantTimeCompare([]byte(candidate), []byte(token)) != 1 {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "csrf token validation failed"})
			return
		}

		c.Next()
	}
}

func LoginRateLimitMiddleware(cfg *config.Config, limiter *RateLimiter) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Method != http.MethodPost {
			c.Next()
			return
		}
		snap := cfg.Snapshot()
		if !snap.Security.RateLimitEnabled {
			c.Next()
			return
		}
		window, err := time.ParseDuration(snap.Security.LoginWindow)
		if err != nil || window <= 0 {
			window = 5 * time.Minute
		}
		allowed, retryAfter := limiter.Allow("login:"+c.ClientIP(), snap.Security.LoginRequests, window)
		if !allowed {
			writeRateLimited(c, retryAfter)
			return
		}
		c.Next()
	}
}

func AdminWriteRateLimitMiddleware(cfg *config.Config, limiter *RateLimiter) gin.HandlerFunc {
	return func(c *gin.Context) {
		if isSafeMethod(c.Request.Method) {
			c.Next()
			return
		}
		snap := cfg.Snapshot()
		if !snap.Security.RateLimitEnabled {
			c.Next()
			return
		}
		window, err := time.ParseDuration(snap.Security.AdminWindow)
		if err != nil || window <= 0 {
			window = time.Minute
		}
		principal := GetPrincipal(c)
		key := "admin:" + c.ClientIP()
		if principal.Authenticated {
			key = "admin:" + principal.Username + ":" + c.ClientIP()
		}
		allowed, retryAfter := limiter.Allow(key, snap.Security.AdminWriteRequests, window)
		if !allowed {
			writeRateLimited(c, retryAfter)
			return
		}
		c.Next()
	}
}

func GetRequestID(c *gin.Context) string {
	if c == nil {
		return ""
	}
	if value, ok := c.Get(requestIDContextKey); ok {
		if requestID, ok := value.(string); ok {
			return requestID
		}
	}
	return ""
}

func (l *RateLimiter) Allow(key string, limit int, window time.Duration) (bool, time.Duration) {
	if l == nil || limit <= 0 || window <= 0 {
		return true, 0
	}
	now := time.Now()
	if l.now != nil {
		now = l.now()
	}
	cutoff := now.Add(-window)

	l.mu.Lock()
	defer l.mu.Unlock()

	l.maybeSweepLocked(now)

	entry := l.entries[key]
	current := entry.timestamps
	kept := current[:0]
	for _, ts := range current {
		if ts.After(cutoff) {
			kept = append(kept, ts)
		}
	}
	current = kept
	if len(current) >= limit {
		retryAfter := window - now.Sub(current[0])
		if retryAfter < 0 {
			retryAfter = 0
		}
		entry.timestamps = current
		entry.window = window
		l.entries[key] = entry
		return false, retryAfter
	}
	current = append(current, now)
	entry.timestamps = current
	entry.window = window
	l.entries[key] = entry
	return true, 0
}

func (l *RateLimiter) maybeSweepLocked(now time.Time) {
	if l == nil || len(l.entries) == 0 {
		return
	}
	if l.sweepInterval > 0 && !l.lastSweep.IsZero() && now.Sub(l.lastSweep) < l.sweepInterval {
		return
	}
	for key, entry := range l.entries {
		if entry.window <= 0 {
			delete(l.entries, key)
			continue
		}
		cutoff := now.Add(-entry.window)
		kept := entry.timestamps[:0]
		for _, ts := range entry.timestamps {
			if ts.After(cutoff) {
				kept = append(kept, ts)
			}
		}
		if len(kept) == 0 {
			delete(l.entries, key)
			continue
		}
		entry.timestamps = kept
		l.entries[key] = entry
	}
	l.lastSweep = now
}

func newRequestID() string {
	buf := make([]byte, 12)
	if _, err := rand.Read(buf); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	return hex.EncodeToString(buf)
}

func requiresCSRF(c *gin.Context, cfg *config.Config) bool {
	if c == nil || cfg == nil || hasTokenAuth(c) {
		return false
	}
	if _, err := c.Cookie(cookieName(cfg)); err == nil {
		return true
	}
	path := c.Request.URL.Path
	return path == "/api/session/login" || path == "/api/session/logout"
}

func ensureCSRFCookie(c *gin.Context, cfg *config.Config) string {
	name := csrfCookieName(cfg)
	if token, err := c.Cookie(name); err == nil && strings.TrimSpace(token) != "" {
		return token
	}
	token, err := newSessionToken()
	if err != nil {
		token = newRequestID()
	}
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(name, token, 86400*30, "/", "", cfg.Auth.CookieSecure, false)
	return token
}

func csrfCookieName(cfg *config.Config) string {
	return cookieName(cfg) + "_csrf"
}

func hasTokenAuth(c *gin.Context) bool {
	if c == nil {
		return false
	}
	if strings.TrimSpace(c.GetHeader("Authorization")) != "" {
		return true
	}
	if strings.TrimSpace(c.GetHeader("X-API-Key")) != "" {
		return true
	}
	return false
}

func isSafeMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	default:
		return false
	}
}

func writeRateLimited(c *gin.Context, retryAfter time.Duration) {
	seconds := int(retryAfter.Seconds())
	if seconds < 1 {
		seconds = 1
	}
	c.Header("Retry-After", strconv.Itoa(seconds))
	c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"error": "rate limit exceeded"})
}
