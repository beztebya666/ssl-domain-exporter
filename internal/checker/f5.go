package checker

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// F5Config holds F5 BIG-IP connection settings for SSL certificate monitoring.
type F5Config struct {
	Enabled            bool   `yaml:"enabled" json:"enabled"`
	Host               string `yaml:"host" json:"host"`
	Username           string `yaml:"username" json:"username"`
	Password           string `yaml:"password" json:"password"`
	InsecureSkipVerify bool   `yaml:"insecure_skip_verify" json:"insecure_skip_verify"`
	Partition          string `yaml:"partition" json:"partition"`
}

// F5Certificate represents an SSL certificate from an F5 device.
type F5Certificate struct {
	Name       string    `json:"name"`
	Partition  string    `json:"partition"`
	Subject    string    `json:"subject"`
	Issuer     string    `json:"issuer"`
	NotBefore  time.Time `json:"not_before"`
	NotAfter   time.Time `json:"not_after"`
	ExpiryDays int       `json:"expiry_days"`
	IsExpired  bool      `json:"is_expired"`
	Serial     string    `json:"serial"`
	KeyType    string    `json:"key_type"`
	Error      string    `json:"error,omitempty"`
}

// F5ScanResult holds all SSL certificates discovered from an F5 device.
type F5ScanResult struct {
	Certificates []F5Certificate `json:"certificates"`
	Total        int             `json:"total"`
	Expired      int             `json:"expired"`
	Warning      int             `json:"warning"`
	Healthy      int             `json:"healthy"`
	Errors       int             `json:"errors"`
	ScannedAt    time.Time       `json:"scanned_at"`
	Error        string          `json:"error,omitempty"`
}

// f5CertResponse represents the F5 iControl REST API response for certificates.
type f5CertResponse struct {
	Items []f5CertItem `json:"items"`
}

type f5CertItem struct {
	Name          string `json:"name"`
	Partition     string `json:"partition"`
	CommonName    string `json:"commonName"`
	SubjectAltStr string `json:"subjectAlternativeName,omitempty"`
	Issuer        string `json:"issuer"`
	CreateTime    string `json:"createTime"`     // F5 date format
	ExpirationStr string `json:"expirationDate"` // F5 date format: "Jan  1 00:00:00 2026 GMT"
	SerialNumber  string `json:"serialNumber"`
	KeyType       string `json:"keyType"`
}

// ScanF5Certificates connects to an F5 BIG-IP device and retrieves SSL certificate information.
func ScanF5Certificates(cfg F5Config, warningDays int) (*F5ScanResult, error) {
	result := &F5ScanResult{
		Certificates: make([]F5Certificate, 0),
		ScannedAt:    time.Now().UTC(),
	}

	if !cfg.Enabled {
		result.Error = "F5 monitoring is not enabled"
		return result, fmt.Errorf("F5 monitoring is not enabled")
	}

	host := strings.TrimRight(strings.TrimSpace(cfg.Host), "/")
	if host == "" {
		result.Error = "F5 host is required"
		return result, fmt.Errorf("F5 host is required")
	}

	if !strings.HasPrefix(host, "https://") && !strings.HasPrefix(host, "http://") {
		host = "https://" + host
	}

	certResp, err := listF5Certificates(cfg, strings.TrimSpace(cfg.Partition), "")
	if err != nil {
		result.Error = err.Error()
		return result, err
	}

	now := time.Now()
	for _, item := range certResp.Items {
		cert := convertF5Cert(item, now)
		result.Certificates = append(result.Certificates, cert)
	}

	for _, cert := range result.Certificates {
		result.Total++
		if cert.Error != "" {
			result.Errors++
		} else if cert.IsExpired {
			result.Expired++
		} else if cert.ExpiryDays <= warningDays {
			result.Warning++
		} else {
			result.Healthy++
		}
	}

	return result, nil
}

