package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type DB struct {
	sql *sql.DB
}

type Domain struct {
	ID            int64             `json:"id"`
	Name          string            `json:"name"`
	Port          int               `json:"port"`
	Enabled       bool              `json:"enabled"`
	CheckInterval int               `json:"check_interval"` // seconds
	Tags          []string          `json:"tags"`
	Metadata      map[string]string `json:"metadata,omitempty"`
	FolderID      *int64            `json:"folder_id,omitempty"`
	SortOrder     int               `json:"sort_order"`
	CustomCAPEM   string            `json:"custom_ca_pem"`
	CheckMode     string            `json:"check_mode"`  // "full" or "ssl_only"
	DNSServers    string            `json:"dns_servers"` // comma-separated custom DNS servers for this domain
	CreatedAt     time.Time         `json:"created_at"`
	UpdatedAt     time.Time         `json:"updated_at"`
	LastCheck     *Check            `json:"last_check,omitempty"`
}

// EffectiveCheckMode returns the check mode, defaulting to "full" if empty.
func (d *Domain) EffectiveCheckMode() string {
	if d.CheckMode == "ssl_only" {
		return "ssl_only"
	}
	return "full"
}

// RegistrationCheckEnabled returns true when RDAP/WHOIS should be performed.
func (d *Domain) RegistrationCheckEnabled() bool {
	return d.EffectiveCheckMode() == "full"
}

// ParseDNSServers returns the per-domain DNS servers as a slice.
func (d *Domain) ParseDNSServers() []string {
	if strings.TrimSpace(d.DNSServers) == "" {
		return nil
	}
	parts := strings.Split(d.DNSServers, ",")
	result := make([]string, 0, len(parts))
	for _, s := range parts {
		s = strings.TrimSpace(s)
		if s != "" {
			result = append(result, s)
		}
	}
	return result
}

