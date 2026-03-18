package checker

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"math"
	"net"
	"strings"
	"time"

	"domain-ssl-checker/internal/db"
)

type SSLResult struct {
	Issuer      string
	Subject     string
	ValidFrom   time.Time
	ValidUntil  time.Time
	ExpiryDays  int
	Version     string
	ChainValid  bool
	ChainLength int
	ChainError  string
	ChainCerts  []db.ChainCert
	Error       string
}

func CheckSSL(domain string, port int, timeout time.Duration, customCAPEM string) *SSLResult {
	result := &SSLResult{}
	hostPort, serverName := tlsHost(domain, port)

	dialer := &net.Dialer{Timeout: timeout}
	conn, err := tls.DialWithDialer(dialer, "tcp", hostPort, &tls.Config{
		InsecureSkipVerify: true,
		ServerName:         serverName,
	})
	if err != nil {
		result.Error = fmt.Sprintf("TLS connect failed: %v", err)
		return result
	}
	defer conn.Close()

	state := conn.ConnectionState()
	certs := state.PeerCertificates
	if len(certs) == 0 {
		result.Error = "no certificates received"
		return result
	}

	leaf := certs[0]
	result.Subject = leaf.Subject.CommonName
	if result.Subject == "" {
		result.Subject = leaf.Subject.String()
	}
	result.Issuer = leaf.Issuer.CommonName
	if result.Issuer == "" {
		result.Issuer = leaf.Issuer.String()
	}
	result.ValidFrom = leaf.NotBefore
	result.ValidUntil = leaf.NotAfter
	result.ExpiryDays = int(math.Ceil(time.Until(leaf.NotAfter).Hours() / 24))
	result.Version = tlsVersionString(state.Version)
	result.ChainLength = len(certs)

	for _, cert := range certs {
		isSelfSigned := cert.Subject.String() == cert.Issuer.String()
		subject := cert.Subject.CommonName
		if subject == "" {
			subject = cert.Subject.String()
		}
		issuer := cert.Issuer.CommonName
		if issuer == "" {
			issuer = cert.Issuer.String()
		}
		result.ChainCerts = append(result.ChainCerts, db.ChainCert{
			Subject:      subject,
			Issuer:       issuer,
			ValidFrom:    cert.NotBefore,
			ValidTo:      cert.NotAfter,
			IsCA:         cert.IsCA,
			IsSelfSigned: isSelfSigned,
		})
	}

	result.ChainValid, result.ChainError = verifyChain(serverName, certs, customCAPEM)
	return result
}

func verifyChain(serverName string, certs []*x509.Certificate, customCAPEM string) (bool, string) {
	if len(certs) == 0 {
		return false, "no certificates"
	}

	roots, err := loadRootPool(customCAPEM)
	if err != nil {
		return false, err.Error()
	}

	intermediates := x509.NewCertPool()
	for _, cert := range certs[1:] {
		intermediates.AddCert(cert)
	}

	opts := x509.VerifyOptions{
		DNSName:       serverName,
		Intermediates: intermediates,
		Roots:         roots,
	}

	_, err = certs[0].Verify(opts)
	if err != nil {
		return false, err.Error()
	}
	return true, ""
}

func loadRootPool(customCAPEM string) (*x509.CertPool, error) {
	roots, err := x509.SystemCertPool()
	if err != nil || roots == nil {
		roots = x509.NewCertPool()
	}
	customCAPEM = strings.TrimSpace(customCAPEM)
	if customCAPEM == "" {
		return roots, nil
	}
	if ok := roots.AppendCertsFromPEM([]byte(customCAPEM)); !ok {
		return nil, fmt.Errorf("invalid custom_ca_pem certificate")
	}
	return roots, nil
}

func tlsVersionString(v uint16) string {
	switch v {
	case tls.VersionTLS10:
		return "TLS 1.0"
	case tls.VersionTLS11:
		return "TLS 1.1"
	case tls.VersionTLS12:
		return "TLS 1.2"
	case tls.VersionTLS13:
		return "TLS 1.3"
	default:
		return fmt.Sprintf("TLS 0x%04X", v)
	}
}
