package db

import "strings"

type TimelineEntry struct {
	DomainID int64  `json:"domain_id"`
	Name     string `json:"name"`
	Kind     string `json:"kind"`
	Days     int    `json:"days"`
	Issuer   string `json:"issuer,omitempty"`
}

type TimelineSummary struct {
	SSLCritical    int `json:"ssl_critical"`
	SSLWarning     int `json:"ssl_warning"`
	DomainCritical int `json:"domain_critical"`
	DomainWarning  int `json:"domain_warning"`
}

func (d *DB) ListTimelineEntries(kind string, page, pageSize int) ([]TimelineEntry, int, error) {
	kind = strings.ToLower(strings.TrimSpace(kind))
	if kind != "domain" {
		kind = "ssl"
	}
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	offset := (page - 1) * pageSize
	if offset < 0 {
		offset = 0
	}

	cte := `
		WITH latest_checks AS (
			SELECT *
			FROM (
				SELECT dc.*, ROW_NUMBER() OVER (PARTITION BY dc.domain_id ORDER BY dc.checked_at DESC, dc.id DESC) AS rn
				FROM domain_checks dc
			)
			WHERE rn = 1
		)
	`

	var countSQL string
	var querySQL string
	var args []any
	if kind == "domain" {
		countSQL = cte + `
			SELECT COUNT(*)
			FROM domains d
			INNER JOIN latest_checks lc ON lc.domain_id = d.id
			WHERE coalesce(lc.registration_check_skipped, 0) = 0 AND lc.domain_expiry_days IS NOT NULL
		`
		querySQL = cte + `
			SELECT d.id, d.name, lc.domain_expiry_days
			FROM domains d
			INNER JOIN latest_checks lc ON lc.domain_id = d.id
			WHERE coalesce(lc.registration_check_skipped, 0) = 0 AND lc.domain_expiry_days IS NOT NULL
			ORDER BY lc.domain_expiry_days ASC, lower(d.name) ASC
			LIMIT ? OFFSET ?
		`
		args = []any{pageSize, offset}
	} else {
		countSQL = cte + `
			SELECT COUNT(*)
			FROM domains d
			INNER JOIN latest_checks lc ON lc.domain_id = d.id
			WHERE lc.ssl_expiry_days IS NOT NULL
		`
		querySQL = cte + `
			SELECT d.id, d.name, lc.ssl_expiry_days, coalesce(lc.ssl_issuer, '')
			FROM domains d
			INNER JOIN latest_checks lc ON lc.domain_id = d.id
			WHERE lc.ssl_expiry_days IS NOT NULL
			ORDER BY lc.ssl_expiry_days ASC, lower(d.name) ASC
			LIMIT ? OFFSET ?
		`
		args = []any{pageSize, offset}
	}

	var total int
	if err := d.sql.QueryRow(countSQL).Scan(&total); err != nil {
		return nil, 0, err
	}

	rows, err := d.sql.Query(querySQL, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	items := make([]TimelineEntry, 0, pageSize)
	for rows.Next() {
		var item TimelineEntry
		item.Kind = kind
		if kind == "domain" {
			if err := rows.Scan(&item.DomainID, &item.Name, &item.Days); err != nil {
				return nil, 0, err
			}
		} else {
			if err := rows.Scan(&item.DomainID, &item.Name, &item.Days, &item.Issuer); err != nil {
				return nil, 0, err
			}
		}
		items = append(items, item)
	}

	return items, total, rows.Err()
}

func (d *DB) GetTimelineSummary(sslWarning, sslCritical, domainWarning, domainCritical int) (TimelineSummary, error) {
	cte := `
		WITH latest_checks AS (
			SELECT *
			FROM (
				SELECT dc.*, ROW_NUMBER() OVER (PARTITION BY dc.domain_id ORDER BY dc.checked_at DESC, dc.id DESC) AS rn
				FROM domain_checks dc
			)
			WHERE rn = 1
		)
	`

	var summary TimelineSummary
	err := d.sql.QueryRow(cte+`
		SELECT
			COALESCE(SUM(CASE WHEN lc.ssl_expiry_days IS NOT NULL AND lc.ssl_expiry_days <= ? THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN lc.ssl_expiry_days IS NOT NULL AND lc.ssl_expiry_days > ? AND lc.ssl_expiry_days <= ? THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN coalesce(lc.registration_check_skipped, 0) = 0 AND lc.domain_expiry_days IS NOT NULL AND lc.domain_expiry_days <= ? THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN coalesce(lc.registration_check_skipped, 0) = 0 AND lc.domain_expiry_days IS NOT NULL AND lc.domain_expiry_days > ? AND lc.domain_expiry_days <= ? THEN 1 ELSE 0 END), 0)
		FROM domains d
		LEFT JOIN latest_checks lc ON lc.domain_id = d.id
	`, sslCritical, sslCritical, sslWarning, domainCritical, domainCritical, domainWarning).
		Scan(&summary.SSLCritical, &summary.SSLWarning, &summary.DomainCritical, &summary.DomainWarning)
	return summary, err
}
