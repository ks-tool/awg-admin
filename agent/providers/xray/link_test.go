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
	"testing"

	"github.com/ks-tool/vpn-agent/models"
)

func TestPrepareLinkBuildsSOCKSAndTUNConnector(t *testing.T) {
	spec := json.RawMessage(`{"name":"public","port":443,"network":"tcp","tls":false}`)
	link := models.Link{ID: "link-a", Kind: "xray"}
	endpoint := models.LinkEndpoint{Address: "192.0.2.1"}
	secret := "123e4567-e89b-12d3-a456-426614174000"
	_, driver, err := (Adapter{}).PrepareLink(spec, link, endpoint, secret)
	if err != nil {
		t.Fatal(err)
	}
	if len(driver.SOCKS5) == 0 || len(driver.TUN) == 0 {
		t.Fatalf("incomplete driver: %+v", driver)
	}
	var document struct {
		Inbounds []struct {
			Protocol string `json:"protocol"`
		} `json:"inbounds"`
		Outbounds []struct {
			Settings struct {
				Next []struct {
					Address string `json:"address"`
					Users   []struct {
						ID string `json:"id"`
					} `json:"users"`
				} `json:"vnext"`
			} `json:"settings"`
		} `json:"outbounds"`
	}
	if err := json.Unmarshal(driver.Config, &document); err != nil {
		t.Fatal(err)
	}
	if len(document.Inbounds) != 2 || document.Inbounds[0].Protocol != "socks" || document.Inbounds[1].Protocol != "tun" {
		t.Fatalf("unexpected inbounds: %+v", document.Inbounds)
	}
	next := document.Outbounds[0].Settings.Next[0]
	if next.Address != endpoint.Address || next.Users[0].ID != secret {
		t.Fatalf("unexpected outbound: %+v", next)
	}
}
