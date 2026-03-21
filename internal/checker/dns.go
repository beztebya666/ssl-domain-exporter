package checker

import (
	"fmt"
	"strings"
	"time"

	mdns "github.com/miekg/dns"
)

type DNSResult struct {
	CAARecords  []string
	CAAPresent  bool
	QueryDomain string
	Error       string
}

func CheckCAA(domain string, timeout time.Duration, fallbackDepth int, rc *ResolveContext) *DNSResult {
	result := &DNSResult{QueryDomain: domain}
	if fallbackDepth <= 0 {
		fallbackDepth = 5
	}

	tiers := rc.AllServerTiers()

	for _, candidate := range candidateDomains(domain, fallbackDepth) {
		result.QueryDomain = candidate

		// Try each DNS tier in order.
		found := false
		for _, servers := range tiers {
			records, err := queryCAA(&mdns.Client{Timeout: timeout}, servers, candidate)
			if err != nil {
				result.Error = err.Error()
				continue
			}
			if len(records) > 0 {
				result.CAARecords = records
				result.CAAPresent = true
				result.Error = ""
				return result
			}

			// Tier responded successfully but had no CAA records.
			result.Error = ""
			found = true
			break
		}

		// Last resort for CAA: try discovered system DNS servers and local stubs.
		if !found && rc.UseSystemDNS {
			records, err := queryCAAViaOS(candidate, timeout)
			if err == nil {
				if len(records) > 0 {
					result.CAARecords = records
					result.CAAPresent = true
					result.Error = ""
					return result
				}
				result.Error = ""
				found = true
			}
		}

		if found {
			// DNS answered for this candidate; continue RFC parent walk if needed.
			continue
		}
	}

	if result.Error == "" && len(tiers) == 0 && !rc.UseSystemDNS {
		result.Error = fmt.Sprintf("no DNS servers available (source: %s)", rc.ServerSource)
	}

	if result.Error != "" {
		return result
	}
	result.CAAPresent = false
	return result
}

func queryCAA(client *mdns.Client, servers []string, domain string) ([]string, error) {
	msg := new(mdns.Msg)
	msg.SetQuestion(mdns.Fqdn(domain), mdns.TypeCAA)

	var lastErr error
	for _, server := range servers {
		resp, _, err := client.Exchange(msg, server)
		if err != nil {
			lastErr = err
			continue
		}
		if resp == nil {
			continue
		}
		if resp.Rcode != mdns.RcodeSuccess && resp.Rcode != mdns.RcodeNameError {
			lastErr = fmt.Errorf("dns rcode %d", resp.Rcode)
			continue
		}

		records := make([]string, 0)
		for _, ans := range resp.Answer {
			if caa, ok := ans.(*mdns.CAA); ok {
				records = append(records, fmt.Sprintf("%d %s \"%s\"", caa.Flag, caa.Tag, caa.Value))
			}
		}
		return records, nil
	}

	if lastErr != nil {
		return nil, lastErr
	}
	return nil, fmt.Errorf("dns query failed for all servers")
}

// queryCAAViaOS performs a best-effort CAA lookup using discovered system DNS
// servers plus common local DNS stubs. Go's stdlib has no native CAA API, so
// this path stays on miekg/dns and returns an error if no local/system resolver
// can answer instead of pretending CAA is absent.
func queryCAAViaOS(domain string, timeout time.Duration) ([]string, error) {
	localServers := caaFallbackServers()
	if len(localServers) == 0 {
		return nil, fmt.Errorf("no local/system DNS servers available for CAA fallback")
	}
	client := &mdns.Client{Timeout: timeout}
	return queryCAA(client, localServers, domain)
}

func caaFallbackServers() []string {
	candidates := append([]string{}, systemDNSServers()...)
	candidates = append(candidates, "127.0.0.53:53", "127.0.0.1:53", "[::1]:53")

	seen := make(map[string]struct{}, len(candidates))
	servers := make([]string, 0, len(candidates))
	for _, server := range candidates {
		server = strings.TrimSpace(server)
		if server == "" {
			continue
		}
		if _, ok := seen[server]; ok {
			continue
		}
		seen[server] = struct{}{}
		servers = append(servers, server)
	}
	return servers
}

// systemDNSServers is defined in dns_system.go (Linux/Mac) and dns_system_windows.go (Windows)
