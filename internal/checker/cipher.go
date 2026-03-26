package checker

import (
	"crypto/tls"
	"fmt"
	"net"
	"strings"
	"time"
)

type CipherResult struct {
	Grade            string
	IsWeak           bool
	WeakReason       string
	SupportedTLS     []string
	WeakCiphers      []string
	NegotiatedCipher string
	Error            string
}

func CheckCipherSuite(domain string, port int, timeout time.Duration, rc *ResolveContext) *CipherResult {
	result := &CipherResult{Grade: "N/A"}
	host, serverName := tlsHost(domain, port)

	// Resolve via custom DNS if needed
	dialAddr := host
	if rc != nil && len(rc.Servers) > 0 {
		h, p := splitHostPort(host)
		if ip := net.ParseIP(h); ip == nil {
			resolved, err := rc.ResolveHost(h)
			if err != nil {
				result.Error = fmt.Sprintf("DNS resolve failed: %v", err)
				return result
			}
			dialAddr = net.JoinHostPort(resolved, p)
		}
	}

	supported := make(map[uint16]bool)
	versions := []struct {
		version uint16
		label   string
	}{
		{tls.VersionTLS10, "TLS 1.0"},
		{tls.VersionTLS11, "TLS 1.1"},
		{tls.VersionTLS12, "TLS 1.2"},
		{tls.VersionTLS13, "TLS 1.3"},
	}

	for _, v := range versions {
		state, err := tlsProbe(dialAddr, serverName, timeout, v.version, v.version, nil)
		if err != nil {
			continue
		}
		supported[v.version] = true
		result.SupportedTLS = append(result.SupportedTLS, v.label)
		if result.NegotiatedCipher == "" {
			result.NegotiatedCipher = tls.CipherSuiteName(state.CipherSuite)
		}
	}

	if len(result.SupportedTLS) == 0 {
		result.Error = "failed to negotiate TLS"
		result.Grade = "F"
		result.IsWeak = true
		result.WeakReason = "tls negotiation failed"
		return result
	}

	weakSuites := []struct {
		id    uint16
		label string
	}{
		{tls.TLS_RSA_WITH_RC4_128_SHA, "RC4"},
		{tls.TLS_RSA_WITH_3DES_EDE_CBC_SHA, "3DES"},
	}

	for _, s := range weakSuites {
		_, err := tlsProbe(dialAddr, serverName, timeout, tls.VersionTLS10, tls.VersionTLS12, []uint16{s.id})
		if err == nil {
			result.WeakCiphers = append(result.WeakCiphers, s.label)
		}
	}

	reasons := make([]string, 0)
	if supported[tls.VersionTLS10] {
		reasons = append(reasons, "supports TLS 1.0")
	}
	if supported[tls.VersionTLS11] {
		reasons = append(reasons, "supports TLS 1.1")
	}
	if len(result.WeakCiphers) > 0 {
		reasons = append(reasons, "accepts weak ciphers: "+strings.Join(result.WeakCiphers, ", "))
	}

	switch {
	case supported[tls.VersionTLS10] || len(result.WeakCiphers) > 0:
		result.Grade = "F"
	case supported[tls.VersionTLS11]:
		result.Grade = "C"
	case supported[tls.VersionTLS12] && !supported[tls.VersionTLS13]:
		result.Grade = "B"
	default:
		result.Grade = "A"
	}

	result.IsWeak = result.Grade == "F" || result.Grade == "C"
	if len(reasons) > 0 {
		result.WeakReason = strings.Join(reasons, "; ")
	}
	return result
}

func tlsProbe(host, serverName string, timeout time.Duration, minVersion, maxVersion uint16, suites []uint16) (*tls.ConnectionState, error) {
	dialer := &net.Dialer{Timeout: timeout}
	//nolint:gosec // Cipher probing intentionally bypasses chain validation to measure protocol/cipher support on broken endpoints too.
	conf := &tls.Config{
		InsecureSkipVerify: true,
		ServerName:         serverName,
		MinVersion:         minVersion,
		MaxVersion:         maxVersion,
		CipherSuites:       suites,
	}
	conn, err := tls.DialWithDialer(dialer, "tcp", host, conf)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	state := conn.ConnectionState()
	return &state, nil
}

func tlsHost(domain string, port int) (hostPort string, serverName string) {
	if port <= 0 {
		port = 443
	}
	if strings.Contains(domain, ":") {
		host, port, err := net.SplitHostPort(domain)
		if err == nil {
			if host == "" {
				host = domain
			}
			if port == "" {
				port = "443"
			}
			return net.JoinHostPort(host, port), host
		}
	}
	return net.JoinHostPort(domain, fmt.Sprintf("%d", port)), domain
}

func CipherSummary(result *CipherResult) string {
	if result == nil {
		return ""
	}
	parts := []string{fmt.Sprintf("grade=%s", result.Grade)}
	if len(result.SupportedTLS) > 0 {
		parts = append(parts, "tls="+strings.Join(result.SupportedTLS, ","))
	}
	if len(result.WeakCiphers) > 0 {
		parts = append(parts, "weak="+strings.Join(result.WeakCiphers, ","))
	}
	if result.NegotiatedCipher != "" {
		parts = append(parts, "cipher="+result.NegotiatedCipher)
	}
	if result.WeakReason != "" {
		parts = append(parts, "reason="+result.WeakReason)
	}
	return strings.Join(parts, " | ")
}
