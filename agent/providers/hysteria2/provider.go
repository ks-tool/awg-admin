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

package hysteria2

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/ks-tool/vpn-agent/providers/proxy"
)

const APIVersion = "hysteria2.ks-tool.dev/v1"

var interfaceName = regexp.MustCompile(`^[a-zA-Z0-9_.-]+$`)

type InterfaceSpec struct {
	Name            string `json:"name" jsonschema:"required,minLength=1,maxLength=63"`
	Listen          string `json:"listen,omitempty"`
	Port            int    `json:"port" jsonschema:"required,minimum=1,maximum=65535"`
	ServerName      string `json:"serverName" jsonschema:"required,minLength=1"`
	CertificateFile string `json:"certificateFile,omitempty"`
	PrivateKeyFile  string `json:"privateKeyFile,omitempty"`
	UpMbps          int    `json:"upMbps,omitempty" jsonschema:"minimum=0"`
	DownMbps        int    `json:"downMbps,omitempty" jsonschema:"minimum=0"`
	MasqueradeURL   string `json:"masqueradeURL,omitempty" jsonschema:"format=uri"`
	Disabled        bool   `json:"disabled,omitempty"`
}

type Adapter struct{}

func New() proxy.Adapter { return Adapter{} }

func (Adapter) Name() string             { return "hysteria2" }
func (Adapter) APIVersion() string       { return APIVersion }
func (Adapter) FirewallProtocol() string { return "udp" }
func (Adapter) InterfaceSchema() any     { return InterfaceSpec{} }
func (Adapter) InterfaceUISchema() json.RawMessage {
	return json.RawMessage(`{"ui:order":["name","listen","port","serverName","certificateFile","privateKeyFile","upMbps","downMbps","masqueradeURL","disabled"]}`)
}

func (Adapter) NewSecret() (string, error) {
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(secret), nil
}

func (Adapter) ValidateSecret(secret string) error {
	if len(secret) < 16 || len(secret) > 256 || strings.ContainsAny(secret, "\r\n\x00") {
		return errors.New("Hysteria 2 password must contain 16 to 256 printable characters")
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
	if len(spec.MasqueradeURL) > 0 {
		parsed, err := url.Parse(spec.MasqueradeURL)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || len(parsed.Host) == 0 {
			return errors.New("masqueradeURL must be an absolute HTTP(S) URL")
		}
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
	certificateFile, privateKeyFile, err := proxy.EnsureCertificate(stateDir, spec.ServerName, spec.CertificateFile, spec.PrivateKeyFile)
	if err != nil {
		return proxy.CoreConfig{}, err
	}
	users := make(map[string]string, len(peers))
	for _, peer := range peers {
		if peer.Spec.Disabled {
			continue
		}
		if _, exists := users[peer.Spec.Name]; exists {
			return proxy.CoreConfig{}, fmt.Errorf("duplicate Hysteria 2 username %q", peer.Spec.Name)
		}
		users[peer.Spec.Name] = peer.Secret
	}
	listen := spec.Listen
	if len(listen) == 0 {
		listen = "::"
	}
	document := map[string]any{
		"listen": net.JoinHostPort(listen, strconv.Itoa(spec.Port)),
		"tls":    map[string]any{"cert": certificateFile, "key": privateKeyFile},
		"auth":   map[string]any{"type": "userpass", "userpass": users},
	}
	bandwidth := map[string]any{}
	if spec.UpMbps > 0 {
		bandwidth["up"] = strconv.Itoa(spec.UpMbps) + " mbps"
	}
	if spec.DownMbps > 0 {
		bandwidth["down"] = strconv.Itoa(spec.DownMbps) + " mbps"
	}
	if len(bandwidth) > 0 {
		document["bandwidth"] = bandwidth
	}
	if len(spec.MasqueradeURL) > 0 {
		document["masquerade"] = map[string]any{"type": "proxy", "proxy": map[string]any{"url": spec.MasqueradeURL, "rewriteHost": true}}
	}
	data, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return proxy.CoreConfig{}, err
	}
	return proxy.CoreConfig{Data: data, Arguments: func(path string) []string { return []string{"server", "-c", path} }}, nil
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
	query.Set("sni", spec.ServerName)
	query.Set("insecure", "1")
	uri := url.URL{Scheme: "hysteria2", User: url.UserPassword(peer.Spec.Name, peer.Secret), Host: endpoint, RawQuery: query.Encode(), Fragment: peer.Spec.Name}
	return uri.String(), nil
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
