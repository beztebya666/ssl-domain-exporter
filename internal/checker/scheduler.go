package checker

import (
	"log/slog"
	"sync"
	"time"

	"ssl-domain-exporter/internal/config"
	"ssl-domain-exporter/internal/db"
	"ssl-domain-exporter/internal/metrics"
)

type Scheduler struct {
	cfg                *config.Config
	db                 *db.DB
	checker            *Checker
	metrics            *metrics.Metrics
	quit               chan struct{}
	wake               chan struct{}
	wg                 sync.WaitGroup
	sem                chan struct{}
	mu                 sync.Mutex
	started            bool
	inFlight           map[int64]time.Time
	lastTick           time.Time
	lastError          string
	lastSessionCleanup time.Time
	lastRetentionSweep time.Time
	lastAuditSweep     time.Time
	nextSessionCleanup time.Time
	nextRetentionSweep time.Time
	nextAuditSweep     time.Time
	now                func() time.Time
}

type Status struct {
	Started              bool       `json:"started"`
	InFlight             int        `json:"in_flight"`
	LastTickAt           *time.Time `json:"last_tick_at,omitempty"`
	LastError            string     `json:"last_error,omitempty"`
	LastSessionCleanupAt *time.Time `json:"last_session_cleanup_at,omitempty"`
	LastRetentionSweepAt *time.Time `json:"last_retention_sweep_at,omitempty"`
	LastAuditSweepAt     *time.Time `json:"last_audit_sweep_at,omitempty"`
}

func NewScheduler(cfg *config.Config, database *db.DB, chk *Checker, m *metrics.Metrics) *Scheduler {
	concurrency := cfg.Checker.ConcurrentChecks
	if concurrency <= 0 {
		concurrency = 5
	}
	return &Scheduler{
		cfg:      cfg,
		db:       database,
		checker:  chk,
		metrics:  m,
		quit:     make(chan struct{}),
		wake:     make(chan struct{}, 1),
		sem:      make(chan struct{}, concurrency),
		inFlight: make(map[int64]time.Time),
		now:      time.Now,
	}
}

func (s *Scheduler) Start() {
	s.mu.Lock()
	s.started = true
	s.mu.Unlock()
	s.wg.Add(1)
	go s.run()
	slog.Info("Scheduler started")
}

func (s *Scheduler) Stop() {
	close(s.quit)
	s.wg.Wait()
	slog.Info("Scheduler stopped")
}

func (s *Scheduler) run() {
	defer s.wg.Done()

	s.tick()

	for {
		delay := s.nextWakeDelay()
		timer := time.NewTimer(delay)
		select {
		case <-timer.C:
			s.tick()
		case <-s.wake:
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
		case <-s.quit:
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			return
		}
	}
}

func (s *Scheduler) tick() {
	s.setLastTick(s.timeNow())
	s.cleanupExpiredSessions()
	s.cleanupExpiredChecks()
	s.cleanupExpiredAuditLogs()

	domains, err := s.db.GetDomainsForScheduling()
	if err != nil {
		s.setLastError(err)
		slog.Error("Scheduler failed to get domains", "error", err)
		return
	}
	s.setLastError(nil)
	if len(domains) == 0 {
		return
	}
	slog.Info("Scheduler checking domains", "count", len(domains))

	for i := range domains {
		dom := domains[i]
		s.enqueue(&dom)
	}
}

func (s *Scheduler) cleanupExpiredSessions() {
	if s.db == nil {
		return
	}
	interval := time.Hour
	now := s.timeNow()
	s.mu.Lock()
	nextCleanup := s.nextSessionCleanup
	s.mu.Unlock()
	if !nextCleanup.IsZero() && now.Before(nextCleanup) {
		return
	}
	if err := s.db.DeleteExpiredSessions(now.UTC()); err != nil {
		s.scheduleNextSessionCleanup(now.Add(maintenanceRetryBackoff(interval)))
		slog.Error("Scheduler failed to cleanup expired sessions", "error", err)
		return
	}
	s.mu.Lock()
	s.lastSessionCleanup = now
	s.nextSessionCleanup = now.Add(interval)
	s.mu.Unlock()
}

