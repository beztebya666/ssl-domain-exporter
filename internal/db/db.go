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
	ID            int64     `json:"id"`
	Name          string    `json:"name"`
	Port          int       `json:"port"`
	Enabled       bool      `json:"enabled"`
	CheckInterval int       `json:"check_interval"` // seconds
	Tags          string    `json:"tags"`
	FolderID      *int64    `json:"folder_id,omitempty"`
	SortOrder     int       `json:"sort_order"`
	CustomCAPEM   string    `json:"custom_ca_pem"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
	LastCheck     *Check    `json:"last_check,omitempty"`
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
	DomainStatus    string     `json:"domain_status"`
	DomainRegistrar string     `json:"domain_registrar"`
	DomainCreatedAt *time.Time `json:"domain_created_at"`
	DomainExpiresAt *time.Time `json:"domain_expires_at"`
	DomainExpiryDays *int      `json:"domain_expiry_days"`
	DomainCheckError string    `json:"domain_check_error"`
	DomainSource     string    `json:"domain_source"` // rdap/whois

	// SSL info
	SSLIssuer       string     `json:"ssl_issuer"`
	SSLSubject      string     `json:"ssl_subject"`
	SSLValidFrom    *time.Time `json:"ssl_valid_from"`
	SSLValidUntil   *time.Time `json:"ssl_valid_until"`
	SSLExpiryDays   *int       `json:"ssl_expiry_days"`
	SSLVersion      string     `json:"ssl_version"`
	SSLCheckError   string     `json:"ssl_check_error"`

	// SSL Chain
	SSLChainValid   bool   `json:"ssl_chain_valid"`
	SSLChainLength  int    `json:"ssl_chain_length"`
	SSLChainError   string `json:"ssl_chain_error"`
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
	CAAPresent    bool   `json:"caa_present"`
	CAA           string `json:"caa"`
	CAAQueryDomain string `json:"caa_query_domain"`
	CAAError      string `json:"caa_error"`

	// Overall
	OverallStatus string `json:"overall_status"` // ok, warning, critical, error
	CheckDuration int64  `json:"check_duration_ms"`
}

type ChainCert struct {
	Subject   string     `json:"subject"`
	Issuer    string     `json:"issuer"`
	ValidFrom time.Time  `json:"valid_from"`
	ValidTo   time.Time  `json:"valid_to"`
	IsCA      bool       `json:"is_ca"`
	IsSelfSigned bool    `json:"is_self_signed"`
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
			folder_id INTEGER REFERENCES folders(id) ON DELETE SET NULL,
			sort_order INTEGER DEFAULT 0,
			custom_ca_pem TEXT DEFAULT '',
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
	// Migrations for existing databases
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
	if err := d.addColumnIfMissing("folders", "sort_order", "INTEGER DEFAULT 0"); err != nil {
		return err
	}

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

func (d *DB) GetDomains() ([]Domain, error) {
	rows, err := d.sql.Query(`
		SELECT id, name, port, enabled, check_interval, tags, folder_id, sort_order, custom_ca_pem, created_at, updated_at
		FROM domains ORDER BY sort_order ASC, name ASC
	`)
	legacyQuery := false
	if err != nil && isMissingCustomCAColumnErr(err) {
		legacyQuery = true
		rows, err = d.sql.Query(`
			SELECT id, name, port, enabled, check_interval, tags, folder_id, sort_order, created_at, updated_at
			FROM domains ORDER BY sort_order ASC, name ASC
		`)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	domains := []Domain{}
	for rows.Next() {
		var dom Domain
		var folderID sql.NullInt64
		if legacyQuery {
			if err := rows.Scan(&dom.ID, &dom.Name, &dom.Port, &dom.Enabled, &dom.CheckInterval,
				&dom.Tags, &folderID, &dom.SortOrder, &dom.CreatedAt, &dom.UpdatedAt); err != nil {
				return nil, err
			}
			dom.CustomCAPEM = ""
		} else if err := rows.Scan(&dom.ID, &dom.Name, &dom.Port, &dom.Enabled, &dom.CheckInterval,
			&dom.Tags, &folderID, &dom.SortOrder, &dom.CustomCAPEM, &dom.CreatedAt, &dom.UpdatedAt); err != nil {
			return nil, err
		}
		if dom.Port <= 0 {
			dom.Port = 443
		}
		if folderID.Valid {
			v := folderID.Int64
			dom.FolderID = &v
		}
		domains = append(domains, dom)
	}
	return domains, rows.Err()
}

func (d *DB) GetDomainByID(id int64) (*Domain, error) {
	var dom Domain
	var folderID sql.NullInt64
	err := d.sql.QueryRow(`
		SELECT id, name, port, enabled, check_interval, tags, folder_id, sort_order, custom_ca_pem, created_at, updated_at
		FROM domains WHERE id = ?`, id).
		Scan(&dom.ID, &dom.Name, &dom.Port, &dom.Enabled, &dom.CheckInterval,
			&dom.Tags, &folderID, &dom.SortOrder, &dom.CustomCAPEM, &dom.CreatedAt, &dom.UpdatedAt)
	if err != nil && isMissingCustomCAColumnErr(err) {
		err = d.sql.QueryRow(`
			SELECT id, name, port, enabled, check_interval, tags, folder_id, sort_order, created_at, updated_at
			FROM domains WHERE id = ?`, id).
			Scan(&dom.ID, &dom.Name, &dom.Port, &dom.Enabled, &dom.CheckInterval,
				&dom.Tags, &folderID, &dom.SortOrder, &dom.CreatedAt, &dom.UpdatedAt)
		dom.CustomCAPEM = ""
	}
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if dom.Port <= 0 {
		dom.Port = 443
	}
	if folderID.Valid {
		v := folderID.Int64
		dom.FolderID = &v
	}
	return &dom, err
}

func (d *DB) CreateDomain(name, tags, customCAPEM string, interval int, port int, folderID *int64) (*Domain, error) {
	if interval == 0 {
		interval = 21600
	}
	if port <= 0 {
		port = 443
	}
	sortOrder, err := d.nextDomainSortOrder()
	if err != nil {
		return nil, err
	}

	res, err := d.sql.Exec(`
		INSERT INTO domains (name, port, tags, folder_id, sort_order, custom_ca_pem, check_interval) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		name, port, tags, folderID, sortOrder, customCAPEM, interval)
	if err != nil && isMissingCustomCAColumnErr(err) {
		res, err = d.sql.Exec(`
			INSERT INTO domains (name, port, tags, folder_id, sort_order, check_interval) VALUES (?, ?, ?, ?, ?, ?)`,
			name, port, tags, folderID, sortOrder, interval)
	}
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return d.GetDomainByID(id)
}

