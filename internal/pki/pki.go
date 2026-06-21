/*
  Copyright © 2026 Alexey Shulutkov <github@shulutkov.ru>

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

// Package pki generates the self-signed CA, server and client certificates
// used for mutual TLS between awg-admin and an agent listening on a public
// ("white") IP. Certificates are generated locally by awg-admin and stored
// in its own database (models.AgentTLS); the server certificate and CA are
// deployed to the agent alongside its binary, the client certificate is
// kept by awg-admin to authenticate itself when dialing the agent directly.
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
	"net"
	"time"

	"github.com/ks-tool/awg-admin/models"
)

const (
	caValidity   = 10 * 365 * 24 * time.Hour
	leafValidity = 825 * 24 * time.Hour // matches the ~2yr ceiling most TLS stacks accept
)

// IssueAgentTLS generates a fresh CA plus a server certificate (valid for
// host, covering the given IPs/DNS names so it matches whatever "white" IP
// or hostname the agent will be reached on) and a client certificate that
// awg-admin will present to the agent.
func IssueAgentTLS(commonName string, ips []net.IP, dnsNames []string) (*models.AgentTLS, error) {
	caCert, caKey, err := generateCA(commonName + " CA")
	if err != nil {
		return nil, fmt.Errorf("generate CA: %w", err)
	}

	serverCert, serverKey, err := issueLeaf(caCert, caKey, leafTemplate{
		commonName:  commonName,
		ips:         ips,
		dnsNames:    dnsNames,
		extKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	})
	if err != nil {
		return nil, fmt.Errorf("issue server cert: %w", err)
	}

	clientCert, clientKey, err := issueLeaf(caCert, caKey, leafTemplate{
		commonName:  "awg-admin",
		extKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	})
	if err != nil {
		return nil, fmt.Errorf("issue client cert: %w", err)
	}

	caCertPEM, caKeyPEM, err := encodePair(caCert.Raw, caKey)
	if err != nil {
		return nil, err
	}
	serverCertPEM, serverKeyPEM, err := encodePair(serverCert, serverKey)
	if err != nil {
		return nil, err
	}
	clientCertPEM, clientKeyPEM, err := encodePair(clientCert, clientKey)
	if err != nil {
		return nil, err
	}

	return &models.AgentTLS{
		CA:     models.CertKeyPair{Certificate: caCertPEM, PrivateKey: caKeyPEM},
		Server: models.CertKeyPair{Certificate: serverCertPEM, PrivateKey: serverKeyPEM},
		Client: models.CertKeyPair{Certificate: clientCertPEM, PrivateKey: clientKeyPEM},
	}, nil
}

func generateCA(commonName string) (*x509.Certificate, *ecdsa.PrivateKey, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, err
	}

	serial, err := randomSerial()
	if err != nil {
		return nil, nil, err
	}

	template := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: commonName},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(caValidity),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return nil, nil, err
	}

	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, nil, err
	}
	return cert, key, nil
}

type leafTemplate struct {
	commonName  string
	ips         []net.IP
	dnsNames    []string
	extKeyUsage []x509.ExtKeyUsage
}

func issueLeaf(caCert *x509.Certificate, caKey *ecdsa.PrivateKey, lt leafTemplate) (certDER []byte, key *ecdsa.PrivateKey, err error) {
	key, err = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, err
	}

	serial, err := randomSerial()
	if err != nil {
		return nil, nil, err
	}

	template := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: lt.commonName},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(leafValidity),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  lt.extKeyUsage,
		IPAddresses:  lt.ips,
		DNSNames:     lt.dnsNames,
	}

	certDER, err = x509.CreateCertificate(rand.Reader, template, caCert, &key.PublicKey, caKey)
	if err != nil {
		return nil, nil, err
	}
	return certDER, key, nil
}

func encodePair(certDER []byte, key *ecdsa.PrivateKey) (certPEM, keyPEM string, err error) {
	certBytes := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})

	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return "", "", err
	}
	keyBytes := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	return string(certBytes), string(keyBytes), nil
}

func randomSerial() (*big.Int, error) {
	limit := new(big.Int).Lsh(big.NewInt(1), 128)
	return rand.Int(rand.Reader, limit)
}
