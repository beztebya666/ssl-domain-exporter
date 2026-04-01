package checker

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// K8SConfig holds Kubernetes connection settings for certificate monitoring.
type K8SConfig struct {
	Enabled            bool   `yaml:"enabled" json:"enabled"`
	APIServer          string `yaml:"api_server" json:"api_server"`
	Token              string `yaml:"token" json:"token"`
	TokenFile          string `yaml:"token_file" json:"token_file"`
	Namespace          string `yaml:"namespace" json:"namespace"`
	LabelSelector      string `yaml:"label_selector" json:"label_selector"`
	InsecureSkipVerify bool   `yaml:"insecure_skip_verify" json:"insecure_skip_verify"`
	CACertFile         string `yaml:"ca_cert_file" json:"ca_cert_file"`
}

// K8SCertificate represents a certificate discovered from Kubernetes.
type K8SCertificate struct {
	Namespace  string    `json:"namespace"`
	SecretName string    `json:"secret_name"`
	Type       string    `json:"type"`
	Subject    string    `json:"subject"`
	Issuer     string    `json:"issuer"`
	Serial     string    `json:"serial"`
	DNSNames   []string  `json:"dns_names"`
	NotBefore  time.Time `json:"not_before"`
	NotAfter   time.Time `json:"not_after"`
	ExpiryDays int       `json:"expiry_days"`
	IsExpired  bool      `json:"is_expired"`
	IsCA       bool      `json:"is_ca"`
	Error      string    `json:"error,omitempty"`
}

// K8SScanResult holds all discovered K8S certificates.
type K8SScanResult struct {
	Certificates []K8SCertificate `json:"certificates"`
	Total        int              `json:"total"`
	Expired      int              `json:"expired"`
	Warning      int              `json:"warning"`
	Healthy      int              `json:"healthy"`
	Errors       int              `json:"errors"`
	ScannedAt    time.Time        `json:"scanned_at"`
	Error        string           `json:"error,omitempty"`
}

type k8sSecretList struct {
	Items []k8sSecret `json:"items"`
}

type k8sSecret struct {
	Metadata struct {
		Name      string `json:"name"`
		Namespace string `json:"namespace"`
	} `json:"metadata"`
	Type string            `json:"type"`
	Data map[string]string `json:"data"`
}