func (s *Scheduler) TriggerCheck(domain *db.Domain) bool {
	accepted := s.enqueue(domain)
	if accepted {
		s.signalWake()
	}
	return accepted
}

func (s *Scheduler) Status() Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	status := Status{
		Started:   s.started,
		InFlight:  len(s.inFlight),
		LastError: s.lastError,
	}
	if !s.lastTick.IsZero() {
		lastTick := s.lastTick
		status.LastTickAt = &lastTick
	}
	if !s.lastSessionCleanup.IsZero() {
		lastCleanup := s.lastSessionCleanup
		status.LastSessionCleanupAt = &lastCleanup
	}
	if !s.lastRetentionSweep.IsZero() {
		lastRetentionSweep := s.lastRetentionSweep
		status.LastRetentionSweepAt = &lastRetentionSweep
	}
	if !s.lastAuditSweep.IsZero() {
		lastAuditSweep := s.lastAuditSweep
		status.LastAuditSweepAt = &lastAuditSweep
	}
	return status
}

func (s *Scheduler) enqueue(domain *db.Domain) bool {
	if domain == nil {
		return false
	}
	if !s.markInFlight(domain.ID) {
		return false
	}
	s.wg.Add(1)
	go func(dom db.Domain) {
		defer s.wg.Done()
		s.sem <- struct{}{}
		defer func() { <-s.sem }()
		defer s.finishInFlight(dom.ID)
		slog.Info("Checking domain", "domain", dom.Name)
		s.checker.CheckDomain(&dom)
	}(*domain)
	return true
}

func (s *Scheduler) markInFlight(domainID int64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.inFlight[domainID]; exists {
		return false
	}
	s.inFlight[domainID] = s.timeNow()
	return true
}

func (s *Scheduler) finishInFlight(domainID int64) {
	s.mu.Lock()
	delete(s.inFlight, domainID)
	s.mu.Unlock()
	s.signalWake()
}

func (s *Scheduler) setLastTick(value time.Time) {
	s.mu.Lock()
	s.lastTick = value
	s.mu.Unlock()
}

func (s *Scheduler) setLastError(err error) {
	s.mu.Lock()
	if err == nil {
		s.lastError = ""
	} else {
		s.lastError = err.Error()
	}
	s.mu.Unlock()
}

func (s *Scheduler) nextWakeDelay() time.Duration {
	now := s.timeNow()
	next := now.Add(time.Hour)

	if due, ok := s.nextSessionCleanupAt(now); ok && due.Before(next) {
		next = due
	}
	if due, ok := s.nextCheckRetentionSweepAt(now); ok && due.Before(next) {
		next = due
	}
	if due, ok := s.nextAuditRetentionSweepAt(now); ok && due.Before(next) {
		next = due
	}
	if s.db != nil {
		due, err := s.db.GetNextScheduledCheckAt(now)
		if err != nil {
			slog.Warn("Scheduler failed to compute next scheduled check", "error", err)
			return 30 * time.Second
		}
		if due != nil && due.Before(next) {
			next = *due
		}
	}

	if !next.After(now) {
		if s.inFlightCount() > 0 {
			return 2 * time.Second
		}
		return 0
	}
	return next.Sub(now)
}

func (s *Scheduler) nextSessionCleanupAt(now time.Time) (time.Time, bool) {
	s.mu.Lock()
	next := s.nextSessionCleanup
	s.mu.Unlock()
	return nextDeadline(now, next), true
}

func (s *Scheduler) nextCheckRetentionSweepAt(now time.Time) (time.Time, bool) {
	snap := s.cfg.Snapshot()
	if snap.Maintenance.CheckRetentionDays <= 0 {
		return time.Time{}, false
	}
	s.mu.Lock()
	next := s.nextRetentionSweep
	s.mu.Unlock()
	return nextDeadline(now, next), true
}

func (s *Scheduler) nextAuditRetentionSweepAt(now time.Time) (time.Time, bool) {
	snap := s.cfg.Snapshot()
	if snap.Maintenance.AuditRetentionDays <= 0 {
		return time.Time{}, false
	}
	s.mu.Lock()
	next := s.nextAuditSweep
	s.mu.Unlock()
	return nextDeadline(now, next), true
}

