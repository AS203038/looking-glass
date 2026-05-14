package utils

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"time"
)

// GenerateSelfSignedPair mints an in-memory ECDSA (P-256) key plus
// a matching self-signed X.509 certificate, suitable for bringing
// up the HTTPS listener without any on-disk PKI material.
//
// The certificate carries:
//
//   - a placeholder CN of "lg.example.com" and organisation
//     "Looking Glass"
//   - a NotAfter 100 years in the future, since the key is
//     ephemeral and re-generated on every process start
//   - KeyUsage covering CertSign / DigitalSignature / KeyEncipherment
//   - ExtKeyUsage limited to ServerAuth
//
// The returned bytes are a DER-encoded certificate, not PEM; callers
// embed them directly into a [tls.Certificate].
//
// Intended only for development and lab deployments — production
// instances should be fronted by a properly-issued certificate via
// [TLSConfig.Cert] / [TLSConfig.Key].
func GenerateSelfSignedPair() (*ecdsa.PrivateKey, []byte, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, err
	}
	tpl := x509.Certificate{
		SerialNumber: big.NewInt(time.Now().Unix()),
		Subject: pkix.Name{
			CommonName:   "lg.example.com",
			Organization: []string{"Looking Glass"},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().AddDate(100, 0, 0),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  false,
	}
	crt, err := x509.CreateCertificate(rand.Reader, &tpl, &tpl, &key.PublicKey, key)
	if err != nil {
		return nil, nil, err
	}
	return key, crt, nil
}
