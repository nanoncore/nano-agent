package pki

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"net"
	"time"
)

// CertificateRequest contains the details for issuing a certificate
type CertificateRequest struct {
	// CommonName is the CN for the certificate (typically node ID)
	CommonName string

	// Organization for the certificate
	Organization string

	// DNSNames for the certificate (optional)
	DNSNames []string

	// IPAddresses for the certificate (optional)
	IPAddresses []net.IP

	// ValidityDays is how long the certificate is valid
	ValidityDays int

	// IsServer indicates if this is a server certificate
	IsServer bool

	// IsClient indicates if this is a client certificate
	IsClient bool
}

// Certificate contains the issued certificate and key
type Certificate struct {
	CertPEM    string    `json:"certificate"`
	KeyPEM     string    `json:"private_key"`
	CACertPEM  string    `json:"ca_certificate"`
	CommonName string    `json:"common_name"`
	NotBefore  time.Time `json:"not_before"`
	NotAfter   time.Time `json:"not_after"`
	Serial     string    `json:"serial"`
}

// IssueCertificate generates a new certificate signed by the CA
func (ca *CertificateAuthority) IssueCertificate(req *CertificateRequest) (*Certificate, error) {
	if req.CommonName == "" {
		return nil, fmt.Errorf("CommonName is required")
	}

	// Default values
	if req.ValidityDays == 0 {
		req.ValidityDays = 365 // 1 year default
	}
	if req.Organization == "" {
		req.Organization = "NanonCore Agents"
	}

	// Generate ECDSA key for the certificate
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("failed to generate key: %w", err)
	}

	// Get serial number
	serial := ca.getNextSerial()

	// Determine key usage
	var keyUsage x509.KeyUsage
	var extKeyUsage []x509.ExtKeyUsage

	keyUsage = x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment

	if req.IsClient {
		extKeyUsage = append(extKeyUsage, x509.ExtKeyUsageClientAuth)
	}
	if req.IsServer {
		extKeyUsage = append(extKeyUsage, x509.ExtKeyUsageServerAuth)
	}
	// Default to client auth if neither specified
	if !req.IsClient && !req.IsServer {
		extKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}
	}

	// Certificate template
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName:   req.CommonName,
			Organization: []string{req.Organization},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().AddDate(0, 0, req.ValidityDays),
		KeyUsage:              keyUsage,
		ExtKeyUsage:           extKeyUsage,
		BasicConstraintsValid: true,
		IsCA:                  false,
	}

	// Add SANs
	if len(req.DNSNames) > 0 {
		template.DNSNames = req.DNSNames
	}
	if len(req.IPAddresses) > 0 {
		template.IPAddresses = req.IPAddresses
	}

	// Sign the certificate with CA
	ca.mu.RLock()
	certDER, err := x509.CreateCertificate(rand.Reader, template, ca.caCert, &privateKey.PublicKey, ca.caKey)
	ca.mu.RUnlock()

	if err != nil {
		return nil, fmt.Errorf("failed to create certificate: %w", err)
	}

	// Encode certificate to PEM
	certPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certDER,
	})

	// Encode private key to PEM
	keyDER, err := x509.MarshalECPrivateKey(privateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal key: %w", err)
	}

	keyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "EC PRIVATE KEY",
		Bytes: keyDER,
	})

	return &Certificate{
		CertPEM:    string(certPEM),
		KeyPEM:     string(keyPEM),
		CACertPEM:  ca.GetCACertPEM(),
		CommonName: req.CommonName,
		NotBefore:  template.NotBefore,
		NotAfter:   template.NotAfter,
		Serial:     serial.String(),
	}, nil
}

// IssueAgentCertificate is a convenience method for issuing agent certificates
func (ca *CertificateAuthority) IssueAgentCertificate(nodeID string, clientIP string) (*Certificate, error) {
	req := &CertificateRequest{
		CommonName:   nodeID,
		Organization: "NanonCore Agents",
		ValidityDays: 365,
		IsClient:     true,
	}

	// Add client IP as SAN if provided
	if clientIP != "" {
		// Remove port if present
		host := clientIP
		if h, _, err := net.SplitHostPort(clientIP); err == nil {
			host = h
		}

		if ip := net.ParseIP(host); ip != nil {
			req.IPAddresses = []net.IP{ip}
		}
	}

	return ca.IssueCertificate(req)
}

// IssueServerCertificate is a convenience method for issuing server certificates
func (ca *CertificateAuthority) IssueServerCertificate(serverName string, dnsNames []string, ips []net.IP) (*Certificate, error) {
	req := &CertificateRequest{
		CommonName:   serverName,
		Organization: "NanonCore",
		DNSNames:     dnsNames,
		IPAddresses:  ips,
		ValidityDays: 365,
		IsServer:     true,
	}

	return ca.IssueCertificate(req)
}

// VerifyCertificate verifies that a certificate was issued by this CA
func (ca *CertificateAuthority) VerifyCertificate(certPEM string) (*x509.Certificate, error) {
	block, _ := pem.Decode([]byte(certPEM))
	if block == nil {
		return nil, fmt.Errorf("failed to decode certificate PEM")
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse certificate: %w", err)
	}

	// Verify the certificate chain
	opts := x509.VerifyOptions{
		Roots: ca.GetCertPool(),
	}

	if _, err := cert.Verify(opts); err != nil {
		return nil, fmt.Errorf("certificate verification failed: %w", err)
	}

	return cert, nil
}

// RevokeCertificate marks a certificate as revoked (stores in a revocation list)
// This is a simplified implementation - production would use CRL or OCSP
func (ca *CertificateAuthority) RevokeCertificate(serial string) error {
	// In a production system, this would:
	// 1. Add to a CRL (Certificate Revocation List)
	// 2. Or update an OCSP responder
	// For now, we'll just log it
	fmt.Printf("[pki] Certificate revoked: serial=%s\n", serial)
	return nil
}
