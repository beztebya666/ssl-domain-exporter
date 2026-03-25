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

	"ssl-domain-exporter/internal/checker"
	"ssl-domain-exporter/internal/config"
	"ssl-domain-exporter/internal/db"
	"ssl-domain-exporter/internal/metrics"
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

	if h.metrics != nil {
		h.metrics.SetTotalDomains(len(domains))
	}
	c.JSON(http.StatusOK, domains)
}

// POST /api/domains
func (h *Handler) CreateDomain(c *gin.Context) {
	var req createDomainRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	cfg := h.cfg.Snapshot()
	in, err := buildCreateInput(req, cfg)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	in.Metadata, err = h.validateDomainMetadata(in.Metadata)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	dom, err := h.db.CreateDomain(in.Name, in.Tags, in.Metadata, in.CustomCAPEM, in.CheckMode, in.DNSServers, in.Interval, in.Port, in.FolderID)
	if err != nil {
		if isDomainAlreadyExistsErr(err) {
			c.JSON(http.StatusConflict, gin.H{"error": "domain already exists"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if in.HasEnabled && !in.Enabled {
		if err := h.db.UpdateDomain(dom.ID, dom.Name, dom.Tags, dom.Metadata, dom.CustomCAPEM, dom.CheckMode, dom.DNSServers, false, dom.CheckInterval, dom.Port, dom.FolderID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		dom, _ = h.db.GetDomainByID(dom.ID)
	}

	if dom != nil && h.metrics != nil {
		h.metrics.SyncDomain(dom)
		h.refreshTotalDomainsMetric()
	}

	if dom != nil && dom.Enabled && h.scheduler != nil {
		h.scheduler.TriggerCheck(dom)
	}
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

	var req updateDomainRequest
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

	in, err := buildUpdateInput(req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	newName := current.Name
	if in.HasName {
		newName = in.Name
	}
	tags := append([]string(nil), current.Tags...)
	if in.HasTags {
		tags = in.Tags
	}
	metadata := cloneMetadata(current.Metadata)
	if in.HasMetadata {
		metadata = cloneMetadata(in.Metadata)
	}
	metadata, err = h.validateDomainMetadata(metadata)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	customCAPEM := current.CustomCAPEM
	if in.HasCustomCAPEM {
		customCAPEM = in.CustomCAPEM
	}
	checkMode := current.CheckMode
	if in.HasCheckMode {
		checkMode = in.CheckMode
	}
	dnsServers := current.DNSServers
	if in.HasDNSServers {
		dnsServers = in.DNSServers
	}
	enabled := current.Enabled
	if in.HasEnabled {
		enabled = in.Enabled
	}
	interval := current.CheckInterval
	if in.HasInterval && in.Interval > 0 {
		interval = in.Interval
	}
	port := current.Port
	if in.HasPort && in.Port > 0 {
		port = in.Port
	}
	folderID := cloneFolderID(current.FolderID)
	if in.HasFolderID {
		folderID = in.FolderID
	}

	needsRecheck := newName != current.Name ||
		checkMode != current.CheckMode ||
		dnsServers != current.DNSServers ||
		customCAPEM != current.CustomCAPEM ||
		port != current.Port ||
		(!current.Enabled && enabled)

	if err := h.db.UpdateDomain(id, newName, tags, metadata, customCAPEM, checkMode, dnsServers, enabled, interval, port, folderID); err != nil {
		if isDomainAlreadyExistsErr(err) {
			c.JSON(http.StatusConflict, gin.H{"error": "domain already exists"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if current.Name != newName && h.metrics != nil {
		h.metrics.CleanupDomain(current.Name)
	}

	dom, _ := h.db.GetDomainByID(id)
	if dom != nil && h.metrics != nil {
		h.metrics.SyncDomain(dom)
	}

	// Trigger immediate re-check when check-affecting fields changed
	if needsRecheck && dom != nil && dom.Enabled && h.scheduler != nil {
		h.scheduler.TriggerCheck(dom)
	}

	c.JSON(http.StatusOK, dom)
}

// POST /api/domains/import
func (h *Handler) ImportDomains(c *gin.Context) {
	var req importDomainsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	mode := strings.ToLower(strings.TrimSpace(req.Mode))
	if mode == "" {
		mode = "create_only"
	}
	if mode != "create_only" && mode != "upsert" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "mode must be create_only or upsert"})
		return
	}

	cfg := h.cfg.Snapshot()
	defaults, err := parseImportMap(req.Defaults, false)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("defaults: %v", err)})
		return
	}
	if !defaults.HasCheckMode {
		defaults.CheckMode = config.ValidateCheckMode(cfg.Domains.DefaultCheckMode)
		defaults.HasCheckMode = true
	}

	existingDomains, err := h.db.GetDomains()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	existingByName := make(map[string]*db.Domain, len(existingDomains))
	for i := range existingDomains {
		domainCopy := existingDomains[i]
		existingByName[domainCopy.Name] = cloneDomain(&domainCopy)
	}

	resp := importDomainsResponse{
		Mode:    mode,
		DryRun:  req.DryRun,
		Results: make([]importDomainResult, 0, len(req.Domains)),
	}

	for idx, raw := range req.Domains {
		item, err := parseImportMap(raw, true)
		if err != nil {
			resp.Summary.Failed++
			resp.Results = append(resp.Results, importDomainResult{
				Index:  idx,
				Action: "failed",
				Error:  err.Error(),
			})
			continue
		}
		item = mergeImportDefaults(defaults, item)

		result := importDomainResult{
			Index: idx,
			Name:  item.Name,
		}

		current := existingByName[item.Name]
		if current == nil {
			preview := buildImportedDomain(cfg, nil, item)
			preview.Metadata, err = h.validateDomainMetadata(preview.Metadata)
			if err != nil {
				result.Action = "failed"
				result.Error = err.Error()
				resp.Summary.Failed++
				resp.Results = append(resp.Results, result)
				continue
			}
			if req.DryRun {
				result.Action = "create"
				result.Domain = preview
				resp.Summary.Created++
				resp.Results = append(resp.Results, result)
				existingByName[preview.Name] = cloneDomain(preview)
				continue
			}

			created, err := h.db.CreateDomain(preview.Name, preview.Tags, preview.Metadata, preview.CustomCAPEM, preview.CheckMode, preview.DNSServers, preview.CheckInterval, preview.Port, preview.FolderID)
			if err != nil {
				result.Action = "failed"
				result.Error = err.Error()
				resp.Summary.Failed++
				resp.Results = append(resp.Results, result)
				continue
			}

			if !preview.Enabled {
				if err := h.db.UpdateDomain(created.ID, created.Name, created.Tags, created.Metadata, created.CustomCAPEM, created.CheckMode, created.DNSServers, false, created.CheckInterval, created.Port, created.FolderID); err != nil {
					result.Action = "failed"
					result.Error = err.Error()
					resp.Summary.Failed++
					resp.Results = append(resp.Results, result)
					continue
				}
				created, _ = h.db.GetDomainByID(created.ID)
			}

			if created != nil {
				existingByName[created.Name] = cloneDomain(created)
				if h.metrics != nil {
					h.metrics.SyncDomain(created)
				}
				if req.TriggerChecks && created.Enabled && h.scheduler != nil {
					h.scheduler.TriggerCheck(created)
				}
			}

			result.Action = "created"
			result.Domain = created
			resp.Summary.Created++
			resp.Results = append(resp.Results, result)
			continue
		}

		if mode == "create_only" {
			result.Action = "skipped"
			result.Error = "domain already exists"
			result.Domain = cloneDomain(current)
			resp.Summary.Skipped++
			resp.Results = append(resp.Results, result)
			continue
		}

		preview := buildImportedDomain(cfg, current, item)
		preview.Metadata, err = h.validateDomainMetadata(preview.Metadata)
		if err != nil {
			result.Action = "failed"
			result.Error = err.Error()
			resp.Summary.Failed++
			resp.Results = append(resp.Results, result)
			continue
		}
		if req.DryRun {
			result.Action = "update"
			result.Domain = preview
			resp.Summary.Updated++
			resp.Results = append(resp.Results, result)
			existingByName[preview.Name] = cloneDomain(preview)
			continue
		}

		if err := h.db.UpdateDomain(current.ID, preview.Name, preview.Tags, preview.Metadata, preview.CustomCAPEM, preview.CheckMode, preview.DNSServers, preview.Enabled, preview.CheckInterval, preview.Port, preview.FolderID); err != nil {
			result.Action = "failed"
			result.Error = err.Error()
			resp.Summary.Failed++
			resp.Results = append(resp.Results, result)
			continue
		}

		updated, _ := h.db.GetDomainByID(current.ID)
		if updated != nil {
			existingByName[updated.Name] = cloneDomain(updated)
			if h.metrics != nil {
				h.metrics.SyncDomain(updated)
			}
			if req.TriggerChecks && updated.Enabled && h.scheduler != nil {
				h.scheduler.TriggerCheck(updated)
			}
		}

		result.Action = "updated"
		result.Domain = updated
		resp.Summary.Updated++
		resp.Results = append(resp.Results, result)
	}

	resp.Summary.Total = len(req.Domains)
	if h.metrics != nil && !req.DryRun {
		h.refreshTotalDomainsMetric()
	}

	c.JSON(http.StatusOK, resp)
}

// DELETE /api/domains/:id
func (h *Handler) DeleteDomain(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	// Fetch domain name before deletion so we can clean up metrics
	dom, _ := h.db.GetDomainByID(id)

	if err := h.db.DeleteDomain(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if dom != nil && h.metrics != nil {
		h.metrics.CleanupDomain(dom.Name)
		h.refreshTotalDomainsMetric()
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

// GET /api/tags
func (h *Handler) ListTags(c *gin.Context) {
	tags, err := h.db.ListTags()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if tags == nil {
		tags = []string{}
	}
	c.JSON(http.StatusOK, tags)
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

	pageStr := strings.TrimSpace(c.Query("page"))
	pageSizeStr := strings.TrimSpace(c.Query("page_size"))
	if pageStr != "" || pageSizeStr != "" {
		page, _ := strconv.Atoi(pageStr)
		pageSize, _ := strconv.Atoi(pageSizeStr)
		if page < 1 {
			page = 1
		}
		if pageSize <= 0 {
			pageSize = 20
		}
		if pageSize > 100 {
			pageSize = 100
		}

		total, err := h.db.CountCheckHistory(id)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		totalPages := 1
		if total > 0 {
			totalPages = (total + pageSize - 1) / pageSize
		}
		if page > totalPages {
			page = totalPages
		}
		offset := (page - 1) * pageSize

		checks, err := h.db.GetCheckHistoryPage(id, pageSize, offset)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if checks == nil {
			checks = []db.Check{}
		}

		c.JSON(http.StatusOK, gin.H{
			"items":       checks,
			"total":       total,
			"page":        page,
			"page_size":   pageSize,
			"total_pages": totalPages,
		})
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
	cfg := h.cfg.Snapshot()
	if !cfg.Features.CSVExport {
		c.JSON(http.StatusForbidden, gin.H{"error": "csv export is disabled"})
		return
	}

	query, err := buildDomainListQuery(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	domains, err := h.db.ListDomainsForExport(query)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	customFields, err := h.db.ListCustomFields(false)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	exportFields := visibleExportCustomFields(customFields)

	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", `attachment; filename="domains_export.csv"`)

	w := csv.NewWriter(c.Writer)
	defer w.Flush()

	header := []string{
		"id", "domain", "port", "folder_id", "tags", "metadata", "custom_ca", "check_mode", "dns_servers", "enabled", "status",
		"ssl_expiry_days", "domain_expiry_days", "registration_check_skipped",
		"http_status_code", "http_response_time_ms", "http_redirects_https", "http_hsts_enabled", "checked_at",
	}
	for _, field := range exportFields {
		header = append(header, field.Key)
	}
	_ = w.Write(header)

	for _, d := range domains {
		row := []string{
			strconv.FormatInt(d.ID, 10),
			d.Name,
			strconv.Itoa(d.Port),
			"",
			db.JoinTags(d.Tags),
			marshalMetadataCSV(d.Metadata),
			strconv.FormatBool(strings.TrimSpace(d.CustomCAPEM) != ""),
			d.EffectiveCheckMode(),
			d.DNSServers,
			strconv.FormatBool(d.Enabled),
			"unknown",
			"",
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
		if chk := d.LastCheck; chk != nil {
			row[10] = chk.OverallStatus
			if chk.SSLExpiryDays != nil {
				row[11] = strconv.Itoa(*chk.SSLExpiryDays)
			}
			if chk.RegistrationCheckSkipped {
				row[12] = "N/A"
				row[13] = "true"
			} else {
				if chk.DomainExpiryDays != nil {
					row[12] = strconv.Itoa(*chk.DomainExpiryDays)
				}
				row[13] = "false"
			}
			row[14] = strconv.Itoa(chk.HTTPStatusCode)
			row[15] = strconv.FormatInt(chk.HTTPResponseTimeMs, 10)
			row[16] = strconv.FormatBool(chk.HTTPRedirectsHTTPS)
			row[17] = strconv.FormatBool(chk.HTTPHSTSEnabled)
			row[18] = chk.CheckedAt.Format("2006-01-02 15:04:05")
		}
		for _, field := range exportFields {
			row = append(row, d.Metadata[field.Key])
		}
		_ = w.Write(row)
	}
}

// GET /api/config
func (h *Handler) GetConfig(c *gin.Context) {
	c.JSON(http.StatusOK, h.cfg.Snapshot())
}

// PUT /api/config
func (h *Handler) UpdateConfig(c *gin.Context) {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to read request body"})
		return
	}

	next := h.cfg.Snapshot()
	if err := json.Unmarshal(body, next); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	h.cfg.ApplyFrom(next)
	if err := h.cfg.Save(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("save config: %v", err)})
		return
	}

	c.JSON(http.StatusOK, h.cfg.Snapshot())
}

// GET /api/notifications/status
func (h *Handler) GetNotificationStatus(c *gin.Context) {
	if h.checker == nil {
		c.JSON(http.StatusOK, []checker.DeliveryStatus{})
		return
	}
	statuses := h.checker.NotificationStatuses()
	if statuses == nil {
		statuses = []checker.DeliveryStatus{}
	}
	c.JSON(http.StatusOK, statuses)
}

// POST /api/notifications/test
func (h *Handler) TestNotifications(c *gin.Context) {
	if h.checker == nil {
		c.JSON(http.StatusOK, []checker.TestResult{})
		return
	}
	results := h.checker.SendTestNotifications()
	if results == nil {
		results = []checker.TestResult{}
	}
	c.JSON(http.StatusOK, results)
}

// GET /api/settings (compatibility endpoint)
func (h *Handler) GetSettings(c *gin.Context) {
	cfg := h.cfg.Snapshot()
	c.JSON(http.StatusOK, map[string]interface{}{
		"checker_interval":                cfg.Checker.Interval,
		"checker_timeout":                 cfg.Checker.Timeout,
		"checker_concurrent_checks":       cfg.Checker.ConcurrentChecks,
		"prometheus_enabled":              cfg.Prometheus.Enabled,
		"prometheus_path":                 cfg.Prometheus.Path,
		"alert_domain_expiry_warning":     cfg.Alerts.DomainExpiryWarningDays,
		"alert_domain_expiry_critical":    cfg.Alerts.DomainExpiryCriticalDays,
		"alert_ssl_expiry_warning":        cfg.Alerts.SSLExpiryWarningDays,
		"alert_ssl_expiry_critical":       cfg.Alerts.SSLExpiryCriticalDays,
		"notifications_enabled":           cfg.Features.Notifications,
		"notifications_webhook_url":       cfg.Notifications.Webhook.URL,
		"webhook_on_critical":             cfg.Notifications.Webhook.OnCritical,
		"webhook_on_warning":              cfg.Notifications.Webhook.OnWarning,
		"telegram_enabled":                cfg.Notifications.Telegram.Enabled,
		"telegram_bot_token":              cfg.Notifications.Telegram.BotToken,
		"telegram_chat_id":                cfg.Notifications.Telegram.ChatID,
		"telegram_on_critical":            cfg.Notifications.Telegram.OnCritical,
		"telegram_on_warning":             cfg.Notifications.Telegram.OnWarning,
		"feature_http_check":              cfg.Features.HTTPCheck,
		"feature_cipher_check":            cfg.Features.CipherCheck,
		"feature_ocsp_check":              cfg.Features.OCSPCheck,
		"feature_crl_check":               cfg.Features.CRLCheck,
		"feature_caa_check":               cfg.Features.CAACheck,
		"feature_csv_export":              cfg.Features.CSVExport,
		"feature_timeline_view":           cfg.Features.TimelineView,
		"feature_dashboard_tag_filter":    cfg.Features.DashboardTagFilter,
		"feature_structured_logs":         cfg.Features.StructuredLogs,
		"domain_subdomain_fallback":       cfg.Domains.SubdomainFallback,
		"domain_subdomain_fallback_depth": cfg.Domains.FallbackDepth,
		"domain_default_check_mode":       cfg.Domains.DefaultCheckMode,
		"dns_servers":                     strings.Join(cfg.DNS.Servers, ","),
		"dns_use_system_dns":              cfg.DNS.UseSystemDNS,
		"dns_timeout":                     cfg.DNS.Timeout,
		"notifications_timeout":           cfg.Notifications.Timeout,
	})
}

// PUT /api/settings (compatibility endpoint)
func (h *Handler) UpdateSettings(c *gin.Context) {
	var req map[string]string
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	cfg := h.cfg.Snapshot()
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
			cfg.Prometheus.Enabled = config.ParseBool(v)
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
			cfg.Features.Notifications = config.ParseBool(v)
		case "notifications_webhook_url":
			cfg.Notifications.Webhook.URL = v
		case "webhook_on_critical":
			cfg.Notifications.Webhook.OnCritical = config.ParseBool(v)
		case "webhook_on_warning":
			cfg.Notifications.Webhook.OnWarning = config.ParseBool(v)
		case "telegram_enabled":
			cfg.Notifications.Telegram.Enabled = config.ParseBool(v)
		case "telegram_bot_token":
			cfg.Notifications.Telegram.BotToken = v
		case "telegram_chat_id":
			cfg.Notifications.Telegram.ChatID = v
		case "telegram_on_critical":
			cfg.Notifications.Telegram.OnCritical = config.ParseBool(v)
		case "telegram_on_warning":
			cfg.Notifications.Telegram.OnWarning = config.ParseBool(v)
		case "feature_http_check":
			cfg.Features.HTTPCheck = config.ParseBool(v)
		case "feature_cipher_check":
			cfg.Features.CipherCheck = config.ParseBool(v)
		case "feature_ocsp_check":
			cfg.Features.OCSPCheck = config.ParseBool(v)
		case "feature_crl_check":
			cfg.Features.CRLCheck = config.ParseBool(v)
		case "feature_caa_check":
			cfg.Features.CAACheck = config.ParseBool(v)
		case "feature_csv_export":
			cfg.Features.CSVExport = config.ParseBool(v)
		case "feature_timeline_view":
			cfg.Features.TimelineView = config.ParseBool(v)
		case "feature_dashboard_tag_filter":
			cfg.Features.DashboardTagFilter = config.ParseBool(v)
		case "feature_structured_logs":
			cfg.Features.StructuredLogs = config.ParseBool(v)
		case "domain_subdomain_fallback":
			cfg.Domains.SubdomainFallback = config.ParseBool(v)
		case "domain_subdomain_fallback_depth":
			if n, err := strconv.Atoi(v); err == nil {
				cfg.Domains.FallbackDepth = n
			}
		case "domain_default_check_mode":
			cfg.Domains.DefaultCheckMode = config.ValidateCheckMode(v)
		case "dns_servers":
			servers := []string{}
			for _, s := range strings.Split(v, ",") {
				s = strings.TrimSpace(s)
				if s != "" {
					servers = append(servers, s)
				}
			}
			cfg.DNS.Servers = servers
		case "dns_use_system_dns":
			cfg.DNS.UseSystemDNS = config.ParseBool(v)
		case "dns_timeout":
			cfg.DNS.Timeout = v
		case "notifications_timeout":
			cfg.Notifications.Timeout = v
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

func (h *Handler) refreshTotalDomainsMetric() {
	if h.metrics == nil {
		return
	}
	count, err := h.db.CountDomains()
	if err != nil {
		return
	}
	h.metrics.SetTotalDomains(count)
}

func buildImportedDomain(cfg *config.Config, current *db.Domain, in domainInput) *db.Domain {
	out := &db.Domain{}
	if current != nil {
		*out = *cloneDomain(current)
	}

	if current == nil {
		out.Name = in.Name
		out.Enabled = true
		out.Port = 443
		out.CheckInterval = 21600
		out.CheckMode = config.ValidateCheckMode(cfg.Domains.DefaultCheckMode)
	}

	if in.HasName {
		out.Name = in.Name
	}
	if in.HasTags {
		out.Tags = append([]string(nil), in.Tags...)
	}
	if in.HasMetadata {
		out.Metadata = cloneMetadata(in.Metadata)
	}
	if in.HasCustomCAPEM {
		out.CustomCAPEM = in.CustomCAPEM
	}
	if in.HasCheckMode {
		out.CheckMode = in.CheckMode
	}
	if in.HasDNSServers {
		out.DNSServers = in.DNSServers
	}
	if in.HasInterval && in.Interval > 0 {
		out.CheckInterval = in.Interval
	}
	if in.HasPort && in.Port > 0 {
		out.Port = in.Port
	}
	if in.HasFolderID {
		out.FolderID = cloneFolderID(in.FolderID)
	}
	if in.HasEnabled {
		out.Enabled = in.Enabled
	}

	out.Tags = db.NormalizeTags(out.Tags)
	if normalized, err := db.ValidateAndNormalizeMetadata(out.Metadata); err == nil {
		out.Metadata = normalized
	}
	if out.Port <= 0 {
		out.Port = 443
	}
	if out.CheckInterval <= 0 {
		out.CheckInterval = 21600
	}
	out.CheckMode = config.ValidateCheckMode(out.CheckMode)

	return out
}

func cloneMetadata(src map[string]string) map[string]string {
	if len(src) == 0 {
		return nil
	}
	out := make(map[string]string, len(src))
	for key, value := range src {
		out[key] = value
	}
	return out
}

func cloneFolderID(folderID *int64) *int64 {
	if folderID == nil {
		return nil
	}
	value := *folderID
	return &value
}

func cloneDomain(src *db.Domain) *db.Domain {
	if src == nil {
		return nil
	}
	out := *src
	out.Tags = append([]string(nil), src.Tags...)
	out.Metadata = cloneMetadata(src.Metadata)
	out.FolderID = cloneFolderID(src.FolderID)
	return &out
}

func marshalMetadataCSV(metadata map[string]string) string {
	if len(metadata) == 0 {
		return ""
	}
	buf, err := json.Marshal(metadata)
	if err != nil {
		return db.MetadataSearchText(metadata)
	}
	return string(buf)
}
