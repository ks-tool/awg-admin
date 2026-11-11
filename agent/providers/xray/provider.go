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

package xray

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strings"

	"github.com/ks-tool/vpn-agent/providers/proxy"

	"github.com/google/uuid"
)

const APIVersion = "xray.ks-tool.dev/v1"

var interfaceName = regexp.MustCompile(`^[a-zA-Z0-9_.-]+$`)

type InterfaceSpec struct {
	Name            string `json:"name" jsonschema:"required,minLength=1,maxLength=63"`
	Listen          string `json:"listen,omitempty"`
	Port            int    `json:"port" jsonschema:"required,minimum=1,maximum=65535"`
	Network         string `json:"network" jsonschema:"required,enum=tcp,enum=ws"`
	Path            string `json:"path,omitempty"`
	TLS             bool   `json:"tls,omitempty"`
	ServerName      string `json:"serverName,omitempty"`
	CertificateFile string `json:"certificateFile,omitempty"`
	PrivateKeyFile  string `json:"privateKeyFile,omitempty"`
	Disabled        bool   `json:"disabled,omitempty"`
}

type Adapter struct{}

func New() proxy.Adapter { return Adapter{} }

func (Adapter) Name() string             { return "xray" }
func (Adapter) APIVersion() string       { return APIVersion }
func (Adapter) FirewallProtocol() string { return "tcp" }
func (Adapter) InterfaceSchema() any     { return InterfaceSpec{} }
func (Adapter) InterfaceUISchema() json.RawMessage {
	return json.RawMessage(`{"ui:order":["name","listen","port","network","path","tls","serverName","certificateFile","privateKeyFile","disabled"]}`)
}

func (Adapter) NewSecret() (string, error) { return uuid.NewString(), nil }

func (Adapter) ValidateSecret(secret string) error {
	if _, err := uuid.Parse(secret); err != nil {
		return errors.New("Xray client secret must be a UUID")
	}
	return nil
}

func (Adapter) ValidateInterface(raw json.RawMessage) error {
	spec, err := decodeInterface(raw)
	if err != nil {
		return err
	}
	if !interfaceName.MatchString(spec.Name) {
		return errors.New("interface name may contain only letters, digits, dots, underscores and dashes")
	}
	if len(spec.Listen) > 0 && net.ParseIP(spec.Listen) == nil {
		return errors.New("listen must be an IP address")
	}
	if spec.Network == "ws" && (len(spec.Path) == 0 || !strings.HasPrefix(spec.Path, "/")) {
		return errors.New("WebSocket path must start with /")
	}
	if spec.TLS && len(spec.ServerName) == 0 && len(spec.CertificateFile) == 0 {
		return errors.New("serverName is required for the automatically generated TLS certificate")
	}
	return nil
}

func (a Adapter) RenderServer(raw json.RawMessage, peers []proxy.Peer, stateDir string) (proxy.CoreConfig, error) {
	spec, err := decodeInterface(raw)
	if err != nil {
		return proxy.CoreConfig{}, err
	}
	if err := a.ValidateInterface(raw); err != nil {
		return proxy.CoreConfig{}, err
	}
	clients := make([]map[string]any, 0, len(peers))
	for _, peer := range peers {
		if peer.Spec.Disabled {
			continue
		}
		clients = append(clients, map[string]any{"id": peer.Secret, "email": peer.Spec.Name})
	}
	stream := map[string]any{"network": spec.Network, "security": "none"}
	if spec.Network == "ws" {
		stream["wsSettings"] = map[string]any{"path": spec.Path}
	}
	if spec.TLS {
		certificateFile, privateKeyFile, err := ensureTLS(stateDir, spec)
		if err != nil {
			return proxy.CoreConfig{}, err
		}
		stream["security"] = "tls"
		stream["tlsSettings"] = map[string]any{
			"serverName":   spec.ServerName,
			"certificates": []map[string]any{{"certificateFile": certificateFile, "keyFile": privateKeyFile}},
		}
	}
	listen := spec.Listen
	if len(listen) == 0 {
		listen = "0.0.0.0"
	}
	document := map[string]any{
		"log": map[string]any{"loglevel": "warning"},
		"inbounds": []map[string]any{{
			"tag": spec.Name, "listen": listen, "port": spec.Port, "protocol": "vless",
			"settings":       map[string]any{"clients": clients, "decryption": "none"},
			"streamSettings": stream,
		}},
		"outbounds": []map[string]any{{"protocol": "freedom", "tag": "direct"}},
	}
	data, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return proxy.CoreConfig{}, err
	}
	return proxy.CoreConfig{Data: data, Arguments: func(path string) []string { return []string{"run", "-config", path} }}, nil
}

func (Adapter) RenderClient(raw json.RawMessage, peer proxy.Peer, endpoint string) (string, error) {
	spec, err := decodeInterface(raw)
	if err != nil {
		return "", err
	}
	if _, _, err := net.SplitHostPort(endpoint); err != nil {
		return "", fmt.Errorf("endpoint must be host:port: %w", err)
	}
	query := url.Values{}
	query.Set("type", spec.Network)
	query.Set("encryption", "none")
	if spec.TLS {
		query.Set("security", "tls")
		query.Set("sni", spec.ServerName)
		query.Set("allowInsecure", "1")
	} else {
		query.Set("security", "none")
	}
	if spec.Network == "ws" {
		query.Set("path", spec.Path)
		query.Set("host", spec.ServerName)
	}
	uri := url.URL{Scheme: "vless", User: url.User(peer.Secret), Host: endpoint, RawQuery: query.Encode(), Fragment: peer.Spec.Name}
	return uri.String(), nil
}

func ensureTLS(stateDir string, spec InterfaceSpec) (string, string, error) {
	return proxy.EnsureCertificate(stateDir, spec.ServerName, spec.CertificateFile, spec.PrivateKeyFile)
}

func decodeInterface(raw json.RawMessage) (InterfaceSpec, error) {
	var spec InterfaceSpec
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&spec); err != nil {
		return InterfaceSpec{}, err
	}
	return spec, nil
}
