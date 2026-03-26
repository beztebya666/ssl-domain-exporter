package metrics

import (
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"

	"ssl-domain-exporter/internal/config"
	"ssl-domain-exporter/internal/db"
)

func TestUpdateDomainSSLOnlyDeletesRegistrationSeries(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewWithRegisterer(reg)
	cfg := config.Default()

	sslDays := 90
	domainDays := 120
	full := &db.Check{
		CheckedAt:                time.Unix(1710000000, 0),
		SSLExpiryDays:            &sslDays,
		DomainExpiryDays:         &domainDays,
		SSLChainValid:            true,
		OverallStatus:            "ok",
		CheckDuration:            25,
		DomainCheckError:         "",
		SSLCheckError:            "",
		DomainSource:             "rdap",
		RegistrationCheckSkipped: false,
	}
	m.UpdateDomain(&db.Domain{Name: "example.internal", Tags: []string{"prod", "web"}}, full, cfg)

	if !metricSeriesExists(t, reg, "domain_expiry_days", map[string]string{"domain": "example.internal"}) {
		t.Fatal("expected domain_expiry_days series to exist after full check")
	}
	if !metricSeriesExists(t, reg, "domain_check_success", map[string]string{"domain": "example.internal", "type": "domain"}) {
		t.Fatal("expected domain_check_success{type=domain} series to exist after full check")
	}

	skipped := &db.Check{
		CheckedAt:                time.Unix(1710000600, 0),
		SSLExpiryDays:            &sslDays,
		SSLChainValid:            true,
		OverallStatus:            "ok",
		CheckDuration:            30,
		RegistrationCheckSkipped: true,
		RegistrationSkipReason:   "check_mode=ssl_only",
	}
	m.UpdateDomain(&db.Domain{Name: "example.internal", Tags: []string{"prod", "web"}, CheckMode: "ssl_only"}, skipped, cfg)

	if metricSeriesExists(t, reg, "domain_expiry_days", map[string]string{"domain": "example.internal"}) {
		t.Fatal("expected domain_expiry_days series to be deleted for ssl_only")
	}
	if metricSeriesExists(t, reg, "domain_check_success", map[string]string{"domain": "example.internal", "type": "domain"}) {
		t.Fatal("expected domain_check_success{type=domain} to be deleted for ssl_only")
	}

	value, ok := gaugeValue(t, reg, "domain_registration_check_enabled", map[string]string{"domain": "example.internal"})
	if !ok {
		t.Fatal("expected domain_registration_check_enabled series to exist")
	}
	if value != 0 {
		t.Fatalf("expected domain_registration_check_enabled=0, got %v", value)
	}
}

