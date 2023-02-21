/*
Copyright 2023 The KubeVela Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package cert

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"time"
)

const (
	defaultCommonName     = "kubevela"
	defaultOrganization   = "oam.dev"
	defaultKeyLength      = 2048
	defaultLocalDirectory = "apiserver.local.config/certificates"
	defaultCertPath       = defaultLocalDirectory + "/apiserver.crt"
	defaultKeyPath        = defaultLocalDirectory + "/apiserver.key"
)

// NewCA create a ca certificate
func NewCA(commonName string, organization []string) *x509.Certificate {
	return &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject: pkix.Name{
			CommonName:   commonName,
			Organization: organization,
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().AddDate(10, 0, 0),
		IsCA:                  true,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}
}

// GenerateSelfSignedCertificate generate self-signed certificate
// keySize is the size of the private key, ca is the certificate for issuing
// Certificate and PrivateKey pem data are returned
func GenerateSelfSignedCertificate(keySize int, ca *x509.Certificate) ([]byte, []byte, error) {
	key, err := rsa.GenerateKey(rand.Reader, keySize)
	if err != nil {
		return nil, nil, err
	}
	cert, err := x509.CreateCertificate(rand.Reader, ca, ca, &key.PublicKey, key)
	if err != nil {
		return nil, nil, err
	}
	certData := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert})
	keyData := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	return certData, keyData, nil
}

// GenerateCertificateRequest generate certificate request for given commonName
// and organization. keySize is the size of the private key.
// CertificateRequest and PrivateKey pem data are returned
func GenerateCertificateRequest(keySize int, commonName string, organization []string) ([]byte, []byte, error) {
	key, err := rsa.GenerateKey(rand.Reader, keySize)
	if err != nil {
		return nil, nil, err
	}
	template := &x509.CertificateRequest{
		Subject: pkix.Name{
			CommonName:   commonName,
			Organization: organization,
		},
		SignatureAlgorithm: x509.SHA256WithRSA,
	}
	csr, err := x509.CreateCertificateRequest(rand.Reader, template, key)
	if err != nil {
		return nil, nil, err
	}
	keyData := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	csrData := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: csr})
	return csrData, keyData, nil
}

// GenerateDefaultSelfSignedCertificateLocally generate self-signed certificate
// in local directory and return the path for generated certificate and key
func GenerateDefaultSelfSignedCertificateLocally() (string, string, error) {
	ca := NewCA(defaultCommonName, []string{defaultOrganization})
	cert, key, err := GenerateSelfSignedCertificate(defaultKeyLength, ca)
	if err != nil {
		return "", "", err
	}
	if err = os.MkdirAll(defaultLocalDirectory, 0700); err != nil {
		return "", "", err
	}
	if err = os.WriteFile(defaultCertPath, cert, 0600); err != nil {
		return "", "", err
	}
	if err = os.WriteFile(defaultKeyPath, key, 0600); err != nil {
		return "", "", err
	}
	return defaultCertPath, defaultKeyPath, nil
}