type Folder struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	SortOrder int       `json:"sort_order"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Check struct {
	ID        int64     `json:"id"`
	DomainID  int64     `json:"domain_id"`
	CheckedAt time.Time `json:"checked_at"`

	// Domain info
	DomainStatus     string     `json:"domain_status"`
	DomainRegistrar  string     `json:"domain_registrar"`
	DomainCreatedAt  *time.Time `json:"domain_created_at"`
	DomainExpiresAt  *time.Time `json:"domain_expires_at"`
	DomainExpiryDays *int       `json:"domain_expiry_days"`
	DomainCheckError string     `json:"domain_check_error"`
	DomainSource     string     `json:"domain_source"` // rdap/whois/skipped/failed

	// Audit: registration check state at time of check
	RegistrationCheckSkipped bool   `json:"registration_check_skipped"`
	RegistrationSkipReason   string `json:"registration_skip_reason"`
	DNSServerUsed            string `json:"dns_server_used"`

	// SSL info
	SSLIssuer     string     `json:"ssl_issuer"`
	SSLSubject    string     `json:"ssl_subject"`
	SSLValidFrom  *time.Time `json:"ssl_valid_from"`
	SSLValidUntil *time.Time `json:"ssl_valid_until"`
	SSLExpiryDays *int       `json:"ssl_expiry_days"`
	SSLVersion    string     `json:"ssl_version"`
	SSLCheckError string     `json:"ssl_check_error"`

	// SSL Chain
	SSLChainValid   bool        `json:"ssl_chain_valid"`
	SSLChainLength  int         `json:"ssl_chain_length"`
	SSLChainError   string      `json:"ssl_chain_error"`
	SSLChainDetails []ChainCert `json:"ssl_chain_details,omitempty"`

	// HTTP check (optional feature)
	HTTPStatusCode     int    `json:"http_status_code"`
	HTTPRedirectsHTTPS bool   `json:"http_redirects_https"`
	HTTPHSTSEnabled    bool   `json:"http_hsts_enabled"`
	HTTPHSTSMaxAge     string `json:"http_hsts_max_age"`
	HTTPResponseTimeMs int64  `json:"http_response_time_ms"`
	HTTPFinalURL       string `json:"http_final_url"`
	HTTPError          string `json:"http_error"`

	// Cipher check (optional feature)
	CipherWeak       bool   `json:"cipher_weak"`
	CipherWeakReason string `json:"cipher_weak_reason"`
	CipherGrade      string `json:"cipher_grade"`
	CipherDetails    string `json:"cipher_details"`

	// Certificate revocation (optional feature)
	OCSPStatus string `json:"ocsp_status"`
	OCSPError  string `json:"ocsp_error"`
	CRLStatus  string `json:"crl_status"`
	CRLError   string `json:"crl_error"`

	// CAA check (optional feature)
	CAAPresent     bool   `json:"caa_present"`
	CAA            string `json:"caa"`
	CAAQueryDomain string `json:"caa_query_domain"`
	CAAError       string `json:"caa_error"`

	// Overall
	OverallStatus string `json:"overall_status"` // ok, warning, critical, error
	CheckDuration int64  `json:"check_duration_ms"`
}

type ChainCert struct {
	Subject      string    `json:"subject"`
	Issuer       string    `json:"issuer"`
	ValidFrom    time.Time `json:"valid_from"`
	ValidTo      time.Time `json:"valid_to"`
	IsCA         bool      `json:"is_ca"`
	IsSelfSigned bool      `json:"is_self_signed"`
}

type Settings struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

func New(path string) (*DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, fmt.Errorf("create db dir: %w", err)
	}
	sqldb, err := sql.Open("sqlite", path+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	sqldb.SetMaxOpenConns(1)
	return &DB{sql: sqldb}, nil
}

func (d *DB) Close() error {
	return d.sql.Close()
}

func (d *DB) Migrate() error {
	_, err := d.sql.Exec(`
		CREATE TABLE IF NOT EXISTS domains (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT UNIQUE NOT NULL,
			port INTEGER DEFAULT 443,
			enabled INTEGER DEFAULT 1,
			check_interval INTEGER DEFAULT 21600,
			tags TEXT DEFAULT '',
			tags_json TEXT DEFAULT '[]',
			metadata_json TEXT DEFAULT '{}',
			folder_id INTEGER REFERENCES folders(id) ON DELETE SET NULL,
			sort_order INTEGER DEFAULT 0,
			custom_ca_pem TEXT DEFAULT '',
			check_mode TEXT DEFAULT 'full',
			dns_servers TEXT DEFAULT '',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);

		CREATE TABLE IF NOT EXISTS folders (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT UNIQUE NOT NULL,
			sort_order INTEGER DEFAULT 0,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);

		CREATE TABLE IF NOT EXISTS domain_checks (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			domain_id INTEGER REFERENCES domains(id) ON DELETE CASCADE,
			checked_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			domain_status TEXT DEFAULT '',
			domain_registrar TEXT DEFAULT '',
			domain_created_at DATETIME,
			domain_expires_at DATETIME,
			domain_expiry_days INTEGER,
			domain_check_error TEXT DEFAULT '',
			domain_source TEXT DEFAULT '',
			registration_check_skipped INTEGER DEFAULT 0,
			registration_skip_reason TEXT DEFAULT '',
			dns_server_used TEXT DEFAULT '',
			ssl_issuer TEXT DEFAULT '',
			ssl_subject TEXT DEFAULT '',
			ssl_valid_from DATETIME,
			ssl_valid_until DATETIME,
			ssl_expiry_days INTEGER,
			ssl_version TEXT DEFAULT '',
			ssl_check_error TEXT DEFAULT '',
			ssl_chain_valid INTEGER DEFAULT 0,
			ssl_chain_length INTEGER DEFAULT 0,
			ssl_chain_error TEXT DEFAULT '',
			ssl_chain_details TEXT DEFAULT '[]',
			http_status_code INTEGER DEFAULT 0,
			http_redirects_https INTEGER DEFAULT 0,
			http_hsts_enabled INTEGER DEFAULT 0,
			http_hsts_max_age TEXT DEFAULT '',
			http_response_time_ms INTEGER DEFAULT 0,
			http_final_url TEXT DEFAULT '',
			http_error TEXT DEFAULT '',
			cipher_weak INTEGER DEFAULT 0,
			cipher_weak_reason TEXT DEFAULT '',
			cipher_grade TEXT DEFAULT '',
			cipher_details TEXT DEFAULT '',
			ocsp_status TEXT DEFAULT '',
			ocsp_error TEXT DEFAULT '',
			crl_status TEXT DEFAULT '',
			crl_error TEXT DEFAULT '',
			caa_present INTEGER DEFAULT 0,
			caa TEXT DEFAULT '',
			caa_query_domain TEXT DEFAULT '',
			caa_error TEXT DEFAULT '',
			overall_status TEXT DEFAULT 'unknown',
			check_duration_ms INTEGER DEFAULT 0
		);

		CREATE TABLE IF NOT EXISTS settings (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL
		);

		CREATE INDEX IF NOT EXISTS idx_checks_domain_id ON domain_checks(domain_id);
		CREATE INDEX IF NOT EXISTS idx_checks_checked_at ON domain_checks(checked_at);
	`)
	if err != nil {
		return err
	}
	// Migrations for existing databases - domains table
	if err := d.addColumnIfMissing("domains", "port", "INTEGER DEFAULT 443"); err != nil {
		return err
	}
	if err := d.addColumnIfMissing("domains", "folder_id", "INTEGER"); err != nil {
		return err
	}
	if err := d.addColumnIfMissing("domains", "sort_order", "INTEGER DEFAULT 0"); err != nil {
		return err
	}
	if err := d.addColumnIfMissing("domains", "custom_ca_pem", "TEXT DEFAULT ''"); err != nil {
		return err
	}
	if err := d.addColumnIfMissing("domains", "tags_json", "TEXT DEFAULT '[]'"); err != nil {
		return err
	}
	if err := d.addColumnIfMissing("domains", "metadata_json", "TEXT DEFAULT '{}'"); err != nil {
		return err
	}
	if err := d.addColumnIfMissing("domains", "check_mode", "TEXT DEFAULT 'full'"); err != nil {
		return err
	}
	if err := d.addColumnIfMissing("domains", "dns_servers", "TEXT DEFAULT ''"); err != nil {
		return err
	}

	if err := d.addColumnIfMissing("folders", "sort_order", "INTEGER DEFAULT 0"); err != nil {
		return err
	}

	// Migrations for existing databases - domain_checks table
	for _, col := range []struct{ name, def string }{
		{"http_status_code", "INTEGER DEFAULT 0"},
		{"http_redirects_https", "INTEGER DEFAULT 0"},
		{"http_hsts_enabled", "INTEGER DEFAULT 0"},
		{"http_hsts_max_age", "TEXT DEFAULT ''"},
		{"http_response_time_ms", "INTEGER DEFAULT 0"},
		{"http_final_url", "TEXT DEFAULT ''"},
		{"http_error", "TEXT DEFAULT ''"},
		{"cipher_weak", "INTEGER DEFAULT 0"},
		{"cipher_weak_reason", "TEXT DEFAULT ''"},
		{"cipher_grade", "TEXT DEFAULT ''"},
		{"cipher_details", "TEXT DEFAULT ''"},
		{"ocsp_status", "TEXT DEFAULT ''"},
		{"ocsp_error", "TEXT DEFAULT ''"},
		{"crl_status", "TEXT DEFAULT ''"},
		{"crl_error", "TEXT DEFAULT ''"},
		{"caa_present", "INTEGER DEFAULT 0"},
		{"caa", "TEXT DEFAULT ''"},
		{"caa_query_domain", "TEXT DEFAULT ''"},
		{"caa_error", "TEXT DEFAULT ''"},
		{"registration_check_skipped", "INTEGER DEFAULT 0"},
		{"registration_skip_reason", "TEXT DEFAULT ''"},
		{"dns_server_used", "TEXT DEFAULT ''"},
	} {
		if err := d.addColumnIfMissing("domain_checks", col.name, col.def); err != nil {
			return err
		}
	}

	if err := d.backfillDomainSortOrder(); err != nil {
		return err
	}
	if err := d.backfillFolderSortOrder(); err != nil {
		return err
	}
	// Create indexes only after all required columns are present for old DBs.
	if _, err := d.sql.Exec(`
		CREATE INDEX IF NOT EXISTS idx_domains_sort_order ON domains(sort_order);
		CREATE INDEX IF NOT EXISTS idx_domains_folder_id ON domains(folder_id);
		CREATE INDEX IF NOT EXISTS idx_folders_sort_order ON folders(sort_order);
	`); err != nil {
		return err
	}

	return nil
}

// ---- Domain CRUD ----

const domainSelectCols = `id, name, port, enabled, check_interval, tags, tags_json, metadata_json, folder_id, sort_order, custom_ca_pem, check_mode, dns_servers, created_at, updated_at`

func (d *DB) GetDomains() ([]Domain, error) {
	rows, err := d.sql.Query(`SELECT ` + domainSelectCols + ` FROM domains ORDER BY sort_order ASC, name ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanDomainRows(rows)
}

