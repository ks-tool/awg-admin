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

package proxy

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/ks-tool/vpn-agent/storage"
)

type secretStore struct {
	root string
}

func newSecretStore(root string) (*secretStore, error) {
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, err
	}
	return &secretStore{root: root}, nil
}

func (s *secretStore) get(resourceID string) (string, error) {
	data, err := os.ReadFile(s.filename(resourceID))
	if os.IsNotExist(err) {
		return "", storage.ErrNotFound
	}
	return string(data), err
}

func (s *secretStore) put(resourceID, secret string) error {
	return writePrivate(s.filename(resourceID), []byte(secret))
}

func (s *secretStore) delete(resourceID string) error {
	if err := os.Remove(s.filename(resourceID)); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (s *secretStore) filename(resourceID string) string {
	sum := sha256.Sum256([]byte(resourceID))
	return filepath.Join(s.root, hex.EncodeToString(sum[:])+".secret")
}

func writePrivate(path string, data []byte) error {
	file, err := os.CreateTemp(filepath.Dir(path), ".secret-*")
	if err != nil {
		return err
	}
	name := file.Name()
	defer func() { _ = os.Remove(name) }()
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		return err
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(name, path)
}

func EnsureCertificate(dir, serverName, certificateFile, privateKeyFile string) (string, string, error) {
	if len(certificateFile) > 0 || len(privateKeyFile) > 0 {
		if len(certificateFile) == 0 || len(privateKeyFile) == 0 {
			return "", "", errors.New("certificateFile and privateKeyFile must be set together")
		}
		if _, err := os.Stat(certificateFile); err != nil {
			return "", "", err
		}
		if _, err := os.Stat(privateKeyFile); err != nil {
			return "", "", err
		}
		return certificateFile, privateKeyFile, nil
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", "", err
	}
	certificateFile = filepath.Join(dir, "tls.crt")
	privateKeyFile = filepath.Join(dir, "tls.key")
	if _, err := os.Stat(certificateFile); err == nil {
		if _, keyErr := os.Stat(privateKeyFile); keyErr == nil {
			return certificateFile, privateKeyFile, nil
		}
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return "", "", err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return "", "", err
	}
	if len(serverName) == 0 {
		serverName = "localhost"
	}
	template := x509.Certificate{
		SerialNumber: serial, Subject: pkix.Name{CommonName: serverName},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	if ip := net.ParseIP(serverName); ip != nil {
		template.IPAddresses = []net.IP{ip}
	} else {
		template.DNSNames = []string{serverName}
	}
	certificate, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		return "", "", err
	}
	privateKey, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return "", "", err
	}
	if err := writePrivate(privateKeyFile, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: privateKey})); err != nil {
		return "", "", err
	}
	if err := writePrivate(certificateFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certificate})); err != nil {
		return "", "", err
	}
	return certificateFile, privateKeyFile, nil
}
