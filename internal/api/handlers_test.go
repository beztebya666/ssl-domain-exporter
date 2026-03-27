package api

import (
	"bytes"
	"encoding/csv"
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

func TestSearchDomainsSupportsServerSideFiltersAndPaging(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := config.Default()
	database := newHandlerTestDB(t)
	defer database.Close()

	folderA, err := database.CreateFolder("platform")
	if err != nil {
		t.Fatalf("create folder A: %v", err)
	}
	folderB, err := database.CreateFolder("public")
	if err != nil {
		t.Fatalf("create folder B: %v", err)
	}

	alpha, err := database.CreateDomain("alpha.internal", []string{"prod"}, nil, "", "full", "", 3600, 443, &folderA.ID)
	if err != nil {
		t.Fatalf("create alpha: %v", err)
	}
	beta, err := database.CreateDomain("beta.internal", []string{"prod"}, nil, "", "full", "", 3600, 443, &folderA.ID)
	if err != nil {
		t.Fatalf("create beta: %v", err)
	}
	_, err = database.CreateDomain("gamma.internal", []string{"dev"}, nil, "", "full", "", 3600, 443, &folderB.ID)
	if err != nil {
		t.Fatalf("create gamma: %v", err)
	}

	alphaSSL := 5
	if err := database.SaveCheck(&db.Check{
		DomainID:      alpha.ID,
		CheckedAt:     time.Now().Add(-2 * time.Hour),
		SSLExpiryDays: &alphaSSL,
		SSLChainValid: true,
		OverallStatus: "warning",
		CheckDuration: 10,
	}); err != nil {
		t.Fatalf("save alpha check: %v", err)
	}

	betaSSL := 8
	if err := database.SaveCheck(&db.Check{
		DomainID:      beta.ID,
		CheckedAt:     time.Now().Add(-1 * time.Hour),
		SSLExpiryDays: &betaSSL,
		SSLChainValid: true,
		OverallStatus: "warning",
		CheckDuration: 10,
	}); err != nil {
		t.Fatalf("save beta check: %v", err)
	}

	handler := NewHandler(cfg, database, nil, nil, nil)
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(
		http.MethodGet,
		fmt.Sprintf("/api/domains/search?folder_id=%d&tag=prod&ssl_expiry_lte=10&sort_by=name&sort_dir=asc&page=1&page_size=1", folderA.ID),
		nil,
	)

	handler.SearchDomains(ctx)

	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status code: got %d body=%s", rec.Code, rec.Body.String())
	}

	var resp domainListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode search response: %v", err)
	}

	if resp.Total != 2 {
		t.Fatalf("expected total=2, got %d", resp.Total)
	}
	if resp.TotalPages != 2 || resp.PageSize != 1 {
		t.Fatalf("unexpected paging: %+v", resp)
	}
	if len(resp.Items) != 1 || resp.Items[0].Name != "alpha.internal" {
		t.Fatalf("unexpected first page items: %+v", resp.Items)
	}
}

