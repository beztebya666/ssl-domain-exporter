package api

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"ssl-domain-exporter/internal/config"
	"ssl-domain-exporter/internal/db"
)

type customFieldOptionRequest struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

type customFieldRequest struct {
	Key              string                     `json:"key"`
	Label            string                     `json:"label"`
	Type             string                     `json:"type"`
	Required         bool                       `json:"required"`
	Placeholder      string                     `json:"placeholder"`
	HelpText         string                     `json:"help_text"`
	SortOrder        int                        `json:"sort_order"`
	VisibleInTable   bool                       `json:"visible_in_table"`
	VisibleInDetails bool                       `json:"visible_in_details"`
	VisibleInExport  bool                       `json:"visible_in_export"`
	Filterable       bool                       `json:"filterable"`
	Enabled          bool                       `json:"enabled"`
	Options          []customFieldOptionRequest `json:"options"`
}

func (h *Handler) ListCustomFields(c *gin.Context) {
	includeDisabled := config.ParseBool(c.Query("include_disabled")) && GetPrincipal(c).CanAdmin()
	fields, err := h.db.ListCustomFields(includeDisabled)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if fields == nil {
		fields = []db.CustomField{}
	}
	c.JSON(http.StatusOK, fields)
}

func (h *Handler) CreateCustomField(c *gin.Context) {
	var req customFieldRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	field, err := h.db.CreateCustomField(customFieldFromRequest(req))
	if err != nil {
		if isCustomFieldAlreadyExistsErr(err) {
			c.JSON(http.StatusConflict, gin.H{"error": "custom field key already exists"})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	h.audit(c, "create", "custom_field", &field.ID, "Created custom field", map[string]any{
		"key":   field.Key,
		"label": field.Label,
		"type":  field.Type,
	})
	c.JSON(http.StatusCreated, field)
}

func (h *Handler) UpdateCustomField(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var req customFieldRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	field, err := h.db.UpdateCustomField(id, customFieldFromRequest(req))
	if err != nil {
		if isCustomFieldAlreadyExistsErr(err) {
			c.JSON(http.StatusConflict, gin.H{"error": "custom field key already exists"})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if field == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	h.audit(c, "update", "custom_field", &field.ID, "Updated custom field", map[string]any{
		"key":   field.Key,
		"label": field.Label,
		"type":  field.Type,
	})
	c.JSON(http.StatusOK, field)
}

func (h *Handler) DeleteCustomField(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	if err := h.db.DeleteCustomField(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	h.audit(c, "delete", "custom_field", &id, "Deleted custom field", nil)
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func customFieldFromRequest(req customFieldRequest) db.CustomField {
	options := make([]db.CustomFieldOption, 0, len(req.Options))
	for idx, option := range req.Options {
		options = append(options, db.CustomFieldOption{
			Value:     option.Value,
			Label:     option.Label,
			SortOrder: idx + 1,
		})
	}
	return db.CustomField{
		Key:              req.Key,
		Label:            req.Label,
		Type:             req.Type,
		Required:         req.Required,
		Placeholder:      req.Placeholder,
		HelpText:         req.HelpText,
		SortOrder:        req.SortOrder,
		VisibleInTable:   req.VisibleInTable,
		VisibleInDetails: req.VisibleInDetails,
		VisibleInExport:  req.VisibleInExport,
		Filterable:       req.Filterable,
		Enabled:          req.Enabled,
		Options:          options,
	}
}

func isCustomFieldAlreadyExistsErr(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unique constraint failed") && strings.Contains(msg, "custom_fields.key")
}

func (h *Handler) validateDomainMetadata(metadata map[string]string) (map[string]string, error) {
	fields, err := h.db.ListCustomFields(false)
	if err != nil {
		return nil, err
	}
	return db.ValidateMetadataWithCustomFields(metadata, fields)
}

func visibleExportCustomFields(fields []db.CustomField) []db.CustomField {
	out := make([]db.CustomField, 0, len(fields))
	for _, field := range fields {
		if field.Enabled && field.VisibleInExport {
			out = append(out, field)
		}
	}
	return out
}
