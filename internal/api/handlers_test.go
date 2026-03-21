package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"

	"ssl-domain-exporter/internal/config"
	"ssl-domain-exporter/internal/db"
	"ssl-domain-exporter/internal/metrics"
)

func TestUpdateDomainPreservesExistingFieldsWhenOmitted(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := config.Default()
	database := newHandlerTestDB(t)
	defer database.Close()

	reg := prometheus.NewRegistry()
	m := metrics.NewWithRegisterer(reg)

	domain, err := database.CreateDomain("old.internal", []string{"infra"}, map[string]string{"owner": "platform"}, "-----BEGIN CERTIFICATE-----\nTEST\n-----END CERTIFICATE-----", "ssl_only", "10.0.0.1:53", 3600, 8443, nil)
	if err != nil {
		t.Fatalf("create domain: %v", err)
	}

	sslDays := 60
	domainDays := 120
	m.UpdateDomain(domain, &db.Check{
		CheckedAt:        time.Unix(1710000000, 0),
		SSLExpiryDays:    &sslDays,
		DomainExpiryDays: &domainDays,
		SSLChainValid:    true,
		OverallStatus:    "ok",
		CheckDuration:    12,
	}, cfg)

	body := []byte(`{"name":"renamed.internal"}`)
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/domains/%d", domain.ID), bytes.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Params = gin.Params{{Key: "id", Value: strconv.FormatInt(domain.ID, 10)}}

	handler := NewHandler(cfg, database, nil, nil, m)
	handler.UpdateDomain(ctx)

	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status code: got %d body=%s", rec.Code, rec.Body.String())
	}

	updated, err := database.GetDomainByID(domain.ID)
	if err != nil {
		t.Fatalf("get updated domain: %v", err)
	}
	if updated == nil {
		t.Fatal("updated domain not found")
	}
	if updated.Name != "renamed.internal" {
		t.Fatalf("name = %q, want renamed.internal", updated.Name)
	}
	if len(updated.Tags) != 1 || updated.Tags[0] != "infra" {
		t.Fatalf("tags = %#v, want [infra]", updated.Tags)
	}
	if updated.Metadata["owner"] != "platform" {
		t.Fatalf("metadata = %#v, want owner=platform", updated.Metadata)
	}
	if !updated.Enabled {
		t.Fatal("expected enabled to be preserved when omitted")
	}
	if updated.CheckInterval != 3600 {
		t.Fatalf("check_interval = %d, want 3600", updated.CheckInterval)
	}
	if updated.CustomCAPEM == "" {
		t.Fatal("expected custom_ca_pem to be preserved when omitted")
	}
	if updated.CheckMode != "ssl_only" {
		t.Fatalf("check_mode = %q, want ssl_only", updated.CheckMode)
	}
	if updated.DNSServers != "10.0.0.1:53" {
		t.Fatalf("dns_servers = %q, want 10.0.0.1:53", updated.DNSServers)
	}

	if metricSeriesExists(t, reg, "domain_ssl_expiry_days", map[string]string{"domain": "old.internal"}) {
		t.Fatal("expected old metric labels to be cleaned up after rename")
	}
}

func TestImportDomainsSupportsMetadataAndUpsert(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := config.Default()
	database := newHandlerTestDB(t)
	defer database.Close()

	existing, err := database.CreateDomain("existing.internal", []string{"legacy"}, map[string]string{"owner": "legacy-team"}, "", "full", "", 3600, 443, nil)
	if err != nil {
		t.Fatalf("create existing domain: %v", err)
	}

	handler := NewHandler(cfg, database, nil, nil, metrics.NewWithRegisterer(prometheus.NewRegistry()))

	payload := map[string]any{
		"mode": "upsert",
		"defaults": map[string]any{
			"tags":       []string{"corp"},
			"metadata":   map[string]string{"zone": "corp"},
			"check_mode": "ssl_only",
		},
		"domains": []map[string]any{
			{
				"domain":        "new.internal",
				"owner":         "platform-team",
				"custom_ca_pem": "-----BEGIN CERTIFICATE-----\nTEST\n-----END CERTIFICATE-----",
			},
			{
				"domain":     "existing.internal",
				"tags":       []string{"db"},
				"metadata":   map[string]string{"owner": "ops-team"},
				"check_mode": "full",
			},
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/domains/import", bytes.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")

	handler.ImportDomains(ctx)

	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status code: got %d body=%s", rec.Code, rec.Body.String())
	}

	var resp importDomainsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.Summary.Created != 1 || resp.Summary.Updated != 1 || resp.Summary.Failed != 0 {
		t.Fatalf("unexpected summary: %+v", resp.Summary)
	}

	created, err := database.GetDomains()
	if err != nil {
		t.Fatalf("get domains after import: %v", err)
	}
	if len(created) != 2 {
		t.Fatalf("expected 2 domains after import, got %d", len(created))
	}

	newDomain, err := findDomainByName(database, "new.internal")
	if err != nil {
		t.Fatalf("find new domain: %v", err)
	}
	if newDomain == nil {
		t.Fatal("new domain not created")
	}
	if len(newDomain.Tags) != 1 || newDomain.Tags[0] != "corp" {
		t.Fatalf("new domain tags = %#v", newDomain.Tags)
	}
	if newDomain.Metadata["owner"] != "platform-team" || newDomain.Metadata["zone"] != "corp" {
		t.Fatalf("new domain metadata = %#v", newDomain.Metadata)
	}
	if newDomain.CheckMode != "ssl_only" {
		t.Fatalf("new domain check mode = %q, want ssl_only", newDomain.CheckMode)
	}

	updated, err := database.GetDomainByID(existing.ID)
	if err != nil {
		t.Fatalf("get updated domain: %v", err)
	}
	if updated == nil {
		t.Fatal("updated domain not found")
	}
	if len(updated.Tags) != 2 || updated.Tags[0] != "corp" || updated.Tags[1] != "db" {
		t.Fatalf("updated domain tags = %#v", updated.Tags)
	}
	if updated.Metadata["owner"] != "ops-team" || updated.Metadata["zone"] != "corp" {
		t.Fatalf("updated domain metadata = %#v", updated.Metadata)
	}
	if updated.CheckMode != "full" {
		t.Fatalf("updated domain check mode = %q, want full", updated.CheckMode)
	}
}

func newHandlerTestDB(t *testing.T) *db.DB {
	t.Helper()

	path := filepath.Join(t.TempDir(), "handler-test.db")
	database, err := db.New(path)
	if err != nil {
		t.Fatalf("open handler test db: %v", err)
	}
	if err := database.Migrate(); err != nil {
		_ = database.Close()
		t.Fatalf("migrate handler test db: %v", err)
	}
	return database
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

func findDomainByName(database *db.DB, name string) (*db.Domain, error) {
	domains, err := database.GetDomains()
	if err != nil {
		return nil, err
	}
	for i := range domains {
		if domains[i].Name == name {
			return &domains[i], nil
		}
	}
	return nil, nil
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
