package pki

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// CertificateAuthority manages the PKI for mTLS agent enrollment
type CertificateAuthority struct {
	mu         sync.RWMutex
	caCert     *x509.Certificate
	caKey      *ecdsa.PrivateKey
	caCertPEM  []byte
	caKeyPEM   []byte
	certsDir   string
	serialFile string
	nextSerial *big.Int
}

// CAConfig contains configuration for the Certificate Authority
type CAConfig struct {
	// CertsDir is the directory to store CA files
	CertsDir string

	// Organization name for the CA
	Organization string

	// Country code (2-letter)
	Country string

	// CA validity period
	ValidityYears int

	// CommonName for the CA certificate
	CommonName string
}

// DefaultCAConfig returns default CA configuration
func DefaultCAConfig() *CAConfig {
	return &CAConfig{
		CertsDir:      "/var/lib/nanoncore/pki",
		Organization:  "NanonCore",
		Country:       "US",
		ValidityYears: 10,
		CommonName:    "NanonCore CA",
	}
}

// NewCertificateAuthority creates or loads a Certificate Authority
func NewCertificateAuthority(config *CAConfig) (*CertificateAuthority, error) {
	if config == nil {
		config = DefaultCAConfig()
	}

	ca := &CertificateAuthority{
		certsDir:   config.CertsDir,
		nextSerial: big.NewInt(1),
	}

	// Create certs directory if not exists
	if err := os.MkdirAll(config.CertsDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create certs directory: %w", err)
	}

	ca.serialFile = filepath.Join(config.CertsDir, "serial")

	// Try to load existing CA
	caCertPath := filepath.Join(config.CertsDir, "ca.crt")
	caKeyPath := filepath.Join(config.CertsDir, "ca.key")

	if fileExists(caCertPath) && fileExists(caKeyPath) {
		if err := ca.loadCA(caCertPath, caKeyPath); err != nil {
			return nil, fmt.Errorf("failed to load existing CA: %w", err)
		}
		fmt.Printf("[pki] Loaded existing CA from %s\n", config.CertsDir)
	} else {
		// Generate new CA
		if err := ca.generateCA(config); err != nil {
			return nil, fmt.Errorf("failed to generate CA: %w", err)
		}
		// Save CA to disk
		if err := ca.saveCA(caCertPath, caKeyPath); err != nil {
			return nil, fmt.Errorf("failed to save CA: %w", err)
		}
		fmt.Printf("[pki] Generated new CA at %s\n", config.CertsDir)
	}

	// Load serial number
	ca.loadSerial()

	return ca, nil
}

// generateCA creates a new CA certificate and key
func (ca *CertificateAuthority) generateCA(config *CAConfig) error {
	// Generate ECDSA key (P-256)
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return fmt.Errorf("failed to generate CA key: %w", err)
	}

	// Serial number for CA cert
	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return fmt.Errorf("failed to generate serial: %w", err)
	}

	// CA certificate template
	template := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{config.Organization},
			Country:      []string{config.Country},
			CommonName:   config.CommonName,
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().AddDate(config.ValidityYears, 0, 0),
		IsCA:                  true,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		MaxPathLen:            1,
	}

	// Self-sign the CA certificate
	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &privateKey.PublicKey, privateKey)
	if err != nil {
		return fmt.Errorf("failed to create CA certificate: %w", err)
	}

	// Parse back to get x509.Certificate
	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		return fmt.Errorf("failed to parse CA certificate: %w", err)
	}

	// Encode to PEM
	ca.caCertPEM = pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certDER,
	})

	keyDER, err := x509.MarshalECPrivateKey(privateKey)
	if err != nil {
		return fmt.Errorf("failed to marshal CA key: %w", err)
	}

	ca.caKeyPEM = pem.EncodeToMemory(&pem.Block{
		Type:  "EC PRIVATE KEY",
		Bytes: keyDER,
	})

	ca.caCert = cert
	ca.caKey = privateKey

	return nil
}

// loadCA loads existing CA certificate and key from disk
func (ca *CertificateAuthority) loadCA(certPath, keyPath string) error {
	// Load certificate
	certPEM, err := os.ReadFile(certPath)
	if err != nil {
		return fmt.Errorf("failed to read CA cert: %w", err)
	}

	block, _ := pem.Decode(certPEM)
	if block == nil {
		return fmt.Errorf("failed to decode CA cert PEM")
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return fmt.Errorf("failed to parse CA cert: %w", err)
	}

	// Load private key
	keyPEM, err := os.ReadFile(keyPath)
	if err != nil {
		return fmt.Errorf("failed to read CA key: %w", err)
	}

	block, _ = pem.Decode(keyPEM)
	if block == nil {
		return fmt.Errorf("failed to decode CA key PEM")
	}

	key, err := x509.ParseECPrivateKey(block.Bytes)
	if err != nil {
		return fmt.Errorf("failed to parse CA key: %w", err)
	}

	ca.caCert = cert
	ca.caKey = key
	ca.caCertPEM = certPEM
	ca.caKeyPEM = keyPEM

	return nil
}

// saveCA saves the CA certificate and key to disk
func (ca *CertificateAuthority) saveCA(certPath, keyPath string) error {
	// Save certificate (restricted permissions for security)
	if err := os.WriteFile(certPath, ca.caCertPEM, 0600); err != nil {
		return fmt.Errorf("failed to write CA cert: %w", err)
	}

	// Save private key (restricted permissions)
	if err := os.WriteFile(keyPath, ca.caKeyPEM, 0600); err != nil {
		return fmt.Errorf("failed to write CA key: %w", err)
	}

	return nil
}

// loadSerial loads the serial number from disk
func (ca *CertificateAuthority) loadSerial() {
	data, err := os.ReadFile(ca.serialFile)
	if err != nil {
		ca.nextSerial = big.NewInt(1)
		return
	}

	ca.nextSerial = new(big.Int)
	ca.nextSerial.SetString(string(data), 10)
}

// saveSerial saves the serial number to disk
func (ca *CertificateAuthority) saveSerial() {
	_ = os.WriteFile(ca.serialFile, []byte(ca.nextSerial.String()), 0600)
}

// getNextSerial returns and increments the serial number
func (ca *CertificateAuthority) getNextSerial() *big.Int {
	ca.mu.Lock()
	defer ca.mu.Unlock()

	serial := new(big.Int).Set(ca.nextSerial)
	ca.nextSerial.Add(ca.nextSerial, big.NewInt(1))
	ca.saveSerial()

	return serial
}

// GetCACertPEM returns the CA certificate in PEM format
func (ca *CertificateAuthority) GetCACertPEM() string {
	ca.mu.RLock()
	defer ca.mu.RUnlock()
	return string(ca.caCertPEM)
}

// GetCACert returns the CA certificate
func (ca *CertificateAuthority) GetCACert() *x509.Certificate {
	ca.mu.RLock()
	defer ca.mu.RUnlock()
	return ca.caCert
}

// GetCertPool returns a certificate pool containing the CA cert
func (ca *CertificateAuthority) GetCertPool() *x509.CertPool {
	pool := x509.NewCertPool()
	pool.AddCert(ca.caCert)
	return pool
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
