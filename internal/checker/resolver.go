package checker

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	"ssl-domain-exporter/internal/config"

	mdns "github.com/miekg/dns"
)

// ResolveContext holds DNS resolver settings for a single check operation.
type ResolveContext struct {
	Servers        []string      // primary DNS servers to query (host:port)
	Fallback       [][]string    // fallback tiers: tried in order if primary fails
	Timeout        time.Duration // per-query timeout
	ServerSource   string        // "per-domain" / "global-config" / "system" / "none"
	UseSystemDNS   bool          // allow net.DefaultResolver as last-resort fallback
	LastUsedServer string        // actual server that answered (set after resolution)
}

// BuildResolveContext determines the effective DNS servers for a domain check.
// Priority: per-domain > global config > system (if allowed) > controlled error.
// Fallback tiers are preserved so that if the primary tier fails, the next is tried.
func BuildResolveContext(domain interface{ ParseDNSServers() []string }, cfg *config.Config) *ResolveContext {
	rc := &ResolveContext{
		Timeout:      5 * time.Second,
		UseSystemDNS: cfg.DNS.UseSystemDNS,
	}

	// Parse DNS timeout from config
	if cfg.DNS.Timeout != "" {
		if d, err := time.ParseDuration(cfg.DNS.Timeout); err == nil && d > 0 {
			rc.Timeout = d
		}
	}

	// Build tiers: per-domain, global, system
	var tiers [][]string
	if perDomain := domain.ParseDNSServers(); len(perDomain) > 0 {
		tiers = append(tiers, normalizeDNSAddrs(perDomain))
	}
	if len(cfg.DNS.Servers) > 0 {
		tiers = append(tiers, normalizeDNSAddrs(cfg.DNS.Servers))
	}
	if cfg.DNS.UseSystemDNS {
		if sys := systemDNSServers(); len(sys) > 0 {
			tiers = append(tiers, sys)
		}
	}

	if len(tiers) == 0 {
		rc.ServerSource = "none"
		return rc
	}

	// Primary = first tier, rest = fallback
	rc.Servers = tiers[0]
	if len(tiers) > 1 {
		rc.Fallback = tiers[1:]
	}

	// Determine source label from first non-empty tier
	perDomain := domain.ParseDNSServers()
	switch {
	case len(perDomain) > 0:
		rc.ServerSource = "per-domain"
	case len(cfg.DNS.Servers) > 0:
		rc.ServerSource = "global-config"
	default:
		rc.ServerSource = "system"
	}

	return rc
}

// ResolveHost resolves a hostname using configured DNS servers with fallback.
// Tries each tier in order. As last resort, uses Go's net.DefaultResolver if UseSystemDNS is true.
// Sets rc.LastUsedServer to the actual server that answered.
func (rc *ResolveContext) ResolveHost(hostname string) (string, error) {
	// If it's already an IP, return as-is
	if ip := net.ParseIP(hostname); ip != nil {
		return hostname, nil
	}

	// Try primary servers
	if len(rc.Servers) > 0 {
		if resolved, server, err := resolveWithServers(hostname, rc.Servers, rc.Timeout); err == nil {
			rc.LastUsedServer = rc.ServerSource + ":" + server
			return resolved, nil
		}
	}

	// Try fallback tiers
	for _, tier := range rc.Fallback {
		if resolved, server, err := resolveWithServers(hostname, tier, rc.Timeout); err == nil {
			rc.LastUsedServer = "fallback:" + server
			return resolved, nil
		}
	}

	// Last resort: OS resolver (works on all platforms without knowing server addresses)
	if rc.UseSystemDNS {
		ctx, cancel := context.WithTimeout(context.Background(), rc.Timeout)
		defer cancel()
		addrs, err := net.DefaultResolver.LookupHost(ctx, hostname)
		if err == nil && len(addrs) > 0 {
			rc.LastUsedServer = "system"
			return addrs[0], nil
		}
	}

	if len(rc.Servers) == 0 && len(rc.Fallback) == 0 {
		return "", fmt.Errorf("no DNS servers configured (source: %s)", rc.ServerSource)
	}
	return "", fmt.Errorf("DNS resolution failed for %s across all configured servers", hostname)
}

// resolveWithServers tries A then AAAA queries against the given DNS servers.
// Returns (resolved IP, server that answered, error).
func resolveWithServers(hostname string, servers []string, timeout time.Duration) (string, string, error) {
	client := &mdns.Client{Timeout: timeout}

	// Try A records
	msg := new(mdns.Msg)
	msg.SetQuestion(mdns.Fqdn(hostname), mdns.TypeA)
	var lastErr error
	for _, server := range servers {
		resp, _, err := client.Exchange(msg, server)
		if err != nil {
			lastErr = err
			continue
		}
		if resp != nil {
			for _, ans := range resp.Answer {
				if a, ok := ans.(*mdns.A); ok {
					return a.A.String(), server, nil
				}
			}
		}
	}

	// Try AAAA records
	msg.SetQuestion(mdns.Fqdn(hostname), mdns.TypeAAAA)
	for _, server := range servers {
		resp, _, err := client.Exchange(msg, server)
		if err != nil {
			lastErr = err
			continue
		}
		if resp != nil {
			for _, ans := range resp.Answer {
				if aaaa, ok := ans.(*mdns.AAAA); ok {
					return aaaa.AAAA.String(), server, nil
				}
			}
		}
	}

	if lastErr != nil {
		return "", "", fmt.Errorf("DNS query failed: %v", lastErr)
	}
	return "", "", fmt.Errorf("no A/AAAA records found")
}

// AllServerTiers returns all DNS server tiers (primary + fallback) for use by
// subsystems like CAA that need the full fallback chain but do their own queries.
func (rc *ResolveContext) AllServerTiers() [][]string {
	var tiers [][]string
	if len(rc.Servers) > 0 {
		tiers = append(tiers, rc.Servers)
	}
	tiers = append(tiers, rc.Fallback...)
	return tiers
}

// EffectiveServerDesc returns a human-readable description of what was used, for audit.
// If resolution already happened (LastUsedServer is set), returns the actual server.
// Otherwise returns a static description of the configured primary tier.
func (rc *ResolveContext) EffectiveServerDesc() string {
	if rc.LastUsedServer != "" {
		return rc.LastUsedServer
	}
	if len(rc.Servers) == 0 {
		if rc.UseSystemDNS {
			return "system"
		}
		return "none"
	}
	return rc.ServerSource + ":" + strings.Join(rc.Servers, ",")
}

// CustomDialer returns a net.Dialer-compatible function that resolves using custom DNS.
// This is used to inject into http.Transport for HTTP checks.
func (rc *ResolveContext) CustomDialer(baseTimeout time.Duration) func(ctx context.Context, network, addr string) (net.Conn, error) {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, err
		}

		resolvedIP, err := rc.ResolveHost(host)
		if err != nil {
			return nil, err
		}

		dialer := &net.Dialer{Timeout: baseTimeout}
		return dialer.DialContext(ctx, network, net.JoinHostPort(resolvedIP, port))
	}
}

// normalizeDNSAddrs ensures each address has a port (defaults to :53).
func normalizeDNSAddrs(addrs []string) []string {
	result := make([]string, 0, len(addrs))
	for _, a := range addrs {
		a = strings.TrimSpace(a)
		if a == "" {
			continue
		}
		if _, _, err := net.SplitHostPort(a); err != nil {
			a = net.JoinHostPort(a, "53")
		}
		result = append(result, a)
	}
	return result
}
