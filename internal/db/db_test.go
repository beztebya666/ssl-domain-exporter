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
	if _, err := database.sql.Exec(`INSERT INTO domains (name, tags) VALUES ('legacy.example.com', 'prod, infra')`); err != nil {
		t.Fatalf("seed legacy tags: %v", err)
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

	legacy, err := database.GetDomainByID(1)
	if err != nil {
		t.Fatalf("get migrated legacy domain: %v", err)
	}
	if legacy == nil || len(legacy.Tags) != 2 || legacy.Tags[0] != "prod" || legacy.Tags[1] != "infra" {
		t.Fatalf("expected legacy tags to be backfilled into tags_json, got %+v", legacy)
	}
}

func TestMigrateLegacyFoldersAddsTimestampsAndListWorks(t *testing.T) {
	database := newTestDB(t)
	defer database.Close()

	if _, err := database.sql.Exec(`
		DROP TABLE IF EXISTS domains;
		DROP TABLE IF EXISTS folders;
		CREATE TABLE folders (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT UNIQUE NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);
		CREATE TABLE domains (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT UNIQUE NOT NULL,
			enabled INTEGER DEFAULT 1,
			check_interval INTEGER DEFAULT 21600,
			tags TEXT DEFAULT '',
			folder_id INTEGER,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);
	`); err != nil {
		t.Fatalf("create legacy folder schema: %v", err)
	}

	if _, err := database.sql.Exec(`INSERT INTO folders (name) VALUES ('Operations')`); err != nil {
		t.Fatalf("seed legacy folder: %v", err)
	}
	if _, err := database.sql.Exec(`INSERT INTO domains (name, folder_id) VALUES ('legacy.example.com', 1)`); err != nil {
		t.Fatalf("seed legacy domain: %v", err)
	}

	if err := database.Migrate(); err != nil {
		t.Fatalf("migrate legacy folder schema: %v", err)
	}

	for _, col := range []string{"sort_order", "created_at", "updated_at"} {
		if !columnExists(t, database, "folders", col) {
			t.Fatalf("expected folders.%s to exist after migration", col)
		}
	}

	folders, err := database.GetFolders()
	if err != nil {
		t.Fatalf("list folders after migration: %v", err)
	}
	if len(folders) != 1 {
		t.Fatalf("expected 1 folder, got %d", len(folders))
	}
	if folders[0].DomainCount != 1 {
		t.Fatalf("expected domain count 1, got %d", folders[0].DomainCount)
	}
	if folders[0].CreatedAt.IsZero() || folders[0].UpdatedAt.IsZero() {
		t.Fatalf("expected non-zero timestamps, got %+v", folders[0])
	}
}

