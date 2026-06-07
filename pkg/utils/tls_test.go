package utils

import (
	"crypto/x509"
	"testing"
)

// TestGenerateSelfSignedPair verifies that the generated ECDSA key and certificate are valid.
func TestGenerateSelfSignedPair(t *testing.T) {
	key, crt, err := GenerateSelfSignedPair()
	if err != nil {
		t.Fatalf("failed to generate self-signed pair: %v", err)
	}

	if key == nil {
		t.Fatalf("generated key is nil")
	}

	if len(crt) == 0 {
		t.Fatalf("generated certificate bytes are empty")
	}

	// Verify certificate can be parsed and is valid
	cert, err := x509.ParseCertificate(crt)
	if err != nil {
		t.Fatalf("failed to parse generated certificate: %v", err)
	}

	if cert.Subject.CommonName != "lg.example.com" {
		t.Errorf("expected CommonName 'lg.example.com', got %q", cert.Subject.CommonName)
	}

	if cert.IsCA {
		t.Errorf("expected IsCA to be false")
	}
}
