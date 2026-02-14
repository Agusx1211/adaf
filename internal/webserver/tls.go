package webserver

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"strings"
	"time"
)

// generateSelfSignedCert generates a self-signed TLS certificate and returns a tls.Certificate.
// It creates a cert valid for localhost, 127.0.0.1, and optionally additional hosts.
func generateSelfSignedCert(hosts ...string) (tls.Certificate, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, err
	}

	serialLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialLimit)
	if err != nil {
		return tls.Certificate{}, err
	}

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"ADAF"},
			CommonName:   "adaf-web",
		},
		NotBefore:             time.Now().Add(-1 * time.Hour),
		NotAfter:              time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	defaultHosts := []string{"127.0.0.1", "localhost", "::1"}
	allHosts := make([]string, 0, len(defaultHosts)+len(hosts))
	allHosts = append(allHosts, defaultHosts...)
	allHosts = append(allHosts, hosts...)

	seenDNS := map[string]struct{}{}
	seenIP := map[string]struct{}{}
	for _, raw := range allHosts {
		host := strings.TrimSpace(raw)
		if host == "" {
			continue
		}

		if ip := net.ParseIP(host); ip != nil {
			key := ip.String()
			if _, ok := seenIP[key]; ok {
				continue
			}
			seenIP[key] = struct{}{}
			template.IPAddresses = append(template.IPAddresses, ip)
			continue
		}

		if _, ok := seenDNS[host]; ok {
			continue
		}
		seenDNS[host] = struct{}{}
		template.DNSNames = append(template.DNSNames, host)
	}

	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		return tls.Certificate{}, err
	}

	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return tls.Certificate{}, err
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return tls.Certificate{}, err
	}
	cert.Leaf, _ = x509.ParseCertificate(certDER)

	return cert, nil
}
