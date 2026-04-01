package api

import (
	"encoding/csv"
	"encoding/json"
	"errors"
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

type notificationTestRequest struct {
	Channel       string                       `json:"channel"`
	Features      *notificationFeatureOverride `json:"features,omitempty"`
	Notifications *config.NotificationsConfig  `json:"notifications,omitempty"`
}

type notificationFeatureOverride struct {
	Notifications bool `json:"notifications"`
}

type adHocNotificationRequest struct {
	Recipients       json.RawMessage `json:"recipients,omitempty"`
	EmailTo          json.RawMessage `json:"email_to,omitempty"`
	Subject          string          `json:"subject"`
	Message          string          `json:"message"`
	Channels         []string        `json:"channels"`
	WebhookURL       string          `json:"webhook_url"`
	TelegramBotToken string          `json:"telegram_bot_token"`
	TelegramChatID   string          `json:"telegram_chat_id"`
}

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

	dom, err := h.db.CreateDomain(in.Name, in.Tags, in.Metadata, in.SourceType, in.SourceRef, in.CustomCAPEM, in.CheckMode, in.DNSServers, in.Interval, in.Port, in.FolderID)
	if err != nil {
		if isDomainAlreadyExistsErr(err) {
			c.JSON(http.StatusConflict, gin.H{"error": "domain already exists"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if in.HasEnabled && !in.Enabled {
		if err := h.db.UpdateDomain(dom.ID, dom.Name, dom.Tags, dom.Metadata, dom.SourceType, dom.SourceRef, dom.CustomCAPEM, dom.CheckMode, dom.DNSServers, false, dom.CheckInterval, dom.Port, dom.FolderID); err != nil {
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
	if dom != nil {
		h.audit(c, "create", "domain", &dom.ID, "Created domain", map[string]any{
			"name":       dom.Name,
			"enabled":    dom.Enabled,
			"check_mode": dom.CheckMode,
		})
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
	sourceType := current.SourceType
	if in.HasSourceType {
		sourceType = in.SourceType
	}
	sourceRef := db.CloneSourceRef(current.SourceRef)
	if in.HasSourceRef {
		sourceRef = db.CloneSourceRef(in.SourceRef)
	}
	sourceRef, err = db.ValidateAndNormalizeSourceRef(sourceType, sourceRef)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	newName = normalizeInventoryName(sourceType, newName)
	if newName == "" {
		newName = db.SourceDisplayName(sourceType, sourceRef)
	}
	if newName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name is required"})
		return
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
		sourceType != current.SourceType ||
		!equalStringMaps(sourceRef, current.SourceRef) ||
		checkMode != current.CheckMode ||
		dnsServers != current.DNSServers ||
		customCAPEM != current.CustomCAPEM ||
		port != current.Port ||
		(!current.Enabled && enabled)

	if err := h.db.UpdateDomain(id, newName, tags, metadata, sourceType, sourceRef, customCAPEM, checkMode, dnsServers, enabled, interval, port, folderID); err != nil {
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
	if dom != nil {
		h.audit(c, "update", "domain", &dom.ID, "Updated domain", map[string]any{
			"before_name": current.Name,
			"after_name":  dom.Name,
			"enabled":     dom.Enabled,
			"check_mode":  dom.CheckMode,
		})
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
		wrapped := fmt.Errorf("defaults: %w", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": wrapped.Error()})
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

			created, err := h.db.CreateDomain(preview.Name, preview.Tags, preview.Metadata, preview.SourceType, preview.SourceRef, preview.CustomCAPEM, preview.CheckMode, preview.DNSServers, preview.CheckInterval, preview.Port, preview.FolderID)
			if err != nil {
				result.Action = "failed"
				result.Error = err.Error()
				resp.Summary.Failed++
				resp.Results = append(resp.Results, result)
				continue
			}

			if !preview.Enabled {
				if err := h.db.UpdateDomain(created.ID, created.Name, created.Tags, created.Metadata, created.SourceType, created.SourceRef, created.CustomCAPEM, created.CheckMode, created.DNSServers, false, created.CheckInterval, created.Port, created.FolderID); err != nil {
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

		if err := h.db.UpdateDomain(current.ID, preview.Name, preview.Tags, preview.Metadata, preview.SourceType, preview.SourceRef, preview.CustomCAPEM, preview.CheckMode, preview.DNSServers, preview.Enabled, preview.CheckInterval, preview.Port, preview.FolderID); err != nil {
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
	if !req.DryRun {
		h.audit(c, "import", "domain", nil, "Imported domain batch", map[string]any{
			"mode":    resp.Mode,
			"created": resp.Summary.Created,
			"updated": resp.Summary.Updated,
			"failed":  resp.Summary.Failed,
			"skipped": resp.Summary.Skipped,
			"total":   resp.Summary.Total,
		})
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
	if dom == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}

	if err := h.db.DeleteDomain(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if dom != nil && h.metrics != nil {
		h.metrics.CleanupDomain(dom.Name)
		h.refreshTotalDomainsMetric()
	}
	if dom != nil {
		h.audit(c, "delete", "domain", &dom.ID, "Deleted domain", map[string]any{
			"name": dom.Name,
		})
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

	if h.scheduler == nil {
		result := h.checker.CheckDomain(dom)
		h.audit(c, "trigger_check", "domain", &dom.ID, "Triggered immediate domain check", map[string]any{
			"name":   dom.Name,
			"mode":   "inline",
			"status": result.OverallStatus,
		})
		c.JSON(http.StatusOK, result)
		return
	}

	accepted := h.scheduler.TriggerCheck(dom)
	if !accepted {
		c.JSON(http.StatusAccepted, gin.H{
			"accepted":        false,
			"already_running": true,
			"domain_id":       dom.ID,
			"name":            dom.Name,
		})
		return
	}
	h.audit(c, "trigger_check", "domain", &dom.ID, "Queued manual domain check", map[string]any{
		"name": dom.Name,
		"mode": "async",
	})
	c.JSON(http.StatusAccepted, gin.H{
		"accepted":        true,
		"already_running": false,
		"domain_id":       dom.ID,
		"name":            dom.Name,
	})
}

// POST /api/domains/:id/notify
func (h *Handler) SendAdHocNotification(c *gin.Context) {
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

	var req adHocNotificationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	recipients, recipientsSet, err := parseOptionalStringListJSON(req.Recipients, "recipients")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	emailRecipientsSet := recipientsSet
	if emailTo, set, err := parseOptionalStringListJSON(req.EmailTo, "email_to"); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	} else if set {
		recipients = emailTo
		emailRecipientsSet = true
	}

	channels, err := normalizeNotificationChannels(req.Channels)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	lastCheck, _ := h.db.GetLastCheck(id)

	cfg := h.cfg.Snapshot()
	if !cfg.Features.Notifications {
		c.JSON(http.StatusBadRequest, gin.H{"error": "notifications feature is not enabled"})
		return
	}

	// Build the notification message
	message := strings.TrimSpace(req.Message)
	if message == "" && lastCheck != nil {
		message = checker.BuildAdHocMessage(dom.Name, lastCheck)
	} else if message == "" {
		message = fmt.Sprintf("Ad-hoc notification for domain %s", dom.Name)
	}

	subject := strings.TrimSpace(req.Subject)
	if subject == "" {
		prefix := strings.TrimSpace(cfg.Notifications.Email.SubjectPrefix)
		if prefix == "" {
			prefix = "[SSL Domain Exporter]"
		}
		status := "unknown"
		if lastCheck != nil {
			status = lastCheck.OverallStatus
		}
		subject = fmt.Sprintf("%s Ad-hoc alert: %s (status: %s)", prefix, dom.Name, strings.ToUpper(status))
	}

	results := make([]gin.H, 0)

	// Determine which channels to send to
	if len(channels) == 0 {
		channels = defaultAdHocNotificationChannels(cfg, req, recipients, emailRecipientsSet)
	}
	if len(channels) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no notification channels are configured for this request"})
		return
	}

	timeout := checker.NotificationTimeoutFromConfig(cfg)

	for _, ch := range channels {
		switch ch {
		case "email":
			emailCfg := cfg.Notifications.Email
			channelRecipients := recipients
			if len(channelRecipients) == 0 {
				channelRecipients = compactStrings(emailCfg.To)
			}
			emailCfg.To = channelRecipients
			if strings.TrimSpace(emailCfg.Host) == "" || emailCfg.Port <= 0 || len(channelRecipients) == 0 {
				results = append(results, gin.H{"channel": "email", "success": false, "error": "email not configured or no recipients"})
				continue
			}
			err := checker.SendEmailDirect(emailCfg, subject, message, timeout)
			entry := gin.H{"channel": "email", "success": err == nil, "recipients": channelRecipients}
			if err != nil {
				entry["error"] = err.Error()
			}
			results = append(results, entry)

		case "webhook":
			webhookURL := strings.TrimSpace(req.WebhookURL)
			if webhookURL == "" {
				webhookURL = strings.TrimSpace(cfg.Notifications.Webhook.URL)
			}
			if webhookURL == "" {
				results = append(results, gin.H{"channel": "webhook", "success": false, "error": "webhook not configured"})
				continue
			}
			err := checker.SendWebhookDirect(webhookURL, message, timeout)
			entry := gin.H{"channel": "webhook", "success": err == nil}
			if err != nil {
				entry["error"] = err.Error()
			}
			results = append(results, entry)

		case "telegram":
			botToken := strings.TrimSpace(req.TelegramBotToken)
			if botToken == "" {
				botToken = strings.TrimSpace(cfg.Notifications.Telegram.BotToken)
			}
			chatID := strings.TrimSpace(req.TelegramChatID)
			if chatID == "" {
				chatID = strings.TrimSpace(cfg.Notifications.Telegram.ChatID)
			}
			if botToken == "" || chatID == "" {
				results = append(results, gin.H{"channel": "telegram", "success": false, "error": "telegram not configured"})
				continue
			}
			err := checker.SendTelegramDirect(botToken, chatID, message, timeout)
			entry := gin.H{"channel": "telegram", "success": err == nil}
			if err != nil {
				entry["error"] = err.Error()
			}
			results = append(results, entry)
		}
	}

	h.audit(c, "notify", "domain", &dom.ID, "Sent ad-hoc notification", map[string]any{
		"domain":     dom.Name,
		"channels":   channels,
		"recipients": recipients,
	})

	c.JSON(http.StatusOK, gin.H{
		"domain":  dom.Name,
		"results": results,
	})
}

func parseOptionalStringListJSON(raw json.RawMessage, field string) ([]string, bool, error) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return nil, false, nil
	}

	var one string
	if err := json.Unmarshal(raw, &one); err == nil {
		return compactStrings([]string{one}), true, nil
	}

	var many []string
	if err := json.Unmarshal(raw, &many); err == nil {
		return compactStrings(many), true, nil
	}

	return nil, false, fmt.Errorf("%s must be a string or an array of strings", field)
}

func normalizeNotificationChannels(values []string) ([]string, error) {
	if len(values) == 0 {
		return nil, nil
	}

	seen := make(map[string]struct{}, len(values))
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		channel := strings.ToLower(strings.TrimSpace(value))
		switch channel {
		case "email", "webhook", "telegram":
		case "":
			continue
		default:
			return nil, fmt.Errorf("channels must only contain email, webhook, or telegram")
		}
		if _, ok := seen[channel]; ok {
			continue
		}
		seen[channel] = struct{}{}
		normalized = append(normalized, channel)
	}
	return normalized, nil
}

func defaultAdHocNotificationChannels(cfg *config.Config, req adHocNotificationRequest, recipients []string, recipientsSet bool) []string {
	channels := make([]string, 0, 3)
	if cfg == nil {
		return channels
	}

	emailRecipients := recipients
	if !recipientsSet && len(emailRecipients) == 0 {
		emailRecipients = compactStrings(cfg.Notifications.Email.To)
	}
	if (cfg.Notifications.Email.Enabled || recipientsSet) &&
		strings.TrimSpace(cfg.Notifications.Email.Host) != "" &&
		cfg.Notifications.Email.Port > 0 &&
		len(emailRecipients) > 0 {
		channels = append(channels, "email")
	}

	if webhookURL := strings.TrimSpace(req.WebhookURL); webhookURL != "" || (cfg.Notifications.Webhook.Enabled && strings.TrimSpace(cfg.Notifications.Webhook.URL) != "") {
		channels = append(channels, "webhook")
	}

	botToken := strings.TrimSpace(req.TelegramBotToken)
	chatID := strings.TrimSpace(req.TelegramChatID)
	if (botToken != "" && chatID != "") ||
		(cfg.Notifications.Telegram.Enabled &&
			strings.TrimSpace(cfg.Notifications.Telegram.BotToken) != "" &&
			strings.TrimSpace(cfg.Notifications.Telegram.ChatID) != "") {
		channels = append(channels, "telegram")
	}

	return channels
}

func compactStrings(values []string) []string {
	compacted := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			compacted = append(compacted, value)
		}
	}
	return compacted
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
	h.audit(c, "create", "folder", &folder.ID, "Created folder", map[string]any{
		"name": folder.Name,
	})
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
	h.audit(c, "update", "folder", &folder.ID, "Updated folder", map[string]any{
		"name": folder.Name,
	})
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
	h.audit(c, "delete", "folder", &id, "Deleted folder", nil)
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
		"id", "domain", "port", "folder_id", "tags", "metadata", "source_type", "source_ref", "custom_ca", "check_mode", "dns_servers", "enabled", "status",
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
			d.EffectiveSourceType(),
			db.SourceRefSearchText(d.SourceRef),
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
			row[12] = chk.OverallStatus
			if chk.SSLExpiryDays != nil {
				row[13] = strconv.Itoa(*chk.SSLExpiryDays)
			}
			if chk.RegistrationCheckSkipped {
				row[14] = "N/A"
				row[15] = "true"
			} else {
				if chk.DomainExpiryDays != nil {
					row[14] = strconv.Itoa(*chk.DomainExpiryDays)
				}
				row[15] = "false"
			}
			row[16] = strconv.Itoa(chk.HTTPStatusCode)
			row[17] = strconv.FormatInt(chk.HTTPResponseTimeMs, 10)
			row[18] = strconv.FormatBool(chk.HTTPRedirectsHTTPS)
			row[19] = strconv.FormatBool(chk.HTTPHSTSEnabled)
			row[20] = chk.CheckedAt.Format("2006-01-02 15:04:05")
		}
		for _, field := range exportFields {
			row = append(row, d.Metadata[field.Key])
		}
		_ = w.Write(row)
	}
}

// GET /api/config
func (h *Handler) GetConfig(c *gin.Context) {
	c.JSON(http.StatusOK, h.cfg.RedactedSnapshot())
}

// PUT /api/config
func (h *Handler) UpdateConfig(c *gin.Context) {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to read request body"})
		return
	}

	current := h.cfg.Snapshot()
	next := current.Clone()
	if err := json.Unmarshal(body, next); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	next.RestoreRedactedSecrets(current)
	if err := next.Validate(); err != nil {
		writeConfigValidationError(c, err)
		return
	}

	h.cfg.ApplyFrom(next)
	if err := h.cfg.Save(); err != nil {
		wrapped := fmt.Errorf("save config: %w", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": wrapped.Error()})
		return
	}
	h.audit(c, "update", "config", nil, "Updated application config", map[string]any{
		"sections": changedConfigSections(current, next),
	})

	c.JSON(http.StatusOK, h.cfg.RedactedSnapshot())
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

	var req notificationTestRequest
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to read request body"})
		return
	}
	if trimmed := strings.TrimSpace(string(body)); trimmed != "" {
		if err := json.Unmarshal(body, &req); err != nil && !errors.Is(err, io.EOF) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
	}

	current := h.cfg.Snapshot()
	effective := current
	if req.Features != nil || req.Notifications != nil {
		effective = current.Clone()
		if req.Features != nil {
			effective.Features.Notifications = req.Features.Notifications
		}
		if req.Notifications != nil {
			effective.Notifications = *req.Notifications
		}
		effective.RestoreRedactedSecrets(current)
		if err := effective.ValidateNotificationsOnly(); err != nil {
			writeConfigValidationError(c, err)
			return
		}
	}

	results, err := h.checker.SendTestNotifications(req.Channel, effective)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if results == nil {
		results = []checker.TestResult{}
	}
	c.JSON(http.StatusOK, results)
}

// GET /api/k8s/certificates
func (h *Handler) ScanK8SCertificates(c *gin.Context) {
	cfg := h.cfg.Snapshot()
	if !cfg.Kubernetes.Enabled {
		c.JSON(http.StatusBadRequest, gin.H{"error": "kubernetes monitoring is not enabled"})
		return
	}

	k8sCfg := checker.K8SConfig{
		Enabled:            cfg.Kubernetes.Enabled,
		APIServer:          cfg.Kubernetes.APIServer,
		Token:              cfg.Kubernetes.Token,
		TokenFile:          cfg.Kubernetes.TokenFile,
		Namespace:          cfg.Kubernetes.Namespace,
		LabelSelector:      cfg.Kubernetes.LabelSelector,
		InsecureSkipVerify: cfg.Kubernetes.InsecureSkipVerify,
		CACertFile:         cfg.Kubernetes.CACertFile,
	}

	warningDays := cfg.Alerts.SSLExpiryWarningDays
	result, err := checker.ScanK8SCertificates(k8sCfg, warningDays)
	if err != nil && result == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, result)
}

// GET /api/f5/certificates
func (h *Handler) ScanF5Certificates(c *gin.Context) {
	cfg := h.cfg.Snapshot()
	if !cfg.F5.Enabled {
		c.JSON(http.StatusBadRequest, gin.H{"error": "F5 monitoring is not enabled"})
		return
	}

	f5Cfg := checker.F5Config{
		Enabled:            cfg.F5.Enabled,
		Host:               cfg.F5.Host,
		Username:           cfg.F5.Username,
		Password:           cfg.F5.Password,
		InsecureSkipVerify: cfg.F5.InsecureSkipVerify,
		Partition:          cfg.F5.Partition,
	}

	warningDays := cfg.Alerts.SSLExpiryWarningDays
	result, err := checker.ScanF5Certificates(f5Cfg, warningDays)
	if err != nil && result == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, result)
}

// POST /api/syslog/test
func (h *Handler) TestSyslog(c *gin.Context) {
	cfg := h.cfg.Snapshot()

	testCfg := cfg.Logging.Syslog
	// Allow overriding syslog settings from the request body for pre-save testing
	if c.Request.ContentLength > 0 {
		if err := c.ShouldBindJSON(&testCfg); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
	}

	if !testCfg.Enabled {
		c.JSON(http.StatusBadRequest, gin.H{"error": "syslog is not enabled in the provided configuration"})
		return
	}

	if err := config.TestSyslog(testCfg); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "success": false})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Test syslog message sent successfully"})
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
		"notifications_webhook_url":       redactLegacySecret(cfg.Notifications.Webhook.URL),
		"webhook_on_critical":             cfg.Notifications.Webhook.OnCritical,
		"webhook_on_warning":              cfg.Notifications.Webhook.OnWarning,
		"telegram_enabled":                cfg.Notifications.Telegram.Enabled,
		"telegram_bot_token":              redactLegacySecret(cfg.Notifications.Telegram.BotToken),
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
			n, err := parseCompatInt(k, v)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			cfg.Checker.ConcurrentChecks = n
		case "prometheus_enabled":
			b, err := parseCompatBool(k, v)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			cfg.Prometheus.Enabled = b
		case "prometheus_path":
			cfg.Prometheus.Path = v
		case "alert_domain_expiry_warning":
			n, err := parseCompatInt(k, v)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			cfg.Alerts.DomainExpiryWarningDays = n
		case "alert_domain_expiry_critical":
			n, err := parseCompatInt(k, v)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			cfg.Alerts.DomainExpiryCriticalDays = n
		case "alert_ssl_expiry_warning":
			n, err := parseCompatInt(k, v)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			cfg.Alerts.SSLExpiryWarningDays = n
		case "alert_ssl_expiry_critical":
			n, err := parseCompatInt(k, v)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			cfg.Alerts.SSLExpiryCriticalDays = n
		case "notifications_enabled":
			b, err := parseCompatBool(k, v)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			cfg.Features.Notifications = b
		case "notifications_webhook_url":
			if strings.TrimSpace(v) == config.RedactedSecret {
				cfg.Notifications.Webhook.URL = h.cfg.Snapshot().Notifications.Webhook.URL
			} else {
				cfg.Notifications.Webhook.URL = v
			}
		case "webhook_on_critical":
			b, err := parseCompatBool(k, v)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			cfg.Notifications.Webhook.OnCritical = b
		case "webhook_on_warning":
			b, err := parseCompatBool(k, v)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			cfg.Notifications.Webhook.OnWarning = b
		case "telegram_enabled":
			b, err := parseCompatBool(k, v)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			cfg.Notifications.Telegram.Enabled = b
		case "telegram_bot_token":
			if strings.TrimSpace(v) == config.RedactedSecret {
				cfg.Notifications.Telegram.BotToken = h.cfg.Snapshot().Notifications.Telegram.BotToken
			} else {
				cfg.Notifications.Telegram.BotToken = v
			}
		case "telegram_chat_id":
			cfg.Notifications.Telegram.ChatID = v
		case "telegram_on_critical":
			b, err := parseCompatBool(k, v)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			cfg.Notifications.Telegram.OnCritical = b
		case "telegram_on_warning":
			b, err := parseCompatBool(k, v)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			cfg.Notifications.Telegram.OnWarning = b
		case "feature_http_check":
			b, err := parseCompatBool(k, v)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			cfg.Features.HTTPCheck = b
		case "feature_cipher_check":
			b, err := parseCompatBool(k, v)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			cfg.Features.CipherCheck = b
		case "feature_ocsp_check":
			b, err := parseCompatBool(k, v)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			cfg.Features.OCSPCheck = b
		case "feature_crl_check":
			b, err := parseCompatBool(k, v)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			cfg.Features.CRLCheck = b
		case "feature_caa_check":
			b, err := parseCompatBool(k, v)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			cfg.Features.CAACheck = b
		case "feature_csv_export":
			b, err := parseCompatBool(k, v)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			cfg.Features.CSVExport = b
		case "feature_timeline_view":
			b, err := parseCompatBool(k, v)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			cfg.Features.TimelineView = b
		case "feature_dashboard_tag_filter":
			b, err := parseCompatBool(k, v)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			cfg.Features.DashboardTagFilter = b
		case "feature_structured_logs":
			b, err := parseCompatBool(k, v)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			cfg.Features.StructuredLogs = b
		case "domain_subdomain_fallback":
			b, err := parseCompatBool(k, v)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			cfg.Domains.SubdomainFallback = b
		case "domain_subdomain_fallback_depth":
			n, err := parseCompatInt(k, v)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			cfg.Domains.FallbackDepth = n
		case "domain_default_check_mode":
			cfg.Domains.DefaultCheckMode = v
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
			b, err := parseCompatBool(k, v)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			cfg.DNS.UseSystemDNS = b
		case "dns_timeout":
			cfg.DNS.Timeout = v
		case "notifications_timeout":
			cfg.Notifications.Timeout = v
		}
	}
	if err := cfg.Validate(); err != nil {
		writeConfigValidationError(c, err)
		return
	}

	h.cfg.ApplyFrom(cfg)
	if err := h.cfg.Save(); err != nil {
		wrapped := fmt.Errorf("save config: %w", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": wrapped.Error()})
		return
	}
	if len(req) > 0 {
		keys := make([]string, 0, len(req))
		for key := range req {
			keys = append(keys, key)
		}
		h.audit(c, "update", "config", nil, "Updated compatibility settings", map[string]any{
			"keys": keys,
		})
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

	counts := map[string]int{
		"total":    len(domains),
		"ok":       0,
		"warning":  0,
		"critical": 0,
		"error":    0,
		"unknown":  0,
	}

	errorDomains := make([]gin.H, 0)

	for _, dom := range domains {
		if chk, ok := lastChecks[dom.ID]; ok {
			status := normalizeSummaryStatus(chk.OverallStatus)
			counts[status]++
			if status == "error" {
				entry := gin.H{
					"id":     dom.ID,
					"name":   dom.Name,
					"reason": chk.PrimaryReasonText,
				}
				if chk.SSLCheckError != "" {
					entry["ssl_error"] = chk.SSLCheckError
				}
				if chk.DomainCheckError != "" {
					entry["domain_error"] = chk.DomainCheckError
				}
				errorDomains = append(errorDomains, entry)
			}
		} else {
			counts["unknown"]++
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"total":         counts["total"],
		"ok":            counts["ok"],
		"warning":       counts["warning"],
		"critical":      counts["critical"],
		"error":         counts["error"],
		"unknown":       counts["unknown"],
		"error_domains": errorDomains,
	})
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

func normalizeSummaryStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "ok", "warning", "critical", "error":
		return strings.ToLower(strings.TrimSpace(status))
	default:
		return "unknown"
	}
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
		out.SourceType = db.DomainSourceManual
		out.SourceRef = map[string]string{}
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
	if in.HasSourceType {
		out.SourceType = in.SourceType
	}
	if in.HasSourceRef {
		out.SourceRef = db.CloneSourceRef(in.SourceRef)
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
	out.SourceType = db.NormalizeSourceType(out.SourceType)
	if normalizedSourceRef, err := db.ValidateAndNormalizeSourceRef(out.SourceType, out.SourceRef); err == nil {
		out.SourceRef = normalizedSourceRef
	}
	out.Name = normalizeInventoryName(out.SourceType, out.Name)
	if out.Name == "" {
		out.Name = db.SourceDisplayName(out.SourceType, out.SourceRef)
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
	out.SourceRef = db.CloneSourceRef(src.SourceRef)
	out.FolderID = cloneFolderID(src.FolderID)
	return &out
}

func equalStringMaps(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for key, value := range a {
		if b[key] != value {
			return false
		}
	}
	return true
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
