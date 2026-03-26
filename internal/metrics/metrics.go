package metrics

import (
	"strings"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"ssl-domain-exporter/internal/config"
	"ssl-domain-exporter/internal/db"
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

	HTTPStatusCode    *prometheus.GaugeVec
	HTTPResponseTime  *prometheus.GaugeVec
	HTTPRedirectHTTPS *prometheus.GaugeVec
	HTTPHSTS          *prometheus.GaugeVec
	CipherGrade       *prometheus.GaugeVec
	OCSPStatus        *prometheus.GaugeVec
	CRLStatus         *prometheus.GaugeVec
	CAAPresent        *prometheus.GaugeVec

	RegistrationCheckEnabled *prometheus.GaugeVec
	RegistrationCheckSkipped *prometheus.CounterVec
	DomainTagInfo            *prometheus.GaugeVec
	DomainMetadataInfo       *prometheus.GaugeVec

	cfg            *config.Config
	mu             sync.Mutex
	domainTags     map[string]map[string]struct{}
	domainMetadata map[string]map[string]struct{}
}

type exportSettings struct {
	exportTags     bool
	exportMetadata bool
	metadataKeys   map[string]struct{}
}

func New(cfg *config.Config) *Metrics {
	return NewWithConfigAndRegisterer(cfg, prometheus.DefaultRegisterer)
}

// NewWithRegisterer creates metrics on the provided registry.
// Tests use this to avoid polluting the global Prometheus registry.
func NewWithRegisterer(reg prometheus.Registerer) *Metrics {
	return NewWithConfigAndRegisterer(nil, reg)
}

