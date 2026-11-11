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

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ks-tool/vpn-agent/internal/resource"
	"github.com/ks-tool/vpn-agent/models"
	"github.com/ks-tool/vpn-agent/storage/fs"
)

type linkSide struct {
	provider *Provider
	store    *fs.Dir
	network  *fakeNetwork
	endpoint models.LinkEndpoint
	key      models.Key
}

func TestDistributedLinkPrepareCommitRetryAndDelete(t *testing.T) {
	source := newLinkSide(t, "source-server", "source-resource", "192.0.2.1", true)
	target := newLinkSide(t, "target-server", "target-resource", "192.0.2.2", false)
	link := models.Link{ID: "link-id", Kind: "awg", Generation: 1, Source: source.endpoint, Target: target.endpoint}
	sourceOffer, err := source.provider.PrepareLink(context.Background(), link, source.endpoint)
	if err != nil {
		t.Fatal(err)
	}
	targetOffer, err := target.provider.PrepareLink(context.Background(), link, target.endpoint)
	if err != nil {
		t.Fatal(err)
	}
	if sourceOffer.Endpoint.Address != "192.0.2.1" || targetOffer.Endpoint.Address != "192.0.2.2" {
		t.Fatalf("endpoint addresses were not preserved: %+v %+v", sourceOffer, targetOffer)
	}
	if err := source.provider.CommitLink(context.Background(), link, source.endpoint, targetOffer); err != nil {
		t.Fatal(err)
	}
	if err := target.provider.CommitLink(context.Background(), link, target.endpoint, sourceOffer); err != nil {
		t.Fatal(err)
	}

	sourceConfig, _ := source.store.Get("awg0")
	targetConfig, _ := target.store.Get("awg0")
	if sourceConfig.Table == 0 || targetConfig.Table != 0 {
		t.Fatalf("link roles were not applied: source table=%d target table=%d", sourceConfig.Table, targetConfig.Table)
	}
	if len(sourceConfig.Peers) != 2 || len(targetConfig.Peers) != 1 {
		t.Fatalf("unexpected peers after commit: source=%d target=%d", len(sourceConfig.Peers), len(targetConfig.Peers))
	}
	if sourceConfig.Peers[1].Endpoint != "192.0.2.2:51820" || targetConfig.Peers[0].Endpoint != "192.0.2.1:51820" {
		t.Fatalf("unexpected endpoints: %q %q", sourceConfig.Peers[1].Endpoint, targetConfig.Peers[0].Endpoint)
	}
	if *targetConfig.Jc != *sourceConfig.Jc {
		t.Fatal("source obfuscation was not applied to target")
	}
	if sourceConfig.Peers[0].AllowedIPs[0] != "10.0.0.10/32" {
		t.Fatal("user peer was not preserved")
	}
	if err := source.provider.CommitLink(context.Background(), link, source.endpoint, targetOffer); err != nil {
		t.Fatal(err)
	}
	if err := target.provider.CommitLink(context.Background(), link, target.endpoint, sourceOffer); err != nil {
		t.Fatal(err)
	}
	sourceConfig, _ = source.store.Get("awg0")
	targetConfig, _ = target.store.Get("awg0")
	if len(sourceConfig.Peers) != 2 || len(targetConfig.Peers) != 1 || len(sourceConfig.PreUp) != 3 || len(targetConfig.PreUp) != 1 {
		t.Fatalf("repeated commit duplicated artifacts: source peers=%d target peers=%d source hooks=%v target hooks=%v", len(sourceConfig.Peers), len(targetConfig.Peers), sourceConfig.PreUp, targetConfig.PreUp)
	}

	if err := source.provider.DeleteLink(context.Background(), link, source.endpoint, nil); err != nil {
		t.Fatal(err)
	}
	if err := target.provider.DeleteLink(context.Background(), link, target.endpoint, nil); err != nil {
		t.Fatal(err)
	}
	if err := source.provider.DeleteLink(context.Background(), link, source.endpoint, nil); err != nil {
		t.Fatalf("idempotent delete: %v", err)
	}
	sourceConfig, _ = source.store.Get("awg0")
	targetConfig, _ = target.store.Get("awg0")
	if len(sourceConfig.Peers) != 1 || sourceConfig.Peers[0].AllowedIPs[0] != "10.0.0.10/32" {
		t.Fatalf("user peer was lost on link delete: %+v", sourceConfig.Peers)
	}
	if len(targetConfig.Peers) != 0 || len(sourceConfig.PreUp) != 0 || len(targetConfig.PreUp) != 0 {
		t.Fatalf("link artifacts survived delete: source=%+v target=%+v", sourceConfig, targetConfig)
	}
}

