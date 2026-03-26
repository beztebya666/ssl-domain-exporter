package db

import (
	"database/sql"
	"fmt"
	"net/mail"
	"net/url"
	"strings"
	"time"
)

type CustomField struct {
	ID               int64               `json:"id"`
	Key              string              `json:"key"`
	Label            string              `json:"label"`
	Type             string              `json:"type"`
	Required         bool                `json:"required"`
	Placeholder      string              `json:"placeholder"`
	HelpText         string              `json:"help_text"`
	SortOrder        int                 `json:"sort_order"`
	VisibleInTable   bool                `json:"visible_in_table"`
	VisibleInDetails bool                `json:"visible_in_details"`
	VisibleInExport  bool                `json:"visible_in_export"`
	Filterable       bool                `json:"filterable"`
	Enabled          bool                `json:"enabled"`
	Options          []CustomFieldOption `json:"options,omitempty"`
	CreatedAt        time.Time           `json:"created_at"`
	UpdatedAt        time.Time           `json:"updated_at"`
}

type CustomFieldOption struct {
	ID        int64     `json:"id"`
	FieldID   int64     `json:"field_id"`
	Value     string    `json:"value"`
	Label     string    `json:"label"`
	SortOrder int       `json:"sort_order"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func NormalizeCustomFieldType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "textarea":
		return "textarea"
	case "email":
		return "email"
	case "url":
		return "url"
	case "date":
		return "date"
	case "select":
		return "select"
	default:
		return "text"
	}
}

func (d *DB) ListCustomFields(includeDisabled bool) ([]CustomField, error) {
	query := `
		SELECT id, key, label, type, required, placeholder, help_text, sort_order,
		       visible_in_table, visible_in_details, visible_in_export, filterable, enabled,
		       created_at, updated_at
		FROM custom_fields
	`
	args := []any{}
	if !includeDisabled {
		query += ` WHERE enabled = 1`
	}
	query += ` ORDER BY sort_order ASC, key ASC`

	rows, err := d.sql.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	fields := make([]CustomField, 0)
	ids := make([]int64, 0)
	for rows.Next() {
		field, err := scanCustomFieldRow(rows)
		if err != nil {
			return nil, err
		}
		fields = append(fields, *field)
		ids = append(ids, field.ID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	optionsByField, err := d.listCustomFieldOptions(ids)
	if err != nil {
		return nil, err
	}
	for i := range fields {
		fields[i].Options = optionsByField[fields[i].ID]
	}
	return fields, nil
}

func (d *DB) GetCustomFieldByID(id int64) (*CustomField, error) {
	row := d.sql.QueryRow(`
		SELECT id, key, label, type, required, placeholder, help_text, sort_order,
		       visible_in_table, visible_in_details, visible_in_export, filterable, enabled,
		       created_at, updated_at
		FROM custom_fields
		WHERE id = ?
	`, id)
	field, err := scanCustomField(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	optionsByField, err := d.listCustomFieldOptions([]int64{id})
	if err != nil {
		return nil, err
	}
	field.Options = optionsByField[id]
	return field, nil
}

func (d *DB) CreateCustomField(field CustomField) (*CustomField, error) {
	normalized, err := NormalizeCustomField(field)
	if err != nil {
		return nil, err
	}

	sortOrder, err := d.nextCustomFieldSortOrder()
	if err != nil {
		return nil, err
	}
	if normalized.SortOrder <= 0 {
		normalized.SortOrder = sortOrder
	}

	tx, err := d.sql.Begin()
	if err != nil {
		return nil, err
	}
	defer rollbackTx(tx)

	res, err := tx.Exec(`
		INSERT INTO custom_fields (
			key, label, type, required, placeholder, help_text, sort_order,
			visible_in_table, visible_in_details, visible_in_export, filterable, enabled
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		normalized.Key, normalized.Label, normalized.Type, normalized.Required, normalized.Placeholder, normalized.HelpText,
		normalized.SortOrder, normalized.VisibleInTable, normalized.VisibleInDetails, normalized.VisibleInExport, normalized.Filterable, normalized.Enabled,
	)
	if err != nil {
		return nil, err
	}
	fieldID, _ := res.LastInsertId()
	if err := insertCustomFieldOptions(tx, fieldID, normalized.Options); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return d.GetCustomFieldByID(fieldID)
}

