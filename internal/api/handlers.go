package api

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"domain-ssl-checker/internal/checker"
	"domain-ssl-checker/internal/config"
	"domain-ssl-checker/internal/db"
	"domain-ssl-checker/internal/metrics"
)

type Handler struct {
	cfg       *config.Config
	db        *db.DB
	checker   *checker.Checker
	scheduler *checker.Scheduler
	metrics   *metrics.Metrics
}

func NewHandler(cfg *config.Config, database *db.DB, chk *checker.Checker, sched *checker.Scheduler, m *metrics.Metrics) *Handler {
	return &Handler{cfg: cfg, db: database, checker: chk, scheduler: sched, metrics: m}
}

// GET /api/domains
func (h *Handler) ListDomains(c *gin.Context) {
	domains, err := h.db.GetDomains()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	lastChecks, err := h.db.GetAllLastChecks()
	if err == nil {
		for i := range domains {
			if chk, ok := lastChecks[domains[i].ID]; ok {
				domains[i].LastCheck = chk
			}
		}
	}

	h.metrics.SetTotalDomains(len(domains))
	c.JSON(http.StatusOK, domains)
}

// POST /api/domains
func (h *Handler) CreateDomain(c *gin.Context) {
	var req struct {
		Name        string `json:"name" binding:"required"`
		Tags        string `json:"tags"`
		CustomCAPEM string `json:"custom_ca_pem"`
		Interval    int    `json:"check_interval"`
		Port        int    `json:"port"`
		FolderID    *int64 `json:"folder_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	name := normalizeDomain(req.Name)
	folderID := req.FolderID
	if folderID != nil && *folderID <= 0 {
		folderID = nil
	}
	dom, err := h.db.CreateDomain(name, req.Tags, strings.TrimSpace(req.CustomCAPEM), req.Interval, req.Port, folderID)
	if err != nil {
		if isDomainAlreadyExistsErr(err) {
			c.JSON(http.StatusConflict, gin.H{"error": "domain already exists"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.scheduler.TriggerCheck(dom)
	c.JSON(http.StatusCreated, dom)
}

// GET /api/domains/:id
func (h *Handler) GetDomain(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	dom, err := h.db.GetDomainByID(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if dom == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}

	lastCheck, _ := h.db.GetLastCheck(id)
	dom.LastCheck = lastCheck
	c.JSON(http.StatusOK, dom)
}

// PUT /api/domains/:id
func (h *Handler) UpdateDomain(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var req struct {
		Name        string  `json:"name" binding:"required"`
		Tags        string  `json:"tags"`
		CustomCAPEM *string `json:"custom_ca_pem"`
		Enabled     bool    `json:"enabled"`
		Interval    int     `json:"check_interval"`
		Port        int     `json:"port"`
		FolderID    *int64  `json:"folder_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	current, err := h.db.GetDomainByID(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if current == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}

	customCAPEM := current.CustomCAPEM
	if req.CustomCAPEM != nil {
		customCAPEM = strings.TrimSpace(*req.CustomCAPEM)
	}
	port := current.Port
	if req.Port > 0 {
		port = req.Port
	}
	folderID := current.FolderID
	if req.FolderID != nil {
		if *req.FolderID <= 0 {
			folderID = nil
		} else {
			folderID = req.FolderID
		}
	}

	if err := h.db.UpdateDomain(id, normalizeDomain(req.Name), req.Tags, customCAPEM, req.Enabled, req.Interval, port, folderID); err != nil {
		if isDomainAlreadyExistsErr(err) {
			c.JSON(http.StatusConflict, gin.H{"error": "domain already exists"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	dom, _ := h.db.GetDomainByID(id)
	c.JSON(http.StatusOK, dom)
}

// DELETE /api/domains/:id
func (h *Handler) DeleteDomain(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	if err := h.db.DeleteDomain(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// POST /api/domains/reorder
func (h *Handler) ReorderDomains(c *gin.Context) {
	var req struct {
		IDs []int64 `json:"ids"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if len(req.IDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ids cannot be empty"})
		return
	}

	if err := h.db.ReorderDomains(req.IDs); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// POST /api/domains/:id/check
func (h *Handler) TriggerCheck(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	dom, err := h.db.GetDomainByID(id)
	if err != nil || dom == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}

	result := h.checker.CheckDomain(dom)
	c.JSON(http.StatusOK, result)
}

// GET /api/folders
func (h *Handler) ListFolders(c *gin.Context) {
	folders, err := h.db.GetFolders()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, folders)
}

// POST /api/folders
func (h *Handler) CreateFolder(c *gin.Context) {
	var req struct {
		Name string `json:"name" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "folder name is required"})
		return
	}

	folder, err := h.db.CreateFolder(name)
	if err != nil {
		if isFolderAlreadyExistsErr(err) {
			c.JSON(http.StatusConflict, gin.H{"error": "folder already exists"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, folder)
}

// PUT /api/folders/:id
func (h *Handler) UpdateFolder(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var req struct {
		Name string `json:"name" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "folder name is required"})
		return
	}

	if err := h.db.UpdateFolder(id, name); err != nil {
		if isFolderAlreadyExistsErr(err) {
			c.JSON(http.StatusConflict, gin.H{"error": "folder already exists"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	folder, _ := h.db.GetFolderByID(id)
	if folder == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	c.JSON(http.StatusOK, folder)
}

// DELETE /api/folders/:id
func (h *Handler) DeleteFolder(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	if err := h.db.DeleteFolder(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// GET /api/domains/:id/history
func (h *Handler) GetHistory(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	limitStr := c.DefaultQuery("limit", "50")
	limit, _ := strconv.Atoi(limitStr)

	checks, err := h.db.GetCheckHistory(id, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if checks == nil {
		checks = []db.Check{}
	}

	c.JSON(http.StatusOK, checks)
}

// GET /api/domains/export.csv
func (h *Handler) ExportDomainsCSV(c *gin.Context) {
	if !h.cfg.Features.CSVExport {
		c.JSON(http.StatusForbidden, gin.H{"error": "csv export is disabled"})
		return
	}

	domains, err := h.db.GetDomains()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	lastChecks, _ := h.db.GetAllLastChecks()

	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", `attachment; filename="domains_export.csv"`)

	w := csv.NewWriter(c.Writer)
	defer w.Flush()

	header := []string{
		"id", "domain", "port", "folder_id", "tags", "custom_ca", "enabled", "status", "ssl_expiry_days", "domain_expiry_days",
		"http_status_code", "http_response_time_ms", "http_redirects_https", "http_hsts_enabled", "checked_at",
	}
	_ = w.Write(header)

	for _, d := range domains {
		row := []string{
			strconv.FormatInt(d.ID, 10),
			d.Name,
			strconv.Itoa(d.Port),
			"",
			d.Tags,
			strconv.FormatBool(strings.TrimSpace(d.CustomCAPEM) != ""),
			strconv.FormatBool(d.Enabled),
			"unknown",
			"",
			"",
			"",
			"",
			"",
			"",
			"",
		}
		if d.FolderID != nil {
			row[3] = strconv.FormatInt(*d.FolderID, 10)
		}
		if chk, ok := lastChecks[d.ID]; ok {
			row[7] = chk.OverallStatus
			if chk.SSLExpiryDays != nil {
				row[8] = strconv.Itoa(*chk.SSLExpiryDays)
			}
			if chk.DomainExpiryDays != nil {
				row[9] = strconv.Itoa(*chk.DomainExpiryDays)
			}
			row[10] = strconv.Itoa(chk.HTTPStatusCode)
			row[11] = strconv.FormatInt(chk.HTTPResponseTimeMs, 10)
			row[12] = strconv.FormatBool(chk.HTTPRedirectsHTTPS)
			row[13] = strconv.FormatBool(chk.HTTPHSTSEnabled)
			row[14] = chk.CheckedAt.Format("2006-01-02 15:04:05")
		}
		_ = w.Write(row)
	}
}

// GET /api/config
func (h *Handler) GetConfig(c *gin.Context) {
	c.JSON(http.StatusOK, h.cfg.Clone())
}

// PUT /api/config
func (h *Handler) UpdateConfig(c *gin.Context) {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to read request body"})
		return
	}

	next := h.cfg.Clone()
	if err := json.Unmarshal(body, next); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	h.cfg.ApplyFrom(next)
	if err := h.cfg.Save(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("save config: %v", err)})
		return
	}

	c.JSON(http.StatusOK, h.cfg.Clone())
}

// GET /api/settings (compatibility endpoint)
func (h *Handler) GetSettings(c *gin.Context) {
	cfg := h.cfg
	c.JSON(http.StatusOK, map[string]interface{}{
		"checker_interval":              cfg.Checker.Interval,
		"checker_timeout":               cfg.Checker.Timeout,
		"checker_concurrent_checks":     cfg.Checker.ConcurrentChecks,
		"prometheus_enabled":            cfg.Prometheus.Enabled,
		"prometheus_path":               cfg.Prometheus.Path,
		"alert_domain_expiry_warning":   cfg.Alerts.DomainExpiryWarningDays,
		"alert_domain_expiry_critical":  cfg.Alerts.DomainExpiryCriticalDays,
		"alert_ssl_expiry_warning":      cfg.Alerts.SSLExpiryWarningDays,
		"alert_ssl_expiry_critical":     cfg.Alerts.SSLExpiryCriticalDays,
		"notifications_enabled":         cfg.Features.Notifications,
		"notifications_webhook_url":     cfg.Notifications.Webhook.URL,
		"webhook_on_critical":           cfg.Notifications.Webhook.OnCritical,
		"webhook_on_warning":            cfg.Notifications.Webhook.OnWarning,
		"telegram_enabled":              cfg.Notifications.Telegram.Enabled,
		"telegram_bot_token":            cfg.Notifications.Telegram.BotToken,
		"telegram_chat_id":              cfg.Notifications.Telegram.ChatID,
		"telegram_on_critical":          cfg.Notifications.Telegram.OnCritical,
		"telegram_on_warning":           cfg.Notifications.Telegram.OnWarning,
		"feature_http_check":            cfg.Features.HTTPCheck,
		"feature_cipher_check":          cfg.Features.CipherCheck,
		"feature_ocsp_check":            cfg.Features.OCSPCheck,
		"feature_crl_check":             cfg.Features.CRLCheck,
		"feature_caa_check":             cfg.Features.CAACheck,
		"feature_csv_export":            cfg.Features.CSVExport,
		"feature_timeline_view":         cfg.Features.TimelineView,
		"feature_dashboard_tag_filter":  cfg.Features.DashboardTagFilter,
		"feature_structured_logs":       cfg.Features.StructuredLogs,
		"domain_subdomain_fallback":     cfg.Domains.SubdomainFallback,
		"domain_subdomain_fallback_depth": cfg.Domains.FallbackDepth,
	})
}

// PUT /api/settings (compatibility endpoint)
func (h *Handler) UpdateSettings(c *gin.Context) {
	var req map[string]string
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	cfg := h.cfg.Clone()
	for k, v := range req {
		switch k {
		case "checker_interval":
			cfg.Checker.Interval = v
		case "checker_timeout":
			cfg.Checker.Timeout = v
		case "checker_concurrent_checks":
			if n, err := strconv.Atoi(v); err == nil {
				cfg.Checker.ConcurrentChecks = n
			}
		case "prometheus_enabled":
			cfg.Prometheus.Enabled = parseBool(v)
		case "prometheus_path":
			cfg.Prometheus.Path = v
		case "alert_domain_expiry_warning":
			if n, err := strconv.Atoi(v); err == nil {
				cfg.Alerts.DomainExpiryWarningDays = n
			}
		case "alert_domain_expiry_critical":
			if n, err := strconv.Atoi(v); err == nil {
				cfg.Alerts.DomainExpiryCriticalDays = n
			}
		case "alert_ssl_expiry_warning":
			if n, err := strconv.Atoi(v); err == nil {
				cfg.Alerts.SSLExpiryWarningDays = n
			}
		case "alert_ssl_expiry_critical":
			if n, err := strconv.Atoi(v); err == nil {
				cfg.Alerts.SSLExpiryCriticalDays = n
			}
		case "notifications_enabled":
			cfg.Features.Notifications = parseBool(v)
		case "notifications_webhook_url":
			cfg.Notifications.Webhook.URL = v
		case "webhook_on_critical":
			cfg.Notifications.Webhook.OnCritical = parseBool(v)
		case "webhook_on_warning":
			cfg.Notifications.Webhook.OnWarning = parseBool(v)
		case "telegram_enabled":
			cfg.Notifications.Telegram.Enabled = parseBool(v)
		case "telegram_bot_token":
			cfg.Notifications.Telegram.BotToken = v
		case "telegram_chat_id":
			cfg.Notifications.Telegram.ChatID = v
		case "telegram_on_critical":
			cfg.Notifications.Telegram.OnCritical = parseBool(v)
		case "telegram_on_warning":
			cfg.Notifications.Telegram.OnWarning = parseBool(v)
		case "feature_http_check":
			cfg.Features.HTTPCheck = parseBool(v)
		case "feature_cipher_check":
			cfg.Features.CipherCheck = parseBool(v)
		case "feature_ocsp_check":
			cfg.Features.OCSPCheck = parseBool(v)
		case "feature_crl_check":
			cfg.Features.CRLCheck = parseBool(v)
		case "feature_caa_check":
			cfg.Features.CAACheck = parseBool(v)
		case "feature_csv_export":
			cfg.Features.CSVExport = parseBool(v)
		case "feature_timeline_view":
			cfg.Features.TimelineView = parseBool(v)
		case "feature_dashboard_tag_filter":
			cfg.Features.DashboardTagFilter = parseBool(v)
		case "feature_structured_logs":
			cfg.Features.StructuredLogs = parseBool(v)
		case "domain_subdomain_fallback":
			cfg.Domains.SubdomainFallback = parseBool(v)
		case "domain_subdomain_fallback_depth":
			if n, err := strconv.Atoi(v); err == nil {
				cfg.Domains.FallbackDepth = n
			}
		}
	}

	h.cfg.ApplyFrom(cfg)
	if err := h.cfg.Save(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("save config: %v", err)})
		return
	}

	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// GET /api/summary
func (h *Handler) GetSummary(c *gin.Context) {
	domains, err := h.db.GetDomains()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	lastChecks, _ := h.db.GetAllLastChecks()

	summary := map[string]int{
		"total":    len(domains),
		"ok":       0,
		"warning":  0,
		"critical": 0,
		"error":    0,
		"unknown":  0,
	}

	for _, dom := range domains {
		if chk, ok := lastChecks[dom.ID]; ok {
			summary[chk.OverallStatus]++
		} else {
			summary["unknown"]++
		}
	}

	c.JSON(http.StatusOK, summary)
}

func normalizeDomain(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	name = strings.TrimPrefix(name, "https://")
	name = strings.TrimPrefix(name, "http://")
	name = strings.Split(name, "/")[0]
	if host, _, err := net.SplitHostPort(name); err == nil && host != "" {
		name = host
	}
	if i := strings.LastIndex(name, ":"); i > 0 {
		if _, err := strconv.Atoi(name[i+1:]); err == nil {
			name = name[:i]
		}
	}
	return name
}

func parseBool(v string) bool {
	v = strings.ToLower(strings.TrimSpace(v))
	return v == "1" || v == "true" || v == "yes" || v == "on"
}

func isDomainAlreadyExistsErr(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unique constraint failed") && strings.Contains(msg, "domains.name")
}

func isFolderAlreadyExistsErr(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unique constraint failed") && strings.Contains(msg, "folders.name")
}
