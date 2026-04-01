package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type DB struct {
	sql  *sql.DB
	path string
}

type Domain struct {
	ID            int64             `json:"id"`
	Name          string            `json:"name"`
	Port          int               `json:"port"`
	Enabled       bool              `json:"enabled"`
	CheckInterval int               `json:"check_interval"` // seconds
	Tags          []string          `json:"tags"`
	Metadata      map[string]string `json:"metadata,omitempty"`
	SourceType    string            `json:"source_type"`
	SourceRef     map[string]string `json:"source_ref,omitempty"`
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

func (d *Domain) EffectiveSourceType() string {
	if d == nil {
		return DomainSourceManual
	}
	return NormalizeSourceType(d.SourceType)
}

func (d *Domain) UsesManualEndpoint() bool {
	return d.EffectiveSourceType() == DomainSourceManual
}

// RegistrationCheckEnabled returns true when RDAP/WHOIS should be performed.
func (d *Domain) RegistrationCheckEnabled() bool {
	return d.UsesManualEndpoint() && d.EffectiveCheckMode() == "full"
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
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	DomainCount int       `json:"domain_count"`
	SortOrder   int       `json:"sort_order"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
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
	PrimaryReasonCode string         `json:"primary_reason_code"`
	PrimaryReasonText string         `json:"primary_reason_text"`
	StatusReasons     []StatusReason `json:"status_reasons,omitempty"`
	OverallStatus     string         `json:"overall_status"` // ok, warning, critical, error
	CheckDuration     int64          `json:"check_duration_ms"`
}

type ChainCert struct {
	Subject      string    `json:"subject"`
	Issuer       string    `json:"issuer"`
	ValidFrom    time.Time `json:"valid_from"`
	ValidTo      time.Time `json:"valid_to"`
	IsCA         bool      `json:"is_ca"`
	IsSelfSigned bool      `json:"is_self_signed"`
}

type StatusReason struct {
	Code     string `json:"code"`
	Severity string `json:"severity"`
	Summary  string `json:"summary"`
	Detail   string `json:"detail,omitempty"`
}

type User struct {
	ID           int64      `json:"id"`
	Username     string     `json:"username"`
	Role         string     `json:"role"`
	Enabled      bool       `json:"enabled"`
	LastLoginAt  *time.Time `json:"last_login_at,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	PasswordHash string     `json:"-"`
}

type Session struct {
	ID         int64     `json:"id"`
	UserID     int64     `json:"user_id"`
	TokenHash  string    `json:"-"`
	ExpiresAt  time.Time `json:"expires_at"`
	CreatedAt  time.Time `json:"created_at"`
	LastSeenAt time.Time `json:"last_seen_at"`
	UserAgent  string    `json:"user_agent"`
	RemoteAddr string    `json:"remote_addr"`
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
	return &DB{sql: sqldb, path: path}, nil
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
			source_type TEXT DEFAULT 'manual',
			source_ref_json TEXT DEFAULT '{}',
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
			primary_reason_code TEXT DEFAULT '',
			primary_reason_text TEXT DEFAULT '',
			status_reasons_json TEXT DEFAULT '[]',
			overall_status TEXT DEFAULT 'unknown',
			check_duration_ms INTEGER DEFAULT 0
		);

		CREATE TABLE IF NOT EXISTS settings (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL
		);

		CREATE TABLE IF NOT EXISTS users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			username TEXT UNIQUE NOT NULL,
			password_hash TEXT NOT NULL,
			role TEXT NOT NULL DEFAULT 'viewer',
			enabled INTEGER DEFAULT 1,
			last_login_at DATETIME,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);

		CREATE TABLE IF NOT EXISTS user_sessions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			token_hash TEXT UNIQUE NOT NULL,
			expires_at DATETIME NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			last_seen_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			user_agent TEXT DEFAULT '',
			remote_addr TEXT DEFAULT ''
		);

		CREATE TABLE IF NOT EXISTS custom_fields (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			key TEXT UNIQUE NOT NULL,
			label TEXT NOT NULL,
			type TEXT NOT NULL DEFAULT 'text',
			required INTEGER DEFAULT 0,
			placeholder TEXT DEFAULT '',
			help_text TEXT DEFAULT '',
			sort_order INTEGER DEFAULT 0,
			visible_in_table INTEGER DEFAULT 0,
			visible_in_details INTEGER DEFAULT 1,
			visible_in_export INTEGER DEFAULT 1,
			filterable INTEGER DEFAULT 0,
			enabled INTEGER DEFAULT 1,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);

		CREATE TABLE IF NOT EXISTS custom_field_options (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			field_id INTEGER NOT NULL REFERENCES custom_fields(id) ON DELETE CASCADE,
			value TEXT NOT NULL,
			label TEXT NOT NULL,
			sort_order INTEGER DEFAULT 0,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(field_id, value)
		);

		CREATE TABLE IF NOT EXISTS audit_logs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			actor_user_id INTEGER REFERENCES users(id) ON DELETE SET NULL,
			actor_username TEXT NOT NULL DEFAULT '',
			actor_role TEXT NOT NULL DEFAULT '',
			actor_source TEXT NOT NULL DEFAULT '',
			action TEXT NOT NULL,
			resource TEXT NOT NULL,
			resource_id INTEGER,
			summary TEXT NOT NULL DEFAULT '',
			details_json TEXT NOT NULL DEFAULT '{}',
			remote_addr TEXT NOT NULL DEFAULT '',
			request_id TEXT NOT NULL DEFAULT '',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);

		CREATE INDEX IF NOT EXISTS idx_checks_domain_id ON domain_checks(domain_id);
		CREATE INDEX IF NOT EXISTS idx_checks_checked_at ON domain_checks(checked_at);
		CREATE INDEX IF NOT EXISTS idx_checks_domain_checked_at ON domain_checks(domain_id, checked_at DESC, id DESC);
		CREATE INDEX IF NOT EXISTS idx_user_sessions_user_id ON user_sessions(user_id);
		CREATE INDEX IF NOT EXISTS idx_user_sessions_expires_at ON user_sessions(expires_at);
		CREATE INDEX IF NOT EXISTS idx_custom_fields_sort_order ON custom_fields(sort_order);
		CREATE INDEX IF NOT EXISTS idx_custom_field_options_field_id ON custom_field_options(field_id);
		CREATE INDEX IF NOT EXISTS idx_audit_logs_created_at ON audit_logs(created_at DESC, id DESC);
		CREATE INDEX IF NOT EXISTS idx_audit_logs_resource ON audit_logs(resource, resource_id, created_at DESC);
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
	if err := d.addColumnIfMissing("domains", "source_type", "TEXT DEFAULT 'manual'"); err != nil {
		return err
	}
	if err := d.addColumnIfMissing("domains", "source_ref_json", "TEXT DEFAULT '{}'"); err != nil {
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
	if err := d.addDateTimeColumnIfMissing("folders", "created_at"); err != nil {
		return err
	}
	if err := d.addDateTimeColumnIfMissing("folders", "updated_at"); err != nil {
		return err
	}
	if err := d.backfillFolderTimestamps(); err != nil {
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
		{"primary_reason_code", "TEXT DEFAULT ''"},
		{"primary_reason_text", "TEXT DEFAULT ''"},
		{"status_reasons_json", "TEXT DEFAULT '[]'"},
	} {
		if err := d.addColumnIfMissing("domain_checks", col.name, col.def); err != nil {
			return err
		}
	}

	if err := d.backfillDomainSortOrder(); err != nil {
		return err
	}
	if err := d.backfillDomainTagsJSON(); err != nil {
		return err
	}
	if err := d.backfillFolderSortOrder(); err != nil {
		return err
	}
	if err := d.backfillCustomFieldSortOrder(); err != nil {
		return err
	}
	// Create indexes only after all required columns are present for old DBs.
	if _, err := d.sql.Exec(`
		CREATE INDEX IF NOT EXISTS idx_domains_sort_order ON domains(sort_order);
		CREATE INDEX IF NOT EXISTS idx_domains_folder_id ON domains(folder_id);
		CREATE INDEX IF NOT EXISTS idx_folders_sort_order ON folders(sort_order);
		CREATE INDEX IF NOT EXISTS idx_custom_fields_sort_order ON custom_fields(sort_order);
		CREATE INDEX IF NOT EXISTS idx_custom_field_options_field_id ON custom_field_options(field_id);
	`); err != nil {
		return err
	}

	return nil
}