func (d *DB) UpdateCustomField(id int64, field CustomField) (*CustomField, error) {
	normalized, err := NormalizeCustomField(field)
	if err != nil {
		return nil, err
	}

	current, err := d.GetCustomFieldByID(id)
	if err != nil {
		return nil, err
	}
	if current == nil {
		return nil, nil
	}
	if normalized.SortOrder <= 0 {
		normalized.SortOrder = current.SortOrder
	}

	tx, err := d.sql.Begin()
	if err != nil {
		return nil, err
	}
	defer rollbackTx(tx)

	if _, err := tx.Exec(`
		UPDATE custom_fields
		SET key = ?, label = ?, type = ?, required = ?, placeholder = ?, help_text = ?, sort_order = ?,
		    visible_in_table = ?, visible_in_details = ?, visible_in_export = ?, filterable = ?, enabled = ?,
		    updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`,
		normalized.Key, normalized.Label, normalized.Type, normalized.Required, normalized.Placeholder, normalized.HelpText,
		normalized.SortOrder, normalized.VisibleInTable, normalized.VisibleInDetails, normalized.VisibleInExport, normalized.Filterable,
		normalized.Enabled, id,
	); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(`DELETE FROM custom_field_options WHERE field_id = ?`, id); err != nil {
		return nil, err
	}
	if err := insertCustomFieldOptions(tx, id, normalized.Options); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return d.GetCustomFieldByID(id)
}

func (d *DB) DeleteCustomField(id int64) error {
	_, err := d.sql.Exec(`DELETE FROM custom_fields WHERE id = ?`, id)
	return err
}

func NormalizeCustomField(field CustomField) (CustomField, error) {
	field.Key = strings.ToLower(strings.TrimSpace(field.Key))
	field.Label = strings.TrimSpace(field.Label)
	field.Type = NormalizeCustomFieldType(field.Type)
	field.Placeholder = strings.TrimSpace(field.Placeholder)
	field.HelpText = strings.TrimSpace(field.HelpText)
	if field.Key == "" {
		return field, fmt.Errorf("custom field key is required")
	}
	if !metadataKeyRE.MatchString(field.Key) {
		return field, fmt.Errorf("invalid custom field key %q: use letters, numbers, dot, dash, underscore", field.Key)
	}
	if field.Label == "" {
		field.Label = defaultCustomFieldLabel(field.Key)
	}
	if field.Type == "" {
		field.Type = "text"
	}
	field.Options = normalizeCustomFieldOptions(field.Type, field.Options)
	if field.Type == "select" && len(field.Options) == 0 {
		return field, fmt.Errorf("select fields require at least one option")
	}
	return field, nil
}

func ValidateMetadataWithCustomFields(metadata map[string]string, fields []CustomField) (map[string]string, error) {
	normalized, err := ValidateAndNormalizeMetadata(metadata)
	if err != nil {
		return nil, err
	}
	fieldMap := make(map[string]CustomField, len(fields))
	for _, field := range fields {
		if !field.Enabled {
			continue
		}
		fieldMap[field.Key] = field
	}

	for _, field := range fields {
		if !field.Enabled || !field.Required {
			continue
		}
		if strings.TrimSpace(normalized[field.Key]) == "" {
			return nil, fmt.Errorf("%s is required", field.Label)
		}
	}

	for key, value := range normalized {
		field, ok := fieldMap[key]
		if !ok {
			continue
		}
		if err := validateCustomFieldValue(field, value); err != nil {
			return nil, err
		}
	}
	return normalized, nil
}

func validateCustomFieldValue(field CustomField, value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		if field.Required {
			return fmt.Errorf("%s is required", field.Label)
		}
		return nil
	}

	switch field.Type {
	case "email":
		if _, err := mail.ParseAddress(value); err != nil {
			return fmt.Errorf("%s must be a valid email address", field.Label)
		}
	case "url":
		parsed, err := url.ParseRequestURI(value)
		if err != nil || parsed.Scheme == "" || parsed.Host == "" {
			return fmt.Errorf("%s must be a valid URL", field.Label)
		}
	case "date":
		if _, err := time.Parse("2006-01-02", value); err != nil {
			return fmt.Errorf("%s must use YYYY-MM-DD format", field.Label)
		}
	case "select":
		allowed := make(map[string]struct{}, len(field.Options))
		for _, option := range field.Options {
			allowed[option.Value] = struct{}{}
		}
		if _, ok := allowed[value]; !ok {
			return fmt.Errorf("%s must be one of the configured options", field.Label)
		}
	}
	return nil
}

