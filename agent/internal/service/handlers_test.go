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
	"reflect"
	"testing"

	"github.com/ks-tool/awg-admin/agent/models"

	"github.com/Jipok/wgctrl-go/wgtypes"
)

func TestOrphanInterfacesNoMismatch(t *testing.T) {
	configs := []models.InterfaceConfig{{Interface: "wg0"}, {Interface: "wg1"}}
	devices := []*wgtypes.Device{{Name: "wg0"}, {Name: "wg1"}}

	got := orphanInterfaces(configs, devices)
	if len(got) != 0 {
		t.Fatalf("orphanInterfaces() = %v, want none", got)
	}
}

func TestOrphanInterfacesFindsUnknownDevice(t *testing.T) {
	configs := []models.InterfaceConfig{{Interface: "wg0"}}
	devices := []*wgtypes.Device{{Name: "wg0"}, {Name: "wg-leftover"}}

	got := orphanInterfaces(configs, devices)
	want := []string{"wg-leftover"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("orphanInterfaces() = %v, want %v", got, want)
	}
}

func TestOrphanInterfacesNoDevices(t *testing.T) {
	configs := []models.InterfaceConfig{{Interface: "wg0"}}
	got := orphanInterfaces(configs, nil)
	if len(got) != 0 {
		t.Fatalf("orphanInterfaces() = %v, want none", got)
	}
}

// A config with no matching live device (e.g. the agent's storage still
// has a record InterfaceCreate hasn't been retried for yet) is not this
// function's concern — it only reports devices the host has that storage
// doesn't, not the reverse.
func TestOrphanInterfacesIgnoresMissingDevice(t *testing.T) {
	configs := []models.InterfaceConfig{{Interface: "wg0"}, {Interface: "wg1"}}
	devices := []*wgtypes.Device{{Name: "wg0"}}

	got := orphanInterfaces(configs, devices)
	if len(got) != 0 {
		t.Fatalf("orphanInterfaces() = %v, want none", got)
	}
}