// NewWithConfigAndRegisterer creates metrics bound to a live config object.
func NewWithConfigAndRegisterer(cfg *config.Config, reg prometheus.Registerer) *Metrics {
	if reg == nil {
		reg = prometheus.DefaultRegisterer
	}
	factory := promauto.With(reg)

	return &Metrics{
		SSLExpiryDays: factory.NewGaugeVec(prometheus.GaugeOpts{
			Name: "domain_ssl_expiry_days",
			Help: "Days until SSL certificate expires",
		}, []string{"domain"}),

		DomainExpiryDays: factory.NewGaugeVec(prometheus.GaugeOpts{
			Name: "domain_expiry_days",
			Help: "Days until domain registration expires",
		}, []string{"domain"}),

		SSLChainValid: factory.NewGaugeVec(prometheus.GaugeOpts{
			Name: "domain_ssl_chain_valid",
			Help: "SSL certificate chain validity (1=valid, 0=invalid)",
		}, []string{"domain"}),

		CheckSuccess: factory.NewGaugeVec(prometheus.GaugeOpts{
			Name: "domain_check_success",
			Help: "Whether the last check succeeded (1=success, 0=failure)",
		}, []string{"domain", "type"}),

		CheckDuration: factory.NewGaugeVec(prometheus.GaugeOpts{
			Name: "domain_check_duration_ms",
			Help: "Duration of the last check in milliseconds",
		}, []string{"domain"}),

		LastCheckTime: factory.NewGaugeVec(prometheus.GaugeOpts{
			Name: "domain_last_check_timestamp",
			Help: "Unix timestamp of the last check",
		}, []string{"domain"}),

		OverallStatus: factory.NewGaugeVec(prometheus.GaugeOpts{
			Name: "domain_overall_status",
			Help: "Overall domain status (0=ok, 1=warning, 2=critical, 3=error, 4=unknown)",
		}, []string{"domain"}),

		TotalDomains: factory.NewGauge(prometheus.GaugeOpts{
			Name: "domain_monitor_total_domains",
			Help: "Total number of monitored domains",
		}),

		ChecksTotal: factory.NewCounterVec(prometheus.CounterOpts{
			Name: "domain_checks_total",
			Help: "Total number of checks performed",
		}, []string{"domain", "status"}),

		HTTPStatusCode: factory.NewGaugeVec(prometheus.GaugeOpts{
			Name: "domain_http_status_code",
			Help: "Last observed HTTP status code",
		}, []string{"domain"}),

		HTTPResponseTime: factory.NewGaugeVec(prometheus.GaugeOpts{
			Name: "domain_http_response_time_ms",
			Help: "HTTP response time in milliseconds",
		}, []string{"domain"}),

		HTTPRedirectHTTPS: factory.NewGaugeVec(prometheus.GaugeOpts{
			Name: "domain_http_redirects_https",
			Help: "Whether HTTP redirects to HTTPS (1=yes, 0=no)",
		}, []string{"domain"}),

		HTTPHSTS: factory.NewGaugeVec(prometheus.GaugeOpts{
			Name: "domain_http_hsts_enabled",
			Help: "Whether HSTS header is present (1=yes, 0=no)",
		}, []string{"domain"}),

		CipherGrade: factory.NewGaugeVec(prometheus.GaugeOpts{
			Name: "domain_cipher_grade",
			Help: "Cipher grade mapped to number (A=4, B=3, C=2, F=1, NA=0)",
		}, []string{"domain"}),

		OCSPStatus: factory.NewGaugeVec(prometheus.GaugeOpts{
			Name: "domain_ocsp_status",
			Help: "OCSP status (good=1, unknown=0, revoked=-1, unavailable=0)",
		}, []string{"domain"}),

		CRLStatus: factory.NewGaugeVec(prometheus.GaugeOpts{
			Name: "domain_crl_status",
			Help: "CRL status (good=1, unknown=0, revoked=-1, unavailable=0)",
		}, []string{"domain"}),

		CAAPresent: factory.NewGaugeVec(prometheus.GaugeOpts{
			Name: "domain_caa_present",
			Help: "Whether CAA records are present (1=yes, 0=no)",
		}, []string{"domain"}),

		RegistrationCheckEnabled: factory.NewGaugeVec(prometheus.GaugeOpts{
			Name: "domain_registration_check_enabled",
			Help: "Whether domain registration check is enabled (1=full, 0=ssl_only)",
		}, []string{"domain"}),

		RegistrationCheckSkipped: factory.NewCounterVec(prometheus.CounterOpts{
			Name: "domain_registration_check_skipped_total",
			Help: "Total number of checks where domain registration lookup was skipped",
		}, []string{"domain"}),

		DomainTagInfo: factory.NewGaugeVec(prometheus.GaugeOpts{
			Name: "domain_tag_info",
			Help: "Static domain tag info metric with value 1 for each domain/tag pair",
		}, []string{"domain", "tag"}),

		DomainMetadataInfo: factory.NewGaugeVec(prometheus.GaugeOpts{
			Name: "domain_metadata_info",
			Help: "Static domain metadata info metric with value 1 for each domain metadata key/value pair",
		}, []string{"domain", "key", "value"}),

		cfg:            cfg,
		domainTags:     make(map[string]map[string]struct{}),
		domainMetadata: make(map[string]map[string]struct{}),
	}
}

func (m *Metrics) SyncDomains(domains []db.Domain) {
	settings := m.exportSettings(nil)
	for i := range domains {
		m.syncDomain(&domains[i], settings)
	}
}

func (m *Metrics) SyncDomain(domain *db.Domain) {
	m.syncDomain(domain, m.exportSettings(nil))
}

func (m *Metrics) syncDomain(domain *db.Domain, settings exportSettings) {
	if domain == nil {
		return
	}
	m.syncDomainTags(domain.Name, domain.Tags, settings)
	m.syncDomainMetadata(domain.Name, domain.Metadata, settings)
	if domain.RegistrationCheckEnabled() {
		m.RegistrationCheckEnabled.WithLabelValues(domain.Name).Set(1)
	} else {
		m.RegistrationCheckEnabled.WithLabelValues(domain.Name).Set(0)
		m.DomainExpiryDays.DeleteLabelValues(domain.Name)
		m.CheckSuccess.DeleteLabelValues(domain.Name, "domain")
	}
}