// ---- Domain CRUD ----

const domainSelectCols = `id, name, port, enabled, check_interval, tags, tags_json, metadata_json, source_type, source_ref_json, folder_id, sort_order, custom_ca_pem, check_mode, dns_servers, created_at, updated_at`

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
	var tagsRaw, tagsJSON, metadataJSON, sourceRefJSON string
	err := d.sql.QueryRow(`SELECT `+domainSelectCols+` FROM domains WHERE id = ?`, id).
		Scan(&dom.ID, &dom.Name, &dom.Port, &dom.Enabled, &dom.CheckInterval,
			&tagsRaw, &tagsJSON, &metadataJSON, &dom.SourceType, &sourceRefJSON, &folderID, &dom.SortOrder, &dom.CustomCAPEM, &dom.CheckMode, &dom.DNSServers, &dom.CreatedAt, &dom.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	normalizeDomainRow(&dom, tagsRaw, tagsJSON, metadataJSON, sourceRefJSON, folderID)
	return &dom, nil
}

func (d *DB) CreateDomain(name string, tags []string, metadata map[string]string, sourceType string, sourceRef map[string]string, customCAPEM, checkMode, dnsServers string, interval int, port int, folderID *int64) (*Domain, error) {
	if interval == 0 {
		interval = 21600
	}
	if port <= 0 {
		port = 443
	}
	if checkMode == "" {
		checkMode = "full"
	}
	sourceType = NormalizeSourceType(sourceType)
	tags = NormalizeTags(tags)
	tagsText := JoinTags(tags)
	metadata, err := ValidateAndNormalizeMetadata(metadata)
	if err != nil {
		return nil, err
	}
	sourceRef, err = ValidateAndNormalizeSourceRef(sourceType, sourceRef)
	if err != nil {
		return nil, err
	}
	tagsJSON, err := json.Marshal(tags)
	if err != nil {
		return nil, fmt.Errorf("marshal domain tags json: %w", err)
	}
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return nil, fmt.Errorf("marshal domain metadata json: %w", err)
	}
	sourceRefJSON, err := json.Marshal(sourceRef)
	if err != nil {
		return nil, fmt.Errorf("marshal domain source_ref json: %w", err)
	}
	sortOrder, err := d.nextDomainSortOrder()
	if err != nil {
		return nil, err
	}

	res, err := d.sql.Exec(`
		INSERT INTO domains (name, port, tags, tags_json, metadata_json, source_type, source_ref_json, folder_id, sort_order, custom_ca_pem, check_mode, dns_servers, check_interval)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		name, port, tagsText, string(tagsJSON), string(metadataJSON), sourceType, string(sourceRefJSON), folderID, sortOrder, customCAPEM, checkMode, dnsServers, interval)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return d.GetDomainByID(id)
}

func (d *DB) UpdateDomain(id int64, name string, tags []string, metadata map[string]string, sourceType string, sourceRef map[string]string, customCAPEM, checkMode, dnsServers string, enabled bool, interval int, port int, folderID *int64) error {
	if port <= 0 {
		port = 443
	}
	if checkMode == "" {
		checkMode = "full"
	}
	sourceType = NormalizeSourceType(sourceType)
	tags = NormalizeTags(tags)
	tagsText := JoinTags(tags)
	metadata, err := ValidateAndNormalizeMetadata(metadata)
	if err != nil {
		return err
	}
	sourceRef, err = ValidateAndNormalizeSourceRef(sourceType, sourceRef)
	if err != nil {
		return err
	}
	tagsJSON, err := json.Marshal(tags)
	if err != nil {
		return fmt.Errorf("marshal domain tags json: %w", err)
	}
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("marshal domain metadata json: %w", err)
	}
	sourceRefJSON, err := json.Marshal(sourceRef)
	if err != nil {
		return fmt.Errorf("marshal domain source_ref json: %w", err)
	}
	_, err = d.sql.Exec(`
		UPDATE domains SET name=?, port=?, tags=?, tags_json=?, metadata_json=?, source_type=?, source_ref_json=?, folder_id=?, custom_ca_pem=?, check_mode=?, dns_servers=?, enabled=?, check_interval=?, updated_at=CURRENT_TIMESTAMP
		WHERE id=?`, name, port, tagsText, string(tagsJSON), string(metadataJSON), sourceType, string(sourceRefJSON), folderID, customCAPEM, checkMode, dnsServers, enabled, interval, id)
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

func (d *DB) GetNextScheduledCheckAt(now time.Time) (*time.Time, error) {
	rows, err := d.sql.Query(`
		WITH latest AS (
			SELECT domain_id, MAX(checked_at) AS last_checked_at
			FROM domain_checks
			GROUP BY domain_id
		)
		SELECT d.check_interval, latest.last_checked_at
		FROM domains d
		LEFT JOIN latest ON latest.domain_id = d.id
		WHERE d.enabled = 1
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var earliest *time.Time
	for rows.Next() {
		var interval int
		var lastChecked sql.NullString
		if err := rows.Scan(&interval, &lastChecked); err != nil {
			return nil, err
		}
		candidate := now.UTC()
		if lastChecked.Valid {
			parsed, err := parseDBTime(lastChecked.String)
			if err != nil {
				return nil, fmt.Errorf("parse last scheduled check time: %w", err)
			}
			candidate = parsed.Add(time.Duration(interval) * time.Second)
		}
		if earliest == nil || candidate.Before(*earliest) {
			next := candidate
			earliest = &next
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return earliest, nil
}

func parseDBTime(raw string) (time.Time, error) {
	value := normalizeDBTimeString(raw)
	formats := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02 15:04:05.999999999 -0700 MST",
		"2006-01-02 15:04:05 -0700 MST",
		"2006-01-02 15:04:05.999999999-07:00",
		"2006-01-02 15:04:05-07:00",
		"2006-01-02 15:04:05.999999999",
		"2006-01-02 15:04:05",
	}
	var lastErr error
	for _, format := range formats {
		parsed, err := time.Parse(format, value)
		if err == nil {
			return parsed.UTC(), nil
		}
		lastErr = err
	}
	return time.Time{}, fmt.Errorf("unsupported time format %q: %w", value, lastErr)
}

func normalizeDBTimeString(raw string) string {
	value := strings.TrimSpace(raw)
	if idx := strings.Index(value, " m=+"); idx >= 0 {
		value = strings.TrimSpace(value[:idx])
	}
	return value
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
	primary_reason_code, primary_reason_text, status_reasons_json,
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
	primary_reason_code, primary_reason_text, status_reasons_json,
	overall_status, check_duration_ms`

func (d *DB) SaveCheck(c *Check) error {
	chainJSON, err := json.Marshal(c.SSLChainDetails)
	if err != nil {
		return fmt.Errorf("marshal ssl chain details json: %w", err)
	}
	reasonsJSON, err := json.Marshal(c.StatusReasons)
	if err != nil {
		return fmt.Errorf("marshal status reasons json: %w", err)
	}
	args := []any{
		c.DomainID, sanitizeDBTime(c.CheckedAt),
		c.DomainStatus, c.DomainRegistrar, sanitizeDBTimePtr(c.DomainCreatedAt), sanitizeDBTimePtr(c.DomainExpiresAt),
		c.DomainExpiryDays, c.DomainCheckError, c.DomainSource,
		c.RegistrationCheckSkipped, c.RegistrationSkipReason, c.DNSServerUsed,
		c.SSLIssuer, c.SSLSubject, sanitizeDBTimePtr(c.SSLValidFrom), sanitizeDBTimePtr(c.SSLValidUntil),
		c.SSLExpiryDays, c.SSLVersion, c.SSLCheckError,
		c.SSLChainValid, c.SSLChainLength, c.SSLChainError, string(chainJSON),
		c.HTTPStatusCode, c.HTTPRedirectsHTTPS, c.HTTPHSTSEnabled, c.HTTPHSTSMaxAge, c.HTTPResponseTimeMs, c.HTTPFinalURL, c.HTTPError,
		c.CipherWeak, c.CipherWeakReason, c.CipherGrade, c.CipherDetails,
		c.OCSPStatus, c.OCSPError, c.CRLStatus, c.CRLError,
		c.CAAPresent, c.CAA, c.CAAQueryDomain, c.CAAError,
		c.PrimaryReasonCode, c.PrimaryReasonText, string(reasonsJSON),
		c.OverallStatus, c.CheckDuration,
	}
	placeholders := strings.TrimRight(strings.Repeat("?,", len(args)), ",")
	//nolint:gosec // Column and placeholder lists are derived from static constants; values remain parameterized.
	_, err = d.sql.Exec(`INSERT INTO domain_checks (`+checkInsertCols+`) VALUES (`+placeholders+`)`, args...)
	return err
}

func (d *DB) GetLastCheck(domainID int64) (*Check, error) {
	return d.scanCheck(d.sql.QueryRow(`SELECT `+checkSelectCols+` FROM domain_checks WHERE domain_id = ? ORDER BY checked_at DESC, id DESC LIMIT 1`, domainID))
}

func (d *DB) GetCheckHistory(domainID int64, limit int) ([]Check, error) {
	if limit == 0 {
		limit = 50
	}
	return d.GetCheckHistoryPage(domainID, limit, 0)
}

func (d *DB) GetCheckHistoryPage(domainID int64, limit, offset int) ([]Check, error) {
	if limit <= 0 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	rows, err := d.sql.Query(`SELECT `+checkSelectCols+` FROM domain_checks WHERE domain_id = ? ORDER BY checked_at DESC, id DESC LIMIT ? OFFSET ?`,
		domainID, limit, offset)
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

func (d *DB) CountCheckHistory(domainID int64) (int, error) {
	var total int
	err := d.sql.QueryRow(`SELECT COUNT(*) FROM domain_checks WHERE domain_id = ?`, domainID).Scan(&total)
	return total, err
}

func (d *DB) GetAllLastChecks() (map[int64]*Check, error) {
	//nolint:gosec // Query shape is built from static column lists to keep scan order in sync.
	rows, err := d.sql.Query(`
		WITH ranked AS (
			SELECT ` + checkSelectCols + `,
				ROW_NUMBER() OVER (PARTITION BY domain_id ORDER BY checked_at DESC, id DESC) AS rn
			FROM domain_checks
		)
		SELECT ` + prefixCols("ranked.", checkSelectCols) + `
		FROM ranked
		WHERE ranked.rn = 1`)
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
		SELECT
			f.id,
			f.name,
			(
				SELECT COUNT(*)
				FROM domains d
				WHERE d.folder_id = f.id
			) AS domain_count,
			f.sort_order,
			f.created_at,
			f.updated_at
		FROM folders f
		ORDER BY f.sort_order ASC, f.name ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("query folders: %w", err)
	}
	defer rows.Close()

	folders := make([]Folder, 0)
	for rows.Next() {
		var f Folder
		if err := rows.Scan(&f.ID, &f.Name, &f.DomainCount, &f.SortOrder, &f.CreatedAt, &f.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan folder row: %w", err)
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
		INSERT INTO folders (name, sort_order, created_at, updated_at) VALUES (?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
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
		SELECT
			f.id,
			f.name,
			(
				SELECT COUNT(*)
				FROM domains d
				WHERE d.folder_id = f.id
			) AS domain_count,
			f.sort_order,
			f.created_at,
			f.updated_at
		FROM folders f
		WHERE f.id = ?
	`, id).Scan(&f.ID, &f.Name, &f.DomainCount, &f.SortOrder, &f.CreatedAt, &f.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get folder by id %d: %w", id, err)
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
	defer rollbackTx(tx)

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
	defer rollbackTx(tx)

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
		var tagsRaw, tagsJSON, metadataJSON, sourceRefJSON string
		if err := rows.Scan(&dom.ID, &dom.Name, &dom.Port, &dom.Enabled, &dom.CheckInterval,
			&tagsRaw, &tagsJSON, &metadataJSON, &dom.SourceType, &sourceRefJSON, &folderID, &dom.SortOrder, &dom.CustomCAPEM, &dom.CheckMode, &dom.DNSServers, &dom.CreatedAt, &dom.UpdatedAt); err != nil {
			return nil, err
		}
		normalizeDomainRow(&dom, tagsRaw, tagsJSON, metadataJSON, sourceRefJSON, folderID)
		domains = append(domains, dom)
	}
	return domains, rows.Err()
}

