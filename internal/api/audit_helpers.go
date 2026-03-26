package api

import (
	"log/slog"
	"reflect"

	"github.com/gin-gonic/gin"

	"ssl-domain-exporter/internal/config"
	"ssl-domain-exporter/internal/db"
)

func (h *Handler) GetAuditLogs(c *gin.Context) {
	logs, err := h.db.ListAuditLogs(200)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	if logs == nil {
		logs = []db.AuditLog{}
	}
	c.JSON(200, logs)
}

func (h *Handler) audit(c *gin.Context, action, resource string, resourceID *int64, summary string, details map[string]any) {
	if h == nil || h.db == nil {
		return
	}
	principal := GetPrincipal(c)
	entry := db.AuditLog{
		ActorUsername: principal.Username,
		ActorRole:     principal.Role,
		ActorSource:   principal.Source,
		Action:        action,
		Resource:      resource,
		ResourceID:    resourceID,
		Summary:       summary,
		Details:       details,
		RemoteAddr:    c.ClientIP(),
		RequestID:     GetRequestID(c),
	}
	if principal.UserID > 0 {
		userID := principal.UserID
		entry.ActorUserID = &userID
	}
	if entry.ActorUsername == "" {
		entry.ActorUsername = "anonymous"
	}
	if entry.ActorRole == "" {
		entry.ActorRole = "anonymous"
	}
	if entry.ActorSource == "" {
		entry.ActorSource = "request"
	}
	if err := h.db.CreateAuditLog(entry); err != nil {
		slog.Warn("Failed to persist audit entry", "resource", resource, "action", action, "error", err)
	}
}

func changedConfigSections(current, next *config.Config) []string {
	if current == nil || next == nil {
		return nil
	}
	sections := make([]string, 0, 12)
	if !reflect.DeepEqual(current.Server, next.Server) {
		sections = append(sections, "server")
	}
	if !reflect.DeepEqual(current.Database, next.Database) {
		sections = append(sections, "database")
	}
	if !reflect.DeepEqual(current.Auth, next.Auth) {
		sections = append(sections, "auth")
	}
	if !reflect.DeepEqual(current.Checker, next.Checker) {
		sections = append(sections, "checker")
	}
	if !reflect.DeepEqual(current.Features, next.Features) {
		sections = append(sections, "features")
	}
	if !reflect.DeepEqual(current.Alerts, next.Alerts) {
		sections = append(sections, "alerts")
	}
	if !reflect.DeepEqual(current.StatusPolicy, next.StatusPolicy) {
		sections = append(sections, "status_policy")
	}
	if !reflect.DeepEqual(current.Notifications, next.Notifications) {
		sections = append(sections, "notifications")
	}
	if !reflect.DeepEqual(current.Domains, next.Domains) {
		sections = append(sections, "domains")
	}
	if !reflect.DeepEqual(current.DNS, next.DNS) {
		sections = append(sections, "dns")
	}
	if !reflect.DeepEqual(current.Security, next.Security) {
		sections = append(sections, "security")
	}
	if !reflect.DeepEqual(current.Prometheus, next.Prometheus) {
		sections = append(sections, "prometheus")
	}
	if !reflect.DeepEqual(current.Maintenance, next.Maintenance) {
		sections = append(sections, "maintenance")
	}
	if !reflect.DeepEqual(current.Logging, next.Logging) {
		sections = append(sections, "logging")
	}
	return sections
}