// ScanK8SCertificates connects to a Kubernetes cluster and scans TLS secrets.
func ScanK8SCertificates(cfg K8SConfig, warningDays int) (*K8SScanResult, error) {
	result := &K8SScanResult{
		Certificates: make([]K8SCertificate, 0),
		ScannedAt:    time.Now().UTC(),
	}

	if !cfg.Enabled {
		result.Error = "kubernetes monitoring is not enabled"
		return result, fmt.Errorf("kubernetes monitoring is not enabled")
	}

	apiServer, token, err := resolveK8SAccess(cfg)
	if err != nil {
		result.Error = err.Error()
		return result, err
	}

	client, err := buildK8SHTTPClient(cfg)
	if err != nil {
		result.Error = fmt.Sprintf("failed to configure kubernetes HTTP client: %v", err)
		return result, err
	}

	secretsURL, err := buildK8SSecretsURL(apiServer, cfg.Namespace, cfg.LabelSelector)
	if err != nil {
		result.Error = fmt.Sprintf("failed to build kubernetes secrets URL: %v", err)
		return result, err
	}

	req, err := http.NewRequest("GET", secretsURL, nil)
	if err != nil {
		result.Error = fmt.Sprintf("failed to build request: %v", err)
		return result, err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		result.Error = fmt.Sprintf("kubernetes API request failed: %v", err)
		return result, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		result.Error = fmt.Sprintf("kubernetes API returned %d: %s", resp.StatusCode, string(body))
		return result, fmt.Errorf("kubernetes API returned %d", resp.StatusCode)
	}

	var secretList k8sSecretList
	if err := json.NewDecoder(resp.Body).Decode(&secretList); err != nil {
		result.Error = fmt.Sprintf("failed to decode response: %v", err)
		return result, err
	}

	now := time.Now()
	for _, secret := range secretList.Items {
		certs := parseK8STLSSecret(secret, now)
		result.Certificates = append(result.Certificates, certs...)
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

func FindK8SCertificate(cfg K8SConfig, namespace, secretName, serial string) (*K8SCertificate, error) {
	if !cfg.Enabled {
		return nil, fmt.Errorf("kubernetes monitoring is not enabled")
	}
	namespace = strings.TrimSpace(namespace)
	secretName = strings.TrimSpace(secretName)
	if namespace == "" || secretName == "" {
		return nil, fmt.Errorf("namespace and secret name are required")
	}

	apiServer, token, err := resolveK8SAccess(cfg)
	if err != nil {
		return nil, err
	}

	client, err := buildK8SHTTPClient(cfg)
	if err != nil {
		return nil, err
	}

	secretURL, err := buildK8SSecretURL(apiServer, namespace, secretName)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("GET", secretURL, nil)
	if err != nil {
		return nil, err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("kubernetes API returned %d: %s", resp.StatusCode, string(body))
	}

	var secret k8sSecret
	if err := json.NewDecoder(resp.Body).Decode(&secret); err != nil {
		return nil, err
	}
	certs := parseK8STLSSecret(secret, time.Now())
	if len(certs) == 0 {
		return nil, fmt.Errorf("no certificates found in kubernetes secret %s/%s", namespace, secretName)
	}

	serial = strings.ToUpper(strings.TrimSpace(serial))
	if serial == "" {
		return firstHealthyK8SCertificate(certs), nil
	}
	for _, cert := range certs {
		if strings.EqualFold(strings.TrimSpace(cert.Serial), serial) {
			certCopy := cert
			return &certCopy, nil
		}
	}
	return nil, fmt.Errorf("certificate serial %s not found in kubernetes secret %s/%s", serial, namespace, secretName)
}

func parseK8STLSSecret(secret k8sSecret, now time.Time) []K8SCertificate {
	certData, ok := secret.Data["tls.crt"]
	if !ok {
		return []K8SCertificate{{
			Namespace:  secret.Metadata.Namespace,
			SecretName: secret.Metadata.Name,
			Type:       secret.Type,
			Error:      "no tls.crt data in secret",
		}}
	}

	decoded, err := base64.StdEncoding.DecodeString(certData)
	if err != nil {
		return []K8SCertificate{{
			Namespace:  secret.Metadata.Namespace,
			SecretName: secret.Metadata.Name,
			Type:       secret.Type,
			Error:      fmt.Sprintf("failed to decode tls.crt: %v", err),
		}}
	}

	certs := parsePEMCertificates(decoded)
	if len(certs) == 0 {
		return []K8SCertificate{{
			Namespace:  secret.Metadata.Namespace,
			SecretName: secret.Metadata.Name,
			Type:       secret.Type,
			Error:      "no valid certificates found in tls.crt",
		}}
	}

	result := make([]K8SCertificate, 0, len(certs))
	for _, cert := range certs {
		expiryDays := int(math.Ceil(cert.NotAfter.Sub(now).Hours() / 24))
		subject := cert.Subject.CommonName
		if subject == "" {
			subject = cert.Subject.String()
		}
		issuer := cert.Issuer.CommonName
		if issuer == "" {
			issuer = cert.Issuer.String()
		}
		k8sCert := K8SCertificate{
			Namespace:  secret.Metadata.Namespace,
			SecretName: secret.Metadata.Name,
			Type:       secret.Type,
			Subject:    subject,
			Issuer:     issuer,
			Serial:     serializeCertificateSerial(cert),
			DNSNames:   cert.DNSNames,
			NotBefore:  cert.NotBefore,
			NotAfter:   cert.NotAfter,
			ExpiryDays: expiryDays,
			IsExpired:  now.After(cert.NotAfter),
			IsCA:       cert.IsCA,
		}
		if k8sCert.DNSNames == nil {
			k8sCert.DNSNames = []string{}
		}
		result = append(result, k8sCert)
	}
	return result
}

func parsePEMCertificates(data []byte) []*x509.Certificate {
	var certs []*x509.Certificate
	for {
		block, rest := pem.Decode(data)
		if block == nil {
			break
		}
		if block.Type == "CERTIFICATE" {
			cert, err := x509.ParseCertificate(block.Bytes)
			if err == nil {
				certs = append(certs, cert)
			}
		}
		data = rest
	}
	return certs
}

func buildK8SSecretsURL(apiServer, namespace, labelSelector string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(apiServer))
	if err != nil {
		return "", err
	}

	namespace = strings.TrimSpace(namespace)
	escapedBasePath := strings.TrimRight(parsed.EscapedPath(), "/")
	unescapedBasePath, err := url.PathUnescape(escapedBasePath)
	if err != nil {
		return "", err
	}
	if namespace != "" {
		parsed.RawPath = escapedBasePath + "/api/v1/namespaces/" + url.PathEscape(namespace) + "/secrets"
		parsed.Path = unescapedBasePath + "/api/v1/namespaces/" + namespace + "/secrets"
	} else {
		parsed.RawPath = ""
		parsed.Path = unescapedBasePath + "/api/v1/secrets"
	}

	query := url.Values{}
	query.Set("fieldSelector", "type=kubernetes.io/tls")
	if selector := strings.TrimSpace(labelSelector); selector != "" {
		query.Set("labelSelector", selector)
	}
	parsed.RawQuery = query.Encode()

	return parsed.String(), nil
}

func buildK8SSecretURL(apiServer, namespace, secretName string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(apiServer))
	if err != nil {
		return "", err
	}
	escapedBasePath := strings.TrimRight(parsed.EscapedPath(), "/")
	unescapedBasePath, err := url.PathUnescape(escapedBasePath)
	if err != nil {
		return "", err
	}
	parsed.RawPath = escapedBasePath + "/api/v1/namespaces/" + url.PathEscape(namespace) + "/secrets/" + url.PathEscape(secretName)
	parsed.Path = unescapedBasePath + "/api/v1/namespaces/" + namespace + "/secrets/" + secretName
	return parsed.String(), nil
}