func TestCleanupDomainRemovesAllKnownSeries(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewWithRegisterer(reg)
	cfg := config.Default()
	cfg.Features.HTTPCheck = true
	cfg.Features.CipherCheck = true
	cfg.Features.OCSPCheck = true
	cfg.Features.CRLCheck = true
	cfg.Features.CAACheck = true

	sslDays := 20
	domainDays := 5
	m.UpdateDomain(&db.Domain{Name: "cleanup.internal", Tags: []string{"legacy"}, Metadata: map[string]string{"owner": "platform"}}, &db.Check{
		CheckedAt:                time.Unix(1710001100, 0),
		SSLExpiryDays:            &sslDays,
		SSLChainValid:            true,
		OverallStatus:            "ok",
		CheckDuration:            10,
		RegistrationCheckSkipped: true,
	}, cfg)
	m.UpdateDomain(&db.Domain{Name: "cleanup.internal", Tags: []string{"legacy", "api"}, Metadata: map[string]string{"owner": "platform", "service": "api"}}, &db.Check{
		CheckedAt:                time.Unix(1710001200, 0),
		SSLExpiryDays:            &sslDays,
		DomainExpiryDays:         &domainDays,
		SSLChainValid:            false,
		OverallStatus:            "warning",
		CheckDuration:            55,
		HTTPStatusCode:           301,
		HTTPResponseTimeMs:       99,
		HTTPRedirectsHTTPS:       true,
		HTTPHSTSEnabled:          true,
		CipherGrade:              "C",
		OCSPStatus:               "unknown",
		CRLStatus:                "good",
		CAAPresent:               true,
		RegistrationCheckSkipped: false,
	}, cfg)

	for _, tc := range []struct {
		name   string
		labels map[string]string
	}{
		{"domain_ssl_expiry_days", map[string]string{"domain": "cleanup.internal"}},
		{"domain_expiry_days", map[string]string{"domain": "cleanup.internal"}},
		{"domain_checks_total", map[string]string{"domain": "cleanup.internal", "status": "warning"}},
		{"domain_registration_check_skipped_total", map[string]string{"domain": "cleanup.internal"}},
		{"domain_tag_info", map[string]string{"domain": "cleanup.internal", "tag": "api"}},
		{"domain_metadata_info", map[string]string{"domain": "cleanup.internal", "key": "owner", "value": "platform"}},
	} {
		if !metricSeriesExists(t, reg, tc.name, tc.labels) {
			t.Fatalf("expected metric series to exist before cleanup: %s %v", tc.name, tc.labels)
		}
	}

	m.CleanupDomain("cleanup.internal")

	for _, tc := range []struct {
		name   string
		labels map[string]string
	}{
		{"domain_ssl_expiry_days", map[string]string{"domain": "cleanup.internal"}},
		{"domain_expiry_days", map[string]string{"domain": "cleanup.internal"}},
		{"domain_check_success", map[string]string{"domain": "cleanup.internal", "type": "ssl"}},
		{"domain_checks_total", map[string]string{"domain": "cleanup.internal", "status": "warning"}},
		{"domain_registration_check_skipped_total", map[string]string{"domain": "cleanup.internal"}},
		{"domain_caa_present", map[string]string{"domain": "cleanup.internal"}},
		{"domain_tag_info", map[string]string{"domain": "cleanup.internal", "tag": "api"}},
		{"domain_metadata_info", map[string]string{"domain": "cleanup.internal", "key": "owner", "value": "platform"}},
	} {
		if metricSeriesExists(t, reg, tc.name, tc.labels) {
			t.Fatalf("expected metric series to be deleted: %s %v", tc.name, tc.labels)
		}
	}
}

func TestSyncDomainReplacesTagSeries(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewWithRegisterer(reg)

	m.SyncDomain(&db.Domain{Name: "tags.internal", Tags: []string{"prod", "api"}})
	if !metricSeriesExists(t, reg, "domain_tag_info", map[string]string{"domain": "tags.internal", "tag": "prod"}) {
		t.Fatal("expected prod tag series to exist")
	}
	if !metricSeriesExists(t, reg, "domain_tag_info", map[string]string{"domain": "tags.internal", "tag": "api"}) {
		t.Fatal("expected api tag series to exist")
	}

	m.SyncDomain(&db.Domain{Name: "tags.internal", Tags: []string{"corp"}})
	if metricSeriesExists(t, reg, "domain_tag_info", map[string]string{"domain": "tags.internal", "tag": "prod"}) {
		t.Fatal("expected prod tag series to be removed")
	}
	if !metricSeriesExists(t, reg, "domain_tag_info", map[string]string{"domain": "tags.internal", "tag": "corp"}) {
		t.Fatal("expected corp tag series to exist")
	}
}

func TestSyncDomainReplacesMetadataSeries(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewWithRegisterer(reg)

	m.SyncDomain(&db.Domain{
		Name:     "metadata.internal",
		Metadata: map[string]string{"owner": "platform", "zone": "corp"},
	})
	if !metricSeriesExists(t, reg, "domain_metadata_info", map[string]string{"domain": "metadata.internal", "key": "owner", "value": "platform"}) {
		t.Fatal("expected owner metadata series to exist")
	}
	if !metricSeriesExists(t, reg, "domain_metadata_info", map[string]string{"domain": "metadata.internal", "key": "zone", "value": "corp"}) {
		t.Fatal("expected zone metadata series to exist")
	}

	m.SyncDomain(&db.Domain{
		Name:     "metadata.internal",
		Metadata: map[string]string{"owner": "ops", "environment": "prod"},
	})
	if metricSeriesExists(t, reg, "domain_metadata_info", map[string]string{"domain": "metadata.internal", "key": "owner", "value": "platform"}) {
		t.Fatal("expected old owner metadata series to be removed")
	}
	if metricSeriesExists(t, reg, "domain_metadata_info", map[string]string{"domain": "metadata.internal", "key": "zone", "value": "corp"}) {
		t.Fatal("expected old zone metadata series to be removed")
	}
	if !metricSeriesExists(t, reg, "domain_metadata_info", map[string]string{"domain": "metadata.internal", "key": "owner", "value": "ops"}) {
		t.Fatal("expected new owner metadata series to exist")
	}
	if !metricSeriesExists(t, reg, "domain_metadata_info", map[string]string{"domain": "metadata.internal", "key": "environment", "value": "prod"}) {
		t.Fatal("expected environment metadata series to exist")
	}
}

