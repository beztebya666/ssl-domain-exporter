package api

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"ssl-domain-exporter/internal/db"
)

type timelinePageResponse struct {
	Items      []db.TimelineEntry `json:"items"`
	Total      int                `json:"total"`
	Page       int                `json:"page"`
	PageSize   int                `json:"page_size"`
	TotalPages int                `json:"total_pages"`
}

type timelineResponse struct {
	Summary db.TimelineSummary   `json:"summary"`
	SSL     timelinePageResponse `json:"ssl"`
	Domain  timelinePageResponse `json:"domain"`
}

func (h *Handler) GetTimeline(c *gin.Context) {
	cfg := h.cfg.Snapshot()
	sslPage := normalizePage(c.DefaultQuery("ssl_page", "1"))
	domainPage := normalizePage(c.DefaultQuery("domain_page", "1"))
	sslPageSize := normalizePageSize(c.DefaultQuery("ssl_page_size", "20"))
	domainPageSize := normalizePageSize(c.DefaultQuery("domain_page_size", "20"))

	summary, err := h.db.GetTimelineSummary(
		cfg.Alerts.SSLExpiryWarningDays,
		cfg.Alerts.SSLExpiryCriticalDays,
		cfg.Alerts.DomainExpiryWarningDays,
		cfg.Alerts.DomainExpiryCriticalDays,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	sslItems, sslTotal, err := h.db.ListTimelineEntries("ssl", sslPage, sslPageSize)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	domainItems, domainTotal, err := h.db.ListTimelineEntries("domain", domainPage, domainPageSize)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, timelineResponse{
		Summary: summary,
		SSL: timelinePageResponse{
			Items:      sslItems,
			Total:      sslTotal,
			Page:       sslPage,
			PageSize:   sslPageSize,
			TotalPages: totalPages(sslTotal, sslPageSize),
		},
		Domain: timelinePageResponse{
			Items:      domainItems,
			Total:      domainTotal,
			Page:       domainPage,
			PageSize:   domainPageSize,
			TotalPages: totalPages(domainTotal, domainPageSize),
		},
	})
}

func totalPages(total, pageSize int) int {
	if total <= 0 || pageSize <= 0 {
		return 1
	}
	pages := total / pageSize
	if total%pageSize != 0 {
		pages++
	}
	if pages < 1 {
		return 1
	}
	return pages
}
