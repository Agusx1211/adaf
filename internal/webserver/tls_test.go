package webserver

import (
	"crypto/ecdsa"
	"crypto/x509"
	"net"
	"testing"
	"time"
)

func TestGenerateSelfSignedCert(t *testing.T) {
	cert, err := generateSelfSignedCert("example.local", "192.168.1.50")
	if err != nil {
		t.Fatalf("generateSelfSignedCert: %v", err)
	}
	if len(cert.Certificate) == 0 {
		t.Fatal("certificate chain is empty")
	}
	if cert.PrivateKey == nil {
		t.Fatal("private key is nil")
	}
	if _, ok := cert.PrivateKey.(*ecdsa.PrivateKey); !ok {
		t.Fatalf("private key type = %T, want *ecdsa.PrivateKey", cert.PrivateKey)
	}

	leaf, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		t.Fatalf("parse certificate: %v", err)
	}

	if got := leaf.Subject.CommonName; got != "adaf-web" {
		t.Fatalf("subject CN = %q, want %q", got, "adaf-web")
	}
	if len(leaf.Subject.Organization) == 0 || leaf.Subject.Organization[0] != "ADAF" {
		t.Fatalf("subject organization = %v, want ADAF", leaf.Subject.Organization)
	}

	if time.Until(leaf.NotAfter) < 300*24*time.Hour {
		t.Fatalf("certificate validity too short: not_after=%s", leaf.NotAfter.Format(time.RFC3339))
	}

	if !containsDNSName(leaf, "localhost") {
		t.Fatal("missing localhost SAN")
	}
	if !containsDNSName(leaf, "example.local") {
		t.Fatal("missing example.local SAN")
	}
	if !containsIP(leaf, net.ParseIP("127.0.0.1")) {
		t.Fatal("missing 127.0.0.1 SAN")
	}
	if !containsIP(leaf, net.ParseIP("::1")) {
		t.Fatal("missing ::1 SAN")
	}
	if !containsIP(leaf, net.ParseIP("192.168.1.50")) {
		t.Fatal("missing 192.168.1.50 SAN")
	}
}

func containsDNSName(cert *x509.Certificate, name string) bool {
	for _, item := range cert.DNSNames {
		if item == name {
			return true
		}
	}
	return false
}

func containsIP(cert *x509.Certificate, ip net.IP) bool {
	for _, item := range cert.IPAddresses {
		if item.Equal(ip) {
			return true
		}
	}
	return false
}