func (d *DB) GetDomainByID(id int64) (*Domain, error) {
	var dom Domain
	var folderID sql.NullInt64
	var tagsRaw, tagsJSON, metadataJSON string
	err := d.sql.QueryRow(`SELECT `+domainSelectCols+` FROM domains WHERE id = ?`, id).
		Scan(&dom.ID, &dom.Name, &dom.Port, &dom.Enabled, &dom.CheckInterval,
			&tagsRaw, &tagsJSON, &metadataJSON, &folderID, &dom.SortOrder, &dom.CustomCAPEM, &dom.CheckMode, &dom.DNSServers, &dom.CreatedAt, &dom.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	normalizeDomainRow(&dom, tagsRaw, tagsJSON, metadataJSON, folderID)
	return &dom, nil
}

func (d *DB) CreateDomain(name string, tags []string, metadata map[string]string, customCAPEM, checkMode, dnsServers string, interval int, port int, folderID *int64) (*Domain, error) {
	if interval == 0 {
		interval = 21600
	}
	if port <= 0 {
		port = 443
	}
	if checkMode == "" {
		checkMode = "full"
	}
	tags = NormalizeTags(tags)
	tagsText := JoinTags(tags)
	metadata, err := ValidateAndNormalizeMetadata(metadata)
	if err != nil {
		return nil, err
	}
	tagsJSON, _ := json.Marshal(tags)
	metadataJSON, _ := json.Marshal(metadata)
	sortOrder, err := d.nextDomainSortOrder()
	if err != nil {
		return nil, err
	}

	res, err := d.sql.Exec(`
		INSERT INTO domains (name, port, tags, tags_json, metadata_json, folder_id, sort_order, custom_ca_pem, check_mode, dns_servers, check_interval)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		name, port, tagsText, string(tagsJSON), string(metadataJSON), folderID, sortOrder, customCAPEM, checkMode, dnsServers, interval)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return d.GetDomainByID(id)
}

func (d *DB) UpdateDomain(id int64, name string, tags []string, metadata map[string]string, customCAPEM, checkMode, dnsServers string, enabled bool, interval int, port int, folderID *int64) error {
	if port <= 0 {
		port = 443
	}
	if checkMode == "" {
		checkMode = "full"
	}
	tags = NormalizeTags(tags)
	tagsText := JoinTags(tags)
	metadata, err := ValidateAndNormalizeMetadata(metadata)
	if err != nil {
		return err
	}
	tagsJSON, _ := json.Marshal(tags)
	metadataJSON, _ := json.Marshal(metadata)
	_, err = d.sql.Exec(`
		UPDATE domains SET name=?, port=?, tags=?, tags_json=?, metadata_json=?, folder_id=?, custom_ca_pem=?, check_mode=?, dns_servers=?, enabled=?, check_interval=?, updated_at=CURRENT_TIMESTAMP
		WHERE id=?`, name, port, tagsText, string(tagsJSON), string(metadataJSON), folderID, customCAPEM, checkMode, dnsServers, enabled, interval, id)
	return err
}

func (d *DB) DeleteDomain(id int64) error {
	_, err := d.sql.Exec(`DELETE FROM domains WHERE id = ?`, id)
	return err
}

func (d *DB) GetDomainsForScheduling() ([]Domain, error) {
	rows, err := d.sql.Query(`
		SELECT ` + domainSelectCols + `
		FROM domains d
		WHERE d.enabled = 1
		AND (
			NOT EXISTS (SELECT 1 FROM domain_checks dc WHERE dc.domain_id = d.id)
			OR (
				SELECT MAX(checked_at) FROM domain_checks dc WHERE dc.domain_id = d.id
			) < datetime('now', '-' || d.check_interval || ' seconds')
		)`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanDomainRows(rows)
}

// ---- Check CRUD ----

const checkInsertCols = `domain_id, checked_at,
	domain_status, domain_registrar, domain_created_at, domain_expires_at,
	domain_expiry_days, domain_check_error, domain_source,
	registration_check_skipped, registration_skip_reason, dns_server_used,
	ssl_issuer, ssl_subject, ssl_valid_from, ssl_valid_until,
	ssl_expiry_days, ssl_version, ssl_check_error,
	ssl_chain_valid, ssl_chain_length, ssl_chain_error, ssl_chain_details,
	http_status_code, http_redirects_https, http_hsts_enabled, http_hsts_max_age, http_response_time_ms, http_final_url, http_error,
	cipher_weak, cipher_weak_reason, cipher_grade, cipher_details,
	ocsp_status, ocsp_error, crl_status, crl_error,
	caa_present, caa, caa_query_domain, caa_error,
	overall_status, check_duration_ms`

const checkSelectCols = `id, domain_id, checked_at,
	domain_status, domain_registrar, domain_created_at, domain_expires_at,
	domain_expiry_days, domain_check_error, domain_source,
	registration_check_skipped, registration_skip_reason, dns_server_used,
	ssl_issuer, ssl_subject, ssl_valid_from, ssl_valid_until,
	ssl_expiry_days, ssl_version, ssl_check_error,
	ssl_chain_valid, ssl_chain_length, ssl_chain_error, ssl_chain_details,
	http_status_code, http_redirects_https, http_hsts_enabled, http_hsts_max_age, http_response_time_ms, http_final_url, http_error,
	cipher_weak, cipher_weak_reason, cipher_grade, cipher_details,
	ocsp_status, ocsp_error, crl_status, crl_error,
	caa_present, caa, caa_query_domain, caa_error,
	overall_status, check_duration_ms`

func (d *DB) SaveCheck(c *Check) error {
	chainJSON, _ := json.Marshal(c.SSLChainDetails)
	_, err := d.sql.Exec(`INSERT INTO domain_checks (`+checkInsertCols+`) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		c.DomainID, c.CheckedAt,
		c.DomainStatus, c.DomainRegistrar, c.DomainCreatedAt, c.DomainExpiresAt,
		c.DomainExpiryDays, c.DomainCheckError, c.DomainSource,
		c.RegistrationCheckSkipped, c.RegistrationSkipReason, c.DNSServerUsed,
		c.SSLIssuer, c.SSLSubject, c.SSLValidFrom, c.SSLValidUntil,
		c.SSLExpiryDays, c.SSLVersion, c.SSLCheckError,
		c.SSLChainValid, c.SSLChainLength, c.SSLChainError, string(chainJSON),
		c.HTTPStatusCode, c.HTTPRedirectsHTTPS, c.HTTPHSTSEnabled, c.HTTPHSTSMaxAge, c.HTTPResponseTimeMs, c.HTTPFinalURL, c.HTTPError,
		c.CipherWeak, c.CipherWeakReason, c.CipherGrade, c.CipherDetails,
		c.OCSPStatus, c.OCSPError, c.CRLStatus, c.CRLError,
		c.CAAPresent, c.CAA, c.CAAQueryDomain, c.CAAError,
		c.OverallStatus, c.CheckDuration,
	)
	return err
}

func (d *DB) GetLastCheck(domainID int64) (*Check, error) {
	return d.scanCheck(d.sql.QueryRow(`SELECT `+checkSelectCols+` FROM domain_checks WHERE domain_id = ? ORDER BY checked_at DESC LIMIT 1`, domainID))
}

func (d *DB) GetCheckHistory(domainID int64, limit int) ([]Check, error) {
	if limit == 0 {
		limit = 50
	}
	rows, err := d.sql.Query(`SELECT `+checkSelectCols+` FROM domain_checks WHERE domain_id = ? ORDER BY checked_at DESC LIMIT ?`,
		domainID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	checks := []Check{}
	for rows.Next() {
		c, err := d.scanCheckRow(rows)
		if err != nil {
			return nil, err
		}
		checks = append(checks, *c)
	}
	return checks, rows.Err()
}

func (d *DB) GetAllLastChecks() (map[int64]*Check, error) {
	rows, err := d.sql.Query(`
		SELECT ` + prefixCols("dc.", checkSelectCols) + `
		FROM domain_checks dc
		INNER JOIN (
			SELECT domain_id, MAX(checked_at) as max_at FROM domain_checks GROUP BY domain_id
		) latest ON dc.domain_id = latest.domain_id AND dc.checked_at = latest.max_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[int64]*Check)
	for rows.Next() {
		c, err := d.scanCheckRow(rows)
		if err != nil {
			return nil, err
		}
		result[c.DomainID] = c
	}
	return result, rows.Err()
}

// ---- Settings ----

func (d *DB) GetSetting(key string) (string, error) {
	var val string
	err := d.sql.QueryRow(`SELECT value FROM settings WHERE key = ?`, key).Scan(&val)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return val, err
}

func (d *DB) SetSetting(key, value string) error {
	_, err := d.sql.Exec(`INSERT OR REPLACE INTO settings (key, value) VALUES (?, ?)`, key, value)
	return err
}

func (d *DB) GetAllSettings() (map[string]string, error) {
	rows, err := d.sql.Query(`SELECT key, value FROM settings`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make(map[string]string)
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		result[k] = v
	}
	return result, rows.Err()
}

// ---- Folder CRUD ----

func (d *DB) GetFolders() ([]Folder, error) {
	rows, err := d.sql.Query(`
		SELECT id, name, sort_order, created_at, updated_at
		FROM folders
		ORDER BY sort_order ASC, name ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	folders := make([]Folder, 0)
	for rows.Next() {
		var f Folder
		if err := rows.Scan(&f.ID, &f.Name, &f.SortOrder, &f.CreatedAt, &f.UpdatedAt); err != nil {
			return nil, err
		}
		folders = append(folders, f)
	}
	return folders, rows.Err()
}

func (d *DB) CreateFolder(name string) (*Folder, error) {
	sortOrder, err := d.nextFolderSortOrder()
	if err != nil {
		return nil, err
	}
	res, err := d.sql.Exec(`
		INSERT INTO folders (name, sort_order) VALUES (?, ?)
	`, name, sortOrder)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return d.GetFolderByID(id)
}

func (d *DB) GetFolderByID(id int64) (*Folder, error) {
	var f Folder
	err := d.sql.QueryRow(`
		SELECT id, name, sort_order, created_at, updated_at
		FROM folders
		WHERE id = ?
	`, id).Scan(&f.ID, &f.Name, &f.SortOrder, &f.CreatedAt, &f.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &f, nil
}

func (d *DB) UpdateFolder(id int64, name string) error {
	_, err := d.sql.Exec(`
		UPDATE folders SET name=?, updated_at=CURRENT_TIMESTAMP WHERE id=?
	`, name, id)
	return err
}

func (d *DB) DeleteFolder(id int64) error {
	tx, err := d.sql.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`UPDATE domains SET folder_id=NULL, updated_at=CURRENT_TIMESTAMP WHERE folder_id=?`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM folders WHERE id=?`, id); err != nil {
		return err
	}
	return tx.Commit()
}

func (d *DB) ReorderDomains(ids []int64) error {
	tx, err := d.sql.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for i, id := range ids {
		if _, err := tx.Exec(`UPDATE domains SET sort_order=?, updated_at=CURRENT_TIMESTAMP WHERE id=?`, i+1, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ---- Internal helpers ----

func scanDomainRows(rows *sql.Rows) ([]Domain, error) {
	domains := []Domain{}
	for rows.Next() {
		var dom Domain
		var folderID sql.NullInt64
		var tagsRaw, tagsJSON, metadataJSON string
		if err := rows.Scan(&dom.ID, &dom.Name, &dom.Port, &dom.Enabled, &dom.CheckInterval,
			&tagsRaw, &tagsJSON, &metadataJSON, &folderID, &dom.SortOrder, &dom.CustomCAPEM, &dom.CheckMode, &dom.DNSServers, &dom.CreatedAt, &dom.UpdatedAt); err != nil {
			return nil, err
		}
		normalizeDomainRow(&dom, tagsRaw, tagsJSON, metadataJSON, folderID)
		domains = append(domains, dom)
	}
	return domains, rows.Err()
}

func normalizeDomainRow(dom *Domain, tagsRaw, tagsJSON, metadataJSON string, folderID sql.NullInt64) {
	if dom.Port <= 0 {
		dom.Port = 443
	}
	if dom.CheckMode == "" {
		dom.CheckMode = "full"
	}
	if strings.TrimSpace(tagsJSON) != "" {
		_ = json.Unmarshal([]byte(tagsJSON), &dom.Tags)
	}
	if len(dom.Tags) == 0 {
		dom.Tags = ParseLegacyTags(tagsRaw)
	}
	dom.Tags = NormalizeTags(dom.Tags)
	if dom.Tags == nil {
		dom.Tags = []string{}
	}
	if strings.TrimSpace(metadataJSON) != "" {
		_ = json.Unmarshal([]byte(metadataJSON), &dom.Metadata)
	}
	if dom.Metadata != nil {
		if normalized, err := ValidateAndNormalizeMetadata(dom.Metadata); err == nil {
			dom.Metadata = normalized
		}
	}
	if dom.Metadata == nil {
		dom.Metadata = map[string]string{}
	}
	if folderID.Valid {
		v := folderID.Int64
		dom.FolderID = &v
	}
}

func (d *DB) scanCheck(row *sql.Row) (*Check, error) {
	var c Check
	var chainJSON string
	var domCreatedAt, domExpiresAt, sslValidFrom, sslValidUntil sql.NullTime
	err := row.Scan(
		&c.ID, &c.DomainID, &c.CheckedAt,
		&c.DomainStatus, &c.DomainRegistrar, &domCreatedAt, &domExpiresAt,
		&c.DomainExpiryDays, &c.DomainCheckError, &c.DomainSource,
		&c.RegistrationCheckSkipped, &c.RegistrationSkipReason, &c.DNSServerUsed,
		&c.SSLIssuer, &c.SSLSubject, &sslValidFrom, &sslValidUntil,
		&c.SSLExpiryDays, &c.SSLVersion, &c.SSLCheckError,
		&c.SSLChainValid, &c.SSLChainLength, &c.SSLChainError, &chainJSON,
		&c.HTTPStatusCode, &c.HTTPRedirectsHTTPS, &c.HTTPHSTSEnabled, &c.HTTPHSTSMaxAge, &c.HTTPResponseTimeMs, &c.HTTPFinalURL, &c.HTTPError,
		&c.CipherWeak, &c.CipherWeakReason, &c.CipherGrade, &c.CipherDetails,
		&c.OCSPStatus, &c.OCSPError, &c.CRLStatus, &c.CRLError,
		&c.CAAPresent, &c.CAA, &c.CAAQueryDomain, &c.CAAError,
		&c.OverallStatus, &c.CheckDuration,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	applyNullTimes(&c, domCreatedAt, domExpiresAt, sslValidFrom, sslValidUntil)
	json.Unmarshal([]byte(chainJSON), &c.SSLChainDetails)
	return &c, nil
}

func (d *DB) scanCheckRow(rows *sql.Rows) (*Check, error) {
	var c Check
	var chainJSON string
	var domCreatedAt, domExpiresAt, sslValidFrom, sslValidUntil sql.NullTime
	err := rows.Scan(
		&c.ID, &c.DomainID, &c.CheckedAt,
		&c.DomainStatus, &c.DomainRegistrar, &domCreatedAt, &domExpiresAt,
		&c.DomainExpiryDays, &c.DomainCheckError, &c.DomainSource,
		&c.RegistrationCheckSkipped, &c.RegistrationSkipReason, &c.DNSServerUsed,
		&c.SSLIssuer, &c.SSLSubject, &sslValidFrom, &sslValidUntil,
		&c.SSLExpiryDays, &c.SSLVersion, &c.SSLCheckError,
		&c.SSLChainValid, &c.SSLChainLength, &c.SSLChainError, &chainJSON,
		&c.HTTPStatusCode, &c.HTTPRedirectsHTTPS, &c.HTTPHSTSEnabled, &c.HTTPHSTSMaxAge, &c.HTTPResponseTimeMs, &c.HTTPFinalURL, &c.HTTPError,
		&c.CipherWeak, &c.CipherWeakReason, &c.CipherGrade, &c.CipherDetails,
		&c.OCSPStatus, &c.OCSPError, &c.CRLStatus, &c.CRLError,
		&c.CAAPresent, &c.CAA, &c.CAAQueryDomain, &c.CAAError,
		&c.OverallStatus, &c.CheckDuration,
	)
	if err != nil {
		return nil, err
	}
	applyNullTimes(&c, domCreatedAt, domExpiresAt, sslValidFrom, sslValidUntil)
	json.Unmarshal([]byte(chainJSON), &c.SSLChainDetails)
	return &c, nil
}

func applyNullTimes(c *Check, domCreatedAt, domExpiresAt, sslValidFrom, sslValidUntil sql.NullTime) {
	if domCreatedAt.Valid {
		c.DomainCreatedAt = &domCreatedAt.Time
	}
	if domExpiresAt.Valid {
		c.DomainExpiresAt = &domExpiresAt.Time
	}
	if sslValidFrom.Valid {
		c.SSLValidFrom = &sslValidFrom.Time
	}
	if sslValidUntil.Valid {
		c.SSLValidUntil = &sslValidUntil.Time
	}
}

// prefixCols adds a table prefix to every column in a comma-separated column list.
func prefixCols(prefix, cols string) string {
	parts := strings.Split(cols, ",")
	for i, p := range parts {
		parts[i] = prefix + strings.TrimSpace(p)
	}
	return strings.Join(parts, ", ")
}

func (d *DB) nextDomainSortOrder() (int, error) {
	var n sql.NullInt64
	if err := d.sql.QueryRow(`SELECT COALESCE(MAX(sort_order), 0) FROM domains`).Scan(&n); err != nil {
		return 0, err
	}
	return int(n.Int64) + 1, nil
}

func (d *DB) nextFolderSortOrder() (int, error) {
	var n sql.NullInt64
	if err := d.sql.QueryRow(`SELECT COALESCE(MAX(sort_order), 0) FROM folders`).Scan(&n); err != nil {
		return 0, err
	}
	return int(n.Int64) + 1, nil
}

func (d *DB) backfillDomainSortOrder() error {
	rows, err := d.sql.Query(`SELECT id FROM domains ORDER BY created_at ASC, id ASC`)
	if err != nil {
		return err
	}
	defer rows.Close()

	ids := make([]int64, 0)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if len(ids) == 0 {
		return nil
	}

	tx, err := d.sql.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for i, id := range ids {
		if _, err := tx.Exec(`UPDATE domains SET sort_order = ? WHERE id = ? AND (sort_order IS NULL OR sort_order <= 0)`, i+1, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (d *DB) backfillFolderSortOrder() error {
	rows, err := d.sql.Query(`SELECT id FROM folders ORDER BY created_at ASC, id ASC`)
	if err != nil {
		return nil
	}
	defer rows.Close()

	ids := make([]int64, 0)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if len(ids) == 0 {
		return nil
	}

	tx, err := d.sql.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for i, id := range ids {
		if _, err := tx.Exec(`UPDATE folders SET sort_order = ? WHERE id = ? AND (sort_order IS NULL OR sort_order <= 0)`, i+1, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (d *DB) addColumnIfMissing(table, column, def string) error {
	exists, err := d.columnExists(table, column)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}

	_, err = d.sql.Exec(fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", table, column, def))
	if err != nil {
		return fmt.Errorf("add column %s.%s: %w", table, column, err)
	}
	return nil
}

func (d *DB) columnExists(table, column string) (bool, error) {
	if !isSafeSQLIdent(table) || !isSafeSQLIdent(column) {
		return false, fmt.Errorf("unsafe table/column identifier")
	}

	rows, err := d.sql.Query(fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return false, err
	}
	defer rows.Close()

	for rows.Next() {
		var (
			cid        int
			name       string
			colType    string
			notNull    int
			defaultV   sql.NullString
			primaryKey int
		)
		if err := rows.Scan(&cid, &name, &colType, &notNull, &defaultV, &primaryKey); err != nil {
			return false, err
		}
		if strings.EqualFold(name, column) {
			return true, nil
		}
	}
	if err := rows.Err(); err != nil {
		return false, err
	}
	return false, nil
}

func isSafeSQLIdent(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			continue
		}
		return false
	}
	return true
}