func TestCreateUpdateAndScheduleDomainWithEnterpriseFields(t *testing.T) {
	database := newTestDB(t)
	defer database.Close()

	const rootPEM = "-----BEGIN CERTIFICATE-----\nTEST\n-----END CERTIFICATE-----"

	domain, err := database.CreateDomain("vcenter.internal", []string{"infra"}, map[string]string{"owner": "platform"}, DomainSourceManual, nil, rootPEM, "ssl_only", "10.0.0.1:53, 10.0.0.2:53", 3600, 8443, nil)
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
	if err := database.UpdateDomain(domain.ID, "vcsa.internal", []string{"platform", "prod"}, map[string]string{"owner": "ops", "zone": "corp"}, DomainSourceManual, nil, updatedPEM, "full", "10.0.0.3:53", true, 7200, 443, nil); err != nil {
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

	domain, err := database.CreateDomain("api.internal", nil, nil, DomainSourceManual, nil, "", "ssl_only", "10.0.0.1:53", 3600, 443, nil)
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

func TestGetLastCheckAndAllLastChecksPreferNewestIDOnTimestampTie(t *testing.T) {
	database := newTestDB(t)
	defer database.Close()

	domain, err := database.CreateDomain("tie.example.com", nil, nil, DomainSourceManual, nil, "", "full", "", 3600, 443, nil)
	if err != nil {
		t.Fatalf("create domain: %v", err)
	}

	checkedAt := time.Now().UTC().Truncate(time.Second)
	first := &Check{
		DomainID:      domain.ID,
		CheckedAt:     checkedAt,
		OverallStatus: "warning",
		CheckDuration: 10,
		SSLChainValid: true,
		DomainSource:  "rdap",
	}
	second := &Check{
		DomainID:      domain.ID,
		CheckedAt:     checkedAt,
		OverallStatus: "critical",
		CheckDuration: 12,
		SSLChainValid: true,
		DomainSource:  "rdap",
	}
	if err := database.SaveCheck(first); err != nil {
		t.Fatalf("save first check: %v", err)
	}
	if err := database.SaveCheck(second); err != nil {
		t.Fatalf("save second check: %v", err)
	}

	last, err := database.GetLastCheck(domain.ID)
	if err != nil {
		t.Fatalf("GetLastCheck: %v", err)
	}
	if last == nil || last.OverallStatus != "critical" {
		t.Fatalf("expected newest tied check to win, got %+v", last)
	}

	all, err := database.GetAllLastChecks()
	if err != nil {
		t.Fatalf("GetAllLastChecks: %v", err)
	}
	if all[domain.ID] == nil || all[domain.ID].OverallStatus != "critical" {
		t.Fatalf("expected newest tied check from GetAllLastChecks, got %+v", all[domain.ID])
	}
}

func TestGetNextScheduledCheckAt(t *testing.T) {
	database := newTestDB(t)
	defer database.Close()

	now := time.Now().UTC()
	domain, err := database.CreateDomain("example.com", nil, nil, DomainSourceManual, nil, "", "full", "", 3600, 443, nil)
	if err != nil {
		t.Fatalf("create domain: %v", err)
	}
	check := &Check{
		DomainID:      domain.ID,
		CheckedAt:     now.Add(-30 * time.Minute),
		OverallStatus: "ok",
		CheckDuration: 10,
		SSLChainValid: true,
		DomainSource:  "rdap",
	}
	if err := database.SaveCheck(check); err != nil {
		t.Fatalf("save check: %v", err)
	}

	nextAt, err := database.GetNextScheduledCheckAt(now)
	if err != nil {
		t.Fatalf("GetNextScheduledCheckAt: %v", err)
	}
	if nextAt == nil {
		t.Fatal("expected next scheduled check time")
	}
	diff := nextAt.Sub(now)
	if diff < 29*time.Minute || diff > 31*time.Minute {
		t.Fatalf("expected next scheduled check around 30m, got %s", diff)
	}
}

func TestGetNextScheduledCheckAtParsesLegacyTimestampWithMonotonicSuffix(t *testing.T) {
	database := newTestDB(t)
	defer database.Close()

	now := time.Date(2026, time.March, 27, 10, 0, 0, 0, time.UTC)
	domain, err := database.CreateDomain("legacy.example.com", nil, nil, DomainSourceManual, nil, "", "full", "", 3600, 443, nil)
	if err != nil {
		t.Fatalf("create domain: %v", err)
	}

	if _, err := database.sql.Exec(`
		INSERT INTO domain_checks (
			domain_id, checked_at, overall_status, check_duration_ms, ssl_chain_valid, domain_source
		) VALUES (?, ?, ?, ?, ?, ?)
	`, domain.ID, "2026-03-27 09:30:00.123456789 +0000 UTC m=+0.126827601", "ok", 10, true, "rdap"); err != nil {
		t.Fatalf("insert legacy check: %v", err)
	}

	nextAt, err := database.GetNextScheduledCheckAt(now)
	if err != nil {
		t.Fatalf("GetNextScheduledCheckAt: %v", err)
	}
	if nextAt == nil {
		t.Fatal("expected next scheduled check time")
	}
	diff := nextAt.Sub(now)
	if diff < 29*time.Minute || diff > 31*time.Minute {
		t.Fatalf("expected next scheduled check around 30m, got %s", diff)
	}
}

func TestListDomainsPageAndListTagsUseTagsJSON(t *testing.T) {
	database := newTestDB(t)
	defer database.Close()

	domain, err := database.CreateDomain("json-tags.example.com", []string{"Prod", "owner-email"}, nil, DomainSourceManual, nil, "", "full", "", 3600, 443, nil)
	if err != nil {
		t.Fatalf("create domain: %v", err)
	}
	if _, err := database.sql.Exec(`UPDATE domains SET tags = '' WHERE id = ?`, domain.ID); err != nil {
		t.Fatalf("desync legacy tags column: %v", err)
	}

	page, total, err := database.ListDomainsPage(DomainListQuery{Search: "prod", Page: 1, PageSize: 20})
	if err != nil {
		t.Fatalf("ListDomainsPage search: %v", err)
	}
	if total != 1 || len(page) != 1 || page[0].ID != domain.ID {
		t.Fatalf("expected search by tags_json to find domain, total=%d items=%+v", total, page)
	}

	page, total, err = database.ListDomainsPage(DomainListQuery{Tag: "prod", Page: 1, PageSize: 20})
	if err != nil {
		t.Fatalf("ListDomainsPage tag filter: %v", err)
	}
	if total != 1 || len(page) != 1 || page[0].ID != domain.ID {
		t.Fatalf("expected tag filter by tags_json to find domain, total=%d items=%+v", total, page)
	}

	tags, err := database.ListTags()
	if err != nil {
		t.Fatalf("ListTags: %v", err)
	}
	if len(tags) != 2 || tags[0] != "owner-email" || tags[1] != "Prod" {
		t.Fatalf("expected tags_json values to be listed, got %+v", tags)
	}
}

func TestCustomFieldSchemaCRUDAndValidation(t *testing.T) {
	database := newTestDB(t)
	defer database.Close()

	field, err := database.CreateCustomField(CustomField{
		Key:              "owner_email",
		Label:            "Owner Email",
		Type:             "email",
		Required:         true,
		VisibleInDetails: true,
		VisibleInExport:  true,
		Filterable:       true,
		Enabled:          true,
	})
	if err != nil {
		t.Fatalf("create email field: %v", err)
	}
	if field.Key != "owner_email" || field.Type != "email" {
		t.Fatalf("unexpected created field: %+v", field)
	}
	if field.Options == nil {
		t.Fatal("expected non-select custom field options to be an empty slice")
	}

	selectField, err := database.CreateCustomField(CustomField{
		Key:       "environment",
		Label:     "Environment",
		Type:      "select",
		Enabled:   true,
		SortOrder: 2,
		Options: []CustomFieldOption{
			{Value: "prod", Label: "Production"},
			{Value: "stage", Label: "Staging"},
		},
	})
	if err != nil {
		t.Fatalf("create select field: %v", err)
	}
	if len(selectField.Options) != 2 {
		t.Fatalf("expected options to round-trip, got %+v", selectField.Options)
	}

	fields, err := database.ListCustomFields(false)
	if err != nil {
		t.Fatalf("list custom fields: %v", err)
	}
	if len(fields) != 2 {
		t.Fatalf("expected 2 enabled fields, got %d", len(fields))
	}
	if fields[0].Key == "owner_email" && fields[0].Options == nil {
		t.Fatal("expected listed non-select custom field options to be an empty slice")
	}
	if fields[1].Key == "owner_email" && fields[1].Options == nil {
		t.Fatal("expected listed non-select custom field options to be an empty slice")
	}

	if _, err := ValidateMetadataWithCustomFields(map[string]string{
		"owner_email": "invalid-email",
		"environment": "prod",
	}, fields); err == nil {
		t.Fatal("expected invalid email metadata to fail validation")
	}

	validMetadata, err := ValidateMetadataWithCustomFields(map[string]string{
		"owner_email": "platform@example.com",
		"environment": "prod",
		"owner":       "platform-team",
	}, fields)
	if err != nil {
		t.Fatalf("validate metadata: %v", err)
	}
	if validMetadata["owner"] != "platform-team" {
		t.Fatalf("expected unknown metadata keys to be preserved, got %#v", validMetadata)
	}

	updated, err := database.UpdateCustomField(selectField.ID, CustomField{
		Key:              selectField.Key,
		Label:            "Environment",
		Type:             "select",
		VisibleInTable:   true,
		VisibleInDetails: true,
		VisibleInExport:  true,
		Filterable:       true,
		Enabled:          false,
		SortOrder:        3,
		Options: []CustomFieldOption{
			{Value: "prod", Label: "Production"},
			{Value: "qa", Label: "QA"},
		},
	})
	if err != nil {
		t.Fatalf("update custom field: %v", err)
	}
	if updated.Enabled {
		t.Fatal("expected updated field to be disabled")
	}
	if len(updated.Options) != 2 || updated.Options[1].Value != "qa" {
		t.Fatalf("expected updated options to round-trip, got %+v", updated.Options)
	}

	enabledFields, err := database.ListCustomFields(false)
	if err != nil {
		t.Fatalf("list enabled custom fields: %v", err)
	}
	if len(enabledFields) != 1 || enabledFields[0].Key != "owner_email" {
		t.Fatalf("expected only owner_email to stay enabled, got %+v", enabledFields)
	}

	if err := database.DeleteCustomField(field.ID); err != nil {
		t.Fatalf("delete custom field: %v", err)
	}
	remaining, err := database.ListCustomFields(true)
	if err != nil {
		t.Fatalf("list all fields after delete: %v", err)
	}
	if len(remaining) != 1 || remaining[0].Key != "environment" {
		t.Fatalf("expected environment field to remain after delete, got %+v", remaining)
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
