package api

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"

	"ssl-domain-exporter/internal/config"
	"ssl-domain-exporter/internal/db"
)

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type userRequest struct {
	Username string  `json:"username"`
	Role     string  `json:"role"`
	Enabled  *bool   `json:"enabled"`
	Password *string `json:"password"`
}

type bootstrapResponse struct {
	Auth struct {
		Enabled           bool   `json:"enabled"`
		PublicUI          bool   `json:"public_ui"`
		AnonymousReadOnly bool   `json:"anonymous_read_only"`
		Mode              string `json:"mode"`
	} `json:"auth"`
	Prometheus struct {
		Enabled bool   `json:"enabled"`
		Path    string `json:"path"`
		Public  bool   `json:"public"`
	} `json:"prometheus"`
	Features config.FeaturesConfig `json:"features"`
	Alerts   config.AlertsConfig   `json:"alerts"`
	Domains  struct {
		DefaultCheckMode string `json:"default_check_mode"`
	} `json:"domains"`
}

func (h *Handler) GetBootstrap(c *gin.Context) {
	cfg := h.cfg.Snapshot()
	var resp bootstrapResponse
	resp.Auth.Enabled = cfg.Auth.Enabled
	resp.Auth.PublicUI = cfg.Auth.AnonymousReadOnlyEnabled()
	resp.Auth.AnonymousReadOnly = cfg.Auth.AnonymousReadOnlyEnabled()
	resp.Auth.Mode = cfg.Auth.Mode
	resp.Prometheus.Enabled = cfg.Prometheus.Enabled
	resp.Prometheus.Path = cfg.Prometheus.Path
	resp.Prometheus.Public = !cfg.Auth.Enabled || !cfg.Auth.ProtectMetrics
	resp.Features = cfg.Features
	resp.Alerts = cfg.Alerts
	resp.Domains.DefaultCheckMode = cfg.Domains.DefaultCheckMode
	c.JSON(http.StatusOK, resp)
}

func (h *Handler) GetMe(c *gin.Context) {
	cfg := h.cfg.Snapshot()
	principal := GetPrincipal(c)
	c.JSON(http.StatusOK, principalToResponse(principal, cfg))
}

func (h *Handler) LoginSession(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	username := strings.TrimSpace(req.Username)
	password := req.Password
	if username == "" || password == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "username and password are required"})
		return
	}

	cfg := h.cfg.Snapshot()
	user, err := h.authenticateUser(cfg, username, password)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if user == nil {
		unauthorized(c)
		return
	}

	token, err := newSessionToken()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create session"})
		return
	}
	sessionTTL, err := parseSessionTTL(cfg.Auth.SessionTTL)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "invalid session_ttl"})
		return
	}
	expiresAt := time.Now().Add(sessionTTL)
	if _, err := h.db.CreateSession(user.ID, hashSessionToken(token), expiresAt, c.Request.UserAgent(), c.ClientIP()); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	_ = h.db.UpdateUserLastLogin(user.ID, time.Now())

	c.SetCookie(cookieName(cfg), token, int(sessionTTL.Seconds()), "/", "", cfg.Auth.CookieSecure, true)
	principal := Principal{
		Authenticated: true,
		UserID:        user.ID,
		Username:      user.Username,
		Role:          db.NormalizeUserRole(user.Role),
		Source:        "session",
	}
	c.JSON(http.StatusOK, principalToResponse(principal, cfg))
}

func (h *Handler) LogoutSession(c *gin.Context) {
	cfg := h.cfg.Snapshot()
	if rawToken, err := c.Cookie(cookieName(cfg)); err == nil && strings.TrimSpace(rawToken) != "" {
		_ = h.db.DeleteSession(hashSessionToken(rawToken))
	}
	clearSessionCookie(c, cfg)
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *Handler) ListUsers(c *gin.Context) {
	users, err := h.db.ListUsers()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, users)
}

