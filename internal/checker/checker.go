package checker

import (
	"log"
	"strings"
	"time"

	"ssl-domain-exporter/internal/config"
	"ssl-domain-exporter/internal/db"
	"ssl-domain-exporter/internal/metrics"
)

const defaultTimeout = 30 * time.Second

type Checker struct {
	cfg      *config.Config
	db       *db.DB
	metrics  *metrics.Metrics
	notifier *Notifier
}

func New(cfg *config.Config, database *db.DB, m *metrics.Metrics, n *Notifier) *Checker {
	return &Checker{cfg: cfg, db: database, metrics: m, notifier: n}
}

func (c *Checker) CheckDomain(domain *db.Domain) *db.Check {
	start := time.Now()

	// Take a snapshot of config for this check (thread-safe)
	cfg := c.cfg.Snapshot()

	timeout, err := time.ParseDuration(cfg.Checker.Timeout)
	if err != nil {
		timeout = defaultTimeout
	}

	prevCheck, _ := c.db.GetLastCheck(domain.ID)
	prevStatus := "unknown"
	if prevCheck != nil {
		prevStatus = prevCheck.OverallStatus
	}

	check := &db.Check{
		DomainID:  domain.ID,
		CheckedAt: time.Now(),
	}
	port := domainPort(domain.Port)

	// Build DNS resolve context for this domain
	rc := BuildResolveContext(domain, cfg)

	// --- SSL Check (always) ---
	sslResult := CheckSSL(domain.Name, port, timeout, domain.CustomCAPEM, rc)

	// Record which DNS server actually responded (set after SSL check triggers resolution)
	check.DNSServerUsed = rc.EffectiveServerDesc()
	check.SSLIssuer = sslResult.Issuer
	check.SSLSubject = sslResult.Subject
	check.SSLVersion = sslResult.Version
	check.SSLChainValid = sslResult.ChainValid
	check.SSLChainLength = sslResult.ChainLength
	check.SSLChainError = sslResult.ChainError
	check.SSLChainDetails = sslResult.ChainCerts
	check.SSLCheckError = sslResult.Error
	if sslResult.Error == "" {
		check.SSLValidFrom = &sslResult.ValidFrom
		check.SSLValidUntil = &sslResult.ValidUntil
		check.SSLExpiryDays = &sslResult.ExpiryDays
	}

	// --- Cipher Check (optional feature, works for all check modes) ---
	if cfg.Features.CipherCheck {
		cipher := CheckCipherSuite(domain.Name, port, timeout, rc)
		check.CipherWeak = cipher.IsWeak
		check.CipherWeakReason = cipher.WeakReason
		check.CipherGrade = cipher.Grade
		check.CipherDetails = CipherSummary(cipher)
	}

	// --- HTTP Check (optional feature, works for all check modes) ---
	if cfg.Features.HTTPCheck {
		httpResult := CheckHTTP(domain.Name, port, timeout, domain.CustomCAPEM, rc)
		check.HTTPStatusCode = httpResult.StatusCode
		check.HTTPRedirectsHTTPS = httpResult.RedirectsHTTPS
		check.HTTPHSTSEnabled = httpResult.HSTSEnabled
		check.HTTPHSTSMaxAge = httpResult.HSTSMaxAge
		check.HTTPResponseTimeMs = httpResult.ResponseTimeMs
		check.HTTPFinalURL = httpResult.FinalURL
		check.HTTPError = httpResult.Error
	}

	// --- Domain Registration Check (RDAP/WHOIS) - only when check_mode is "full" ---
	if domain.RegistrationCheckEnabled() {
		domResult := CheckDomainRegistration(domain.Name, timeout)
		if domResult.Error != "" && cfg.Domains.SubdomainFallback && isSubdomain(domain.Name) {
			candidates := candidateDomains(domain.Name, cfg.Domains.FallbackDepth)
			for i := 1; i < len(candidates); i++ {
				candidate := candidates[i]
				fallbackResult := CheckDomainRegistration(candidate, timeout)
				if fallbackResult.Error == "" {
					log.Printf("[domain] %s: fallback to parent %s", domain.Name, candidate)
					fallbackResult.Source = fallbackResult.Source + " (parent lookup: " + candidate + ")"
					domResult = fallbackResult
					break
				}
			}
		}
		check.DomainStatus = domResult.Status
		check.DomainRegistrar = domResult.Registrar
		check.DomainCreatedAt = domResult.CreatedAt
		check.DomainExpiresAt = domResult.ExpiresAt
		check.DomainExpiryDays = domResult.ExpiryDays
		check.DomainCheckError = domResult.Error
		check.DomainSource = domResult.Source
		check.RegistrationCheckSkipped = false
	} else {
		// SSL-only mode: skip RDAP/WHOIS entirely
		check.DomainStatus = "not_applicable"
		check.DomainSource = "skipped"
		check.RegistrationCheckSkipped = true
		check.RegistrationSkipReason = "check_mode=ssl_only"
		log.Printf("[domain] %s: registration check skipped (ssl_only mode)", domain.Name)
	}

	// --- CAA Check (optional feature, works for all check modes) ---
	if cfg.Features.CAACheck {
		dnsResult := CheckCAA(domain.Name, timeout, cfg.Domains.FallbackDepth, rc)
		check.CAAPresent = dnsResult.CAAPresent
		check.CAA = strings.Join(dnsResult.CAARecords, ", ")
		if dnsResult.QueryDomain != "" && dnsResult.QueryDomain != domain.Name {
			check.CAAQueryDomain = dnsResult.QueryDomain
		}
		if dnsResult.Error != "" {
			check.CAAError = dnsResult.Error
		}
	}

	// --- Revocation Check (optional feature, works for all check modes) ---
	if cfg.Features.OCSPCheck || cfg.Features.CRLCheck {
		rev := CheckRevocation(domain.Name, port, timeout, cfg.Features.OCSPCheck, cfg.Features.CRLCheck, rc)
		check.OCSPStatus = rev.OCSPStatus
		check.OCSPError = rev.OCSPError
		check.CRLStatus = rev.CRLStatus
		check.CRLError = rev.CRLError
	}

	check.CheckDuration = time.Since(start).Milliseconds()
	check.OverallStatus = computeStatus(check, cfg)

	if err := c.db.SaveCheck(check); err != nil {
		log.Printf("Error saving check for %s: %v", domain.Name, err)
	}

	c.metrics.UpdateDomain(domain, check, cfg)
	c.notifier.Notify(domain.Name, check, prevStatus)
	return check
}