func scanCustomField(row interface{ Scan(dest ...any) error }) (*CustomField, error) {
	var field CustomField
	err := row.Scan(
		&field.ID, &field.Key, &field.Label, &field.Type, &field.Required, &field.Placeholder, &field.HelpText,
		&field.SortOrder, &field.VisibleInTable, &field.VisibleInDetails, &field.VisibleInExport,
		&field.Filterable, &field.Enabled, &field.CreatedAt, &field.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &field, nil
}

func scanCustomFieldRow(rows *sql.Rows) (*CustomField, error) {
	return scanCustomField(rows)
}

func scanCustomFieldOption(row interface{ Scan(dest ...any) error }) (*CustomFieldOption, error) {
	var option CustomFieldOption
	err := row.Scan(&option.ID, &option.FieldID, &option.Value, &option.Label, &option.SortOrder, &option.CreatedAt, &option.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &option, nil
}

func (d *DB) listCustomFieldOptions(fieldIDs []int64) (map[int64][]CustomFieldOption, error) {
	result := make(map[int64][]CustomFieldOption, len(fieldIDs))
	if len(fieldIDs) == 0 {
		return result, nil
	}

	placeholders := make([]string, len(fieldIDs))
	args := make([]any, 0, len(fieldIDs))
	for i, id := range fieldIDs {
		placeholders[i] = "?"
		args = append(args, id)
	}

	//nolint:gosec // Placeholder list is constructed from argument count only; values remain parameterized.
	rows, err := d.sql.Query(`
		SELECT id, field_id, value, label, sort_order, created_at, updated_at
		FROM custom_field_options
		WHERE field_id IN (`+strings.Join(placeholders, ",")+`)
		ORDER BY sort_order ASC, value ASC
	`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		option, err := scanCustomFieldOption(rows)
		if err != nil {
			return nil, err
		}
		result[option.FieldID] = append(result[option.FieldID], *option)
	}
	return result, rows.Err()
}

func (d *DB) nextCustomFieldSortOrder() (int, error) {
	var sortOrder int
	if err := d.sql.QueryRow(`SELECT coalesce(MAX(sort_order), 0) + 1 FROM custom_fields`).Scan(&sortOrder); err != nil {
		return 0, err
	}
	return sortOrder, nil
}

func insertCustomFieldOptions(tx *sql.Tx, fieldID int64, options []CustomFieldOption) error {
	for idx, option := range normalizeCustomFieldOptions("select", options) {
		if _, err := tx.Exec(`
			INSERT INTO custom_field_options (field_id, value, label, sort_order)
			VALUES (?, ?, ?, ?)
		`, fieldID, option.Value, option.Label, idx+1); err != nil {
			return err
		}
	}
	return nil
}

func normalizeCustomFieldOptions(fieldType string, options []CustomFieldOption) []CustomFieldOption {
	if fieldType != "select" || len(options) == 0 {
		return nil
	}

	seen := map[string]struct{}{}
	result := make([]CustomFieldOption, 0, len(options))
	for _, option := range options {
		option.Value = strings.TrimSpace(option.Value)
		option.Label = strings.TrimSpace(option.Label)
		if option.Value == "" {
			continue
		}
		if option.Label == "" {
			option.Label = option.Value
		}
		if _, ok := seen[option.Value]; ok {
			continue
		}
		seen[option.Value] = struct{}{}
		result = append(result, CustomFieldOption{
			Value: option.Value,
			Label: option.Label,
		})
	}
	return result
}

func defaultCustomFieldLabel(key string) string {
	key = strings.ReplaceAll(key, ".", " ")
	key = strings.ReplaceAll(key, "_", " ")
	parts := strings.Fields(key)
	for i, part := range parts {
		if part == "" {
			continue
		}
		parts[i] = strings.ToUpper(part[:1]) + part[1:]
	}
	return strings.Join(parts, " ")
}
