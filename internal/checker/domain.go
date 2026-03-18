package checker

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/likexian/whois"
	whoisparser "github.com/likexian/whois-parser"
)

type DomainResult struct {
	Status     string
	Registrar  string
	CreatedAt  *time.Time
	ExpiresAt  *time.Time
	ExpiryDays *int
	Source     string // rdap, whois, failed
	Error      string
}

// Bootstrap cache — fetched once, reused for 24h
var (
	bootstrapCache     map[string]string // tld -> rdap base url
	bootstrapCacheTime time.Time
	bootstrapMu        sync.RWMutex
	bootstrapTTL       = 24 * time.Hour
)

type rdapResponse struct {
	Status   []string     `json:"status"`
	Entities []rdapEntity `json:"entities"`
	Events   []rdapEvent  `json:"events"`
}

type rdapEntity struct {
	Roles      []string    `json:"roles"`
	VCardArray interface{} `json:"vcardArray"`
}

type rdapEvent struct {
	EventAction string `json:"eventAction"`
	EventDate   string `json:"eventDate"`
}

type rdapBootstrapJSON struct {
	Services [][][]string `json:"services"`
}

func CheckDomain(domain string, timeout time.Duration) *DomainResult {
	rdapResult := checkRDAP(domain, timeout)
	if rdapResult.Error == "" {
		rdapResult.Source = "rdap"
		log.Printf("[domain] %s: RDAP ok (expires in %v days)", domain, ptrStr(rdapResult.ExpiryDays))
		return rdapResult
	}
	log.Printf("[domain] %s: RDAP failed (%s), trying WHOIS", domain, rdapResult.Error)

	whoisResult := checkWHOIS(domain, timeout)
	if whoisResult.Error == "" {
		whoisResult.Source = "whois"
		log.Printf("[domain] %s: WHOIS ok (expires in %v days)", domain, ptrStr(whoisResult.ExpiryDays))
		return whoisResult
	}
	log.Printf("[domain] %s: WHOIS failed (%s)", domain, whoisResult.Error)

	rdapResult.Source = "failed"
	rdapResult.Error = fmt.Sprintf("RDAP: %s | WHOIS: %s", rdapResult.Error, whoisResult.Error)
	return rdapResult
}

func ptrStr(v *int) string {
	if v == nil {
		return "nil"
	}
	return fmt.Sprintf("%d", *v)
}

// getRDAPBootstrap returns cached bootstrap map or fetches it
func getRDAPBootstrap(timeout time.Duration) (map[string]string, error) {
	bootstrapMu.RLock()
	if bootstrapCache != nil && time.Since(bootstrapCacheTime) < bootstrapTTL {
		defer bootstrapMu.RUnlock()
		return bootstrapCache, nil
	}
	bootstrapMu.RUnlock()

	bootstrapMu.Lock()
	defer bootstrapMu.Unlock()

	// Double-check after acquiring write lock
	if bootstrapCache != nil && time.Since(bootstrapCacheTime) < bootstrapTTL {
		return bootstrapCache, nil
	}

	client := &http.Client{Timeout: timeout}
	resp, err := client.Get("https://data.iana.org/rdap/dns.json")
	if err != nil {
		return nil, fmt.Errorf("fetch bootstrap: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read bootstrap: %w", err)
	}

	var raw rdapBootstrapJSON
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("parse bootstrap: %w", err)
	}

	result := make(map[string]string)
	for _, service := range raw.Services {
		if len(service) < 2 || len(service[1]) == 0 {
			continue
		}
		baseURL := service[1][0]
		for _, tld := range service[0] {
			result[strings.ToLower(tld)] = baseURL
		}
	}

	bootstrapCache = result
	bootstrapCacheTime = time.Now()
	log.Printf("[rdap] bootstrap cached: %d TLDs", len(result))
	return result, nil
}

