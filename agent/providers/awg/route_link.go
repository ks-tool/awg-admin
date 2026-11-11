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
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/ks-tool/vpn-agent/models"
	"github.com/ks-tool/vpn-agent/providers/proxy"
	"github.com/ks-tool/vpn-agent/storage"
)

type routeClientConfig struct {
	PrivateKey models.Key           `json:"privateKey"`
	Address    string               `json:"address"`
	Peer       models.InterfacePeer `json:"peer"`
	Interface  InterfaceSpec        `json:"interface"`
	SOCKS5     string               `json:"socks5"`
}

func (p *Provider) prepareRouteLink(_ context.Context, link models.Link, endpoint models.LinkEndpoint) (models.LinkOffer, error) {
	_, spec, err := p.peerInterface(endpoint.ResourceID)
	if err != nil {
		return models.LinkOffer{}, err
	}
	role, err := localLinkRole(link, endpoint)
	if err != nil {
		return models.LinkOffer{}, err
	}
	offer := models.LinkOffer{LinkID: link.ID, Endpoint: endpoint, Data: json.RawMessage(`{}`)}
	if role == "source" {
		return offer, p.links.put(link.ID, linkState{LinkID: link.ID, Interface: spec.Interface, Role: role})
	}
	iface, err := p.store.Get(spec.Interface)
	if err != nil {
		return models.LinkOffer{}, err
	}
	clientKey, err := models.GeneratePrivateKey()
	if err != nil {
		return models.LinkOffer{}, err
	}
	clientAddress, err := routeClientAddress(*iface, link.ID)
	if err != nil {
		return models.LinkOffer{}, err
	}
	clientPeer := models.InterfacePeer{Key: clientKey.PublicKey(), AllowedIPs: []string{clientAddress}}
	remotePeer := models.InterfacePeer{
		Key: iface.PrivateKey.PublicKey(), Endpoint: routeRemoteEndpoint(endpoint, spec), AllowedIPs: []string{"0.0.0.0/0"}, KeepaliveInterval: 25 * time.Second,
	}
	socksAddress, _, _, _ := proxy.RouteIdentifiers(link.ID)
	clientConfig := routeClientConfig{PrivateKey: clientKey, Address: clientAddress, Peer: remotePeer, Interface: spec, SOCKS5: socksAddress}
	configData, err := json.Marshal(clientConfig)
	if err != nil {
		return models.LinkOffer{}, err
	}
	driver := &models.RouteDriver{
		Binary: "/usr/local/bin/vpn-provider-awg", Args: []string{"route-client", "--config", "{{config}}"}, Config: configData, SOCKS5: socksAddress,
	}
	remoteSpec := awgLinkOffer{PublicKey: publicKeyString(iface.PrivateKey), Interface: spec, Driver: driver, RoutePeer: &clientPeer}
	offer.Data, err = json.Marshal(remoteSpec)
	if err != nil {
		return models.LinkOffer{}, err
	}
	state := linkState{LinkID: link.ID, Interface: spec.Interface, Role: role, PeerPublicKey: clientPeer.Key.String(), Driver: driver}
	return offer, p.links.put(link.ID, state)
}

func (p *Provider) commitRouteLink(_ context.Context, link models.Link, endpoint models.LinkEndpoint, remote models.LinkOffer) error {
	role, err := localLinkRole(link, endpoint)
	if err != nil {
		return err
	}
	if role == "source" {
		var driver models.RouteDriver
		if err := json.Unmarshal(remote.Data, &driver); err != nil {
			return err
		}
		if len(driver.Binary) == 0 {
			var remoteSpec awgLinkOffer
			if err := json.Unmarshal(remote.Data, &remoteSpec); err != nil {
				return err
			}
			if remoteSpec.Driver != nil {
				driver = *remoteSpec.Driver
			}
		}
		if len(driver.Binary) == 0 || len(driver.TUN) == 0 {
			return errors.New("remote offer has no TUN route connector")
		}
		_, spec, err := p.peerInterface(endpoint.ResourceID)
		if err != nil {
			return err
		}
		if previous, stateErr := p.links.get(link.ID); stateErr == nil && previous.Driver != nil {
			_ = p.connectors.Stop(link.ID)
			removeRoutePolicy(previous.Interface, previous.Table)
		}
		if err := p.connectors.Apply(link.ID, driver); err != nil {
			return err
		}
		table := proxy.RouteTable(link.ID)
		if err := applyRoutePolicy(spec.Interface, driver.TUN, table); err != nil {
			_ = p.connectors.Stop(link.ID)
			return err
		}
		return p.links.put(link.ID, linkState{LinkID: link.ID, Interface: spec.Interface, Role: role, Table: table, Priority: table, Driver: &driver})
	}

	state, err := p.links.get(link.ID)
	if err != nil {
		return err
	}
	old, err := p.store.Get(state.Interface)
	if err != nil {
		return err
	}
	cfg := *old
	cfg.Peers = append([]models.InterfacePeer(nil), old.Peers...)
	cfg.PreUp = append([]string(nil), old.PreUp...)
	cfg.PostUp = append([]string(nil), old.PostUp...)
	cfg.PreDown = append([]string(nil), old.PreDown...)
	cfg.PostDown = append([]string(nil), old.PostDown...)
	removeLinkArtifacts(&cfg, state)
	publicKey, err := models.ParseKey(state.PeerPublicKey)
	if err != nil {
		return err
	}
	var clientAddress string
	if state.Driver != nil {
		var clientConfig routeClientConfig
		if err := json.Unmarshal(state.Driver.Config, &clientConfig); err != nil {
			return err
		}
		clientAddress = clientConfig.Address
	}
	if len(clientAddress) == 0 {
		return errors.New("route client address is missing")
	}
	cfg.Peers = append(cfg.Peers, models.InterfacePeer{Key: publicKey, AllowedIPs: []string{clientAddress}})
	sum := sha256.Sum256([]byte(link.ID))
	state.Mark = "0x" + hex.EncodeToString(sum[:4])
	state.PreUp = []string{firewallHook("exit", "add", 0, state.Mark)}
	state.PostDown = []string{firewallHook("exit", "remove", 0, state.Mark)}
	cfg.PreUp = append(cfg.PreUp, state.PreUp...)
	cfg.PostDown = append(cfg.PostDown, state.PostDown...)
	if err := p.network.One(old, cfg); err != nil {
		return err
	}
	if err := p.store.Set(&cfg); err != nil {
		return err
	}
	return p.links.put(link.ID, state)
}