func normalizeDomainRow(dom *Domain, tagsRaw, tagsJSON, metadataJSON, sourceRefJSON string, folderID sql.NullInt64) {
	if dom.Port <= 0 {
		dom.Port = 443
	}
	if dom.CheckMode == "" {
		dom.CheckMode = "full"
	}
	dom.SourceType = NormalizeSourceType(dom.SourceType)
	if strings.TrimSpace(tagsJSON) != "" {
		if err := json.Unmarshal([]byte(tagsJSON), &dom.Tags); err != nil {
			slog.Warn("DB failed to parse tags_json", "domain", dom.Name, "error", err)
		}
	}
	if len(dom.Tags) == 0 {
		dom.Tags = ParseLegacyTags(tagsRaw)
	}
	dom.Tags = NormalizeTags(dom.Tags)
	if dom.Tags == nil {
		dom.Tags = []string{}
	}
	if strings.TrimSpace(metadataJSON) != "" {
		if err := json.Unmarshal([]byte(metadataJSON), &dom.Metadata); err != nil {
			slog.Warn("DB failed to parse metadata_json", "domain", dom.Name, "error", err)
		}
	}
	if strings.TrimSpace(sourceRefJSON) != "" {
		if err := json.Unmarshal([]byte(sourceRefJSON), &dom.SourceRef); err != nil {
			slog.Warn("DB failed to parse source_ref_json", "domain", dom.Name, "error", err)
		}
	}
	if dom.Metadata != nil {
		if normalized, err := ValidateAndNormalizeMetadata(dom.Metadata); err == nil {
			dom.Metadata = normalized
		}
	}
	if dom.Metadata == nil {
		dom.Metadata = map[string]string{}
	}
	if normalizedSourceRef, err := ValidateAndNormalizeSourceRef(dom.SourceType, dom.SourceRef); err == nil {
		dom.SourceRef = normalizedSourceRef
	}
	if dom.SourceRef == nil {
		dom.SourceRef = map[string]string{}
	}
	if folderID.Valid {
		v := folderID.Int64
		dom.FolderID = &v
	}
}

