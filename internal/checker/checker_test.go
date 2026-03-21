package checker

import (
	"testing"
	"time"

	"ssl-domain-exporter/internal/config"
	"ssl-domain-exporter/internal/db"
)

func intPtr(v int) *int { return &v }

func defaultCfg() *config.Config {
	return config.Default()
}

// --- computeStatus tests ---

func TestComputeStatus_OK(t *testing.T) {
	cfg := defaultCfg()
	check := &db.Check{
		SSLExpiryDays: intPtr(90),
		SSLChainValid: true,
	}
	if s := computeStatus(check, cfg); s != "ok" {
		t.Errorf("expected ok, got %s", s)
	}
}

func TestComputeStatus_SSLCritical(t *testing.T) {
	cfg := defaultCfg()
	check := &db.Check{
		SSLExpiryDays: intPtr(2), // below default critical=3
		SSLChainValid: true,
	}
	if s := computeStatus(check, cfg); s != "critical" {
		t.Errorf("expected critical, got %s", s)
	}
}

func TestComputeStatus_SSLWarning(t *testing.T) {
	cfg := defaultCfg()
	check := &db.Check{
		SSLExpiryDays: intPtr(10), // below warning=14 but above critical=3
		SSLChainValid: true,
	}
	if s := computeStatus(check, cfg); s != "warning" {
		t.Errorf("expected warning, got %s", s)
	}
}

func TestComputeStatus_DomainExpiryIgnoredForSSLOnly(t *testing.T) {
	cfg := defaultCfg()
	check := &db.Check{
		SSLExpiryDays:            intPtr(90),
		SSLChainValid:            true,
		DomainExpiryDays:         intPtr(1), // would be critical if checked
		RegistrationCheckSkipped: true,
	}
	if s := computeStatus(check, cfg); s != "ok" {
		t.Errorf("expected ok (domain expiry ignored for ssl_only), got %s", s)
	}
}

func TestComputeStatus_DomainExpiryCritical_FullMode(t *testing.T) {
	cfg := defaultCfg()
	check := &db.Check{
		SSLExpiryDays:            intPtr(90),
		SSLChainValid:            true,
		DomainExpiryDays:         intPtr(1), // below critical=7
		RegistrationCheckSkipped: false,
	}
	if s := computeStatus(check, cfg); s != "critical" {
		t.Errorf("expected critical, got %s", s)
	}
}

func TestComputeStatus_SSLOnlyErrorWhenSSLFails(t *testing.T) {
	cfg := defaultCfg()
	check := &db.Check{
		SSLCheckError:            "connection refused",
		RegistrationCheckSkipped: true,
	}
	if s := computeStatus(check, cfg); s != "error" {
		t.Errorf("expected error for ssl_only with SSL failure, got %s", s)
	}
}

func TestComputeStatus_FullModeErrorWhenBothFail(t *testing.T) {
	cfg := defaultCfg()
	check := &db.Check{
		SSLCheckError:            "timeout",
		DomainCheckError:         "RDAP failed",
		RegistrationCheckSkipped: false,
	}
	if s := computeStatus(check, cfg); s != "error" {
		t.Errorf("expected error when both SSL and domain fail in full mode, got %s", s)
	}
}

func TestComputeStatus_ChainInvalid(t *testing.T) {
	cfg := defaultCfg()
	check := &db.Check{
		SSLExpiryDays: intPtr(90),
		SSLChainValid: false,
	}
	if s := computeStatus(check, cfg); s != "warning" {
		t.Errorf("expected warning for invalid chain, got %s", s)
	}
}

func TestComputeStatus_CAAAbsentWarning(t *testing.T) {
	cfg := defaultCfg()
	cfg.Features.CAACheck = true
	check := &db.Check{
		SSLExpiryDays: intPtr(90),
		SSLChainValid: true,
		CAAPresent:    false,
		CAAError:      "",
	}
	if s := computeStatus(check, cfg); s != "warning" {
		t.Errorf("expected warning for CAA absent, got %s", s)
	}
}

func TestComputeStatus_CAAErrorNotWarning(t *testing.T) {
	cfg := defaultCfg()
	cfg.Features.CAACheck = true
	check := &db.Check{
		SSLExpiryDays: intPtr(90),
		SSLChainValid: true,
		CAAPresent:    false,
		CAAError:      "dns query failed for all servers",
	}
	// CAAError is set -> should NOT trigger the "CAA absent" warning
	if s := computeStatus(check, cfg); s != "ok" {
		t.Errorf("expected ok (CAA error should not trigger absent warning), got %s", s)
	}
}

func TestComputeStatus_CipherGradeF(t *testing.T) {
	cfg := defaultCfg()
	cfg.Features.CipherCheck = true
	check := &db.Check{
		SSLExpiryDays: intPtr(90),
		SSLChainValid: true,
		CipherGrade:   "F",
	}
	if s := computeStatus(check, cfg); s != "critical" {
		t.Errorf("expected critical for cipher grade F, got %s", s)
	}
}

