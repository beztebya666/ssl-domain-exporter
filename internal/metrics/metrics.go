package metrics

import (
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"domain-ssl-checker/internal/config"
	"domain-ssl-checker/internal/db"
)

type Metrics struct {
	SSLExpiryDays    *prometheus.GaugeVec
	DomainExpiryDays *prometheus.GaugeVec
	SSLChainValid    *prometheus.GaugeVec
	CheckSuccess     *prometheus.GaugeVec
	CheckDuration    *prometheus.GaugeVec
	LastCheckTime    *prometheus.GaugeVec
	OverallStatus    *prometheus.GaugeVec
	TotalDomains     prometheus.Gauge
	ChecksTotal      *prometheus.CounterVec

	HTTPStatusCode   *prometheus.GaugeVec
	HTTPResponseTime *prometheus.GaugeVec
	HTTPRedirectHTTPS *prometheus.GaugeVec
	HTTPHSTS         *prometheus.GaugeVec
	CipherGrade      *prometheus.GaugeVec
	OCSPStatus       *prometheus.GaugeVec
	CRLStatus        *prometheus.GaugeVec
	CAAPresent       *prometheus.GaugeVec
}

func New() *Metrics {
	return &Metrics{
		SSLExpiryDays: promauto.NewGaugeVec(prometheus.GaugeOpts{
			Name: "domain_ssl_expiry_days",
			Help: "Days until SSL certificate expires",
		}, []string{"domain"}),

		DomainExpiryDays: promauto.NewGaugeVec(prometheus.GaugeOpts{
			Name: "domain_expiry_days",
			Help: "Days until domain registration expires",
		}, []string{"domain"}),

		SSLChainValid: promauto.NewGaugeVec(prometheus.GaugeOpts{
			Name: "domain_ssl_chain_valid",
			Help: "SSL certificate chain validity (1=valid, 0=invalid)",
		}, []string{"domain"}),

		CheckSuccess: promauto.NewGaugeVec(prometheus.GaugeOpts{
			Name: "domain_check_success",
			Help: "Whether the last check succeeded (1=success, 0=failure)",
		}, []string{"domain", "type"}),

		CheckDuration: promauto.NewGaugeVec(prometheus.GaugeOpts{
			Name: "domain_check_duration_ms",
			Help: "Duration of the last check in milliseconds",
		}, []string{"domain"}),

		LastCheckTime: promauto.NewGaugeVec(prometheus.GaugeOpts{
			Name: "domain_last_check_timestamp",
			Help: "Unix timestamp of the last check",
		}, []string{"domain"}),

		OverallStatus: promauto.NewGaugeVec(prometheus.GaugeOpts{
			Name: "domain_overall_status",
			Help: "Overall domain status (0=ok, 1=warning, 2=critical, 3=error)",
		}, []string{"domain"}),

		TotalDomains: promauto.NewGauge(prometheus.GaugeOpts{
			Name: "domain_monitor_total_domains",
			Help: "Total number of monitored domains",
		}),

		ChecksTotal: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "domain_checks_total",
			Help: "Total number of checks performed",
		}, []string{"domain", "status"}),

		HTTPStatusCode: promauto.NewGaugeVec(prometheus.GaugeOpts{
			Name: "domain_http_status_code",
			Help: "Last observed HTTP status code",
		}, []string{"domain"}),

		HTTPResponseTime: promauto.NewGaugeVec(prometheus.GaugeOpts{
			Name: "domain_http_response_time_ms",
			Help: "HTTP response time in milliseconds",
		}, []string{"domain"}),

		HTTPRedirectHTTPS: promauto.NewGaugeVec(prometheus.GaugeOpts{
			Name: "domain_http_redirects_https",
			Help: "Whether HTTP redirects to HTTPS (1=yes, 0=no)",
		}, []string{"domain"}),

		HTTPHSTS: promauto.NewGaugeVec(prometheus.GaugeOpts{
			Name: "domain_http_hsts_enabled",
			Help: "Whether HSTS header is present (1=yes, 0=no)",
		}, []string{"domain"}),

		CipherGrade: promauto.NewGaugeVec(prometheus.GaugeOpts{
			Name: "domain_cipher_grade",
			Help: "Cipher grade mapped to number (A=4, B=3, C=2, F=1, NA=0)",
		}, []string{"domain"}),

		OCSPStatus: promauto.NewGaugeVec(prometheus.GaugeOpts{
			Name: "domain_ocsp_status",
			Help: "OCSP status (good=1, unknown=0, revoked=-1, unavailable=0)",
		}, []string{"domain"}),

		CRLStatus: promauto.NewGaugeVec(prometheus.GaugeOpts{
			Name: "domain_crl_status",
			Help: "CRL status (good=1, unknown=0, revoked=-1, unavailable=0)",
		}, []string{"domain"}),

		CAAPresent: promauto.NewGaugeVec(prometheus.GaugeOpts{
			Name: "domain_caa_present",
			Help: "Whether CAA records are present (1=yes, 0=no)",
		}, []string{"domain"}),
	}
}

