package checker

import (
	"log"
	"strings"
	"time"

	"domain-ssl-checker/internal/config"
	"domain-ssl-checker/internal/db"
	"domain-ssl-checker/internal/metrics"
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

	timeout, err := time.ParseDuration(c.cfg.Checker.Timeout)
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

	sslResult := CheckSSL(domain.Name, port, timeout, domain.CustomCAPEM)
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

	if c.cfg.Features.CipherCheck {
		cipher := CheckCipherSuite(domain.Name, port, timeout)
		check.CipherWeak = cipher.IsWeak
		check.CipherWeakReason = cipher.WeakReason
		check.CipherGrade = cipher.Grade
		check.CipherDetails = CipherSummary(cipher)
	}

	if c.cfg.Features.HTTPCheck {
		httpResult := CheckHTTP(domain.Name, port, timeout, domain.CustomCAPEM)
		check.HTTPStatusCode = httpResult.StatusCode
		check.HTTPRedirectsHTTPS = httpResult.RedirectsHTTPS
		check.HTTPHSTSEnabled = httpResult.HSTSEnabled
		check.HTTPHSTSMaxAge = httpResult.HSTSMaxAge
		check.HTTPResponseTimeMs = httpResult.ResponseTimeMs
		check.HTTPFinalURL = httpResult.FinalURL
		check.HTTPError = httpResult.Error
	}

	domResult := CheckDomain(domain.Name, timeout)
	if domResult.Error != "" && c.cfg.Domains.SubdomainFallback && isSubdomain(domain.Name) {
		candidates := candidateDomains(domain.Name, c.cfg.Domains.FallbackDepth)
		for i := 1; i < len(candidates); i++ {
			candidate := candidates[i]
			fallbackResult := CheckDomain(candidate, timeout)
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

	if c.cfg.Features.CAACheck {
		dnsResult := CheckCAA(domain.Name, timeout, c.cfg.Domains.FallbackDepth)
		check.CAAPresent = dnsResult.CAAPresent
		check.CAA = strings.Join(dnsResult.CAARecords, ", ")
		if dnsResult.QueryDomain != "" && dnsResult.QueryDomain != domain.Name {
			check.CAAQueryDomain = dnsResult.QueryDomain
		}
		if dnsResult.Error != "" {
			check.CAAError = dnsResult.Error
		}
	}

	if c.cfg.Features.OCSPCheck || c.cfg.Features.CRLCheck {
		rev := CheckRevocation(domain.Name, port, timeout, c.cfg.Features.OCSPCheck, c.cfg.Features.CRLCheck)
		check.OCSPStatus = rev.OCSPStatus
		check.OCSPError = rev.OCSPError
		check.CRLStatus = rev.CRLStatus
		check.CRLError = rev.CRLError
	}

	check.CheckDuration = time.Since(start).Milliseconds()
	check.OverallStatus = c.computeStatus(check)

	if err := c.db.SaveCheck(check); err != nil {
		log.Printf("Error saving check for %s: %v", domain.Name, err)
	}

	c.metrics.UpdateDomain(domain.Name, check, c.cfg)
	c.notifier.Notify(domain.Name, check, prevStatus)
	return check
}

func domainPort(port int) int {
	if port <= 0 {
		return 443
	}
	return port
}

func (c *Checker) computeStatus(check *db.Check) string {
	if check.SSLCheckError != "" && check.DomainCheckError != "" {
		return "error"
	}

	cfg := c.cfg.Alerts

	if check.SSLExpiryDays != nil {
		if *check.SSLExpiryDays <= cfg.SSLExpiryCriticalDays {
			return "critical"
		}
		if *check.SSLExpiryDays <= cfg.SSLExpiryWarningDays {
			return "warning"
		}
	}

	if check.DomainExpiryDays != nil {
		if *check.DomainExpiryDays <= cfg.DomainExpiryCriticalDays {
			return "critical"
		}
		if *check.DomainExpiryDays <= cfg.DomainExpiryWarningDays {
			return "warning"
		}
	}

	if check.SSLCheckError == "" && !check.SSLChainValid {
		return "warning"
	}

	if c.cfg.Features.HTTPCheck {
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

	if c.cfg.Features.CipherCheck {
		switch strings.ToUpper(check.CipherGrade) {
		case "F":
			return "critical"
		case "C":
			return "warning"
		}
	}

	if c.cfg.Features.OCSPCheck {
		if check.OCSPStatus == "revoked" {
			return "critical"
		}
		if check.OCSPStatus == "unknown" {
			return "warning"
		}
	}
	if c.cfg.Features.CRLCheck {
		if check.CRLStatus == "revoked" {
			return "critical"
		}
		if check.CRLStatus == "unknown" {
			return "warning"
		}
	}

	if c.cfg.Features.CAACheck && !check.CAAPresent && check.CAAError == "" {
		return "warning"
	}

	if check.SSLCheckError != "" || check.DomainCheckError != "" {
		return "warning"
	}

	return "ok"
}
