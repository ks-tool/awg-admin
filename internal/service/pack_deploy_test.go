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

package service

import (
	"crypto/x509"
	"encoding/pem"
	"testing"

	"github.com/ks-tool/vpn-admin/models"
)

func TestConfigureAgentDeployment(t *testing.T) {
	svc := newTestService(t)
	srv, err := svc.CreateServer(ServerInput{
		Name: "srv1",
		SSH:  models.SSHConfig{Host: "192.0.2.10", User: "root", Password: "secret"},
		Agent: models.Agent{
			Address:            "127.0.0.1:8080",
			MonitoringDisabled: true,
			ProfilingEnabled:   true,
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := svc.configureAgentDeployment(srv, &models.AgentDeploymentSettings{Address: "vpn.example.com:8443", MTLS: true}); err != nil {
		t.Fatal(err)
	}
	stored, err := svc.store.Servers().Get(srv.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Agent.Address != "vpn.example.com:8443" || stored.Agent.TLS.IsZero() {
		t.Fatalf("mTLS settings were not saved: %+v", stored.Agent)
	}
	if !stored.Agent.MonitoringDisabled || !stored.Agent.ProfilingEnabled {
		t.Fatalf("unrelated agent settings changed: %+v", stored.Agent)
	}
	block, _ := pem.Decode([]byte(stored.Agent.TLS.Server.Certificate))
	if block == nil {
		t.Fatal("server certificate is not PEM")
	}
	certificate, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	if len(certificate.DNSNames) != 1 || certificate.DNSNames[0] != "vpn.example.com" {
		t.Fatalf("certificate DNS names = %v", certificate.DNSNames)
	}

	if err := svc.configureAgentDeployment(stored, &models.AgentDeploymentSettings{Address: "127.0.0.1:9090"}); err != nil {
		t.Fatal(err)
	}
	stored, err = svc.store.Servers().Get(srv.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Agent.Address != "127.0.0.1:9090" || !stored.Agent.TLS.IsZero() {
		t.Fatalf("SSH tunnel settings were not saved: %+v", stored.Agent)
	}
}

func TestConfigureAgentDeploymentRejectsInvalidSettings(t *testing.T) {
	svc := newTestService(t)
	srv, err := svc.CreateServer(ServerInput{
		Name:  "srv1",
		SSH:   models.SSHConfig{Host: "192.0.2.10", User: "root", Password: "secret"},
		Agent: models.Agent{Address: "127.0.0.1:8080"},
	})
	if err != nil {
		t.Fatal(err)
	}

	for _, settings := range []*models.AgentDeploymentSettings{
		{Address: ""},
		{Address: "vpn.example.com"},
		{Address: "127.0.0.1:0"},
		{Address: "127.0.0.1:http"},
		{Address: "192.0.2.10:8080"},
	} {
		if err := svc.configureAgentDeployment(srv, settings); err == nil {
			t.Errorf("settings were accepted: %+v", settings)
		}
	}
	stored, err := svc.store.Servers().Get(srv.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Agent.Address != "127.0.0.1:8080" {
		t.Fatalf("invalid settings changed the stored address: %q", stored.Agent.Address)
	}
}