func (m *Metrics) UpdateDomain(domain string, check *db.Check, cfg *config.Config) {
	if check.SSLExpiryDays != nil {
		m.SSLExpiryDays.WithLabelValues(domain).Set(float64(*check.SSLExpiryDays))
	}
	if check.DomainExpiryDays != nil {
		m.DomainExpiryDays.WithLabelValues(domain).Set(float64(*check.DomainExpiryDays))
	}

	chainVal := 0.0
	if check.SSLChainValid {
		chainVal = 1.0
	}
	m.SSLChainValid.WithLabelValues(domain).Set(chainVal)

	sslSuccess := 1.0
	if check.SSLCheckError != "" {
		sslSuccess = 0.0
	}
	m.CheckSuccess.WithLabelValues(domain, "ssl").Set(sslSuccess)

	domainSuccess := 1.0
	if check.DomainCheckError != "" {
		domainSuccess = 0.0
	}
	m.CheckSuccess.WithLabelValues(domain, "domain").Set(domainSuccess)

	m.CheckDuration.WithLabelValues(domain).Set(float64(check.CheckDuration))
	m.LastCheckTime.WithLabelValues(domain).Set(float64(check.CheckedAt.Unix()))
	m.OverallStatus.WithLabelValues(domain).Set(statusToFloat(check.OverallStatus))
	m.ChecksTotal.WithLabelValues(domain, check.OverallStatus).Inc()

	if cfg.Features.HTTPCheck {
		m.HTTPStatusCode.WithLabelValues(domain).Set(float64(check.HTTPStatusCode))
		m.HTTPResponseTime.WithLabelValues(domain).Set(float64(check.HTTPResponseTimeMs))
		m.HTTPRedirectHTTPS.WithLabelValues(domain).Set(boolFloat(check.HTTPRedirectsHTTPS))
		m.HTTPHSTS.WithLabelValues(domain).Set(boolFloat(check.HTTPHSTSEnabled))
	}
	if cfg.Features.CipherCheck {
		m.CipherGrade.WithLabelValues(domain).Set(cipherGradeToFloat(check.CipherGrade))
	}
	if cfg.Features.OCSPCheck {
		m.OCSPStatus.WithLabelValues(domain).Set(revocationToFloat(check.OCSPStatus))
	}
	if cfg.Features.CRLCheck {
		m.CRLStatus.WithLabelValues(domain).Set(revocationToFloat(check.CRLStatus))
	}
	if cfg.Features.CAACheck {
		m.CAAPresent.WithLabelValues(domain).Set(boolFloat(check.CAAPresent))
	}
}

func (m *Metrics) SetTotalDomains(n int) {
	m.TotalDomains.Set(float64(n))
}

func statusToFloat(s string) float64 {
	switch s {
	case "ok":
		return 0
	case "warning":
		return 1
	case "critical":
		return 2
	case "error":
		return 3
	default:
		return 3
	}
}

func boolFloat(v bool) float64 {
	if v {
		return 1
	}
	return 0
}

func cipherGradeToFloat(grade string) float64 {
	switch strings.ToUpper(strings.TrimSpace(grade)) {
	case "A":
		return 4
	case "B":
		return 3
	case "C":
		return 2
	case "F":
		return 1
	default:
		return 0
	}
}

func revocationToFloat(status string) float64 {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "good":
		return 1
	case "revoked":
		return -1
	default:
		return 0
	}
}
