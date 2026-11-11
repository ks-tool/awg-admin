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
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ks-tool/vpn-agent/providers/proxy"
)

func TestHysteriaConfigAndClientURI(t *testing.T) {
	spec := json.RawMessage(`{"name":"public","port":443,"serverName":"vpn.example.com"}`)
	peer := proxy.Peer{Spec: proxy.PeerSpec{Name: "alice"}, Secret: "test-password-123"}
	state := t.TempDir()
	config, err := (Adapter{}).RenderServer(spec, []proxy.Peer{peer}, nil, state)
	if err != nil {
		t.Fatal(err)
	}
	var document struct {
		Listen string `json:"listen"`
		Auth   struct {
			Type     string            `json:"type"`
			Userpass map[string]string `json:"userpass"`
		} `json:"auth"`
	}
	if err := json.Unmarshal(config.Data, &document); err != nil {
		t.Fatal(err)
	}
	if document.Listen != "[::]:443" || document.Auth.Type != "userpass" || document.Auth.Userpass["alice"] != peer.Secret {
		t.Fatalf("unexpected Hysteria config: %s", config.Data)
	}
	if _, err := os.Stat(filepath.Join(state, "tls.crt")); err != nil {
		t.Fatal(err)
	}
	uri, err := (Adapter{}).RenderClient(spec, peer, "192.0.2.1:443")
	if err != nil || !strings.HasPrefix(uri, "hysteria2://alice:test-password-123@") || !strings.Contains(uri, "sni=vpn.example.com") {
		t.Fatalf("unexpected client URI %q: %v", uri, err)
	}
	parsed, err := url.Parse(uri)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Query().Has("insecure") {
		t.Fatalf("TLS verification was disabled by default: %q", uri)
	}
	insecureSpec := json.RawMessage(`{"name":"public","port":443,"serverName":"vpn.example.com","insecure":true}`)
	uri, err = (Adapter{}).RenderClient(insecureSpec, peer, "192.0.2.1:443")
	if err != nil {
		t.Fatal(err)
	}
	parsed, _ = url.Parse(uri)
	if parsed.Query().Get("insecure") != "1" {
		t.Fatalf("explicit insecure mode was not applied: %q", uri)
	}
}