func (p *Provider) deleteRouteLink(_ context.Context, link models.Link, _ models.LinkEndpoint, _ *models.LinkOffer) error {
	state, err := p.links.get(link.ID)
	if err != nil {
		if storage.IsNotFound(err) {
			return nil
		}
		return err
	}
	if state.Role == "source" {
		if err := p.connectors.Stop(link.ID); err != nil {
			return err
		}
		removeRoutePolicy(state.Interface, state.Table)
		return p.links.delete(link.ID)
	}
	old, err := p.store.Get(state.Interface)
	if err != nil {
		if storage.IsNotFound(err) {
			return p.links.delete(link.ID)
		}
		return err
	}
	cfg := *old
	cfg.Peers = append([]models.InterfacePeer(nil), old.Peers...)
	cfg.PreUp = append([]string(nil), old.PreUp...)
	cfg.PostUp = append([]string(nil), old.PostUp...)
	cfg.PreDown = append([]string(nil), old.PreDown...)
	cfg.PostDown = append([]string(nil), old.PostDown...)
	removeLinkArtifacts(&cfg, state)
	if err := p.network.One(old, cfg); err != nil {
		return err
	}
	if err := p.store.Set(&cfg); err != nil {
		return err
	}
	return p.links.delete(link.ID)
}

func (p *Provider) RestoreLinks() error {
	states, err := p.links.list()
	if err != nil {
		return err
	}
	for _, state := range states {
		if state.Role != "source" || state.Driver == nil {
			continue
		}
		if err := p.connectors.Apply(state.LinkID, *state.Driver); err != nil {
			return err
		}
		if len(state.Driver.TUN) > 0 {
			if err := applyRoutePolicy(state.Interface, state.Driver.TUN, state.Table); err != nil {
				return err
			}
		}
	}
	return nil
}

func routeRemoteEndpoint(endpoint models.LinkEndpoint, spec InterfaceSpec) string {
	if len(endpoint.Address) == 0 || spec.ListenPort == 0 {
		return ""
	}
	return net.JoinHostPort(endpoint.Address, strconv.Itoa(int(spec.ListenPort)))
}

func routeClientAddress(config models.InterfaceConfig, linkID string) (string, error) {
	_, network, err := net.ParseCIDR(config.Address)
	if err != nil {
		return "", err
	}
	prefix, err := netip.ParsePrefix(network.String())
	if err != nil {
		return "", errors.New("invalid interface subnet")
	}
	hostIP, _, err := net.ParseCIDR(config.Address)
	if err != nil {
		return "", err
	}
	broadcast := append(net.IP(nil), network.IP.Mask(network.Mask)...)
	for index := range broadcast {
		broadcast[index] |= ^network.Mask[index]
	}
	sum := sha256.Sum256([]byte(linkID))
	address := prefix.Addr()
	for range 200 {
		for range sum[:2] {
			address = address.Next()
		}
		candidate := address.String() + "/32"
		used := false
		for _, peer := range config.Peers {
			for _, allowed := range peer.AllowedIPs {
				if allowed == candidate {
					used = true
				}
			}
		}
		if !used && address.String() != network.IP.String() && address.String() != hostIP.String() && address.String() != broadcast.String() {
			return candidate, nil
		}
	}
	return "", errors.New("no free route client address in interface subnet")
}

func applyRoutePolicy(iface, tun string, table int) error {
	_ = runIP("rule", "del", "iif", iface, "table", strconv.Itoa(table), "priority", strconv.Itoa(table))
	if err := runIP("rule", "add", "iif", iface, "table", strconv.Itoa(table), "priority", strconv.Itoa(table)); err != nil {
		return err
	}
	return runIP("route", "replace", "default", "dev", tun, "table", strconv.Itoa(table))
}

func removeRoutePolicy(iface string, table int) {
	if len(iface) == 0 || table == 0 {
		return
	}
	_ = runIP("route", "del", "default", "table", strconv.Itoa(table))
	_ = runIP("rule", "del", "iif", iface, "table", strconv.Itoa(table), "priority", strconv.Itoa(table))
}

func runIP(arguments ...string) error {
	output, err := exec.Command("ip", arguments...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("ip %s: %w: %s", strings.Join(arguments, " "), err, strings.TrimSpace(string(output)))
	}
	return nil
}