func TestSyncDomainHonorsMetadataExportDisable(t *testing.T) {
	reg := prometheus.NewRegistry()
	cfg := config.Default()
	cfg.Prometheus.Labels.ExportMetadata = false
	m := NewWithConfigAndRegisterer(cfg, reg)

	m.SyncDomain(&db.Domain{
		Name:     "metadata.internal",
		Metadata: map[string]string{"owner": "platform"},
	})
	if metricSeriesExists(t, reg, "domain_metadata_info", map[string]string{"domain": "metadata.internal", "key": "owner", "value": "platform"}) {
		t.Fatal("expected metadata export to stay disabled")
	}
}

func TestSyncDomainHonorsMetadataKeyWhitelist(t *testing.T) {
	reg := prometheus.NewRegistry()
	cfg := config.Default()
	cfg.Prometheus.Labels.MetadataKeys = []string{"env"}
	m := NewWithConfigAndRegisterer(cfg, reg)

	m.SyncDomain(&db.Domain{
		Name:     "metadata.internal",
		Metadata: map[string]string{"env": "prod", "owner_email": "ops@example.com"},
	})
	if !metricSeriesExists(t, reg, "domain_metadata_info", map[string]string{"domain": "metadata.internal", "key": "env", "value": "prod"}) {
		t.Fatal("expected allowlisted metadata series to exist")
	}
	if metricSeriesExists(t, reg, "domain_metadata_info", map[string]string{"domain": "metadata.internal", "key": "owner_email", "value": "ops@example.com"}) {
		t.Fatal("expected non-allowlisted metadata series to be skipped")
	}
}

func TestUpdateDomainMapsUnknownStatusSeparatelyFromError(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewWithRegisterer(reg)
	cfg := config.Default()
	sslDays := 120

	m.UpdateDomain(&db.Domain{Name: "unknown.internal"}, &db.Check{
		CheckedAt:     time.Unix(1710002400, 0),
		SSLExpiryDays: &sslDays,
		SSLChainValid: true,
		OverallStatus: "unknown",
		CheckDuration: 10,
		SSLCheckError: "",
		DomainSource:  "",
	}, cfg)

	value, ok := gaugeValue(t, reg, "domain_overall_status", map[string]string{"domain": "unknown.internal"})
	if !ok {
		t.Fatal("expected domain_overall_status series to exist")
	}
	if value != 4 {
		t.Fatalf("expected unknown status to map to 4, got %v", value)
	}
}

func metricSeriesExists(t *testing.T, reg *prometheus.Registry, name string, labels map[string]string) bool {
	t.Helper()

	families, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather metrics: %v", err)
	}
	for _, family := range families {
		if family.GetName() != name {
			continue
		}
		for _, metric := range family.Metric {
			if labelsMatch(metric.Label, labels) {
				return true
			}
		}
	}
	return false
}

func gaugeValue(t *testing.T, reg *prometheus.Registry, name string, labels map[string]string) (float64, bool) {
	t.Helper()

	families, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather metrics: %v", err)
	}
	for _, family := range families {
		if family.GetName() != name {
			continue
		}
		for _, metric := range family.Metric {
			if labelsMatch(metric.Label, labels) && metric.Gauge != nil {
				return metric.Gauge.GetValue(), true
			}
		}
	}
	return 0, false
}

func labelsMatch(pairs []*dto.LabelPair, want map[string]string) bool {
	if len(pairs) != len(want) {
		return false
	}
	for _, pair := range pairs {
		if want[pair.GetName()] != pair.GetValue() {
			return false
		}
	}
	return true
}
