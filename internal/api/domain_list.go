package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"ssl-domain-exporter/internal/db"
)

type domainListResponse struct {
	Items      []db.Domain `json:"items"`
	Total      int         `json:"total"`
	Page       int         `json:"page"`
	PageSize   int         `json:"page_size"`
	TotalPages int         `json:"total_pages"`
	SortBy     string      `json:"sort_by"`
	SortDir    string      `json:"sort_dir"`
}

func (h *Handler) SearchDomains(c *gin.Context) {
	query, err := buildDomainListQuery(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	items, total, err := h.db.ListDomainsPage(query)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	totalPages := 0
	if total > 0 && query.PageSize > 0 {
		totalPages = (total + query.PageSize - 1) / query.PageSize
	}

	c.JSON(http.StatusOK, domainListResponse{
		Items:      items,
		Total:      total,
		Page:       query.Page,
		PageSize:   query.PageSize,
		TotalPages: totalPages,
		SortBy:     query.SortBy,
		SortDir:    query.SortDir,
	})
}

func buildDomainListQuery(c *gin.Context) (db.DomainListQuery, error) {
	sortBy := normalizeSortBy(c.DefaultQuery("sort_by", "custom"))
	sortDir := normalizeSortDir(c.DefaultQuery("sort_dir", "asc"))
	page := normalizePage(c.DefaultQuery("page", "1"))
	pageSize := normalizePageSize(c.DefaultQuery("page_size", "20"))

	query := db.DomainListQuery{
		Search:   strings.TrimSpace(c.Query("search")),
		Status:   strings.ToLower(strings.TrimSpace(c.Query("status"))),
		Tag:      strings.TrimSpace(c.Query("tag")),
		SortBy:   sortBy,
		SortDir:  strings.ToLower(sortDir),
		Page:     page,
		PageSize: pageSize,
	}

	if raw := strings.TrimSpace(c.Query("metadata_filters")); raw != "" {
		var filters map[string]string
		if err := json.Unmarshal([]byte(raw), &filters); err != nil {
			return query, fmt.Errorf("metadata_filters must be a JSON object")
		}
		normalized, err := db.ValidateAndNormalizeMetadata(filters)
		if err != nil {
			return query, err
		}
		query.MetadataFilters = normalized
	}

	if raw := strings.TrimSpace(c.Query("folder_id")); raw != "" && raw != "all" {
		parsed, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || parsed <= 0 {
			return query, fmt.Errorf("folder_id must be a positive integer")
		}
		query.FolderID = &parsed
	}

	if value, err := parseOptionalPositiveInt(c.Query("ssl_expiry_lte")); err != nil {
		return query, err
	} else {
		query.SSLExpiryLTE = value
	}

	if value, err := parseOptionalPositiveInt(c.Query("domain_expiry_lte")); err != nil {
		return query, err
	} else {
		query.DomainExpiryLTE = value
	}

	return query, nil
}

func normalizeSortBy(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "custom", "status", "ssl_expiry", "domain_expiry", "last_check", "created_at", "name":
		return strings.ToLower(strings.TrimSpace(raw))
	default:
		return "custom"
	}
}

func normalizeSortDir(raw string) string {
	if strings.EqualFold(strings.TrimSpace(raw), "desc") {
		return "desc"
	}
	return "asc"
}

func normalizePage(raw string) int {
	page, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || page <= 0 {
		return 1
	}
	return page
}

func normalizePageSize(raw string) int {
	size, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || size <= 0 {
		return 20
	}
	if size > 100 {
		return 100
	}
	return size
}

func parseOptionalPositiveInt(raw string) (*int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 0 {
		return nil, fmt.Errorf("value must be zero or a positive integer")
	}
	return &value, nil
}
