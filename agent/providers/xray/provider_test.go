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
	"encoding/json"
	"net/url"
	"strings"
	"testing"

	"github.com/ks-tool/vpn-agent/providers/proxy"
)

func TestVLESSConfigAndClientURI(t *testing.T) {
	spec := json.RawMessage(`{"name":"public","listen":"0.0.0.0","port":443,"network":"ws","path":"/vpn","tls":false}`)
	peer := proxy.Peer{Spec: proxy.PeerSpec{Name: "alice"}, Secret: "123e4567-e89b-12d3-a456-426614174000"}
	config, err := (Adapter{}).RenderServer(spec, []proxy.Peer{peer}, nil, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	var document struct {
		Inbounds []struct {
			Protocol string `json:"protocol"`
			Settings struct {
				Clients []struct {
					ID string `json:"id"`
				} `json:"clients"`
			} `json:"settings"`
			Stream struct {
				Network string `json:"network"`
				WS      struct {
					Path string `json:"path"`
				} `json:"wsSettings"`
			} `json:"streamSettings"`
		} `json:"inbounds"`
	}
	if err := json.Unmarshal(config.Data, &document); err != nil {
		t.Fatal(err)
	}
	if len(document.Inbounds) != 1 || document.Inbounds[0].Protocol != "vless" || document.Inbounds[0].Settings.Clients[0].ID != peer.Secret {
		t.Fatalf("unexpected Xray config: %s", config.Data)
	}
	if document.Inbounds[0].Stream.Network != "ws" || document.Inbounds[0].Stream.WS.Path != "/vpn" {
		t.Fatalf("unexpected stream config: %s", config.Data)
	}
	uri, err := (Adapter{}).RenderClient(spec, peer, "192.0.2.1:443")
	if err != nil || !strings.HasPrefix(uri, "vless://") || !strings.Contains(uri, peer.Secret) {
		t.Fatalf("unexpected client URI %q: %v", uri, err)
	}
}

func TestClientTLSVerificationRequiresExplicitInsecureOptIn(t *testing.T) {
	peer := proxy.Peer{Spec: proxy.PeerSpec{Name: "alice"}, Secret: "123e4567-e89b-12d3-a456-426614174000"}
	secureSpec := json.RawMessage(`{"name":"public","port":443,"network":"tcp","tls":true,"serverName":"vpn.example.com"}`)
	uri, err := (Adapter{}).RenderClient(secureSpec, peer, "192.0.2.1:443")
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(uri)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Query().Has("allowInsecure") {
		t.Fatalf("TLS verification was disabled by default: %q", uri)
	}
	insecureSpec := json.RawMessage(`{"name":"public","port":443,"network":"tcp","tls":true,"serverName":"vpn.example.com","insecure":true}`)
	uri, err = (Adapter{}).RenderClient(insecureSpec, peer, "192.0.2.1:443")
	if err != nil {
		t.Fatal(err)
	}
	parsed, _ = url.Parse(uri)
	if parsed.Query().Get("allowInsecure") != "1" {
		t.Fatalf("explicit insecure mode was not applied: %q", uri)
	}
}