func newLinkSide(t *testing.T, serverID, resourceID, address string, obfuscation bool) linkSide {
	t.Helper()
	store, err := fs.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	privateKey := mustPeerKey(t)
	userKey := mustPeerKey(t)
	jc := 3
	legacy := models.InterfaceConfig{Interface: "awg0", Address: "10.0.0.1/24", ListenPort: 51820, PrivateKey: privateKey, Peers: []models.InterfacePeer{}}
	if obfuscation {
		legacy.Jc = &jc
		legacy.Peers = append(legacy.Peers, models.InterfacePeer{Key: userKey.PublicKey(), AllowedIPs: []string{"10.0.0.10/32"}})
	}
	if err := store.Set(&legacy); err != nil {
		t.Fatal(err)
	}
	inventory, err := resource.NewFileStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	spec := InterfaceSpec{Interface: "awg0", Address: "10.0.0.1/24", ListenPort: 51820, Jc: legacy.Jc}
	rawSpec, _ := json.Marshal(spec)
	item := models.Resource{ID: resourceID, ServerID: serverID, APIVersion: APIVersion, Kind: "Interface", Name: "office", Generation: 1, Spec: rawSpec}
	if err := inventory.Put(resource.StoredResource{Provider: "awg", Resource: item}); err != nil {
		t.Fatal(err)
	}
	network := &fakeNetwork{}
	provider, err := New(store, filepath.Join(t.TempDir(), "agent", "secrets"), network, "test", inventory)
	if err != nil {
		t.Fatal(err)
	}
	return linkSide{
		provider: provider, store: store, network: network, key: privateKey,
		endpoint: models.LinkEndpoint{ServerID: serverID, ResourceID: resourceID, APIVersion: APIVersion, Kind: "Interface", Name: "office", Port: "tunnel", Address: address},
	}
}

func TestDeleteMigratedLegacyLinkWithoutState(t *testing.T) {
	source := newLinkSide(t, "source-server", "source-resource", "192.0.2.1", true)
	target := newLinkSide(t, "target-server", "target-resource", "192.0.2.2", false)
	link := models.Link{ID: "legacy-link", Kind: "awg", Generation: 1, Source: source.endpoint, Target: target.endpoint}
	legacy, _ := source.store.Get("awg0")
	legacy.Table = 100
	legacy.PreUp = []string{
		"sysctl -w net.ipv4.ip_forward=1",
		"ip rule del iif %i table 100 priority 100 2>/dev/null; ip rule add iif %i table 100 priority 100",
		"ip route replace default dev %i table 100",
	}
	legacy.PostDown = []string{"ip route del default dev %i table 100 2>/dev/null || true"}
	legacy.Peers = append(legacy.Peers, models.InterfacePeer{Key: target.key.PublicKey(), AllowedIPs: []string{"0.0.0.0/0"}})
	if err := source.store.Set(legacy); err != nil {
		t.Fatal(err)
	}
	targetOffer, err := target.provider.PrepareLink(context.Background(), link, target.endpoint)
	if err != nil {
		t.Fatal(err)
	}
	if err := source.provider.DeleteLink(context.Background(), link, source.endpoint, &targetOffer); err != nil {
		t.Fatal(err)
	}
	updated, _ := source.store.Get("awg0")
	if len(updated.Peers) != 1 || updated.Peers[0].AllowedIPs[0] != "10.0.0.10/32" {
		t.Fatalf("legacy delete removed the wrong peers: %+v", updated.Peers)
	}
	if updated.Table != 0 || len(updated.PreUp) != 0 || len(updated.PostDown) != 0 {
		t.Fatalf("legacy artifacts survived delete: %+v", updated)
	}
}