func TestComputeStatus_OCSPRevoked(t *testing.T) {
	cfg := defaultCfg()
	cfg.Features.OCSPCheck = true
	check := &db.Check{
		SSLExpiryDays: intPtr(90),
		SSLChainValid: true,
		OCSPStatus:    "revoked",
	}
	if s := computeStatus(check, cfg); s != "critical" {
		t.Errorf("expected critical for OCSP revoked, got %s", s)
	}
}

// --- ResolveContext tests ---

func TestBuildResolveContext_PerDomain(t *testing.T) {
	cfg := defaultCfg()
	cfg.DNS.Servers = []string{"1.1.1.1:53"}
	cfg.DNS.UseSystemDNS = false

	domain := &db.Domain{DNSServers: "10.0.0.1:53, 10.0.0.2:53"}
	rc := BuildResolveContext(domain, cfg)

	if rc.ServerSource != "per-domain" {
		t.Errorf("expected per-domain, got %s", rc.ServerSource)
	}
	if len(rc.Servers) != 2 || rc.Servers[0] != "10.0.0.1:53" {
		t.Errorf("expected per-domain servers, got %v", rc.Servers)
	}
	if len(rc.Fallback) != 1 || rc.Fallback[0][0] != "1.1.1.1:53" {
		t.Errorf("expected global config as fallback, got %v", rc.Fallback)
	}
}

func TestBuildResolveContext_GlobalOnly(t *testing.T) {
	cfg := defaultCfg()
	cfg.DNS.Servers = []string{"8.8.8.8"}
	cfg.DNS.UseSystemDNS = false

	domain := &db.Domain{}
	rc := BuildResolveContext(domain, cfg)

	if rc.ServerSource != "global-config" {
		t.Errorf("expected global-config, got %s", rc.ServerSource)
	}
	if len(rc.Servers) != 1 || rc.Servers[0] != "8.8.8.8:53" {
		t.Errorf("expected normalized 8.8.8.8:53, got %v", rc.Servers)
	}
}

func TestBuildResolveContext_NoServers(t *testing.T) {
	cfg := defaultCfg()
	cfg.DNS.Servers = []string{}
	cfg.DNS.UseSystemDNS = false

	domain := &db.Domain{}
	rc := BuildResolveContext(domain, cfg)

	// No system DNS will be found in test environment necessarily,
	// but with UseSystemDNS=false, source should be "none"
	if rc.ServerSource != "none" {
		t.Errorf("expected none, got %s", rc.ServerSource)
	}
}

func TestBuildResolveContext_Timeout(t *testing.T) {
	cfg := defaultCfg()
	cfg.DNS.Timeout = "10s"

	domain := &db.Domain{}
	rc := BuildResolveContext(domain, cfg)

	if rc.Timeout != 10*time.Second {
		t.Errorf("expected 10s timeout, got %v", rc.Timeout)
	}
}

func TestNormalizeDNSAddrs(t *testing.T) {
	tests := []struct {
		input []string
		want  []string
	}{
		{[]string{"1.1.1.1"}, []string{"1.1.1.1:53"}},
		{[]string{"1.1.1.1:5353"}, []string{"1.1.1.1:5353"}},
		{[]string{"  ", ""}, []string{}},
		{[]string{"10.0.0.1", "10.0.0.2:53"}, []string{"10.0.0.1:53", "10.0.0.2:53"}},
	}
	for _, tt := range tests {
		got := normalizeDNSAddrs(tt.input)
		if len(got) != len(tt.want) {
			t.Errorf("normalizeDNSAddrs(%v) = %v, want %v", tt.input, got, tt.want)
			continue
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Errorf("normalizeDNSAddrs(%v)[%d] = %s, want %s", tt.input, i, got[i], tt.want[i])
			}
		}
	}
}

func TestResolveHost_IP(t *testing.T) {
	rc := &ResolveContext{Timeout: 5 * time.Second}
	resolved, err := rc.ResolveHost("192.168.1.1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resolved != "192.168.1.1" {
		t.Errorf("expected 192.168.1.1, got %s", resolved)
	}
}

func TestResolveHost_IPv6(t *testing.T) {
	rc := &ResolveContext{Timeout: 5 * time.Second}
	resolved, err := rc.ResolveHost("::1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resolved != "::1" {
		t.Errorf("expected ::1, got %s", resolved)
	}
}

func TestResolveHost_NoServersError(t *testing.T) {
	rc := &ResolveContext{
		Timeout:      1 * time.Second,
		ServerSource: "none",
		UseSystemDNS: false,
	}
	_, err := rc.ResolveHost("example.com")
	if err == nil {
		t.Fatal("expected error for no DNS servers, got nil")
	}
}