func (h *Handler) CreateUser(c *gin.Context) {
	var req userRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	username := strings.TrimSpace(req.Username)
	if username == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "username is required"})
		return
	}
	if req.Password == nil || strings.TrimSpace(*req.Password) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "password is required"})
		return
	}

	hash, err := hashPassword(*req.Password)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to hash password"})
		return
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	user, err := h.db.CreateUser(username, hash, req.Role, enabled)
	if err != nil {
		if isUserAlreadyExistsErr(err) {
			c.JSON(http.StatusConflict, gin.H{"error": "user already exists"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, user)
}

func (h *Handler) UpdateUser(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var req userRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	current, err := h.db.GetUserByID(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if current == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}

	username := current.Username
	if strings.TrimSpace(req.Username) != "" {
		username = strings.TrimSpace(req.Username)
	}
	role := current.Role
	if strings.TrimSpace(req.Role) != "" {
		role = req.Role
	}
	enabled := current.Enabled
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	if blocked, err := h.wouldRemoveLastEnabledAdmin(current, role, enabled, false); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	} else if blocked {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cannot remove or disable the last enabled admin user"})
		return
	}

	var passwordHash *string
	if req.Password != nil && strings.TrimSpace(*req.Password) != "" {
		hash, err := hashPassword(*req.Password)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to hash password"})
			return
		}
		passwordHash = &hash
	}

	if err := h.db.UpdateUser(id, username, role, enabled, passwordHash); err != nil {
		if isUserAlreadyExistsErr(err) {
			c.JSON(http.StatusConflict, gin.H{"error": "user already exists"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if !enabled {
		_ = h.db.DeleteSessionsByUser(id)
	}

	user, _ := h.db.GetUserByID(id)
	c.JSON(http.StatusOK, user)
}

func (h *Handler) DeleteUser(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	principal := GetPrincipal(c)
	if principal.UserID == id {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cannot delete the current user"})
		return
	}

	current, err := h.db.GetUserByID(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if current == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	if blocked, err := h.wouldRemoveLastEnabledAdmin(current, current.Role, current.Enabled, true); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	} else if blocked {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cannot delete the last enabled admin user"})
		return
	}

	if err := h.db.DeleteUser(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *Handler) authenticateUser(cfg *config.Config, username, password string) (*db.User, error) {
	user, err := h.db.GetUserByUsername(username)
	if err != nil {
		return nil, err
	}
	if user != nil && user.Enabled && comparePassword(user.PasswordHash, password) == nil {
		return user, nil
	}

	if matchesLegacyCredentials(cfg, username, password) {
		return h.ensureLegacyAdminUser(username, password)
	}

	return nil, nil
}

func (h *Handler) ensureLegacyAdminUser(username, password string) (*db.User, error) {
	user, err := h.db.GetUserByUsername(username)
	if err != nil {
		return nil, err
	}
	hash, err := hashPassword(password)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return h.db.CreateUser(username, hash, "admin", true)
	}
	if err := h.db.UpdateUser(user.ID, username, "admin", true, &hash); err != nil {
		return nil, err
	}
	return h.db.GetUserByID(user.ID)
}

func matchesLegacyCredentials(cfg *config.Config, username, password string) bool {
	if cfg == nil {
		return false
	}
	mode := strings.ToLower(strings.TrimSpace(cfg.Auth.Mode))
	if mode == "api_key" {
		return false
	}
	username = strings.TrimSpace(username)
	return subtleCompare(username, cfg.Auth.Username) && subtleCompare(password, cfg.Auth.Password)
}

func subtleCompare(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

func hashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

func comparePassword(hash, password string) error {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
}

func newSessionToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func parseSessionTTL(raw string) (time.Duration, error) {
	if strings.TrimSpace(raw) == "" {
		return 24 * time.Hour, nil
	}
	return time.ParseDuration(raw)
}

func isUserAlreadyExistsErr(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unique constraint failed") && strings.Contains(msg, "users.username")
}

func (h *Handler) wouldRemoveLastEnabledAdmin(current *db.User, nextRole string, nextEnabled bool, deleting bool) (bool, error) {
	if current == nil || !current.Enabled || db.NormalizeUserRole(current.Role) != "admin" {
		return false, nil
	}
	if !deleting && nextEnabled && db.NormalizeUserRole(nextRole) == "admin" {
		return false, nil
	}

	users, err := h.db.ListUsers()
	if err != nil {
		return false, err
	}
	for _, user := range users {
		if user.ID == current.ID {
			continue
		}
		if user.Enabled && db.NormalizeUserRole(user.Role) == "admin" {
			return false, nil
		}
	}
	return true, nil
}
