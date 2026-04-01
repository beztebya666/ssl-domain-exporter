package checker

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"ssl-domain-exporter/internal/config"
	"ssl-domain-exporter/internal/db"
)

func TestCleanupExpiredAuditLogs(t *testing.T) {
	cfg := config.Default()
	cfg.Maintenance.AuditRetentionDays = 30
	cfg.Maintenance.RetentionSweepInterval = "1m"

	database := newSchedulerTestDB(t)
	raw, err := sql.Open("sqlite", database.Path())
	if err != nil {
		t.Fatalf("open raw sqlite handle: %v", err)
	}
	t.Cleanup(func() {
		_ = raw.Close()
	})

	oldCreatedAt := time.Now().AddDate(0, 0, -40).UTC()
	newCreatedAt := time.Now().UTC()
	if _, err := raw.Exec(`
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

	scheduler := NewScheduler(cfg, database, nil, nil)
	scheduler.cleanupExpiredAuditLogs()

	var remaining int
	if err := raw.QueryRow(`SELECT COUNT(*) FROM audit_logs`).Scan(&remaining); err != nil {
		t.Fatalf("count audit logs: %v", err)
	}
	if remaining != 1 {
		t.Fatalf("expected one audit log after retention sweep, got %d", remaining)
	}
	if scheduler.Status().LastAuditSweepAt == nil {
		t.Fatal("expected audit retention sweep timestamp to be recorded")
	}
}

func TestNextWakeDelayUsesEarliestScheduledDomain(t *testing.T) {
	cfg := config.Default()
	database := newSchedulerTestDB(t)

	domain, err := database.CreateDomain("example.com", nil, nil, db.DomainSourceManual, nil, "", "full", "", 1800, 443, nil)
	if err != nil {
		t.Fatalf("create domain: %v", err)
	}
	check := &db.Check{
		DomainID:      domain.ID,
		CheckedAt:     time.Date(2026, time.March, 26, 10, 0, 0, 0, time.UTC),
		OverallStatus: "ok",
		CheckDuration: 10,
		SSLChainValid: true,
		DomainSource:  "rdap",
	}
	if err := database.SaveCheck(check); err != nil {
		t.Fatalf("save check: %v", err)
	}

	scheduler := NewScheduler(cfg, database, nil, nil)
	scheduler.now = func() time.Time {
		return time.Date(2026, time.March, 26, 10, 10, 0, 0, time.UTC)
	}
	scheduler.nextSessionCleanup = scheduler.timeNow().Add(time.Hour)

	delay := scheduler.nextWakeDelay()
	if delay < 19*time.Minute || delay > 21*time.Minute {
		t.Fatalf("expected next wake around 20m, got %s", delay)
	}
}

func TestCleanupExpiredSessionsBacksOffAfterError(t *testing.T) {
	cfg := config.Default()
	database := newSchedulerTestDB(t)
	scheduler := NewScheduler(cfg, database, nil, nil)
	now := time.Date(2026, time.March, 26, 12, 0, 0, 0, time.UTC)
	scheduler.now = func() time.Time {
		return now
	}

	if err := database.Close(); err != nil {
		t.Fatalf("close db: %v", err)
	}

	scheduler.cleanupExpiredSessions()

	if scheduler.Status().LastSessionCleanupAt != nil {
		t.Fatal("expected failed cleanup to avoid reporting a successful timestamp")
	}
	nextCleanup, ok := scheduler.nextSessionCleanupAt(now)
	if !ok {
		t.Fatal("expected session cleanup deadline to be scheduled")
	}
	if !nextCleanup.After(now) {
		t.Fatalf("expected failed cleanup to back off, got next deadline %s", nextCleanup)
	}
	if got := nextCleanup.Sub(now); got != maintenanceRetryBackoff(time.Hour) {
		t.Fatalf("expected retry backoff %s, got %s", maintenanceRetryBackoff(time.Hour), got)
	}
}

func newSchedulerTestDB(t *testing.T) *db.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "checker.db")
	database, err := db.New(path)
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
