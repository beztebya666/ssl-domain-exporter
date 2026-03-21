package checker

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"strings"
	"testing"
	"time"
)

func TestLoadRootPool_InvalidCustomCAPEM(t *testing.T) {
	_, err := loadRootPool("not a certificate")
	if err == nil {
		t.Fatal("expected invalid custom CA PEM to return an error")
	}
	if !strings.Contains(err.Error(), "invalid custom_ca_pem") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestVerifyChain_WithCustomCA(t *testing.T) {
	rootCert, leafCert, rootPEM := mustCreateTestChain(t, "example.internal")

	if ok, _ := verifyChain("example.internal", []*x509.Certificate{leafCert, rootCert}, ""); ok {
		t.Fatal("expected verification without custom CA to fail for a private test root")
	}

	ok, errMsg := verifyChain("example.internal", []*x509.Certificate{leafCert, rootCert}, rootPEM)
	if !ok {
		t.Fatalf("expected verification with custom CA to succeed, got error: %s", errMsg)
	}
}

func TestVerifyChain_WithWrongServerNameFails(t *testing.T) {
	rootCert, leafCert, rootPEM := mustCreateTestChain(t, "example.internal")

	ok, errMsg := verifyChain("wrong.internal", []*x509.Certificate{leafCert, rootCert}, rootPEM)
	if ok {
		t.Fatal("expected verification to fail for wrong server name")
	}
	if errMsg == "" {
		t.Fatal("expected verification failure to return an error message")
	}
}

func mustCreateTestChain(t *testing.T, dnsName string) (*x509.Certificate, *x509.Certificate, string) {
	t.Helper()

	rootKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate root key: %v", err)
	}
	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate leaf key: %v", err)
	}

	now := time.Now()
	rootTpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "Test Root CA"},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLen:            1,
	}
	rootDER, err := x509.CreateCertificate(rand.Reader, rootTpl, rootTpl, &rootKey.PublicKey, rootKey)
	if err != nil {
		t.Fatalf("create root certificate: %v", err)
	}
	rootCert, err := x509.ParseCertificate(rootDER)
	if err != nil {
		t.Fatalf("parse root certificate: %v", err)
	}

	leafTpl := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: dnsName},
		DNSNames:     []string{dnsName},
		NotBefore:    now.Add(-time.Hour),
		NotAfter:     now.Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	leafDER, err := x509.CreateCertificate(rand.Reader, leafTpl, rootCert, &leafKey.PublicKey, rootKey)
	if err != nil {
		t.Fatalf("create leaf certificate: %v", err)
	}
	leafCert, err := x509.ParseCertificate(leafDER)
	if err != nil {
		t.Fatalf("parse leaf certificate: %v", err)
	}

	rootPEMBytes := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: rootDER})
	return rootCert, leafCert, string(rootPEMBytes)
}