func TestDistributedLinksShareTransitInterface(t *testing.T) {
	a := newLinkSide(t, "a", "a-resource", "192.0.2.1", true)
	b := newLinkSide(t, "b", "b-resource", "192.0.2.2", false)
	c := newLinkSide(t, "c", "c-resource", "192.0.2.3", false)
	ab := models.Link{ID: "ab", Kind: "awg", Generation: 1, Source: a.endpoint, Target: b.endpoint}
	bc := models.Link{ID: "bc", Kind: "awg", Generation: 1, Source: b.endpoint, Target: c.endpoint}

	aOffer, err := a.provider.PrepareLink(context.Background(), ab, a.endpoint)
	if err != nil {
		t.Fatal(err)
	}
	bOfferAB, err := b.provider.PrepareLink(context.Background(), ab, b.endpoint)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.provider.CommitLink(context.Background(), ab, a.endpoint, bOfferAB); err != nil {
		t.Fatal(err)
	}
	if err := b.provider.CommitLink(context.Background(), ab, b.endpoint, aOffer); err != nil {
		t.Fatal(err)
	}

	bOfferBC, err := b.provider.PrepareLink(context.Background(), bc, b.endpoint)
	if err != nil {
		t.Fatal(err)
	}
	cOffer, err := c.provider.PrepareLink(context.Background(), bc, c.endpoint)
	if err != nil {
		t.Fatal(err)
	}
	if err := b.provider.CommitLink(context.Background(), bc, b.endpoint, cOffer); err != nil {
		t.Fatal(err)
	}
	if err := c.provider.CommitLink(context.Background(), bc, c.endpoint, bOfferBC); err != nil {
		t.Fatal(err)
	}

	config, _ := b.store.Get("awg0")
	if len(config.Peers) != 2 || len(config.PreUp) != 4 || len(config.PostDown) != 4 || config.Table == 0 {
		t.Fatalf("transit links did not compose on one interface: %+v", config)
	}
	if len(config.Peers[0].AllowedIPs) != 1 || config.Peers[0].AllowedIPs[0] != "10.0.0.1/32" {
		t.Fatalf("incoming peer is not limited to its immediate neighbor: %+v", config.Peers[0].AllowedIPs)
	}
	if len(config.Peers[1].AllowedIPs) != 2 || config.Peers[1].AllowedIPs[1] != "10.0.0.1/32" {
		t.Fatalf("outgoing peer has no host-specific route: %+v", config.Peers[1].AllowedIPs)
	}
	if config.PreUp[3] != firewallHook("transit", "add", 0, "") {
		t.Fatalf("transit traffic has no SNAT: %+v", config.PreUp)
	}
	configC, _ := c.store.Get("awg0")
	if configC.Jc == nil || config.Jc == nil || *configC.Jc != *config.Jc {
		t.Fatalf("outgoing link did not use live obfuscation: B=%v C=%v", config.Jc, configC.Jc)
	}
	rawSpec, _ := json.Marshal(InterfaceSpec{Interface: "awg0", Address: "10.0.0.1/24", ListenPort: 51820})
	if _, err := b.provider.Reconcile(context.Background(), models.Resource{ID: "b-resource", Kind: "Interface", Spec: rawSpec}); err != nil {
		t.Fatal(err)
	}
	reconciled, _ := b.store.Get("awg0")
	if reconciled.Table != config.Table || reconciled.Jc == nil || config.Jc == nil || *reconciled.Jc != *config.Jc {
		t.Fatalf("reconcile replaced link-owned fields: before=%+v after=%+v", config, reconciled)
	}
	if err := b.provider.CommitLink(context.Background(), bc, b.endpoint, cOffer); err != nil {
		t.Fatal(err)
	}
	config, _ = b.store.Get("awg0")
	if len(config.Peers) != 2 || len(config.PreUp) != 5 || len(config.PostDown) != 5 {
		t.Fatalf("repeated commit duplicated transit artifacts: %+v", config)
	}

	if err := b.provider.DeleteLink(context.Background(), ab, b.endpoint, nil); err != nil {
		t.Fatal(err)
	}
	config, _ = b.store.Get("awg0")
	if len(config.Peers) != 1 || len(config.PreUp) != 4 || len(config.PostDown) != 4 || config.Table == 0 {
		t.Fatalf("deleting incoming link damaged outgoing link: %+v", config)
	}
	if err := b.provider.DeleteLink(context.Background(), bc, b.endpoint, nil); err != nil {
		t.Fatal(err)
	}
	config, _ = b.store.Get("awg0")
	if len(config.Peers) != 0 || len(config.PreUp) != 1 || len(config.PostDown) != 1 || config.Table != 0 {
		t.Fatalf("link artifacts survived final delete: %+v", config)
	}
}

