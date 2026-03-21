package db

import (
	"path/filepath"
	"testing"
	"time"
)

func TestMigrateLegacyDatabaseAddsEnterpriseColumns(t *testing.T) {
	database := newTestDB(t)
	defer database.Close()

	if _, err := database.sql.Exec(`
		DROP TABLE IF EXISTS domain_checks;
		DROP TABLE IF EXISTS domains;
		CREATE TABLE domains (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT UNIQUE NOT NULL,
			enabled INTEGER DEFAULT 1,
			check_interval INTEGER DEFAULT 21600,
			tags TEXT DEFAULT '',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);
		CREATE TABLE domain_checks (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			domain_id INTEGER,
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
			overall_status TEXT DEFAULT 'unknown',
			check_duration_ms INTEGER DEFAULT 0
		);
	`); err != nil {
		t.Fatalf("create legacy schema: %v", err)
	}

	if err := database.Migrate(); err != nil {
		t.Fatalf("migrate legacy schema: %v", err)
	}

	for _, col := range []string{"port", "folder_id", "sort_order", "custom_ca_pem", "check_mode", "dns_servers"} {
		if !columnExists(t, database, "domains", col) {
			t.Fatalf("expected domains.%s to exist after migration", col)
		}
	}
	for _, col := range []string{"registration_check_skipped", "registration_skip_reason", "dns_server_used", "http_status_code", "caa_present"} {
		if !columnExists(t, database, "domain_checks", col) {
			t.Fatalf("expected domain_checks.%s to exist after migration", col)
		}
	}
}

func TestCreateUpdateAndScheduleDomainWithEnterpriseFields(t *testing.T) {
	database := newTestDB(t)
	defer database.Close()

	const rootPEM = "-----BEGIN CERTIFICATE-----\nTEST\n-----END CERTIFICATE-----"

	domain, err := database.CreateDomain("vcenter.internal", []string{"infra"}, map[string]string{"owner": "platform"}, rootPEM, "ssl_only", "10.0.0.1:53, 10.0.0.2:53", 3600, 8443, nil)
	if err != nil {
		t.Fatalf("create domain: %v", err)
	}
	if domain.CustomCAPEM != rootPEM {
		t.Fatalf("custom CA not preserved on create: %q", domain.CustomCAPEM)
	}
	if domain.CheckMode != "ssl_only" {
		t.Fatalf("check mode on create = %q, want ssl_only", domain.CheckMode)
	}
	if domain.DNSServers != "10.0.0.1:53, 10.0.0.2:53" {
		t.Fatalf("dns servers on create = %q", domain.DNSServers)
	}
	if len(domain.Tags) != 1 || domain.Tags[0] != "infra" {
		t.Fatalf("tags on create = %#v", domain.Tags)
	}
	if domain.Metadata["owner"] != "platform" {
		t.Fatalf("metadata on create = %#v", domain.Metadata)
	}

	const updatedPEM = "-----BEGIN CERTIFICATE-----\nUPDATED\n-----END CERTIFICATE-----"
	if err := database.UpdateDomain(domain.ID, "vcsa.internal", []string{"platform", "prod"}, map[string]string{"owner": "ops", "zone": "corp"}, updatedPEM, "full", "10.0.0.3:53", true, 7200, 443, nil); err != nil {
		t.Fatalf("update domain: %v", err)
	}

	updated, err := database.GetDomainByID(domain.ID)
	if err != nil {
		t.Fatalf("get updated domain: %v", err)
	}
	if updated == nil {
		t.Fatal("updated domain not found")
	}
	if updated.Name != "vcsa.internal" {
		t.Fatalf("updated name = %q, want vcsa.internal", updated.Name)
	}
	if updated.CustomCAPEM != updatedPEM {
		t.Fatalf("updated custom CA = %q", updated.CustomCAPEM)
	}
	if updated.CheckMode != "full" {
		t.Fatalf("updated check mode = %q, want full", updated.CheckMode)
	}
	if updated.DNSServers != "10.0.0.3:53" {
		t.Fatalf("updated DNS servers = %q", updated.DNSServers)
	}
	if updated.CheckInterval != 7200 {
		t.Fatalf("updated interval = %d, want 7200", updated.CheckInterval)
	}
	if len(updated.Tags) != 2 || updated.Tags[0] != "platform" || updated.Tags[1] != "prod" {
		t.Fatalf("updated tags = %#v", updated.Tags)
	}
	if updated.Metadata["owner"] != "ops" || updated.Metadata["zone"] != "corp" {
		t.Fatalf("updated metadata = %#v", updated.Metadata)
	}

	scheduled, err := database.GetDomainsForScheduling()
	if err != nil {
		t.Fatalf("GetDomainsForScheduling: %v", err)
	}
	if len(scheduled) != 1 {
		t.Fatalf("expected 1 scheduled domain, got %d", len(scheduled))
	}
	if scheduled[0].CustomCAPEM != updatedPEM {
		t.Fatalf("scheduled domain lost custom CA: %q", scheduled[0].CustomCAPEM)
	}
	if scheduled[0].CheckMode != "full" {
		t.Fatalf("scheduled domain lost check mode: %q", scheduled[0].CheckMode)
	}
	if scheduled[0].DNSServers != "10.0.0.3:53" {
		t.Fatalf("scheduled domain lost DNS servers: %q", scheduled[0].DNSServers)
	}
}