func domainPort(port int) int {
	if port <= 0 {
		return 443
	}
	return port
}

// computeStatus determines the overall status based on the check results.
// It uses RegistrationCheckSkipped from the check record itself (audit-safe).
func computeStatus(check *db.Check, cfg *config.Config) string {
	registrationSkipped := check.RegistrationCheckSkipped

	// If both SSL and domain checks failed (and domain was actually checked), it's an error
	if check.SSLCheckError != "" && !registrationSkipped && check.DomainCheckError != "" {
		return "error"
	}
	// If SSL check failed entirely and it's ssl_only mode, that's an error
	if check.SSLCheckError != "" && registrationSkipped {
		return "error"
	}

	alerts := cfg.Alerts

	// SSL expiry thresholds
	if check.SSLExpiryDays != nil {
		if *check.SSLExpiryDays <= alerts.SSLExpiryCriticalDays {
			return "critical"
		}
		if *check.SSLExpiryDays <= alerts.SSLExpiryWarningDays {
			return "warning"
		}
	}

	// Domain expiry thresholds (only when registration was actually checked)
	if !registrationSkipped && check.DomainExpiryDays != nil {
		if *check.DomainExpiryDays <= alerts.DomainExpiryCriticalDays {
			return "critical"
		}
		if *check.DomainExpiryDays <= alerts.DomainExpiryWarningDays {
			return "warning"
		}
	}

	// SSL chain validity
	if check.SSLCheckError == "" && !check.SSLChainValid {
		return "warning"
	}

	// HTTP checks
	if cfg.Features.HTTPCheck {
		if check.HTTPError != "" {
			return "warning"
		}
		if check.HTTPStatusCode >= 500 {
			return "critical"
		}
		if check.HTTPStatusCode >= 400 {
			return "warning"
		}
	}

	// Cipher checks
	if cfg.Features.CipherCheck {
		switch strings.ToUpper(check.CipherGrade) {
		case "F":
			return "critical"
		case "C":
			return "warning"
		}
	}

	// OCSP
	if cfg.Features.OCSPCheck {
		if check.OCSPStatus == "revoked" {
			return "critical"
		}
		if check.OCSPStatus == "unknown" {
			return "warning"
		}
	}

	// CRL
	if cfg.Features.CRLCheck {
		if check.CRLStatus == "revoked" {
			return "critical"
		}
		if check.CRLStatus == "unknown" {
			return "warning"
		}
	}

	// CAA
	if cfg.Features.CAACheck && !check.CAAPresent && check.CAAError == "" {
		return "warning"
	}

	// SSL error alone (without domain error) is a warning
	if check.SSLCheckError != "" {
		return "warning"
	}

	// Domain check error (only when it was actually checked) is a warning
	if !registrationSkipped && check.DomainCheckError != "" {
		return "warning"
	}

	return "ok"
}
