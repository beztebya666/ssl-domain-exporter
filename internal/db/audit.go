package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

type AuditLog struct {
	ID            int64          `json:"id"`
	ActorUserID   *int64         `json:"actor_user_id,omitempty"`
	ActorUsername string         `json:"actor_username"`
	ActorRole     string         `json:"actor_role"`
	ActorSource   string         `json:"actor_source"`
	Action        string         `json:"action"`
	Resource      string         `json:"resource"`
	ResourceID    *int64         `json:"resource_id,omitempty"`
	Summary       string         `json:"summary"`
	Details       map[string]any `json:"details,omitempty"`
	RemoteAddr    string         `json:"remote_addr"`
	RequestID     string         `json:"request_id"`
	CreatedAt     time.Time      `json:"created_at"`
}

func (d *DB) CreateAuditLog(entry AuditLog) error {
	if d == nil || d.sql == nil {
		return fmt.Errorf("database is not initialized")
	}
	detailsJSON := "{}"
	if len(entry.Details) > 0 {
		data, err := json.Marshal(entry.Details)
		if err != nil {
			return fmt.Errorf("marshal audit details: %w", err)
		}
		detailsJSON = string(data)
	}
	_, err := d.sql.Exec(`
		INSERT INTO audit_logs (
			actor_user_id, actor_username, actor_role, actor_source,
			action, resource, resource_id, summary, details_json,
			remote_addr, request_id
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		entry.ActorUserID,
		entry.ActorUsername,
		entry.ActorRole,
		entry.ActorSource,
		entry.Action,
		entry.Resource,
		entry.ResourceID,
		entry.Summary,
		detailsJSON,
		entry.RemoteAddr,
		entry.RequestID,
	)
	return err
}

func (d *DB) ListAuditLogs(limit int) ([]AuditLog, error) {
	if d == nil || d.sql == nil {
		return nil, fmt.Errorf("database is not initialized")
	}
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}
	rows, err := d.sql.Query(`
		SELECT id, actor_user_id, actor_username, actor_role, actor_source,
		       action, resource, resource_id, summary, details_json,
		       remote_addr, request_id, created_at
		FROM audit_logs
		ORDER BY id DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]AuditLog, 0, limit)
	for rows.Next() {
		var item AuditLog
		var actorUserID sql.NullInt64
		var resourceID sql.NullInt64
		var detailsJSON string
		if err := rows.Scan(
			&item.ID,
			&actorUserID,
			&item.ActorUsername,
			&item.ActorRole,
			&item.ActorSource,
			&item.Action,
			&item.Resource,
			&resourceID,
			&item.Summary,
			&detailsJSON,
			&item.RemoteAddr,
			&item.RequestID,
			&item.CreatedAt,
		); err != nil {
			return nil, err
		}
		if actorUserID.Valid {
			v := actorUserID.Int64
			item.ActorUserID = &v
		}
		if resourceID.Valid {
			v := resourceID.Int64
			item.ResourceID = &v
		}
		if detailsJSON != "" && detailsJSON != "{}" {
			if err := json.Unmarshal([]byte(detailsJSON), &item.Details); err != nil {
				return nil, fmt.Errorf("unmarshal audit details: %w", err)
			}
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (d *DB) DeleteAuditLogsOlderThan(cutoff time.Time) (int64, error) {
	if d == nil || d.sql == nil {
		return 0, fmt.Errorf("database is not initialized")
	}
	result, err := d.sql.Exec(`DELETE FROM audit_logs WHERE created_at < ?`, cutoff.UTC())
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}