func TestDistributedRouteLinkPreparePreservesCommittedState(t *testing.T) {
	source := newLinkSide(t, "source-server", "source-resource", "192.0.2.1", false)
	target := newLinkSide(t, "target-server", "target-resource", "192.0.2.2", false)
	link := models.Link{ID: "route-link", Kind: models.RouteLinkKind, Generation: 1, Source: source.endpoint, Target: target.endpoint}

	committedSource := linkState{LinkID: link.ID, Interface: "awg0", Role: "source", Table: 1234, Priority: 1234, Driver: &models.RouteDriver{TUN: "tun-test"}}
	if err := source.provider.links.put(link.ID, committedSource); err != nil {
		t.Fatal(err)
	}
	if _, err := source.provider.PrepareLink(context.Background(), link, source.endpoint); err != nil {
		t.Fatal(err)
	}
	sourceState, err := source.provider.links.get(link.ID)
	if err != nil {
		t.Fatal(err)
	}
	if sourceState.Table != committedSource.Table || sourceState.Driver == nil || sourceState.Driver.TUN != committedSource.Driver.TUN {
		t.Fatalf("source prepare discarded committed state: %+v", sourceState)
	}

	firstOffer, err := target.provider.PrepareLink(context.Background(), link, target.endpoint)
	if err != nil {
		t.Fatal(err)
	}
	if err := target.provider.CommitLink(context.Background(), link, target.endpoint, models.LinkOffer{}); err != nil {
		t.Fatal(err)
	}
	committedTarget, err := target.provider.links.get(link.ID)
	if err != nil {
		t.Fatal(err)
	}
	secondOffer, err := target.provider.PrepareLink(context.Background(), link, target.endpoint)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(firstOffer.Data, secondOffer.Data) {
		t.Fatal("target prepare generated new credentials")
	}
	replayedTarget, err := target.provider.links.get(link.ID)
	if err != nil {
		t.Fatal(err)
	}
	if replayedTarget.PeerPublicKey != committedTarget.PeerPublicKey || len(replayedTarget.PreUp) != 1 || len(replayedTarget.PostDown) != 1 {
		t.Fatalf("target prepare discarded committed state: %+v", replayedTarget)
	}
	if err := target.provider.DeleteLink(context.Background(), link, target.endpoint, nil); err != nil {
		t.Fatal(err)
	}
	config, err := target.store.Get("awg0")
	if err != nil {
		t.Fatal(err)
	}
	if len(config.Peers) != 0 || len(config.PreUp) != 0 || len(config.PostDown) != 0 {
		t.Fatalf("route artifacts survived prepare and delete: %+v", config)
	}
}

func TestRouteLinkRejectsSecondOutgoingRouteOnAWGInterface(t *testing.T) {
	source := newLinkSide(t, "source-server", "source-resource", "192.0.2.1", false)
	target := newLinkSide(t, "target-server", "target-resource", "192.0.2.2", false)
	first := models.Link{ID: "route-first", Kind: models.RouteLinkKind, Source: source.endpoint, Target: target.endpoint}
	second := models.Link{ID: "route-second", Kind: models.RouteLinkKind, Source: source.endpoint, Target: target.endpoint}
	if _, err := source.provider.PrepareLink(context.Background(), first, source.endpoint); err != nil {
		t.Fatal(err)
	}
	if _, err := source.provider.PrepareLink(context.Background(), second, source.endpoint); err == nil || !strings.Contains(err.Error(), "outgoing link") {
		t.Fatalf("second outgoing route was accepted: %v", err)
	}
	if _, err := source.provider.PrepareLink(context.Background(), first, source.endpoint); err != nil {
		t.Fatalf("idempotent prepare of the first route failed: %v", err)
	}
}
