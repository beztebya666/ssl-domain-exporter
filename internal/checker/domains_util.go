package checker

import "strings"

func candidateDomains(domain string, depth int) []string {
	domain = strings.Trim(strings.ToLower(strings.TrimSpace(domain)), ".")
	if domain == "" {
		return nil
	}
	if depth <= 0 {
		depth = 5
	}

	parts := strings.Split(domain, ".")
	if len(parts) <= 2 {
		return []string{domain}
	}

	result := []string{domain}
	maxSkips := len(parts) - 2
	if depth < maxSkips {
		maxSkips = depth
	}
	for skip := 1; skip <= maxSkips; skip++ {
		candidate := strings.Join(parts[skip:], ".")
		result = append(result, candidate)
	}
	return result
}

func isSubdomain(domain string) bool {
	return strings.Count(domain, ".") >= 2
}
