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
	"context"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ks-tool/vpn-admin/models"
	agentmodels "github.com/ks-tool/vpn-agent/models"
	"uuid"
)

func TestFirewallSyncCoalescesChangesAndReloadsInterfaces(t *testing.T) {
	svc, srv := newValidationTestServer(t)
	interfaces := svc.store.Servers().Interfaces(srv.ID)
	if err := interfaces.Set(&models.Interface{ID: uuid.New(), InterfaceConfig: agentmodels.InterfaceConfig{
		Interface: "awg0", Address: "10.0.0.1/24",
	}}); err != nil {
		t.Fatal(err)
	}

	started := make(chan []string, 1)
	release := make(chan struct{})
	applied := make(chan []string, 2)
	var calls atomic.Int32
	svc.applyFirewall = func(_ context.Context, _ models.Server, names []string, _, _ string) error {
		if calls.Add(1) == 1 {
			started <- names
			<-release
		}
		applied <- names
		return nil
	}

	svc.scheduleFirewallSync(srv.ID)
	select {
	case names := <-started:
		if !reflect.DeepEqual(names, []string{"awg0"}) {
			t.Fatalf("initial interfaces = %#v", names)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("firewall sync did not start")
	}

	if err := interfaces.Set(&models.Interface{ID: uuid.New(), InterfaceConfig: agentmodels.InterfaceConfig{
		Interface: "awg1", Address: "10.0.1.1/24",
	}}); err != nil {
		t.Fatal(err)
	}
	svc.scheduleFirewallSync(srv.ID)
	svc.scheduleFirewallSync(srv.ID)
	close(release)

	for i, want := range [][]string{{"awg0"}, {"awg0", "awg1"}} {
		select {
		case got := <-applied:
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("apply %d interfaces = %#v, want %#v", i+1, got, want)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("firewall apply %d did not finish", i+1)
		}
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("firewall apply calls = %d, want 2", got)
	}

	deadline := time.Now().Add(5 * time.Second)
	for {
		svc.firewallMu.Lock()
		running := svc.firewallRunning[srv.ID.String()]
		svc.firewallMu.Unlock()
		if !running {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("firewall sync did not stop")
		}
		time.Sleep(10 * time.Millisecond)
	}
	svc.scheduleFirewallSync(srv.ID)
	time.Sleep(100 * time.Millisecond)
	if got := calls.Load(); got != 2 {
		t.Fatalf("unchanged interfaces triggered another apply; calls = %d", got)
	}
}

func TestFirewallSyncAfterDeploySkipsInitialInterfaces(t *testing.T) {
	svc, srv := newValidationTestServer(t)
	if err := svc.store.Servers().Interfaces(srv.ID).Set(&models.Interface{ID: uuid.New(), InterfaceConfig: agentmodels.InterfaceConfig{
		Interface: "awg0", Address: "10.0.0.1/24",
	}}); err != nil {
		t.Fatal(err)
	}
	svc.applyFirewall = func(context.Context, models.Server, []string, string, string) error {
		t.Fatal("deployed firewall rules must not be applied again")
		return nil
	}
	svc.markFirewallApplied(srv.ID)
	svc.scheduleFirewallSync(srv.ID)
}
