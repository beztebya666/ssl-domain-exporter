package api

import (
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"ssl-domain-exporter/internal/db"
)

type pruneChecksRequest struct {
	Days int `json:"days"`
}

func (h *Handler) Health(c *gin.Context) {
	statusCode, payload := h.healthPayload()
	c.JSON(statusCode, payload)
}

func (h *Handler) Readiness(c *gin.Context) {
	statusCode, payload := h.healthPayload()
	if payload["database"] != "ok" {
		c.JSON(http.StatusServiceUnavailable, payload)
		return
	}
	if scheduler, ok := payload["scheduler"].(map[string]any); ok {
		if started, _ := scheduler["started"].(bool); !started {
			c.JSON(http.StatusServiceUnavailable, payload)
			return
		}
	}
	c.JSON(statusCode, payload)
}

func (h *Handler) ListBackups(c *gin.Context) {
	cfg := h.cfg.Snapshot()
	files, err := db.ListBackupFiles(cfg.Maintenance.BackupsDir)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, files)
}

func (h *Handler) CreateBackup(c *gin.Context) {
	cfg := h.cfg.Snapshot()
	filename := "checker-" + time.Now().UTC().Format("20060102-150405") + ".db"
	dest := filepath.Join(cfg.Maintenance.BackupsDir, filename)
	if err := h.db.BackupTo(dest); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	files, err := db.ListBackupFiles(cfg.Maintenance.BackupsDir)
	if err != nil {
		c.JSON(http.StatusCreated, gin.H{"name": filename, "path": dest})
		return
	}
	for _, file := range files {
		if strings.EqualFold(file.Path, dest) {
			h.audit(c, "create", "backup", nil, "Created sqlite backup", map[string]any{
				"path": file.Path,
				"name": file.Name,
			})
			c.JSON(http.StatusCreated, file)
			return
		}
	}
	h.audit(c, "create", "backup", nil, "Created sqlite backup", map[string]any{
		"path": dest,
		"name": filename,
	})
	c.JSON(http.StatusCreated, gin.H{"name": filename, "path": dest})
}

func (h *Handler) PruneChecks(c *gin.Context) {
	var req pruneChecksRequest
	if err := c.ShouldBindJSON(&req); err != nil && err != io.EOF {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	days := req.Days
	if days <= 0 {
		days = h.cfg.Snapshot().Maintenance.CheckRetentionDays
	}
	if days <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "retention days must be greater than 0"})
		return
	}
	cutoff := time.Now().AddDate(0, 0, -days)
	removed, err := h.db.DeleteChecksOlderThan(cutoff)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	h.audit(c, "prune", "domain_checks", nil, "Pruned historical domain checks", map[string]any{
		"days":    days,
		"cutoff":  cutoff.UTC().Format(time.RFC3339),
		"removed": removed,
	})
	c.JSON(http.StatusOK, gin.H{
		"ok":      true,
		"days":    days,
		"cutoff":  cutoff.UTC(),
		"removed": removed,
	})
}

func (h *Handler) healthPayload() (int, gin.H) {
	payload := gin.H{
		"status":   "ok",
		"database": "ok",
	}
	if err := h.db.Ping(); err != nil {
		payload["status"] = "degraded"
		payload["database"] = err.Error()
	}

	if h.scheduler != nil {
		schedulerStatus := h.scheduler.Status()
		payload["scheduler"] = gin.H{
			"started":                 schedulerStatus.Started,
			"in_flight":               schedulerStatus.InFlight,
			"last_error":              schedulerStatus.LastError,
			"last_tick_at":            schedulerStatus.LastTickAt,
			"last_session_cleanup_at": schedulerStatus.LastSessionCleanupAt,
			"last_retention_sweep_at": schedulerStatus.LastRetentionSweepAt,
			"last_audit_sweep_at":     schedulerStatus.LastAuditSweepAt,
		}
		if schedulerStatus.LastError != "" || !schedulerStatus.Started {
			payload["status"] = "degraded"
		}
	} else {
		payload["scheduler"] = gin.H{"started": false}
	}

	if payload["status"] == "ok" {
		return http.StatusOK, payload
	}
	return http.StatusServiceUnavailable, payload
}
