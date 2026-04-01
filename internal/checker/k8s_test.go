package checker

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"math/big"
	"net/url"
	"testing"
	"time"
)

func TestParseK8STLSSecretIncludesSerial(t *testing.T) {
	rootCert, leafCert, _ := mustCreateTestChain(t, "example.internal")

	bundle := append(
		pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: leafCert.Raw}),
		pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: rootCert.Raw})...,
	)
	secret := k8sSecret{
		Type: "kubernetes.io/tls",
		Data: map[string]string{
			"tls.crt": base64.StdEncoding.EncodeToString(bundle),
		},
	}
	secret.Metadata.Name = "example-tls"
	secret.Metadata.Namespace = "platform"

	certs := parseK8STLSSecret(secret, time.Now())
	if len(certs) != 2 {
		t.Fatalf("expected 2 certificates, got %d", len(certs))
	}
	if certs[0].Serial != serializeCertificateSerial(leafCert) {
		t.Fatalf("unexpected leaf serial: got %q want %q", certs[0].Serial, serializeCertificateSerial(leafCert))
	}
	if certs[1].Serial != serializeCertificateSerial(rootCert) {
		t.Fatalf("unexpected root serial: got %q want %q", certs[1].Serial, serializeCertificateSerial(rootCert))
	}
}

func TestParseK8STLSSecretFallsBackToFullNames(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	template := &x509.Certificate{
		Subject:      pkix.Name{Organization: []string{"Example Org"}},
		Issuer:       pkix.Name{Organization: []string{"Example Issuer"}},
		SerialNumber: big.NewInt(42),
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create certificate: %v", err)
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})

	secret := k8sSecret{
		Type: "kubernetes.io/tls",
		Data: map[string]string{
			"tls.crt": base64.StdEncoding.EncodeToString(pemBytes),
		},
	}
	secret.Metadata.Name = "fallback-tls"
	secret.Metadata.Namespace = "platform"

	certs := parseK8STLSSecret(secret, time.Now())
	if len(certs) != 1 {
		t.Fatalf("expected 1 certificate, got %d", len(certs))
	}
	if certs[0].Subject == "" || certs[0].Issuer == "" {
		t.Fatalf("expected subject and issuer fallbacks, got %+v", certs[0])
	}
}

func TestBuildK8SSecretsURLEscapesNamespaceAndSelector(t *testing.T) {
	rawURL, err := buildK8SSecretsURL("https://cluster.example.local/base", "team/a", "app=my-app,env in (prod,stage)")
	if err != nil {
		t.Fatalf("build url: %v", err)
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}

	if parsed.EscapedPath() != "/base/api/v1/namespaces/team%2Fa/secrets" {
		t.Fatalf("unexpected escaped path: %s", parsed.EscapedPath())
	}
	if got := parsed.Query().Get("fieldSelector"); got != "type=kubernetes.io/tls" {
		t.Fatalf("unexpected fieldSelector: %q", got)
	}
	if got := parsed.Query().Get("labelSelector"); got != "app=my-app,env in (prod,stage)" {
		t.Fatalf("unexpected labelSelector: %q", got)
	}
}
