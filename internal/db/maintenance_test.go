package db

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCreateAuditLogAndListAuditLogs(t *testing.T) {
	database := newMaintenanceTestDB(t)

	if err := database.CreateAuditLog(AuditLog{
		ActorUsername: "admin",
		ActorRole:     "admin",
		ActorSource:   "session",
		Action:        "update",
		Resource:      "config",
		Summary:       "Updated configuration",
		Details:       map[string]any{"sections": []string{"security"}},
		RequestID:     "req-1",
		RemoteAddr:    "127.0.0.1",
	}); err != nil {
		t.Fatalf("create audit log: %v", err)
	}

	items, err := database.ListAuditLogs(10)
	if err != nil {
		t.Fatalf("list audit logs: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected one audit log, got %d", len(items))
	}
	if items[0].Summary != "Updated configuration" {
		t.Fatalf("unexpected summary: %q", items[0].Summary)
	}
	if items[0].Details["sections"] == nil {
		t.Fatal("expected details to be decoded")
	}
}

func TestBackupToAndRestoreSQLiteFile(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "checker.db")
	database, err := New(source)
	if err != nil {
		t.Fatalf("new db: %v", err)
	}
	defer database.Close()
	if err := database.Migrate(); err != nil {
		t.Fatalf("migrate db: %v", err)
	}
	if _, err := database.CreateDomain("backup.example.com", []string{"prod"}, map[string]string{"owner": "platform"}, "", "full", "", 3600, 443, nil); err != nil {
		t.Fatalf("seed domain before backup: %v", err)
	}

	backupPath := filepath.Join(dir, "backup.db")
	if err := database.BackupTo(backupPath); err != nil {
		t.Fatalf("backup db: %v", err)
	}
	if _, err := os.Stat(backupPath); err != nil {
		t.Fatalf("stat backup: %v", err)
	}

	restoredPath := filepath.Join(dir, "restored.db")
	if err := RestoreSQLiteFile(backupPath, restoredPath); err != nil {
		t.Fatalf("restore db: %v", err)
	}
	if _, err := os.Stat(restoredPath); err != nil {
		t.Fatalf("stat restored db: %v", err)
	}

	restored, err := New(restoredPath)
	if err != nil {
		t.Fatalf("open restored db: %v", err)
	}
	defer restored.Close()
	if err := restored.Ping(); err != nil {
		t.Fatalf("ping restored db: %v", err)
	}
	domains, err := restored.GetDomains()
	if err != nil {
		t.Fatalf("read restored domains: %v", err)
	}
	if len(domains) != 1 || domains[0].Name != "backup.example.com" {
		t.Fatalf("unexpected restored domains: %+v", domains)
	}
}

func TestDeleteChecksOlderThan(t *testing.T) {
	database := newMaintenanceTestDB(t)

	domain, err := database.CreateDomain("example.com", nil, nil, "", "full", "", 3600, 443, nil)
	if err != nil {
		t.Fatalf("create domain: %v", err)
	}
	oldCheck := &Check{
		DomainID:         domain.ID,
		CheckedAt:        time.Now().AddDate(0, 0, -40),
		OverallStatus:    "ok",
		CheckDuration:    10,
		SSLChainValid:    true,
		DomainSource:     "rdap",
		SSLCheckError:    "",
		DomainCheckError: "",
	}
	newCheck := &Check{
		DomainID:         domain.ID,
		CheckedAt:        time.Now(),
		OverallStatus:    "ok",
		CheckDuration:    10,
		SSLChainValid:    true,
		DomainSource:     "rdap",
		SSLCheckError:    "",
		DomainCheckError: "",
	}
	if err := database.SaveCheck(oldCheck); err != nil {
		t.Fatalf("save old check: %v", err)
	}
	if err := database.SaveCheck(newCheck); err != nil {
		t.Fatalf("save new check: %v", err)
	}

	removed, err := database.DeleteChecksOlderThan(time.Now().AddDate(0, 0, -30))
	if err != nil {
		t.Fatalf("delete old checks: %v", err)
	}
	if removed != 1 {
		t.Fatalf("expected one removed check, got %d", removed)
	}
}

func TestDeleteAuditLogsOlderThan(t *testing.T) {
	database := newMaintenanceTestDB(t)

	oldCreatedAt := time.Now().AddDate(0, 0, -40).UTC()
	newCreatedAt := time.Now().UTC()
	if _, err := database.sql.Exec(`
		INSERT INTO audit_logs (
			actor_username, actor_role, actor_source, action, resource, summary, details_json,
			remote_addr, request_id, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?), (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		"admin", "admin", "session", "update", "config", "old entry", "{}", "127.0.0.1", "req-old", oldCreatedAt,
		"admin", "admin", "session", "update", "config", "new entry", "{}", "127.0.0.1", "req-new", newCreatedAt,
	); err != nil {
		t.Fatalf("seed audit logs: %v", err)
	}

	removed, err := database.DeleteAuditLogsOlderThan(time.Now().AddDate(0, 0, -30))
	if err != nil {
		t.Fatalf("delete old audit logs: %v", err)
	}
	if removed != 1 {
		t.Fatalf("expected one removed audit log, got %d", removed)
	}

	var remaining int
	if err := database.sql.QueryRow(`SELECT COUNT(*) FROM audit_logs`).Scan(&remaining); err != nil {
		t.Fatalf("count remaining audit logs: %v", err)
	}
	if remaining != 1 {
		t.Fatalf("expected one remaining audit log, got %d", remaining)
	}
}

func newMaintenanceTestDB(t *testing.T) *DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "checker.db")
	database, err := New(path)
	if err != nil {
		t.Fatalf("new db: %v", err)
	}
	t.Cleanup(func() {
		_ = database.Close()
	})
	if err := database.Migrate(); err != nil {
		t.Fatalf("migrate db: %v", err)
	}
	return database
}