func (m *Metrics) UpdateDomain(domain *db.Domain, check *db.Check, cfg *config.Config) {
	if domain == nil || check == nil {
		return
	}
	name := domain.Name
	m.syncDomain(domain, m.exportSettings(cfg))

	registrationSkipped := check.RegistrationCheckSkipped

	// Registration check enabled gauge: 1 = full mode, 0 = ssl_only
	if registrationSkipped {
		m.RegistrationCheckEnabled.WithLabelValues(name).Set(0)
		m.RegistrationCheckSkipped.WithLabelValues(name).Inc()
		// Remove stale domain_expiry_days gauge - it's not applicable for ssl_only
		m.DomainExpiryDays.DeleteLabelValues(name)
		// Remove stale domain check success for "domain" type
		m.CheckSuccess.DeleteLabelValues(name, "domain")
	} else {
		m.RegistrationCheckEnabled.WithLabelValues(name).Set(1)
		if check.DomainExpiryDays != nil {
			m.DomainExpiryDays.WithLabelValues(name).Set(float64(*check.DomainExpiryDays))
		}
		domainSuccess := 1.0
		if check.DomainCheckError != "" {
			domainSuccess = 0.0
		}
		m.CheckSuccess.WithLabelValues(name, "domain").Set(domainSuccess)
	}

	if check.SSLExpiryDays != nil {
		m.SSLExpiryDays.WithLabelValues(name).Set(float64(*check.SSLExpiryDays))
	}

	chainVal := 0.0
	if check.SSLChainValid {
		chainVal = 1.0
	}
	m.SSLChainValid.WithLabelValues(name).Set(chainVal)

	sslSuccess := 1.0
	if check.SSLCheckError != "" {
		sslSuccess = 0.0
	}
	m.CheckSuccess.WithLabelValues(name, "ssl").Set(sslSuccess)

	m.CheckDuration.WithLabelValues(name).Set(float64(check.CheckDuration))
	m.LastCheckTime.WithLabelValues(name).Set(float64(check.CheckedAt.Unix()))
	m.OverallStatus.WithLabelValues(name).Set(statusToFloat(check.OverallStatus))
	m.ChecksTotal.WithLabelValues(name, check.OverallStatus).Inc()

	if cfg.Features.HTTPCheck {
		m.HTTPStatusCode.WithLabelValues(name).Set(float64(check.HTTPStatusCode))
		m.HTTPResponseTime.WithLabelValues(name).Set(float64(check.HTTPResponseTimeMs))
		m.HTTPRedirectHTTPS.WithLabelValues(name).Set(boolFloat(check.HTTPRedirectsHTTPS))
		m.HTTPHSTS.WithLabelValues(name).Set(boolFloat(check.HTTPHSTSEnabled))
	}
	if cfg.Features.CipherCheck {
		m.CipherGrade.WithLabelValues(name).Set(cipherGradeToFloat(check.CipherGrade))
	}
	if cfg.Features.OCSPCheck {
		m.OCSPStatus.WithLabelValues(name).Set(revocationToFloat(check.OCSPStatus))
	}
	if cfg.Features.CRLCheck {
		m.CRLStatus.WithLabelValues(name).Set(revocationToFloat(check.CRLStatus))
	}
	if cfg.Features.CAACheck {
		m.CAAPresent.WithLabelValues(name).Set(boolFloat(check.CAAPresent))
	}
}

// CleanupDomain removes all metric labels for a deleted domain.
// Note: counter series (ChecksTotal, RegistrationCheckSkipped) are also deleted.
// Prometheus will stop reporting them after the next scrape.
func (m *Metrics) CleanupDomain(domain string) {
	m.mu.Lock()
	for tag := range m.domainTags[domain] {
		m.DomainTagInfo.DeleteLabelValues(domain, tag)
	}
	delete(m.domainTags, domain)
	for pair := range m.domainMetadata[domain] {
		key, value := splitMetricPair(pair)
		m.DomainMetadataInfo.DeleteLabelValues(domain, key, value)
	}
	delete(m.domainMetadata, domain)
	m.mu.Unlock()

	m.SSLExpiryDays.DeleteLabelValues(domain)
	m.DomainExpiryDays.DeleteLabelValues(domain)
	m.SSLChainValid.DeleteLabelValues(domain)
	m.CheckSuccess.DeleteLabelValues(domain, "ssl")
	m.CheckSuccess.DeleteLabelValues(domain, "domain")
	m.CheckDuration.DeleteLabelValues(domain)
	m.LastCheckTime.DeleteLabelValues(domain)
	m.OverallStatus.DeleteLabelValues(domain)
	m.HTTPStatusCode.DeleteLabelValues(domain)
	m.HTTPResponseTime.DeleteLabelValues(domain)
	m.HTTPRedirectHTTPS.DeleteLabelValues(domain)
	m.HTTPHSTS.DeleteLabelValues(domain)
	m.CipherGrade.DeleteLabelValues(domain)
	m.OCSPStatus.DeleteLabelValues(domain)
	m.CRLStatus.DeleteLabelValues(domain)
	m.CAAPresent.DeleteLabelValues(domain)
	m.RegistrationCheckEnabled.DeleteLabelValues(domain)
	m.RegistrationCheckSkipped.DeleteLabelValues(domain)
	// Clean up counter series for all known statuses
	for _, status := range []string{"ok", "warning", "critical", "error"} {
		m.ChecksTotal.DeleteLabelValues(domain, status)
	}
}