func FindF5Certificate(cfg F5Config, partition, certificateName, serial string) (*F5Certificate, error) {
	if !cfg.Enabled {
		return nil, fmt.Errorf("F5 monitoring is not enabled")
	}
	certificateName = strings.TrimSpace(certificateName)
	if certificateName == "" {
		return nil, fmt.Errorf("certificate name is required")
	}

	certResp, err := listF5Certificates(cfg, strings.TrimSpace(partition), certificateName)
	if err != nil {
		return nil, err
	}
	if len(certResp.Items) == 0 {
		if partition != "" {
			return nil, fmt.Errorf("F5 certificate %s/%s was not found", partition, certificateName)
		}
		return nil, fmt.Errorf("F5 certificate %s was not found", certificateName)
	}

	now := time.Now()
	serial = strings.TrimSpace(serial)
	for _, item := range certResp.Items {
		cert := convertF5Cert(item, now)
		if serial == "" || strings.EqualFold(cert.Serial, serial) {
			return &cert, nil
		}
	}

	return nil, fmt.Errorf("F5 certificate %s/%s with serial %s was not found", partition, certificateName, serial)
}

func convertF5Cert(item f5CertItem, now time.Time) F5Certificate {
	cert := F5Certificate{
		Name:      item.Name,
		Partition: item.Partition,
		Subject:   item.CommonName,
		Issuer:    item.Issuer,
		Serial:    item.SerialNumber,
		KeyType:   item.KeyType,
	}

	// Parse F5 date format: "Jan  1 00:00:00 2026 GMT" or other variations
	if item.ExpirationStr != "" {
		expiry, err := parseF5Date(item.ExpirationStr)
		if err == nil {
			cert.NotAfter = expiry
			cert.ExpiryDays = int(math.Ceil(expiry.Sub(now).Hours() / 24))
			cert.IsExpired = now.After(expiry)
		} else {
			cert.Error = fmt.Sprintf("failed to parse expiration date: %v", err)
		}
	}

	if item.CreateTime != "" {
		created, err := parseF5Date(item.CreateTime)
		if err == nil {
			cert.NotBefore = created
		}
	}

	return cert
}

func parseF5Date(dateStr string) (time.Time, error) {
	dateStr = strings.TrimSpace(dateStr)
	// F5 uses multiple date formats
	formats := []string{
		"Jan  2 15:04:05 2006 MST",
		"Jan 2 15:04:05 2006 MST",
		"2006-01-02T15:04:05Z",
		"2006-01-02 15:04:05",
		time.RFC3339,
	}
	for _, format := range formats {
		if t, err := time.Parse(format, dateStr); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("unrecognized date format: %q", dateStr)
}

func buildF5HTTPClient(cfg F5Config) *http.Client {
	//nolint:gosec // F5 API TLS verification is admin-controlled via config.
	tlsCfg := &tls.Config{
		InsecureSkipVerify: cfg.InsecureSkipVerify,
	}

	return &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: tlsCfg,
		},
	}
}

func listF5Certificates(cfg F5Config, partition, certificateName string) (*f5CertResponse, error) {
	host := strings.TrimRight(strings.TrimSpace(cfg.Host), "/")
	if host == "" {
		return nil, fmt.Errorf("F5 host is required")
	}
	if !strings.HasPrefix(host, "https://") && !strings.HasPrefix(host, "http://") {
		host = "https://" + host
	}

	certURL := fmt.Sprintf("%s/mgmt/tm/sys/file/ssl-cert", host)
	query := url.Values{}
	filters := make([]string, 0, 2)
	if strings.TrimSpace(partition) != "" {
		filters = append(filters, "partition eq "+strings.TrimSpace(partition))
	}
	if strings.TrimSpace(certificateName) != "" {
		filters = append(filters, "name eq "+strings.TrimSpace(certificateName))
	}
	if len(filters) > 0 {
		query.Set("$filter", strings.Join(filters, " and "))
		certURL += "?" + query.Encode()
	}

	req, err := http.NewRequest("GET", certURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to build request: %w", err)
	}
	req.SetBasicAuth(cfg.Username, cfg.Password)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")

	client := buildF5HTTPClient(cfg)
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("F5 API request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("F5 API returned %d: %s", resp.StatusCode, string(body))
	}

	var certResp f5CertResponse
	if err := json.NewDecoder(resp.Body).Decode(&certResp); err != nil {
		return nil, fmt.Errorf("failed to decode F5 response: %w", err)
	}
	return &certResp, nil
}