func serializeCertificateSerial(cert *x509.Certificate) string {
	if cert == nil || cert.SerialNumber == nil {
		return ""
	}
	return strings.ToUpper(cert.SerialNumber.Text(16))
}

func buildK8SHTTPClient(cfg K8SConfig) (*http.Client, error) {
	//nolint:gosec // K8S API TLS verification is admin-controlled via config.
	tlsCfg := &tls.Config{
		InsecureSkipVerify: cfg.InsecureSkipVerify,
	}

	if cfg.CACertFile != "" {
		caCert, err := os.ReadFile(cfg.CACertFile)
		if err != nil {
			return nil, fmt.Errorf("read kubernetes CA cert file: %w", err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(caCert) {
			return nil, fmt.Errorf("parse kubernetes CA cert file: no certificates found")
		}
		tlsCfg.RootCAs = pool
	}

	return &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: tlsCfg,
		},
	}, nil
}

func resolveK8SAccess(cfg K8SConfig) (string, string, error) {
	apiServer := strings.TrimRight(strings.TrimSpace(cfg.APIServer), "/")
	if apiServer == "" {
		apiServer = "https://kubernetes.default.svc"
	}

	token := strings.TrimSpace(cfg.Token)
	if token == "" && cfg.TokenFile != "" {
		tokenBytes, err := os.ReadFile(cfg.TokenFile)
		if err != nil {
			return "", "", fmt.Errorf("read token file: %w", err)
		}
		token = strings.TrimSpace(string(tokenBytes))
	}

	return apiServer, token, nil
}

func firstHealthyK8SCertificate(certs []K8SCertificate) *K8SCertificate {
	for _, cert := range certs {
		if cert.Error == "" && !cert.IsCA {
			certCopy := cert
			return &certCopy
		}
	}
	for _, cert := range certs {
		if cert.Error == "" {
			certCopy := cert
			return &certCopy
		}
	}
	if len(certs) == 0 {
		return nil
	}
	certCopy := certs[0]
	return &certCopy
}