func TestAllServerTiers(t *testing.T) {
	rc := &ResolveContext{
		Servers:  []string{"10.0.0.1:53"},
		Fallback: [][]string{{"8.8.8.8:53"}, {"1.1.1.1:53"}},
	}
	tiers := rc.AllServerTiers()
	if len(tiers) != 3 {
		t.Fatalf("expected 3 tiers, got %d", len(tiers))
	}
	if tiers[0][0] != "10.0.0.1:53" {
		t.Errorf("first tier wrong: %v", tiers[0])
	}
	if tiers[2][0] != "1.1.1.1:53" {
		t.Errorf("third tier wrong: %v", tiers[2])
	}
}

func TestEffectiveServerDesc_LastUsed(t *testing.T) {
	rc := &ResolveContext{
		Servers:        []string{"10.0.0.1:53"},
		ServerSource:   "per-domain",
		LastUsedServer: "fallback:8.8.8.8:53",
	}
	if desc := rc.EffectiveServerDesc(); desc != "fallback:8.8.8.8:53" {
		t.Errorf("expected LastUsedServer, got %s", desc)
	}
}

func TestEffectiveServerDesc_Static(t *testing.T) {
	rc := &ResolveContext{
		Servers:      []string{"10.0.0.1:53"},
		ServerSource: "per-domain",
	}
	if desc := rc.EffectiveServerDesc(); desc != "per-domain:10.0.0.1:53" {
		t.Errorf("expected static desc, got %s", desc)
	}
}

func TestEffectiveServerDesc_System(t *testing.T) {
	rc := &ResolveContext{UseSystemDNS: true}
	if desc := rc.EffectiveServerDesc(); desc != "system" {
		t.Errorf("expected system, got %s", desc)
	}
}

// --- candidateDomains tests ---

func TestCandidateDomains(t *testing.T) {
	candidates := candidateDomains("sub.example.com", 5)
	if len(candidates) < 2 {
		t.Fatalf("expected at least 2 candidates, got %d: %v", len(candidates), candidates)
	}
	if candidates[0] != "sub.example.com" {
		t.Errorf("first candidate should be the domain itself, got %s", candidates[0])
	}
	if candidates[1] != "example.com" {
		t.Errorf("second candidate should be parent, got %s", candidates[1])
	}
}

func TestIsSubdomain(t *testing.T) {
	if !isSubdomain("sub.example.com") {
		t.Error("sub.example.com should be a subdomain")
	}
	if isSubdomain("example.com") {
		t.Error("example.com should NOT be a subdomain")
	}
}

// --- Domain model tests ---

func TestDomainEffectiveCheckMode(t *testing.T) {
	tests := []struct {
		mode string
		want string
	}{
		{"", "full"},
		{"full", "full"},
		{"ssl_only", "ssl_only"},
		{"invalid", "full"},
	}
	for _, tt := range tests {
		d := &db.Domain{CheckMode: tt.mode}
		if got := d.EffectiveCheckMode(); got != tt.want {
			t.Errorf("CheckMode=%q -> EffectiveCheckMode()=%q, want %q", tt.mode, got, tt.want)
		}
	}
}

func TestDomainRegistrationCheckEnabled(t *testing.T) {
	d := &db.Domain{CheckMode: "full"}
	if !d.RegistrationCheckEnabled() {
		t.Error("full mode should enable registration check")
	}

	d.CheckMode = "ssl_only"
	if d.RegistrationCheckEnabled() {
		t.Error("ssl_only mode should disable registration check")
	}
}

func TestDomainParseDNSServers(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"", 0},
		{"  ", 0},
		{"10.0.0.1:53", 1},
		{"10.0.0.1:53, 10.0.0.2:53", 2},
		{"10.0.0.1:53,,10.0.0.2:53", 2},
	}
	for _, tt := range tests {
		d := &db.Domain{DNSServers: tt.input}
		got := d.ParseDNSServers()
		if len(got) != tt.want {
			t.Errorf("ParseDNSServers(%q) returned %d servers, want %d", tt.input, len(got), tt.want)
		}
	}
}

func TestCAAFallbackServersIncludeLoopbackAndNoDuplicates(t *testing.T) {
	servers := caaFallbackServers()
	if len(servers) == 0 {
		t.Fatal("expected CAA fallback servers to be present")
	}

	seen := make(map[string]bool, len(servers))
	foundLoopback := false
	for _, server := range servers {
		if seen[server] {
			t.Fatalf("duplicate fallback server found: %s", server)
		}
		seen[server] = true
		if server == "127.0.0.53:53" || server == "127.0.0.1:53" || server == "[::1]:53" {
			foundLoopback = true
		}
	}

	if !foundLoopback {
		t.Fatal("expected at least one loopback DNS stub in CAA fallback servers")
	}
}
