package checker

import (
	"log"
	"sync"
	"time"

	"domain-ssl-checker/internal/config"
	"domain-ssl-checker/internal/db"
	"domain-ssl-checker/internal/metrics"
)

type Scheduler struct {
	cfg     *config.Config
	db      *db.DB
	checker *Checker
	metrics *metrics.Metrics
	quit    chan struct{}
	wg      sync.WaitGroup
	sem     chan struct{}
}

func NewScheduler(cfg *config.Config, database *db.DB, chk *Checker, m *metrics.Metrics) *Scheduler {
	concurrency := cfg.Checker.ConcurrentChecks
	if concurrency <= 0 {
		concurrency = 5
	}
	return &Scheduler{
		cfg:     cfg,
		db:      database,
		checker: chk,
		metrics: m,
		quit:    make(chan struct{}),
		sem:     make(chan struct{}, concurrency),
	}
}

func (s *Scheduler) Start() {
	s.wg.Add(1)
	go s.run()
	log.Println("Scheduler started")
}

func (s *Scheduler) Stop() {
	close(s.quit)
	s.wg.Wait()
	log.Println("Scheduler stopped")
}

func (s *Scheduler) run() {
	defer s.wg.Done()

	// Run immediately on start
	s.tick()

	ticker := time.NewTicker(60 * time.Second) // Check every minute if any domain needs checking
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.tick()
		case <-s.quit:
			return
		}
	}
}

func (s *Scheduler) tick() {
	domains, err := s.db.GetDomainsForScheduling()
	if err != nil {
		log.Printf("Scheduler: failed to get domains: %v", err)
		return
	}
	if len(domains) == 0 {
		return
	}
	log.Printf("Scheduler: checking %d domain(s)", len(domains))

	for i := range domains {
		dom := domains[i]
		s.wg.Add(1)
		s.sem <- struct{}{} // acquire
		go func() {
			defer s.wg.Done()
			defer func() { <-s.sem }() // release
			log.Printf("Checking domain: %s", dom.Name)
			s.checker.CheckDomain(&dom)
		}()
	}
}

// TriggerCheck triggers an immediate check for a specific domain
func (s *Scheduler) TriggerCheck(domain *db.Domain) {
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.sem <- struct{}{}
		defer func() { <-s.sem }()
		s.checker.CheckDomain(domain)
	}()
}