func (d *DB) scanCheck(row *sql.Row) (*Check, error) {
	var c Check
	var chainJSON, reasonsJSON string
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
		&c.PrimaryReasonCode, &c.PrimaryReasonText, &reasonsJSON,
		&c.OverallStatus, &c.CheckDuration,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	applyNullTimes(&c, domCreatedAt, domExpiresAt, sslValidFrom, sslValidUntil)
	if err := json.Unmarshal([]byte(chainJSON), &c.SSLChainDetails); err != nil && strings.TrimSpace(chainJSON) != "" {
		slog.Warn("DB failed to parse ssl_chain_details", "domain_id", c.DomainID, "check_id", c.ID, "error", err)
	}
	if err := json.Unmarshal([]byte(reasonsJSON), &c.StatusReasons); err != nil && strings.TrimSpace(reasonsJSON) != "" {
		slog.Warn("DB failed to parse status_reasons_json", "domain_id", c.DomainID, "check_id", c.ID, "error", err)
	}
	return &c, nil
}

func (d *DB) scanCheckRow(rows *sql.Rows) (*Check, error) {
	var c Check
	var chainJSON, reasonsJSON string
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
		&c.PrimaryReasonCode, &c.PrimaryReasonText, &reasonsJSON,
		&c.OverallStatus, &c.CheckDuration,
	)
	if err != nil {
		return nil, err
	}
	applyNullTimes(&c, domCreatedAt, domExpiresAt, sslValidFrom, sslValidUntil)
	if err := json.Unmarshal([]byte(chainJSON), &c.SSLChainDetails); err != nil && strings.TrimSpace(chainJSON) != "" {
		slog.Warn("DB failed to parse ssl_chain_details", "domain_id", c.DomainID, "check_id", c.ID, "error", err)
	}
	if err := json.Unmarshal([]byte(reasonsJSON), &c.StatusReasons); err != nil && strings.TrimSpace(reasonsJSON) != "" {
		slog.Warn("DB failed to parse status_reasons_json", "domain_id", c.DomainID, "check_id", c.ID, "error", err)
	}
	return &c, nil
}

