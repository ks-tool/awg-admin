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

package awg

import "github.com/ks-tool/vpn-admin/agent/models"

const APIVersion = "awg/v1alpha1"

type InterfaceSpec struct {
	Interface    string   `json:"iface" jsonschema:"required,minLength=1,maxLength=15"`
	ListenPort   uint16   `json:"listen" jsonschema:"required,minimum=1"`
	Address      string   `json:"addr" jsonschema:"required"`
	MTU          int      `json:"mtu,omitempty" jsonschema:"minimum=576,maximum=65535"`
	DNS          []string `json:"dns,omitempty"`
	Disabled     bool     `json:"disabled,omitempty"`
	Table        int      `json:"table,omitempty"`
	FirewallMark *int     `json:"fwMark,omitempty"`

	Jc   *int `json:"jc,omitempty" jsonschema:"minimum=0,maximum=128"`
	Jmin *int `json:"jmin,omitempty" jsonschema:"minimum=0,maximum=65535"`
	Jmax *int `json:"jmax,omitempty" jsonschema:"minimum=0,maximum=65535"`

	S1 *int `json:"s1,omitempty" jsonschema:"minimum=0,maximum=65535"`
	S2 *int `json:"s2,omitempty" jsonschema:"minimum=0,maximum=65535"`
	S3 *int `json:"s3,omitempty" jsonschema:"minimum=0,maximum=65535"`
	S4 *int `json:"s4,omitempty" jsonschema:"minimum=0,maximum=65535"`

	H1 *string `json:"h1,omitempty"`
	H2 *string `json:"h2,omitempty"`
	H3 *string `json:"h3,omitempty"`
	H4 *string `json:"h4,omitempty"`

	I1 *string `json:"i1,omitempty"`
	I2 *string `json:"i2,omitempty"`
	I3 *string `json:"i3,omitempty"`
	I4 *string `json:"i4,omitempty"`
	I5 *string `json:"i5,omitempty"`
}

func (s InterfaceSpec) config() models.InterfaceConfig {
	return models.InterfaceConfig{
		Interface: s.Interface, ListenPort: s.ListenPort, Address: s.Address,
		MTU: s.MTU, DNS: s.DNS, Disabled: s.Disabled, Table: s.Table, FirewallMark: s.FirewallMark,
		Jc: s.Jc, Jmin: s.Jmin, Jmax: s.Jmax,
		S1: s.S1, S2: s.S2, S3: s.S3, S4: s.S4,
		H1: s.H1, H2: s.H2, H3: s.H3, H4: s.H4,
		I1: s.I1, I2: s.I2, I3: s.I3, I4: s.I4, I5: s.I5,
	}
}
