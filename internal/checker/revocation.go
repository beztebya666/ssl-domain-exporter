package checker

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"golang.org/x/crypto/ocsp"
)

type RevocationResult struct {
	OCSPStatus string
	OCSPError  string
	CRLStatus  string
	CRLError   string
}

func CheckRevocation(domain string, port int, timeout time.Duration, checkOCSP bool, checkCRL bool) *RevocationResult {
	result := &RevocationResult{}
	host, serverName := tlsHost(domain, port)

	dialer := &net.Dialer{Timeout: timeout}
	conn, err := tls.DialWithDialer(dialer, "tcp", host, &tls.Config{
		InsecureSkipVerify: true,
		ServerName:         serverName,
	})
	if err != nil {
		errText := fmt.Sprintf("tls connect failed: %v", err)
		if checkOCSP {
			result.OCSPStatus = "unavailable"
			result.OCSPError = errText
		}
		if checkCRL {
			result.CRLStatus = "unavailable"
			result.CRLError = errText
		}
		return result
	}
	defer conn.Close()

	state := conn.ConnectionState()
	if len(state.PeerCertificates) == 0 {
		if checkOCSP {
			result.OCSPStatus = "unavailable"
			result.OCSPError = "no peer certificates"
		}
		if checkCRL {
			result.CRLStatus = "unavailable"
			result.CRLError = "no peer certificates"
		}
		return result
	}

	leaf := state.PeerCertificates[0]
	var issuer *x509.Certificate
	if len(state.PeerCertificates) > 1 {
		issuer = state.PeerCertificates[1]
	}

	if checkOCSP {
		result.OCSPStatus, result.OCSPError = checkOCSPStatus(leaf, issuer, timeout)
	}
	if checkCRL {
		result.CRLStatus, result.CRLError = checkCRLStatus(leaf, timeout)
	}

	return result
}

func checkOCSPStatus(leaf, issuer *x509.Certificate, timeout time.Duration) (string, string) {
	if leaf == nil {
		return "unavailable", "missing leaf certificate"
	}
	if len(leaf.OCSPServer) == 0 {
		return "unavailable", "no OCSP responder in certificate"
	}
	if issuer == nil {
		return "unavailable", "missing issuer certificate for OCSP"
	}

	reqBytes, err := ocsp.CreateRequest(leaf, issuer, nil)
	if err != nil {
		return "unavailable", fmt.Sprintf("create ocsp request: %v", err)
	}

	client := &http.Client{Timeout: timeout}
	for _, responder := range leaf.OCSPServer {
		resp, err := client.Post(responder, "application/ocsp-request", bytes.NewReader(reqBytes))
		if err != nil {
			continue
		}
		body, readErr := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if readErr != nil {
			continue
		}
		parsed, parseErr := ocsp.ParseResponseForCert(body, leaf, issuer)
		if parseErr != nil {
			continue
		}
		switch parsed.Status {
		case ocsp.Good:
			return "good", ""
		case ocsp.Revoked:
			return "revoked", "certificate is revoked"
		default:
			return "unknown", "ocsp returned unknown status"
		}
	}

	return "unavailable", "unable to query OCSP responder"
}

func checkCRLStatus(leaf *x509.Certificate, timeout time.Duration) (string, string) {
	if leaf == nil {
		return "unavailable", "missing leaf certificate"
	}
	if len(leaf.CRLDistributionPoints) == 0 {
		return "unavailable", "no CRL distribution points"
	}

	client := &http.Client{Timeout: timeout}
	for _, crlURL := range leaf.CRLDistributionPoints {
		resp, err := client.Get(crlURL)
		if err != nil {
			continue
		}
		body, readErr := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if readErr != nil {
			continue
		}

		revocationList, parseErr := x509.ParseRevocationList(body)
		if parseErr != nil {
			continue
		}

		for _, revoked := range revocationList.RevokedCertificateEntries {
			if revoked.SerialNumber != nil && revoked.SerialNumber.Cmp(leaf.SerialNumber) == 0 {
				return "revoked", "certificate serial found in CRL"
			}
		}
		return "good", ""
	}

	return "unavailable", "unable to download/parse CRL"
}
