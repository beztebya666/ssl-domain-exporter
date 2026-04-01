package checker

import (
	"fmt"
	"strings"
	"time"

	"ssl-domain-exporter/internal/db"
)

func (c *Checker) runKubernetesSourceCheck(domain *db.Domain) *db.Check {
	check := baseSourceCheck(domain)
	cfg := c.cfg.Snapshot()

	cert, err := FindK8SCertificate(
		K8SConfig{
			Enabled:            cfg.Kubernetes.Enabled,
			APIServer:          cfg.Kubernetes.APIServer,
			Token:              cfg.Kubernetes.Token,
			TokenFile:          cfg.Kubernetes.TokenFile,
			Namespace:          cfg.Kubernetes.Namespace,
			LabelSelector:      cfg.Kubernetes.LabelSelector,
			InsecureSkipVerify: cfg.Kubernetes.InsecureSkipVerify,
			CACertFile:         cfg.Kubernetes.CACertFile,
		},
		domain.SourceRef["namespace"],
		domain.SourceRef["secret_name"],
		domain.SourceRef["certificate_serial"],
	)
	if err != nil {
		check.SSLCheckError = err.Error()
		return check
	}

	populateSourceCertificateCheck(check, cert.Subject, cert.Issuer, cert.NotBefore, cert.NotAfter, cert.ExpiryDays, cert.IsExpired)
	check.SSLVersion = "inventory:kubernetes_secret"
	check.PrimaryReasonText = fmt.Sprintf("Tracked from Kubernetes secret %s/%s", cert.Namespace, cert.SecretName)
	return check
}

func (c *Checker) runF5SourceCheck(domain *db.Domain) *db.Check {
	check := baseSourceCheck(domain)
	cfg := c.cfg.Snapshot()

	cert, err := FindF5Certificate(
		F5Config{
			Enabled:            cfg.F5.Enabled,
			Host:               cfg.F5.Host,
			Username:           cfg.F5.Username,
			Password:           cfg.F5.Password,
			InsecureSkipVerify: cfg.F5.InsecureSkipVerify,
			Partition:          cfg.F5.Partition,
		},
		domain.SourceRef["partition"],
		domain.SourceRef["certificate_name"],
		domain.SourceRef["serial"],
	)
	if err != nil {
		check.SSLCheckError = err.Error()
		return check
	}

	populateSourceCertificateCheck(check, cert.Subject, cert.Issuer, cert.NotBefore, cert.NotAfter, cert.ExpiryDays, cert.IsExpired)
	check.SSLVersion = "inventory:f5_certificate"
	check.PrimaryReasonText = fmt.Sprintf("Tracked from F5 certificate %s/%s", cert.Partition, cert.Name)
	return check
}

func baseSourceCheck(domain *db.Domain) *db.Check {
	reason := fmt.Sprintf("source_type=%s", domain.EffectiveSourceType())
	return &db.Check{
		DomainID:                 domain.ID,
		CheckedAt:                time.Now(),
		DomainStatus:             "not_applicable",
		DomainSource:             "source",
		RegistrationCheckSkipped: true,
		RegistrationSkipReason:   reason,
		SSLChainValid:            true,
		SSLChainLength:           1,
	}
}

func populateSourceCertificateCheck(check *db.Check, subject, issuer string, notBefore, notAfter time.Time, expiryDays int, isExpired bool) {
	if check == nil {
		return
	}
	subject = strings.TrimSpace(subject)
	issuer = strings.TrimSpace(issuer)
	check.SSLSubject = subject
	check.SSLIssuer = issuer
	if !notBefore.IsZero() {
		check.SSLValidFrom = &notBefore
	}
	if !notAfter.IsZero() {
		check.SSLValidUntil = &notAfter
	}
	check.SSLExpiryDays = &expiryDays
	if isExpired {
		check.SSLChainError = "source certificate has expired"
	}
}
