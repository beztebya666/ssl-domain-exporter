package checker

import (
	"fmt"
	"log/slog"
	"sort"
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

func (c *Checker) NotificationStatuses() []DeliveryStatus {
	if c == nil || c.notifier == nil {
		return nil
	}
	return c.notifier.Status()
}

func (c *Checker) SendTestNotifications(channel string, cfgOverride *config.Config) ([]TestResult, error) {
	if c == nil || c.notifier == nil {
		return nil, nil
	}
	return c.notifier.SendTest(cfgOverride, channel)
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

	attempts := cfg.Checker.RetryCount + 1
	if attempts < 1 {
		attempts = 1
	}

	var check *db.Check
	for attempt := 1; attempt <= attempts; attempt++ {
		check = c.runCheckOnce(domain, cfg, timeout)
		assessment := evaluateStatus(check, cfg)
		check.PrimaryReasonCode = assessment.PrimaryReasonCode
		check.PrimaryReasonText = assessment.PrimaryReasonText
		check.StatusReasons = assessment.Reasons
		check.OverallStatus = assessment.Status

		if attempt == attempts || !shouldRetryCheck(check) {
			break
		}

		slog.Warn("Retrying domain check due to transient errors", "domain", domain.Name, "attempt", attempt+1, "max_attempts", attempts)
		time.Sleep(time.Duration(attempt) * 500 * time.Millisecond)
	}

	check.CheckDuration = time.Since(start).Milliseconds()

	if err := c.db.SaveCheck(check); err != nil {
		slog.Error("Failed to save check", "domain", domain.Name, "error", err)
	}

	c.metrics.UpdateDomain(domain, check, cfg)
	c.notifier.Notify(domain.Name, check, prevStatus)
	return check
}

func (c *Checker) runCheckOnce(domain *db.Domain, cfg *config.Config, timeout time.Duration) *db.Check {
	switch domain.EffectiveSourceType() {
	case db.DomainSourceManual:
		return c.runManualCheckOnce(domain, cfg, timeout)
	case db.DomainSourceKubernetesSecret:
		return c.runKubernetesSourceCheck(domain)
	case db.DomainSourceF5Certificate:
		return c.runF5SourceCheck(domain)
	default:
		check := &db.Check{
			DomainID:                  domain.ID,
			CheckedAt:                 time.Now(),
			RegistrationCheckSkipped:  true,
			RegistrationSkipReason:    "unsupported_source_type",
			DomainStatus:              "not_applicable",
			DomainSource:              "source",
			SSLCheckError:             fmt.Sprintf("unsupported source_type %q", domain.SourceType),
			PrimaryReasonCode:         "ssl_check_failed",
			PrimaryReasonText:         fmt.Sprintf("Unsupported source type %q", domain.SourceType),
			OverallStatus:             "error",
		}
		return check
	}
}

func (c *Checker) runManualCheckOnce(domain *db.Domain, cfg *config.Config, timeout time.Duration) *db.Check {
	check := &db.Check{
		DomainID:  domain.ID,
		CheckedAt: time.Now(),
	}
	port := domainPort(domain.Port)
	rc := BuildResolveContext(domain, cfg)

	sslResult := CheckSSL(domain.Name, port, timeout, domain.CustomCAPEM, rc)
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

	if cfg.Features.CipherCheck {
		cipher := CheckCipherSuite(domain.Name, port, timeout, rc)
		check.CipherWeak = cipher.IsWeak
		check.CipherWeakReason = cipher.WeakReason
		check.CipherGrade = cipher.Grade
		check.CipherDetails = CipherSummary(cipher)
	}

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

	if domain.RegistrationCheckEnabled() {
		domResult := CheckDomainRegistration(domain.Name, timeout)
		if domResult.Error != "" && cfg.Domains.SubdomainFallback && isSubdomain(domain.Name) {
			candidates := candidateDomains(domain.Name, cfg.Domains.FallbackDepth)
			for i := 1; i < len(candidates); i++ {
				candidate := candidates[i]
				fallbackResult := CheckDomainRegistration(candidate, timeout)
				if fallbackResult.Error == "" {
					slog.Info("Falling back to parent domain registration lookup", "domain", domain.Name, "candidate", candidate)
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
		check.DomainStatus = "not_applicable"
		check.DomainSource = "skipped"
		check.RegistrationCheckSkipped = true
		check.RegistrationSkipReason = "check_mode=ssl_only"
		slog.Info("Registration check skipped for ssl_only mode", "domain", domain.Name)
	}

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

	if cfg.Features.OCSPCheck || cfg.Features.CRLCheck {
		rev := CheckRevocation(domain.Name, port, timeout, cfg.Features.OCSPCheck, cfg.Features.CRLCheck, rc)
		check.OCSPStatus = rev.OCSPStatus
		check.OCSPError = rev.OCSPError
		check.CRLStatus = rev.CRLStatus
		check.CRLError = rev.CRLError
	}

	return check
}

func shouldRetryCheck(check *db.Check) bool {
	if check == nil {
		return false
	}
	if check.SSLCheckError != "" || check.HTTPError != "" || check.CAAError != "" || check.OCSPError != "" || check.CRLError != "" {
		return true
	}
	if !check.RegistrationCheckSkipped && check.DomainCheckError != "" {
		return true
	}
	return check.OverallStatus == "error"
}

func domainPort(port int) int {
	if port <= 0 {
		return 443
	}
	return port
}

type statusAssessment struct {
	Status            string
	PrimaryReasonCode string
	PrimaryReasonText string
	Reasons           []db.StatusReason
}

// computeStatus preserves the legacy testable helper while status reasons are computed separately.
func computeStatus(check *db.Check, cfg *config.Config) string {
	return evaluateStatus(check, cfg).Status
}

func evaluateStatus(check *db.Check, cfg *config.Config) statusAssessment {
	registrationSkipped := check.RegistrationCheckSkipped
	reasons := make([]db.StatusReason, 0, 8)
	addReason := func(code, severity, summary, detail string) {
		reasons = append(reasons, db.StatusReason{
			Code:     code,
			Severity: severity,
			Summary:  summary,
			Detail:   detail,
		})
	}
	addPolicyReason := func(code, severity, summary, detail string, affectsBadge bool) {
		if !affectsBadge {
			severity = "advisory"
		}
		addReason(code, severity, summary, detail)
	}

	// If both SSL and domain checks failed (and domain was actually checked), it's an error
	if check.SSLCheckError != "" && !registrationSkipped && check.DomainCheckError != "" {
		addReason(
			"ssl_and_domain_check_failed",
			"error",
			"SSL and domain registration checks failed",
			fmt.Sprintf("SSL check failed: %s; domain registration check failed: %s", check.SSLCheckError, check.DomainCheckError),
		)
	}
	// If SSL check failed entirely and it's ssl_only mode, that's an error
	if check.SSLCheckError != "" && registrationSkipped {
		addReason(
			"ssl_check_failed",
			"error",
			"SSL check failed",
			check.SSLCheckError,
		)
	}

	alerts := cfg.Alerts

	// SSL expiry thresholds
	if check.SSLExpiryDays != nil {
		if *check.SSLExpiryDays <= alerts.SSLExpiryCriticalDays {
			addReason(
				"ssl_expiry_critical",
				"critical",
				"SSL certificate expiry is in the critical range",
				fmt.Sprintf("SSL certificate expires in %d days (critical threshold: %d)", *check.SSLExpiryDays, alerts.SSLExpiryCriticalDays),
			)
		} else if *check.SSLExpiryDays <= alerts.SSLExpiryWarningDays {
			addReason(
				"ssl_expiry_warning",
				"warning",
				"SSL certificate expiry is in the warning range",
				fmt.Sprintf("SSL certificate expires in %d days (warning threshold: %d)", *check.SSLExpiryDays, alerts.SSLExpiryWarningDays),
			)
		}
	}

	// Domain expiry thresholds (only when registration was actually checked)
	if !registrationSkipped && check.DomainExpiryDays != nil {
		if *check.DomainExpiryDays <= alerts.DomainExpiryCriticalDays {
			addReason(
				"domain_expiry_critical",
				"critical",
				"Domain registration expiry is in the critical range",
				fmt.Sprintf("Domain registration expires in %d days (critical threshold: %d)", *check.DomainExpiryDays, alerts.DomainExpiryCriticalDays),
			)
		} else if *check.DomainExpiryDays <= alerts.DomainExpiryWarningDays {
			addReason(
				"domain_expiry_warning",
				"warning",
				"Domain registration expiry is in the warning range",
				fmt.Sprintf("Domain registration expires in %d days (warning threshold: %d)", *check.DomainExpiryDays, alerts.DomainExpiryWarningDays),
			)
		}
	}

	// SSL chain validity
	if check.SSLCheckError == "" && !check.SSLChainValid {
		detail := "The certificate chain could not be validated"
		if check.SSLChainError != "" {
			detail = check.SSLChainError
		}
		if hasSelfSignedLeaf(check) {
			addPolicyReason("self_signed_certificate", "warning", "The certificate is self-signed", detail, cfg.StatusPolicy.BadgeOnSelfSigned)
		} else {
			addPolicyReason("ssl_chain_invalid", "warning", "SSL certificate chain is invalid", detail, cfg.StatusPolicy.BadgeOnInvalidChain)
		}
	}

	// HTTP checks
	if cfg.Features.HTTPCheck {
		if check.HTTPError != "" {
			addPolicyReason("http_check_failed", "warning", "HTTP check failed", check.HTTPError, cfg.StatusPolicy.BadgeOnHTTPProbeError)
		}
		if check.HTTPStatusCode >= 500 {
			addReason(
				"http_status_critical",
				"critical",
				"HTTP endpoint is returning a server error",
				fmt.Sprintf("HTTP status code %d returned by %s", check.HTTPStatusCode, effectiveHTTPURL(check)),
			)
		} else if check.HTTPStatusCode >= 400 {
			addPolicyReason(
				"http_status_warning",
				"warning",
				"HTTP endpoint is returning a client error",
				fmt.Sprintf("HTTP status code %d returned by %s", check.HTTPStatusCode, effectiveHTTPURL(check)),
				cfg.StatusPolicy.BadgeOnHTTPClientError,
			)
		}
	}

	// Cipher checks
	if cfg.Features.CipherCheck {
		switch strings.ToUpper(check.CipherGrade) {
		case "F":
			addReason("cipher_grade_f", "critical", "TLS cipher configuration is critically weak", reasonOrFallback(check.CipherDetails, check.CipherWeakReason, "Cipher grade F"))
		case "C":
			addPolicyReason("cipher_grade_c", "warning", "TLS cipher configuration needs attention", reasonOrFallback(check.CipherDetails, check.CipherWeakReason, "Cipher grade C"), cfg.StatusPolicy.BadgeOnCipherWarning)
		}
	}

	// OCSP
	if cfg.Features.OCSPCheck {
		if check.OCSPStatus == "revoked" {
			addReason("ocsp_revoked", "critical", "OCSP reports the certificate as revoked", "The upstream OCSP responder returned revoked")
		}
		if check.OCSPStatus == "unknown" {
			addPolicyReason("ocsp_unknown", "warning", "OCSP status is unknown", reasonOrFallback(check.OCSPError, "", "The OCSP responder did not provide a definitive status"), cfg.StatusPolicy.BadgeOnOCSPUnknown)
		}
	}

	// CRL
	if cfg.Features.CRLCheck {
		if check.CRLStatus == "revoked" {
			addReason("crl_revoked", "critical", "CRL reports the certificate as revoked", "The certificate appears on the certificate revocation list")
		} else if check.CRLStatus == "unknown" {
			addPolicyReason("crl_unknown", "warning", "CRL status is unknown", reasonOrFallback(check.CRLError, "", "The CRL check did not provide a definitive status"), cfg.StatusPolicy.BadgeOnCRLUnknown)
		}
	}

	// CAA
	if cfg.Features.CAACheck && !check.CAAPresent && check.CAAError == "" {
		target := check.CAAQueryDomain
		if target == "" {
			target = "the checked domain"
		}
		addPolicyReason("caa_missing", "warning", "CAA records are not present", fmt.Sprintf("No CAA records were found for %s", target), cfg.StatusPolicy.BadgeOnCAAMissing)
	}

	// SSL error alone (without domain error) is a warning
	if check.SSLCheckError != "" && !registrationSkipped {
		addReason("ssl_check_warning", "warning", "SSL check returned an error", check.SSLCheckError)
	}

	// Domain check error (only when it was actually checked) is a warning
	if !registrationSkipped && check.DomainCheckError != "" {
		addPolicyReason("domain_check_warning", "warning", "Domain registration lookup returned an error", check.DomainCheckError, cfg.StatusPolicy.BadgeOnDomainLookupError)
	}

	if len(reasons) == 0 {
		return statusAssessment{Status: "ok"}
	}

	sort.SliceStable(reasons, func(i, j int) bool {
		return severityRank(reasons[i].Severity) > severityRank(reasons[j].Severity)
	})

	primary := reasons[0]
	for _, reason := range reasons {
		if reason.Severity != "advisory" {
			primary = reason
			break
		}
	}

	status := "ok"
	for _, reason := range reasons {
		if reason.Severity == "advisory" {
			continue
		}
		status = reason.Severity
		break
	}

	return statusAssessment{
		Status:            status,
		PrimaryReasonCode: primary.Code,
		PrimaryReasonText: firstNonEmpty(primary.Detail, primary.Summary),
		Reasons:           reasons,
	}
}

func severityRank(severity string) int {
	switch severity {
	case "error":
		return 4
	case "critical":
		return 3
	case "warning":
		return 2
	case "advisory":
		return 1
	default:
		return 0
	}
}

func reasonOrFallback(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func effectiveHTTPURL(check *db.Check) string {
	if strings.TrimSpace(check.HTTPFinalURL) != "" {
		return check.HTTPFinalURL
	}
	return "the checked HTTP endpoint"
}

func hasSelfSignedLeaf(check *db.Check) bool {
	if check == nil {
		return false
	}
	for _, cert := range check.SSLChainDetails {
		if cert.IsSelfSigned && !cert.IsCA {
			return true
		}
	}
	if len(check.SSLChainDetails) == 1 && check.SSLChainDetails[0].IsSelfSigned {
		return true
	}
	return false
}