func nextDeadline(now, next time.Time) time.Time {
	if next.IsZero() {
		return now
	}
	if next.Before(now) {
		return now
	}
	return next
}

func (s *Scheduler) inFlightCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.inFlight)
}

func (s *Scheduler) signalWake() {
	if s == nil {
		return
	}
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

func (s *Scheduler) timeNow() time.Time {
	if s != nil && s.now != nil {
		return s.now()
	}
	return time.Now()
}

func (s *Scheduler) cleanupExpiredChecks() {
	snap := s.cfg.Snapshot()
	retentionDays := snap.Maintenance.CheckRetentionDays
	if retentionDays <= 0 {
		return
	}
	now := s.timeNow()
	sweepInterval := maintenanceSweepInterval(snap)
	s.mu.Lock()
	nextSweep := s.nextRetentionSweep
	s.mu.Unlock()
	if !nextSweep.IsZero() && now.Before(nextSweep) {
		return
	}
	cutoff := now.AddDate(0, 0, -retentionDays)
	removed, err := s.db.DeleteChecksOlderThan(cutoff)
	if err != nil {
		s.scheduleNextRetentionSweep(now.Add(maintenanceRetryBackoff(sweepInterval)))
		slog.Error("Scheduler failed to cleanup expired checks", "error", err)
		return
	}
	if removed > 0 {
		slog.Info("Scheduler removed historical checks", "removed", removed, "retention_days", retentionDays)
	}
	s.mu.Lock()
	s.lastRetentionSweep = now
	s.nextRetentionSweep = now.Add(sweepInterval)
	s.mu.Unlock()
}

func (s *Scheduler) cleanupExpiredAuditLogs() {
	snap := s.cfg.Snapshot()
	retentionDays := snap.Maintenance.AuditRetentionDays
	if retentionDays <= 0 {
		return
	}
	now := s.timeNow()
	sweepInterval := maintenanceSweepInterval(snap)
	s.mu.Lock()
	nextSweep := s.nextAuditSweep
	s.mu.Unlock()
	if !nextSweep.IsZero() && now.Before(nextSweep) {
		return
	}
	cutoff := now.AddDate(0, 0, -retentionDays)
	removed, err := s.db.DeleteAuditLogsOlderThan(cutoff)
	if err != nil {
		s.scheduleNextAuditSweep(now.Add(maintenanceRetryBackoff(sweepInterval)))
		slog.Error("Scheduler failed to cleanup expired audit logs", "error", err)
		return
	}
	if removed > 0 {
		slog.Info("Scheduler removed audit log entries", "removed", removed, "retention_days", retentionDays)
	}
	s.mu.Lock()
	s.lastAuditSweep = now
	s.nextAuditSweep = now.Add(sweepInterval)
	s.mu.Unlock()
}

func (s *Scheduler) scheduleNextSessionCleanup(next time.Time) {
	s.mu.Lock()
	s.nextSessionCleanup = next
	s.mu.Unlock()
}

func (s *Scheduler) scheduleNextRetentionSweep(next time.Time) {
	s.mu.Lock()
	s.nextRetentionSweep = next
	s.mu.Unlock()
}

func (s *Scheduler) scheduleNextAuditSweep(next time.Time) {
	s.mu.Lock()
	s.nextAuditSweep = next
	s.mu.Unlock()
}

func maintenanceSweepInterval(cfg *config.Config) time.Duration {
	if cfg == nil {
		return 24 * time.Hour
	}
	sweepInterval, err := time.ParseDuration(cfg.Maintenance.RetentionSweepInterval)
	if err != nil || sweepInterval <= 0 {
		return 24 * time.Hour
	}
	return sweepInterval
}

func maintenanceRetryBackoff(interval time.Duration) time.Duration {
	backoff := interval / 10
	if backoff < 30*time.Second {
		backoff = 30 * time.Second
	}
	if backoff > 5*time.Minute {
		backoff = 5 * time.Minute
	}
	return backoff
}