func TestSaveCheckAndGetAllLastChecksKeepAuditFields(t *testing.T) {
	database := newTestDB(t)
	defer database.Close()

	domain, err := database.CreateDomain("api.internal", nil, nil, "", "ssl_only", "10.0.0.1:53", 3600, 443, nil)
	if err != nil {
		t.Fatalf("create domain: %v", err)
	}

	sslDays := 45
	oldCheck := &Check{
		DomainID:        domain.ID,
		CheckedAt:       time.Now().Add(-2 * time.Hour),
		SSLExpiryDays:   &sslDays,
		SSLChainValid:   true,
		DomainSource:    "rdap",
		OverallStatus:   "ok",
		CheckDuration:   10,
		DNSServerUsed:   "per-domain:10.0.0.1:53",
		SSLChainDetails: []ChainCert{{Subject: "old", Issuer: "root"}},
	}
	if err := database.SaveCheck(oldCheck); err != nil {
		t.Fatalf("save old check: %v", err)
	}

	newerDays := 30
	newCheck := &Check{
		DomainID:                 domain.ID,
		CheckedAt:                time.Now().Add(-1 * time.Hour),
		SSLExpiryDays:            &newerDays,
		SSLChainValid:            true,
		DomainStatus:             "not_applicable",
		DomainSource:             "skipped",
		RegistrationCheckSkipped: true,
		RegistrationSkipReason:   "check_mode=ssl_only",
		DNSServerUsed:            "fallback:10.0.0.2:53",
		CAAQueryDomain:           "internal",
		CAAPresent:               true,
		OverallStatus:            "ok",
		CheckDuration:            22,
		SSLChainDetails:          []ChainCert{{Subject: "leaf", Issuer: "root", IsCA: false}},
	}
	if err := database.SaveCheck(newCheck); err != nil {
		t.Fatalf("save new check: %v", err)
	}

	last, err := database.GetLastCheck(domain.ID)
	if err != nil {
		t.Fatalf("GetLastCheck: %v", err)
	}
	if last == nil {
		t.Fatal("expected last check to exist")
	}
	if !last.RegistrationCheckSkipped {
		t.Fatal("expected registration_check_skipped to round-trip")
	}
	if last.RegistrationSkipReason != "check_mode=ssl_only" {
		t.Fatalf("registration skip reason = %q", last.RegistrationSkipReason)
	}
	if last.DNSServerUsed != "fallback:10.0.0.2:53" {
		t.Fatalf("dns_server_used = %q", last.DNSServerUsed)
	}
	if len(last.SSLChainDetails) != 1 || last.SSLChainDetails[0].Subject != "leaf" {
		t.Fatalf("ssl_chain_details not restored correctly: %+v", last.SSLChainDetails)
	}

	all, err := database.GetAllLastChecks()
	if err != nil {
		t.Fatalf("GetAllLastChecks: %v", err)
	}
	got := all[domain.ID]
	if got == nil {
		t.Fatal("expected domain to be present in GetAllLastChecks")
	}
	if got.DNSServerUsed != "fallback:10.0.0.2:53" {
		t.Fatalf("latest dns_server_used = %q", got.DNSServerUsed)
	}
	if !got.CAAPresent {
		t.Fatal("expected CAA presence to round-trip")
	}
}

func newTestDB(t *testing.T) *DB {
	t.Helper()

	path := filepath.Join(t.TempDir(), "test.db")
	database, err := New(path)
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	if err := database.Migrate(); err != nil {
		_ = database.Close()
		t.Fatalf("migrate test db: %v", err)
	}
	return database
}

func columnExists(t *testing.T, database *DB, table, column string) bool {
	t.Helper()

	rows, err := database.sql.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		t.Fatalf("table info for %s: %v", table, err)
	}
	defer rows.Close()

	for rows.Next() {
		var cid int
		var name, dataType string
		var notNull, pk int
		var defaultValue any
		if err := rows.Scan(&cid, &name, &dataType, &notNull, &defaultValue, &pk); err != nil {
			t.Fatalf("scan table info for %s: %v", table, err)
		}
		if name == column {
			return true
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate table info for %s: %v", table, err)
	}
	return false
}