func (d *DB) UpdateDomain(id int64, name, tags, customCAPEM string, enabled bool, interval int, port int, folderID *int64) error {
	if port <= 0 {
		port = 443
	}
	_, err := d.sql.Exec(`
		UPDATE domains SET name=?, port=?, tags=?, folder_id=?, custom_ca_pem=?, enabled=?, check_interval=?, updated_at=CURRENT_TIMESTAMP
		WHERE id=?`, name, port, tags, folderID, customCAPEM, enabled, interval, id)
	if err != nil && isMissingCustomCAColumnErr(err) {
		_, err = d.sql.Exec(`
			UPDATE domains SET name=?, port=?, tags=?, folder_id=?, enabled=?, check_interval=?, updated_at=CURRENT_TIMESTAMP
			WHERE id=?`, name, port, tags, folderID, enabled, interval, id)
	}
	return err
}

func (d *DB) DeleteDomain(id int64) error {
	_, err := d.sql.Exec(`DELETE FROM domains WHERE id = ?`, id)
	return err
}

func (d *DB) SaveCheck(c *Check) error {
	chainJSON, _ := json.Marshal(c.SSLChainDetails)
	_, err := d.sql.Exec(`
		INSERT INTO domain_checks (
			domain_id, checked_at,
			domain_status, domain_registrar, domain_created_at, domain_expires_at,
			domain_expiry_days, domain_check_error, domain_source,
			ssl_issuer, ssl_subject, ssl_valid_from, ssl_valid_until,
			ssl_expiry_days, ssl_version, ssl_check_error,
			ssl_chain_valid, ssl_chain_length, ssl_chain_error, ssl_chain_details,
			http_status_code, http_redirects_https, http_hsts_enabled, http_hsts_max_age, http_response_time_ms, http_final_url, http_error,
			cipher_weak, cipher_weak_reason, cipher_grade, cipher_details,
			ocsp_status, ocsp_error, crl_status, crl_error,
			caa_present, caa, caa_query_domain, caa_error,
			overall_status, check_duration_ms
		) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		c.DomainID, c.CheckedAt,
		c.DomainStatus, c.DomainRegistrar, c.DomainCreatedAt, c.DomainExpiresAt,
		c.DomainExpiryDays, c.DomainCheckError, c.DomainSource,
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
	return d.scanCheck(d.sql.QueryRow(`
		SELECT id, domain_id, checked_at,
			domain_status, domain_registrar, domain_created_at, domain_expires_at,
			domain_expiry_days, domain_check_error, domain_source,
			ssl_issuer, ssl_subject, ssl_valid_from, ssl_valid_until,
			ssl_expiry_days, ssl_version, ssl_check_error,
			ssl_chain_valid, ssl_chain_length, ssl_chain_error, ssl_chain_details,
			http_status_code, http_redirects_https, http_hsts_enabled, http_hsts_max_age, http_response_time_ms, http_final_url, http_error,
			cipher_weak, cipher_weak_reason, cipher_grade, cipher_details,
			ocsp_status, ocsp_error, crl_status, crl_error,
			caa_present, caa, caa_query_domain, caa_error,
			overall_status, check_duration_ms
		FROM domain_checks WHERE domain_id = ? ORDER BY checked_at DESC LIMIT 1`, domainID))
}

func (d *DB) GetCheckHistory(domainID int64, limit int) ([]Check, error) {
	if limit == 0 {
		limit = 50
	}
	rows, err := d.sql.Query(`
		SELECT id, domain_id, checked_at,
			domain_status, domain_registrar, domain_created_at, domain_expires_at,
			domain_expiry_days, domain_check_error, domain_source,
			ssl_issuer, ssl_subject, ssl_valid_from, ssl_valid_until,
			ssl_expiry_days, ssl_version, ssl_check_error,
			ssl_chain_valid, ssl_chain_length, ssl_chain_error, ssl_chain_details,
			http_status_code, http_redirects_https, http_hsts_enabled, http_hsts_max_age, http_response_time_ms, http_final_url, http_error,
			cipher_weak, cipher_weak_reason, cipher_grade, cipher_details,
			ocsp_status, ocsp_error, crl_status, crl_error,
			caa_present, caa, caa_query_domain, caa_error,
			overall_status, check_duration_ms
		FROM domain_checks WHERE domain_id = ? ORDER BY checked_at DESC LIMIT ?`,
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
		SELECT dc.id, dc.domain_id, dc.checked_at,
			dc.domain_status, dc.domain_registrar, dc.domain_created_at, dc.domain_expires_at,
			dc.domain_expiry_days, dc.domain_check_error, dc.domain_source,
			dc.ssl_issuer, dc.ssl_subject, dc.ssl_valid_from, dc.ssl_valid_until,
			dc.ssl_expiry_days, dc.ssl_version, dc.ssl_check_error,
			dc.ssl_chain_valid, dc.ssl_chain_length, dc.ssl_chain_error, dc.ssl_chain_details,
			dc.http_status_code, dc.http_redirects_https, dc.http_hsts_enabled, dc.http_hsts_max_age, dc.http_response_time_ms, dc.http_final_url, dc.http_error,
			dc.cipher_weak, dc.cipher_weak_reason, dc.cipher_grade, dc.cipher_details,
			dc.ocsp_status, dc.ocsp_error, dc.crl_status, dc.crl_error,
			dc.caa_present, dc.caa, dc.caa_query_domain, dc.caa_error,
			dc.overall_status, dc.check_duration_ms
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

func (d *DB) scanCheck(row *sql.Row) (*Check, error) {
	var c Check
	var chainJSON string
	var domCreatedAt, domExpiresAt, sslValidFrom, sslValidUntil sql.NullTime
	err := row.Scan(
		&c.ID, &c.DomainID, &c.CheckedAt,
		&c.DomainStatus, &c.DomainRegistrar, &domCreatedAt, &domExpiresAt,
		&c.DomainExpiryDays, &c.DomainCheckError, &c.DomainSource,
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
	json.Unmarshal([]byte(chainJSON), &c.SSLChainDetails)
	return &c, nil
}

func (d *DB) GetDomainsForScheduling() ([]Domain, error) {
	rows, err := d.sql.Query(`
		SELECT d.id, d.name, d.port, d.enabled, d.check_interval, d.tags, d.folder_id, d.sort_order, d.custom_ca_pem, d.created_at, d.updated_at
		FROM domains d
		WHERE d.enabled = 1
		AND (
			NOT EXISTS (SELECT 1 FROM domain_checks dc WHERE dc.domain_id = d.id)
			OR (
				SELECT MAX(checked_at) FROM domain_checks dc WHERE dc.domain_id = d.id
			) < datetime('now', '-' || d.check_interval || ' seconds')
		)`)
	legacyQuery := false
	if err != nil && isMissingCustomCAColumnErr(err) {
		legacyQuery = true
		rows, err = d.sql.Query(`
			SELECT d.id, d.name, d.port, d.enabled, d.check_interval, d.tags, d.folder_id, d.sort_order, d.created_at, d.updated_at
			FROM domains d
			WHERE d.enabled = 1
			AND (
				NOT EXISTS (SELECT 1 FROM domain_checks dc WHERE dc.domain_id = d.id)
				OR (
					SELECT MAX(checked_at) FROM domain_checks dc WHERE dc.domain_id = d.id
				) < datetime('now', '-' || d.check_interval || ' seconds')
			)`)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	domains := []Domain{}
	for rows.Next() {
		var dom Domain
		var folderID sql.NullInt64
		if legacyQuery {
			if err := rows.Scan(&dom.ID, &dom.Name, &dom.Port, &dom.Enabled, &dom.CheckInterval,
				&dom.Tags, &folderID, &dom.SortOrder, &dom.CreatedAt, &dom.UpdatedAt); err != nil {
				return nil, err
			}
			dom.CustomCAPEM = ""
		} else if err := rows.Scan(&dom.ID, &dom.Name, &dom.Port, &dom.Enabled, &dom.CheckInterval,
			&dom.Tags, &folderID, &dom.SortOrder, &dom.CustomCAPEM, &dom.CreatedAt, &dom.UpdatedAt); err != nil {
			return nil, err
		}
		if dom.Port <= 0 {
			dom.Port = 443
		}
		if folderID.Valid {
			v := folderID.Int64
			dom.FolderID = &v
		}
		domains = append(domains, dom)
	}
	return domains, rows.Err()
}

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

func isMissingCustomCAColumnErr(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	if !strings.Contains(msg, "custom_ca_pem") {
		return false
	}
	return strings.Contains(msg, "no such column") || strings.Contains(msg, "has no column named")
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
			cid       int
			name      string
			colType   string
			notNull   int
			defaultV  sql.NullString
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
