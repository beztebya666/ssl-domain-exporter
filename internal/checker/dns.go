package checker

import (
	"fmt"
	"net"
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

func CheckCAA(domain string, timeout time.Duration, fallbackDepth int) *DNSResult {
	result := &DNSResult{QueryDomain: domain}
	if fallbackDepth <= 0 {
		fallbackDepth = 5
	}

	servers := systemDNSServers()
	if len(servers) == 0 {
		result.Error = "no DNS servers available"
		return result
	}

	client := &mdns.Client{Timeout: timeout}
	for _, candidate := range candidateDomains(domain, fallbackDepth) {
		result.QueryDomain = candidate
		records, err := queryCAA(client, servers, candidate)
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
	return nil, fmt.Errorf("dns query failed")
}

func systemDNSServers() []string {
	cfg, err := mdns.ClientConfigFromFile("/etc/resolv.conf")
	if err != nil || cfg == nil || len(cfg.Servers) == 0 {
		return []string{"8.8.8.8:53", "1.1.1.1:53"}
	}

	servers := make([]string, 0, len(cfg.Servers))
	for _, s := range cfg.Servers {
		if strings.Contains(s, ":") {
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
