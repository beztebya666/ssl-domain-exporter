package db

import (
	"database/sql"
	"fmt"
	"log"
	"sort"
	"strings"
)

type DomainListQuery struct {
	Search          string
	Status          string
	Tag             string
	FolderID        *int64
	MetadataFilters map[string]string
	SSLExpiryLTE    *int
	DomainExpiryLTE *int
	SortBy          string
	SortDir         string
	Page            int
	PageSize        int
}

func (d *DB) ListDomainsPage(query DomainListQuery) ([]Domain, int, error) {
	ids, total, err := d.searchDomainIDs(query)
	if err != nil {
		return nil, 0, err
	}
	if len(ids) == 0 {
		return []Domain{}, total, nil
	}

	domains, err := d.GetDomainsByIDs(ids)
	if err != nil {
		return nil, 0, err
	}
	lastChecks, err := d.GetLastChecksByDomainIDs(ids)
	if err != nil {
		return nil, 0, err
	}
	for i := range domains {
		if chk, ok := lastChecks[domains[i].ID]; ok {
			domains[i].LastCheck = chk
		}
	}
	return domains, total, nil
}

func (d *DB) ListDomainsForExport(query DomainListQuery) ([]Domain, error) {
	query.Page = 1
	query.PageSize = 0

	ids, _, err := d.searchDomainIDs(query)
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return []Domain{}, nil
	}

	domains, err := d.GetDomainsByIDs(ids)
	if err != nil {
		return nil, err
	}
	lastChecks, err := d.GetLastChecksByDomainIDs(ids)
	if err != nil {
		return nil, err
	}
	for i := range domains {
		if chk, ok := lastChecks[domains[i].ID]; ok {
			domains[i].LastCheck = chk
		}
	}
	return domains, nil
}

