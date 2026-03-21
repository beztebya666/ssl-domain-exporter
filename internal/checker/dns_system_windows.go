//go:build windows

package checker

import (
	"net"
	"os/exec"
	"strings"
)

func systemDNSServers() []string {
	// On Windows, parse "netsh interface ip show dns" output to discover system DNS servers.
	out, err := exec.Command("netsh", "interface", "ip", "show", "dns").Output()
	if err != nil {
		return nil
	}

	seen := make(map[string]bool)
	var servers []string
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		// Lines with DNS IPs often look like:
		//   "Statically Configured DNS Servers:    10.0.0.1"
		//   "DNS servers configured through DHCP:  10.0.0.2"
		//   or just an IP on its own line as continuation
		parts := strings.Fields(line)
		if len(parts) == 0 {
			continue
		}
		candidate := parts[len(parts)-1]
		if ip := net.ParseIP(candidate); ip != nil {
			addr := net.JoinHostPort(candidate, "53")
			if !seen[addr] {
				seen[addr] = true
				servers = append(servers, addr)
			}
		}
	}
	return servers
}