func TestGetTimelineReturnsPagedServerSideData(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := config.Default()
	cfg.Alerts.SSLExpiryWarningDays = 30
	cfg.Alerts.SSLExpiryCriticalDays = 7
	cfg.Alerts.DomainExpiryWarningDays = 60
	cfg.Alerts.DomainExpiryCriticalDays = 14

	database := newHandlerTestDB(t)
	defer database.Close()

	alpha, err := database.CreateDomain("alpha.internal", []string{"prod"}, nil, "", "full", "", 3600, 443, nil)
	if err != nil {
		t.Fatalf("create alpha: %v", err)
	}
	beta, err := database.CreateDomain("beta.internal", []string{"prod"}, nil, "", "full", "", 3600, 443, nil)
	if err != nil {
		t.Fatalf("create beta: %v", err)
	}
	gamma, err := database.CreateDomain("gamma.internal", []string{"prod"}, nil, "", "ssl_only", "", 3600, 443, nil)
	if err != nil {
		t.Fatalf("create gamma: %v", err)
	}

	alphaSSL := 5
	alphaDomain := 40
	if err := database.SaveCheck(&db.Check{
		DomainID:         alpha.ID,
		CheckedAt:        time.Now().Add(-3 * time.Hour),
		SSLExpiryDays:    &alphaSSL,
		SSLIssuer:        "Internal PKI",
		DomainExpiryDays: &alphaDomain,
		SSLChainValid:    true,
		OverallStatus:    "warning",
		CheckDuration:    11,
	}); err != nil {
		t.Fatalf("save alpha check: %v", err)
	}

	betaSSL := 25
	betaDomain := 10
	if err := database.SaveCheck(&db.Check{
		DomainID:         beta.ID,
		CheckedAt:        time.Now().Add(-2 * time.Hour),
		SSLExpiryDays:    &betaSSL,
		SSLIssuer:        "Public CA",
		DomainExpiryDays: &betaDomain,
		SSLChainValid:    true,
		OverallStatus:    "critical",
		CheckDuration:    9,
	}); err != nil {
		t.Fatalf("save beta check: %v", err)
	}

	gammaSSL := 90
	if err := database.SaveCheck(&db.Check{
		DomainID:                 gamma.ID,
		CheckedAt:                time.Now().Add(-1 * time.Hour),
		SSLExpiryDays:            &gammaSSL,
		SSLIssuer:                "Internal CA",
		RegistrationCheckSkipped: true,
		SSLChainValid:            true,
		OverallStatus:            "ok",
		CheckDuration:            8,
	}); err != nil {
		t.Fatalf("save gamma check: %v", err)
	}

	handler := NewHandler(cfg, database, nil, nil, nil)
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/timeline?ssl_page=1&ssl_page_size=2&domain_page=1&domain_page_size=1", nil)

	handler.GetTimeline(ctx)

	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status code: got %d body=%s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Summary struct {
			SSLCritical    int `json:"ssl_critical"`
			SSLWarning     int `json:"ssl_warning"`
			DomainCritical int `json:"domain_critical"`
			DomainWarning  int `json:"domain_warning"`
		} `json:"summary"`
		SSL struct {
			Items []db.TimelineEntry `json:"items"`
			Total int                `json:"total"`
		} `json:"ssl"`
		Domain struct {
			Items    []db.TimelineEntry `json:"items"`
			Total    int                `json:"total"`
			PageSize int                `json:"page_size"`
		} `json:"domain"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	if resp.Summary.SSLCritical != 1 || resp.Summary.SSLWarning != 1 {
		t.Fatalf("unexpected ssl summary: %+v", resp.Summary)
	}
	if resp.Summary.DomainCritical != 1 || resp.Summary.DomainWarning != 1 {
		t.Fatalf("unexpected domain summary: %+v", resp.Summary)
	}
	if resp.SSL.Total != 3 {
		t.Fatalf("ssl total = %d, want 3", resp.SSL.Total)
	}
	if len(resp.SSL.Items) != 2 || resp.SSL.Items[0].Name != "alpha.internal" || resp.SSL.Items[1].Name != "beta.internal" {
		t.Fatalf("unexpected ssl items: %+v", resp.SSL.Items)
	}
	if resp.Domain.Total != 2 || resp.Domain.PageSize != 1 {
		t.Fatalf("unexpected domain paging: %+v", resp.Domain)
	}
	if len(resp.Domain.Items) != 1 || resp.Domain.Items[0].Name != "beta.internal" {
		t.Fatalf("unexpected domain items: %+v", resp.Domain.Items)
	}
}

func TestExportDomainsCSVRespectsFilters(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := config.Default()
	cfg.Features.CSVExport = true
	database := newHandlerTestDB(t)
	defer database.Close()

	prod, err := database.CreateDomain("prod.internal", []string{"prod"}, map[string]string{"owner": "platform"}, "", "full", "", 3600, 443, nil)
	if err != nil {
		t.Fatalf("create prod: %v", err)
	}
	dev, err := database.CreateDomain("dev.internal", []string{"dev"}, map[string]string{"owner": "qa"}, "", "full", "", 3600, 443, nil)
	if err != nil {
		t.Fatalf("create dev: %v", err)
	}

	sslDays := 40
	for _, domain := range []*db.Domain{prod, dev} {
		if err := database.SaveCheck(&db.Check{
			DomainID:      domain.ID,
			CheckedAt:     time.Now().UTC(),
			SSLExpiryDays: &sslDays,
			SSLChainValid: true,
			OverallStatus: "ok",
			CheckDuration: 15,
		}); err != nil {
			t.Fatalf("save check for %s: %v", domain.Name, err)
		}
	}

	handler := NewHandler(cfg, database, nil, nil, nil)
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/domains/export.csv?tag=prod", nil)

	handler.ExportDomainsCSV(ctx)

	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status code: got %d body=%s", rec.Code, rec.Body.String())
	}

	reader := csv.NewReader(bytes.NewReader(rec.Body.Bytes()))
	rows, err := reader.ReadAll()
	if err != nil {
		t.Fatalf("read csv: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected header + 1 data row, got %d rows", len(rows))
	}
	if rows[1][1] != "prod.internal" {
		t.Fatalf("expected exported row to be prod.internal, got %q", rows[1][1])
	}
}

func TestCreateDomainValidatesRequiredCustomFieldsAndMetadataFiltersDriveSearchAndExport(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := config.Default()
	cfg.Features.CSVExport = true
	database := newHandlerTestDB(t)
	defer database.Close()

	if _, err := database.CreateCustomField(db.CustomField{
		Key:              "owner_email",
		Label:            "Owner Email",
		Type:             "email",
		Required:         true,
		VisibleInExport:  true,
		VisibleInDetails: true,
		Filterable:       true,
		Enabled:          true,
	}); err != nil {
		t.Fatalf("create custom field: %v", err)
	}

	handler := NewHandler(cfg, database, nil, nil, nil)

	invalidBody := []byte(`{"name":"missing-owner.internal","metadata":{"zone":"corp"}}`)
	invalidRec := httptest.NewRecorder()
	invalidCtx, _ := gin.CreateTestContext(invalidRec)
	invalidCtx.Request = httptest.NewRequest(http.MethodPost, "/api/domains", bytes.NewReader(invalidBody))
	invalidCtx.Request.Header.Set("Content-Type", "application/json")

	handler.CreateDomain(invalidCtx)

	if invalidRec.Code != http.StatusBadRequest {
		t.Fatalf("expected custom field validation error, got %d body=%s", invalidRec.Code, invalidRec.Body.String())
	}
	if !bytes.Contains(bytes.ToLower(invalidRec.Body.Bytes()), []byte("owner email")) {
		t.Fatalf("expected required field error, got %s", invalidRec.Body.String())
	}

	alpha, err := database.CreateDomain("alpha.internal", []string{"prod"}, map[string]string{
		"owner_email": "alpha@example.com",
		"zone":        "corp",
	}, "", "full", "", 3600, 443, nil)
	if err != nil {
		t.Fatalf("create alpha: %v", err)
	}
	beta, err := database.CreateDomain("beta.internal", []string{"prod"}, map[string]string{
		"owner_email": "beta@example.com",
		"zone":        "corp",
	}, "", "full", "", 3600, 443, nil)
	if err != nil {
		t.Fatalf("create beta: %v", err)
	}

	sslDays := 25
	for _, domain := range []*db.Domain{alpha, beta} {
		if err := database.SaveCheck(&db.Check{
			DomainID:      domain.ID,
			CheckedAt:     time.Now().UTC(),
			SSLExpiryDays: &sslDays,
			SSLChainValid: true,
			OverallStatus: "ok",
			CheckDuration: 12,
		}); err != nil {
			t.Fatalf("save check for %s: %v", domain.Name, err)
		}
	}

	searchRec := httptest.NewRecorder()
	searchCtx, _ := gin.CreateTestContext(searchRec)
	searchCtx.Request = httptest.NewRequest(http.MethodGet, "/api/domains/search?metadata_filters=%7B%22owner_email%22%3A%22alpha%40example.com%22%7D", nil)

	handler.SearchDomains(searchCtx)

	if searchRec.Code != http.StatusOK {
		t.Fatalf("unexpected search status: got %d body=%s", searchRec.Code, searchRec.Body.String())
	}

	var searchResp domainListResponse
	if err := json.Unmarshal(searchRec.Body.Bytes(), &searchResp); err != nil {
		t.Fatalf("decode search response: %v", err)
	}
	if searchResp.Total != 1 || len(searchResp.Items) != 1 || searchResp.Items[0].Name != "alpha.internal" {
		t.Fatalf("unexpected metadata-filtered search result: %+v", searchResp)
	}

	exportRec := httptest.NewRecorder()
	exportCtx, _ := gin.CreateTestContext(exportRec)
	exportCtx.Request = httptest.NewRequest(http.MethodGet, "/api/domains/export.csv?metadata_filters=%7B%22owner_email%22%3A%22alpha%40example.com%22%7D", nil)

	handler.ExportDomainsCSV(exportCtx)

	if exportRec.Code != http.StatusOK {
		t.Fatalf("unexpected export status: got %d body=%s", exportRec.Code, exportRec.Body.String())
	}

	reader := csv.NewReader(bytes.NewReader(exportRec.Body.Bytes()))
	rows, err := reader.ReadAll()
	if err != nil {
		t.Fatalf("read export csv: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected header + one filtered row, got %d rows", len(rows))
	}
	if rows[0][len(rows[0])-1] != "owner_email" {
		t.Fatalf("expected custom field export column, got header %#v", rows[0])
	}
	if rows[1][1] != "alpha.internal" || rows[1][len(rows[1])-1] != "alpha@example.com" {
		t.Fatalf("unexpected filtered export row %#v", rows[1])
	}
}

func TestCustomFieldHandlersCRUD(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := config.Default()
	database := newHandlerTestDB(t)
	defer database.Close()

	handler := NewHandler(cfg, database, nil, nil, nil)

	createPayload := map[string]any{
		"key":                "service_tier",
		"label":              "Service Tier",
		"type":               "select",
		"required":           false,
		"visible_in_table":   true,
		"visible_in_details": true,
		"visible_in_export":  true,
		"filterable":         true,
		"enabled":            true,
		"options": []map[string]string{
			{"value": "gold", "label": "Gold"},
			{"value": "silver", "label": "Silver"},
		},
	}
	body, err := json.Marshal(createPayload)
	if err != nil {
		t.Fatalf("marshal create payload: %v", err)
	}

	createRec := httptest.NewRecorder()
	createCtx, _ := gin.CreateTestContext(createRec)
	createCtx.Request = httptest.NewRequest(http.MethodPost, "/api/custom-fields", bytes.NewReader(body))
	createCtx.Request.Header.Set("Content-Type", "application/json")

	handler.CreateCustomField(createCtx)

	if createRec.Code != http.StatusCreated {
		t.Fatalf("unexpected create status: got %d body=%s", createRec.Code, createRec.Body.String())
	}

	var created db.CustomField
	if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode created field: %v", err)
	}
	if created.Key != "service_tier" || len(created.Options) != 2 {
		t.Fatalf("unexpected created field: %+v", created)
	}

	textPayload := map[string]any{
		"key":                "owner",
		"label":              "Owner",
		"type":               "text",
		"required":           false,
		"visible_in_table":   true,
		"visible_in_details": true,
		"visible_in_export":  true,
		"filterable":         true,
		"enabled":            true,
	}
	textBody, err := json.Marshal(textPayload)
	if err != nil {
		t.Fatalf("marshal text field payload: %v", err)
	}

	textRec := httptest.NewRecorder()
	textCtx, _ := gin.CreateTestContext(textRec)
	textCtx.Request = httptest.NewRequest(http.MethodPost, "/api/custom-fields", bytes.NewReader(textBody))
	textCtx.Request.Header.Set("Content-Type", "application/json")

	handler.CreateCustomField(textCtx)

	if textRec.Code != http.StatusCreated {
		t.Fatalf("unexpected text field create status: got %d body=%s", textRec.Code, textRec.Body.String())
	}

	var textField db.CustomField
	if err := json.Unmarshal(textRec.Body.Bytes(), &textField); err != nil {
		t.Fatalf("decode text field: %v", err)
	}
	if textField.Options == nil {
		t.Fatal("expected text custom field response to include an empty options array")
	}

	updatePayload := map[string]any{
		"key":                "service_tier",
		"label":              "Service Tier",
		"type":               "select",
		"required":           false,
		"visible_in_table":   false,
		"visible_in_details": true,
		"visible_in_export":  true,
		"filterable":         true,
		"enabled":            false,
		"options": []map[string]string{
			{"value": "gold", "label": "Gold"},
			{"value": "platinum", "label": "Platinum"},
		},
	}
	updateBody, err := json.Marshal(updatePayload)
	if err != nil {
		t.Fatalf("marshal update payload: %v", err)
	}

	updateRec := httptest.NewRecorder()
	updateCtx, _ := gin.CreateTestContext(updateRec)
	updateCtx.Request = httptest.NewRequest(http.MethodPut, "/api/custom-fields/"+strconv.FormatInt(created.ID, 10), bytes.NewReader(updateBody))
	updateCtx.Request.Header.Set("Content-Type", "application/json")
	updateCtx.Params = gin.Params{{Key: "id", Value: strconv.FormatInt(created.ID, 10)}}

	handler.UpdateCustomField(updateCtx)

	if updateRec.Code != http.StatusOK {
		t.Fatalf("unexpected update status: got %d body=%s", updateRec.Code, updateRec.Body.String())
	}

	listRec := httptest.NewRecorder()
	listCtx, _ := gin.CreateTestContext(listRec)
	listCtx.Request = httptest.NewRequest(http.MethodGet, "/api/custom-fields?include_disabled=true", nil)
	listCtx.Set(principalContextKey, Principal{Authenticated: true, Role: "admin"})

	handler.ListCustomFields(listCtx)

	if listRec.Code != http.StatusOK {
		t.Fatalf("unexpected list status: got %d body=%s", listRec.Code, listRec.Body.String())
	}

	var fields []db.CustomField
	if err := json.Unmarshal(listRec.Body.Bytes(), &fields); err != nil {
		t.Fatalf("decode listed fields: %v", err)
	}
	if len(fields) != 2 {
		t.Fatalf("expected two fields in list, got %+v", fields)
	}
	if fields[0].Options == nil || fields[1].Options == nil {
		t.Fatalf("expected listed custom fields to always include options arrays, got %+v", fields)
	}
	if fields[0].Key == "service_tier" && fields[0].Enabled {
		t.Fatalf("expected service_tier to be disabled, got %+v", fields[0])
	}
	if fields[1].Key == "service_tier" && fields[1].Enabled {
		t.Fatalf("expected one disabled field, got %+v", fields)
	}

	deleteRec := httptest.NewRecorder()
	deleteCtx, _ := gin.CreateTestContext(deleteRec)
	deleteCtx.Request = httptest.NewRequest(http.MethodDelete, "/api/custom-fields/"+strconv.FormatInt(created.ID, 10), nil)
	deleteCtx.Params = gin.Params{{Key: "id", Value: strconv.FormatInt(created.ID, 10)}}

	handler.DeleteCustomField(deleteCtx)

	if deleteRec.Code != http.StatusOK {
		t.Fatalf("unexpected delete status: got %d body=%s", deleteRec.Code, deleteRec.Body.String())
	}

	if err := database.DeleteCustomField(textField.ID); err != nil {
		t.Fatalf("delete text field: %v", err)
	}

	remaining, err := database.ListCustomFields(true)
	if err != nil {
		t.Fatalf("list remaining fields: %v", err)
	}
	if len(remaining) != 0 {
		t.Fatalf("expected no fields after delete, got %+v", remaining)
	}
}

func TestDeleteDomainReturnsNotFoundWhenDomainDoesNotExist(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := config.Default()
	database := newHandlerTestDB(t)
	defer database.Close()

	handler := NewHandler(cfg, database, nil, nil, nil)
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodDelete, "/api/domains/999", nil)
	ctx.Params = gin.Params{{Key: "id", Value: "999"}}

	handler.DeleteDomain(ctx)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for missing domain, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestGetSummaryNormalizesUnknownStatuses(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := config.Default()
	database := newHandlerTestDB(t)
	defer database.Close()

	domain, err := database.CreateDomain("mystery.internal", nil, nil, "", "full", "", 3600, 443, nil)
	if err != nil {
		t.Fatalf("create domain: %v", err)
	}
	if err := database.SaveCheck(&db.Check{
		DomainID:      domain.ID,
		CheckedAt:     time.Now().UTC(),
		SSLChainValid: true,
		OverallStatus: "mystery",
		CheckDuration: 10,
	}); err != nil {
		t.Fatalf("save check: %v", err)
	}

	handler := NewHandler(cfg, database, nil, nil, nil)
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/summary", nil)

	handler.GetSummary(ctx)

	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status code: got %d body=%s", rec.Code, rec.Body.String())
	}

	var summary map[string]int
	if err := json.Unmarshal(rec.Body.Bytes(), &summary); err != nil {
		t.Fatalf("decode summary: %v", err)
	}
	if summary["unknown"] != 1 {
		t.Fatalf("expected unknown summary bucket to be incremented, got %+v", summary)
	}
	if _, ok := summary["mystery"]; ok {
		t.Fatalf("expected unexpected status to be normalized away, got %+v", summary)
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