func checkRDAP(domain string, timeout time.Duration) *DomainResult {
	result := &DomainResult{}

	parts := strings.Split(domain, ".")
	if len(parts) < 2 {
		result.Error = "invalid domain"
		return result
	}
	tld := strings.ToLower(parts[len(parts)-1])

	bootstrap, err := getRDAPBootstrap(timeout / 2) // use half timeout for bootstrap
	if err != nil {
		result.Error = err.Error()
		return result
	}

	rdapBase, ok := bootstrap[tld]
	if !ok {
		result.Error = fmt.Sprintf("no RDAP server for .%s", tld)
		return result
	}

	rdapURL := strings.TrimRight(rdapBase, "/") + "/domain/" + domain
	client := &http.Client{Timeout: timeout / 2}
	rdapResp, err := client.Get(rdapURL)
	if err != nil {
		result.Error = fmt.Sprintf("rdap query: %v", err)
		return result
	}
	defer rdapResp.Body.Close()

	switch rdapResp.StatusCode {
	case http.StatusOK:
		// continue
	case http.StatusNotFound:
		result.Error = "domain not found in RDAP"
		return result
	case http.StatusTooManyRequests:
		result.Error = "rdap rate limited (429)"
		return result
	default:
		result.Error = fmt.Sprintf("rdap HTTP %d", rdapResp.StatusCode)
		return result
	}

	body, err := io.ReadAll(rdapResp.Body)
	if err != nil {
		result.Error = fmt.Sprintf("rdap read: %v", err)
		return result
	}

	var data rdapResponse
	if err := json.Unmarshal(body, &data); err != nil {
		result.Error = fmt.Sprintf("rdap parse: %v", err)
		return result
	}

	result.Status = strings.Join(data.Status, ", ")

	for _, event := range data.Events {
		t, err := time.Parse(time.RFC3339, event.EventDate)
		if err != nil {
			// try without timezone
			t, err = time.Parse("2006-01-02T15:04:05", event.EventDate)
		}
		if err != nil {
			continue
		}
		switch event.EventAction {
		case "registration":
			result.CreatedAt = &t
		case "expiration":
			result.ExpiresAt = &t
			days := int(math.Ceil(time.Until(t).Hours() / 24))
			result.ExpiryDays = &days
		}
	}

	// Extract registrar name from vCard
	for _, entity := range data.Entities {
		for _, role := range entity.Roles {
			if role != "registrar" {
				continue
			}
			result.Registrar = extractVCardFN(entity.VCardArray)
			break
		}
	}

	return result
}

func extractVCardFN(vcardArray interface{}) string {
	vc, ok := vcardArray.([]interface{})
	if !ok || len(vc) < 2 {
		return ""
	}
	items, ok := vc[1].([]interface{})
	if !ok {
		return ""
	}
	for _, item := range items {
		arr, ok := item.([]interface{})
		if !ok || len(arr) < 4 {
			continue
		}
		if field, ok := arr[0].(string); ok && field == "fn" {
			if name, ok := arr[3].(string); ok {
				return name
			}
		}
	}
	return ""
}

func checkWHOIS(domain string, timeout time.Duration) *DomainResult {
	result := &DomainResult{}

	raw, err := whois.Whois(domain)
	if err != nil {
		result.Error = fmt.Sprintf("whois query: %v", err)
		return result
	}

	parsed, err := whoisparser.Parse(raw)
	if err != nil {
		result.Error = fmt.Sprintf("whois parse: %v", err)
		return result
	}

	if parsed.Domain != nil {
		result.Status = strings.Join(parsed.Domain.Status, ", ")
		result.ExpiresAt, result.ExpiryDays = parseDate(parsed.Domain.ExpirationDate)
		result.CreatedAt, _ = parseDate(parsed.Domain.CreatedDate)
	}
	if parsed.Registrar != nil {
		result.Registrar = parsed.Registrar.Name
	}

	return result
}

func parseDate(s string) (*time.Time, *int) {
	if s == "" {
		return nil, nil
	}
	formats := []string{
		time.RFC3339,
		"2006-01-02T15:04:05Z",
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05",
		"2006-01-02",
		"02-Jan-2006",
		"January 2, 2006",
	}
	for _, layout := range formats {
		t, err := time.Parse(layout, s)
		if err == nil {
			days := int(math.Ceil(time.Until(t).Hours() / 24))
			return &t, &days
		}
	}
	return nil, nil
}
