package config

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := Default()
	if cfg.Server.Host != "0.0.0.0" {
		t.Errorf("expected host 0.0.0.0, got %s", cfg.Server.Host)
	}
	if cfg.Server.Port != "8080" {
		t.Errorf("expected port 8080, got %s", cfg.Server.Port)
	}
	if cfg.Domains.DefaultCheckMode != "full" {
		t.Errorf("expected default check mode full, got %s", cfg.Domains.DefaultCheckMode)
	}
	if !cfg.DNS.UseSystemDNS {
		t.Error("expected use_system_dns=true by default")
	}
}

func TestValidateCheckMode(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"full", "full"},
		{"ssl_only", "ssl_only"},
		{"SSL_ONLY", "ssl_only"},
		{"", "full"},
		{"invalid", "full"},
		{" ssl_only ", "ssl_only"},
	}
	for _, tt := range tests {
		got := ValidateCheckMode(tt.input)
		if got != tt.want {
			t.Errorf("ValidateCheckMode(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestNormalize(t *testing.T) {
	cfg := &Config{}
	cfg.normalize()

	if cfg.Server.Host != "0.0.0.0" {
		t.Errorf("normalize didn't set default host")
	}
	if cfg.Checker.ConcurrentChecks != 5 {
		t.Errorf("normalize didn't set default concurrent_checks")
	}
	if cfg.Prometheus.Path != "/metrics" {
		t.Errorf("normalize didn't set default prometheus path")
	}
	if cfg.DNS.Servers == nil {
		t.Errorf("normalize didn't initialize DNS.Servers")
	}
	if cfg.Domains.DefaultCheckMode != "full" {
		t.Errorf("normalize didn't set default check mode")
	}
}

func TestSnapshotIsDeepCopy(t *testing.T) {
	cfg := Default()
	cfg.DNS.Servers = []string{"10.0.0.1:53"}

	snap := cfg.Snapshot()
	snap.DNS.Servers[0] = "MODIFIED"

	if cfg.DNS.Servers[0] == "MODIFIED" {
		t.Error("Snapshot() returned a shallow copy - DNS.Servers slice shared")
	}
}

func TestApplyFromPreservesFilePath(t *testing.T) {
	cfg := Default()
	cfg.filePath = "/original/path.yaml"

	next := Default()
	next.Server.Port = "9090"

	cfg.ApplyFrom(next)

	if cfg.filePath != "/original/path.yaml" {
		t.Errorf("ApplyFrom overwrote filePath: got %s", cfg.filePath)
	}
	if cfg.Server.Port != "9090" {
		t.Errorf("ApplyFrom didn't apply new port: got %s", cfg.Server.Port)
	}
}

func TestSaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test_config.yaml")

	cfg := Default()
	cfg.filePath = path
	cfg.DNS.Servers = []string{"1.1.1.1:53", "8.8.8.8:53"}
	cfg.Domains.DefaultCheckMode = "ssl_only"

	if err := cfg.Save(); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if loaded.Domains.DefaultCheckMode != "ssl_only" {
		t.Errorf("loaded check mode = %s, want ssl_only", loaded.Domains.DefaultCheckMode)
	}
	if len(loaded.DNS.Servers) != 2 || loaded.DNS.Servers[0] != "1.1.1.1:53" {
		t.Errorf("loaded DNS servers = %v, want [1.1.1.1:53 8.8.8.8:53]", loaded.DNS.Servers)
	}
}

func TestLoadCreatesDefaultIfMissing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "missing_config.yaml")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error on missing file: %v", err)
	}

	if cfg.Server.Port != "8080" {
		t.Errorf("default config not applied: port = %s", cfg.Server.Port)
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Error("Load() did not create the default config file")
	}
}

func TestConcurrentSnapshotAndApply(t *testing.T) {
	cfg := Default()
	cfg.filePath = filepath.Join(t.TempDir(), "concurrent.yaml")

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			_ = cfg.Snapshot()
		}()
		go func() {
			defer wg.Done()
			next := Default()
			next.Server.Port = "9999"
			cfg.ApplyFrom(next)
		}()
	}
	wg.Wait()
}

func TestConcurrentSave(t *testing.T) {
	dir := t.TempDir()
	cfg := Default()
	cfg.filePath = filepath.Join(dir, "save_race.yaml")

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			next := Default()
			next.Server.Port = "7777"
			cfg.ApplyFrom(next)
		}()
		go func() {
			defer wg.Done()
			_ = cfg.Save()
		}()
	}
	wg.Wait()
}

func TestApplyEnvOverridesSupportsK8SLabelSelector(t *testing.T) {
	t.Setenv("K8S_LABEL_SELECTOR", "app=ingress,environment=prod")

	cfg := Default()
	applyEnvOverrides(cfg)

	if cfg.Kubernetes.LabelSelector != "app=ingress,environment=prod" {
		t.Fatalf("expected label selector env override to apply, got %q", cfg.Kubernetes.LabelSelector)
	}
}