func sanitizeDBTime(value time.Time) time.Time {
	if value.IsZero() {
		return value
	}
	return value.UTC().Round(0)
}

func sanitizeDBTimePtr(value *time.Time) any {
	if value == nil {
		return nil
	}
	sanitized := sanitizeDBTime(*value)
	return sanitized
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
	defer rollbackTx(tx)

	for i, id := range ids {
		if _, err := tx.Exec(`UPDATE domains SET sort_order = ? WHERE id = ? AND (sort_order IS NULL OR sort_order <= 0)`, i+1, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (d *DB) backfillDomainTagsJSON() error {
	rows, err := d.sql.Query(`SELECT id, tags, tags_json FROM domains`)
	if err != nil {
		return err
	}
	defer rows.Close()

	type update struct {
		id       int64
		tagsJSON string
	}
	updates := make([]update, 0)
	for rows.Next() {
		var (
			id       int64
			rawTags  string
			tagsJSON string
		)
		if err := rows.Scan(&id, &rawTags, &tagsJSON); err != nil {
			return err
		}
		if strings.TrimSpace(rawTags) == "" {
			continue
		}
		if strings.TrimSpace(tagsJSON) != "" && strings.TrimSpace(tagsJSON) != "[]" {
			continue
		}
		normalized := NormalizeTags(ParseLegacyTags(rawTags))
		encoded, err := json.Marshal(normalized)
		if err != nil {
			return fmt.Errorf("marshal backfilled tags_json: %w", err)
		}
		updates = append(updates, update{id: id, tagsJSON: string(encoded)})
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if len(updates) == 0 {
		return nil
	}

	tx, err := d.sql.Begin()
	if err != nil {
		return err
	}
	defer rollbackTx(tx)

	for _, item := range updates {
		if _, err := tx.Exec(`UPDATE domains SET tags_json = ? WHERE id = ?`, item.tagsJSON, item.id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (d *DB) backfillFolderSortOrder() error {
	rows, err := d.sql.Query(`SELECT id FROM folders ORDER BY created_at ASC, id ASC`)
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
	defer rollbackTx(tx)

	for i, id := range ids {
		if _, err := tx.Exec(`UPDATE folders SET sort_order = ? WHERE id = ? AND (sort_order IS NULL OR sort_order <= 0)`, i+1, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (d *DB) backfillCustomFieldSortOrder() error {
	rows, err := d.sql.Query(`SELECT id FROM custom_fields ORDER BY created_at ASC, id ASC`)
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
	defer rollbackTx(tx)

	for i, id := range ids {
		if _, err := tx.Exec(`UPDATE custom_fields SET sort_order = ? WHERE id = ? AND (sort_order IS NULL OR sort_order <= 0)`, i+1, id); err != nil {
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

func (d *DB) addDateTimeColumnIfMissing(table, column string) error {
	exists, err := d.columnExists(table, column)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}

	if !isSafeSQLIdent(table) || !isSafeSQLIdent(column) {
		return fmt.Errorf("unsafe table/column identifier")
	}

	if _, err := d.sql.Exec(fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s DATETIME", table, column)); err != nil {
		return fmt.Errorf("add datetime column %s.%s: %w", table, column, err)
	}
	return nil
}

func (d *DB) backfillFolderTimestamps() error {
	if _, err := d.sql.Exec(`
		UPDATE folders
		SET created_at = CURRENT_TIMESTAMP
		WHERE created_at IS NULL OR TRIM(CAST(created_at AS TEXT)) = ''
	`); err != nil {
		return fmt.Errorf("backfill folders.created_at: %w", err)
	}
	if _, err := d.sql.Exec(`
		UPDATE folders
		SET updated_at = COALESCE(updated_at, created_at, CURRENT_TIMESTAMP)
		WHERE updated_at IS NULL OR TRIM(CAST(updated_at AS TEXT)) = ''
	`); err != nil {
		return fmt.Errorf("backfill folders.updated_at: %w", err)
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

func rollbackTx(tx *sql.Tx) {
	if tx == nil {
		return
	}
	if err := tx.Rollback(); err != nil && err != sql.ErrTxDone {
		slog.Debug("DB rollback failed", "error", err)
	}
}
