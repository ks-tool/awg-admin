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
	"testing"

	"github.com/ks-tool/vpn-agent/models"
)

func TestPrepareLinkBuildsSOCKSAndTUNConnector(t *testing.T) {
	spec := json.RawMessage(`{"name":"public","port":443,"serverName":"localhost"}`)
	link := models.Link{ID: "link-a", Kind: "hysteria2"}
	endpoint := models.LinkEndpoint{Address: "192.0.2.1"}
	_, driver, err := (Adapter{}).PrepareLink(spec, link, endpoint, "secret-password", link.ID)
	if err != nil {
		t.Fatal(err)
	}
	var document struct {
		Server string `json:"server"`
		Auth   string `json:"auth"`
		Socks  struct {
			Listen string `json:"listen"`
		} `json:"socks5"`
		Tun struct {
			Name string `json:"name"`
		} `json:"tun"`
	}
	if err := json.Unmarshal(driver.Config, &document); err != nil {
		t.Fatal(err)
	}
	if document.Server != "192.0.2.1:443" || document.Auth != "link-link-a:secret-password" {
		t.Fatalf("unexpected client config: %+v", document)
	}
	if document.Socks.Listen != driver.SOCKS5 || document.Tun.Name != driver.TUN {
		t.Fatalf("connector mismatch: %+v", driver)
	}
}