func (d *DB) GetDomainsByIDs(ids []int64) ([]Domain, error) {
	if len(ids) == 0 {
		return []Domain{}, nil
	}

	placeholders := make([]string, len(ids))
	args := make([]any, 0, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args = append(args, id)
	}

	rows, err := d.sql.Query(`SELECT `+domainSelectCols+` FROM domains WHERE id IN (`+strings.Join(placeholders, ",")+`)`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	domains, err := scanDomainRows(rows)
	if err != nil {
		return nil, err
	}

	byID := make(map[int64]Domain, len(domains))
	for _, domain := range domains {
		byID[domain.ID] = domain
	}

	ordered := make([]Domain, 0, len(ids))
	for _, id := range ids {
		if domain, ok := byID[id]; ok {
			ordered = append(ordered, domain)
		}
	}
	return ordered, nil
}

func (d *DB) GetLastChecksByDomainIDs(ids []int64) (map[int64]*Check, error) {
	if len(ids) == 0 {
		return map[int64]*Check{}, nil
	}

	placeholders := make([]string, len(ids))
	args := make([]any, 0, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args = append(args, id)
	}

	rows, err := d.sql.Query(`
		WITH latest AS (
			SELECT domain_id, MAX(checked_at) AS max_at
			FROM domain_checks
			WHERE domain_id IN (`+strings.Join(placeholders, ",")+`)
			GROUP BY domain_id
		)
		SELECT `+prefixCols("dc.", checkSelectCols)+`
		FROM domain_checks dc
		INNER JOIN latest ON latest.domain_id = dc.domain_id AND latest.max_at = dc.checked_at
	`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[int64]*Check, len(ids))
	for rows.Next() {
		check, err := d.scanCheckRow(rows)
		if err != nil {
			return nil, err
		}
		result[check.DomainID] = check
	}
	return result, rows.Err()
}

func (d *DB) CountDomains() (int, error) {
	var count int
	if err := d.sql.QueryRow(`SELECT COUNT(*) FROM domains`).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func (d *DB) searchDomainIDs(query DomainListQuery) ([]int64, int, error) {
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

	whereSQL, args := buildDomainListWhere(query)

	countSQL := cte + `
		SELECT COUNT(*)
		FROM domains d
		LEFT JOIN latest_checks lc ON lc.domain_id = d.id
	` + whereSQL

	var total int
	if err := d.sql.QueryRow(countSQL, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	orderSQL := buildDomainListOrder(query)
	querySQL := cte + `
		SELECT d.id
		FROM domains d
		LEFT JOIN latest_checks lc ON lc.domain_id = d.id
	` + whereSQL + orderSQL

	queryArgs := append([]any(nil), args...)
	if query.PageSize > 0 {
		page := query.Page
		if page <= 0 {
			page = 1
		}
		offset := (page - 1) * query.PageSize
		if offset < 0 {
			offset = 0
		}
		querySQL += ` LIMIT ? OFFSET ?`
		queryArgs = append(queryArgs, query.PageSize, offset)
	}

	rows, err := d.sql.Query(querySQL, queryArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	ids := make([]int64, 0)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, 0, err
		}
		ids = append(ids, id)
	}
	return ids, total, rows.Err()
}

func buildDomainListWhere(query DomainListQuery) (string, []any) {
	clauses := make([]string, 0, 6)
	args := make([]any, 0, 10)

	if search := strings.ToLower(strings.TrimSpace(query.Search)); search != "" {
		pattern := "%" + search + "%"
		clauses = append(clauses, `(lower(d.name) LIKE ? OR lower(coalesce(d.tags, '')) LIKE ? OR lower(coalesce(d.metadata_json, '')) LIKE ?)`)
		args = append(args, pattern, pattern, pattern)
	}

	if status := strings.ToLower(strings.TrimSpace(query.Status)); status != "" && status != "all" {
		if status == "unknown" {
			clauses = append(clauses, `(lc.domain_id IS NULL OR lower(coalesce(lc.overall_status, '')) = '' OR lower(lc.overall_status) = 'unknown')`)
		} else {
			clauses = append(clauses, `lower(coalesce(lc.overall_status, '')) = ?`)
			args = append(args, status)
		}
	}

	if tag := strings.ToLower(strings.TrimSpace(query.Tag)); tag != "" && tag != "all" {
		clauses = append(clauses, `(',' || replace(lower(coalesce(d.tags, '')), ' ', '') || ',') LIKE ?`)
		args = append(args, "%,"+strings.ReplaceAll(tag, " ", "")+",%")
	}

	if query.FolderID != nil {
		clauses = append(clauses, `d.folder_id = ?`)
		args = append(args, *query.FolderID)
	}

	if len(query.MetadataFilters) > 0 {
		keys := make([]string, 0, len(query.MetadataFilters))
		for key := range query.MetadataFilters {
			keys = append(keys, key)
		}
		sortStrings(keys)
		for _, key := range keys {
			clauses = append(clauses, `json_extract(coalesce(d.metadata_json, '{}'), ?) = ?`)
			args = append(args, "$."+key, query.MetadataFilters[key])
		}
	}

	if query.SSLExpiryLTE != nil {
		clauses = append(clauses, `lc.ssl_expiry_days IS NOT NULL AND lc.ssl_expiry_days <= ?`)
		args = append(args, *query.SSLExpiryLTE)
	}

	if query.DomainExpiryLTE != nil {
		clauses = append(clauses, `coalesce(lc.registration_check_skipped, 0) = 0 AND lc.domain_expiry_days IS NOT NULL AND lc.domain_expiry_days <= ?`)
		args = append(args, *query.DomainExpiryLTE)
	}

	if len(clauses) == 0 {
		return "", args
	}
	return " WHERE " + strings.Join(clauses, " AND "), args
}

func buildDomainListOrder(query DomainListQuery) string {
	sortBy := strings.ToLower(strings.TrimSpace(query.SortBy))
	sortDir := strings.ToUpper(strings.TrimSpace(query.SortDir))
	if sortDir != "DESC" {
		sortDir = "ASC"
	}

	switch sortBy {
	case "status":
		return fmt.Sprintf(`
		 ORDER BY
			CASE lower(coalesce(lc.overall_status, 'unknown'))
				WHEN 'critical' THEN 0
				WHEN 'error' THEN 1
				WHEN 'warning' THEN 2
				WHEN 'ok' THEN 3
				ELSE 4
			END %s,
			lower(d.name) ASC`, sortDir)
	case "ssl_expiry":
		return fmt.Sprintf(`
		 ORDER BY
			CASE WHEN lc.ssl_expiry_days IS NULL THEN 1 ELSE 0 END ASC,
			lc.ssl_expiry_days %s,
			lower(d.name) ASC`, sortDir)
	case "domain_expiry":
		return fmt.Sprintf(`
		 ORDER BY
			CASE WHEN coalesce(lc.registration_check_skipped, 0) = 1 OR lc.domain_expiry_days IS NULL THEN 1 ELSE 0 END ASC,
			lc.domain_expiry_days %s,
			lower(d.name) ASC`, sortDir)
	case "last_check":
		return fmt.Sprintf(`
		 ORDER BY
			CASE WHEN lc.checked_at IS NULL THEN 1 ELSE 0 END ASC,
			lc.checked_at %s,
			lower(d.name) ASC`, sortDir)
	case "created_at":
		return fmt.Sprintf(` ORDER BY d.created_at %s, lower(d.name) ASC`, sortDir)
	case "custom":
		return fmt.Sprintf(` ORDER BY d.sort_order %s, lower(d.name) ASC`, sortDir)
	default:
		return fmt.Sprintf(` ORDER BY lower(d.name) %s`, sortDir)
	}
}

func nullInt64(value *int) sql.NullInt64 {
	if value == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: int64(*value), Valid: true}
}

func (d *DB) ListTags() ([]string, error) {
	rows, err := d.sql.Query(`SELECT tags FROM domains WHERE trim(coalesce(tags, '')) <> ''`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	set := make(map[string]struct{})
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		for _, tag := range ParseLegacyTags(raw) {
			if tag == "" {
				continue
			}
			set[tag] = struct{}{}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	tags := make([]string, 0, len(set))
	for tag := range set {
		tags = append(tags, tag)
	}
	sortStrings(tags)
	return tags, nil
}

func sortStrings(values []string) {
	if len(values) < 2 {
		return
	}
	sort.Slice(values, func(i, j int) bool { return strings.ToLower(values[i]) < strings.ToLower(values[j]) })
	if len(values) > 5000 {
		log.Printf("[db] unusually high tag cardinality detected: %d distinct tags", len(values))
	}
}
