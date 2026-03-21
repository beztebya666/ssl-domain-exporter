//go:build !windows

package checker

import (
	"net"

	mdns "github.com/miekg/dns"
)

func systemDNSServers() []string {
	cfg, err := mdns.ClientConfigFromFile("/etc/resolv.conf")
	if err != nil || cfg == nil || len(cfg.Servers) == 0 {
		return nil // Do NOT fall back to public resolvers silently
	}

	servers := make([]string, 0, len(cfg.Servers))
	for _, s := range cfg.Servers {
		if _, _, splitErr := net.SplitHostPort(s); splitErr == nil {
			servers = append(servers, s)
			continue
		}
		port := cfg.Port
		if port == "" {
			port = "53"
		}
		servers = append(servers, net.JoinHostPort(s, port))
	}
	return servers
}