func (m *Metrics) syncDomainTags(domain string, tags []string, settings exportSettings) {
	normalized := make(map[string]struct{}, len(tags))
	if settings.exportTags {
		for _, tag := range tags {
			tag = strings.TrimSpace(tag)
			if tag == "" {
				continue
			}
			normalized[tag] = struct{}{}
		}
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	previous := m.domainTags[domain]
	for tag := range previous {
		if _, stillPresent := normalized[tag]; !stillPresent {
			m.DomainTagInfo.DeleteLabelValues(domain, tag)
		}
	}
	for tag := range normalized {
		m.DomainTagInfo.WithLabelValues(domain, tag).Set(1)
	}
	m.domainTags[domain] = normalized
}

func (m *Metrics) syncDomainMetadata(domain string, metadata map[string]string, settings exportSettings) {
	normalized := make(map[string]struct{}, len(metadata))
	if settings.exportMetadata {
		for key, value := range metadata {
			key = strings.TrimSpace(key)
			value = strings.TrimSpace(value)
			if key == "" || value == "" || !settings.metadataKeyAllowed(key) {
				continue
			}
			normalized[joinMetricPair(key, value)] = struct{}{}
		}
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	previous := m.domainMetadata[domain]
	for pair := range previous {
		if _, stillPresent := normalized[pair]; !stillPresent {
			key, value := splitMetricPair(pair)
			m.DomainMetadataInfo.DeleteLabelValues(domain, key, value)
		}
	}
	for pair := range normalized {
		key, value := splitMetricPair(pair)
		m.DomainMetadataInfo.WithLabelValues(domain, key, value).Set(1)
	}
	m.domainMetadata[domain] = normalized
}

func joinMetricPair(key, value string) string {
	return key + "\x00" + value
}

func splitMetricPair(pair string) (string, string) {
	parts := strings.SplitN(pair, "\x00", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return pair, ""
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
		return 4
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

func (m *Metrics) exportSettings(cfg *config.Config) exportSettings {
	settings := exportSettings{
		exportTags:     true,
		exportMetadata: true,
	}
	if cfg == nil {
		if m == nil || m.cfg == nil {
			return settings
		}
		cfg = m.cfg.Snapshot()
	}
	settings.exportTags = cfg.Prometheus.Labels.ExportTags
	settings.exportMetadata = cfg.Prometheus.Labels.ExportMetadata
	if len(cfg.Prometheus.Labels.MetadataKeys) == 0 {
		return settings
	}
	settings.metadataKeys = make(map[string]struct{}, len(cfg.Prometheus.Labels.MetadataKeys))
	for _, key := range cfg.Prometheus.Labels.MetadataKeys {
		key = strings.ToLower(strings.TrimSpace(key))
		if key == "" {
			continue
		}
		settings.metadataKeys[key] = struct{}{}
	}
	return settings
}

func (s exportSettings) metadataKeyAllowed(key string) bool {
	if len(s.metadataKeys) == 0 {
		return true
	}
	_, ok := s.metadataKeys[strings.ToLower(strings.TrimSpace(key))]
	return ok
}
