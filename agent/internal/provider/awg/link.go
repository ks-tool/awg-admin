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
	"strconv"
	"strings"
	"time"

	"github.com/ks-tool/vpn-admin/agent/models"
	"github.com/ks-tool/vpn-admin/agent/storage"
)

type awgLinkOffer struct {
	PublicKey string        `json:"publicKey"`
	Interface InterfaceSpec `json:"interface"`
}

func (p *Provider) PrepareLink(_ context.Context, link models.Link, endpoint models.LinkEndpoint) (models.LinkOffer, error) {
	if link.Kind != "awg" {
		return models.LinkOffer{}, errors.New("unsupported link kind")
	}
	_, spec, err := p.peerInterface(endpoint.ResourceID)
	if err != nil {
		return models.LinkOffer{}, err
	}
	iface, err := p.store.Get(spec.Interface)
	if err != nil {
		return models.LinkOffer{}, err
	}
	data, err := json.Marshal(awgLinkOffer{PublicKey: publicKeyString(iface.PrivateKey), Interface: spec})
	if err != nil {
		return models.LinkOffer{}, err
	}
	return models.LinkOffer{LinkID: link.ID, Endpoint: endpoint, Data: data}, nil
}

func (p *Provider) CommitLink(_ context.Context, link models.Link, endpoint models.LinkEndpoint, remote models.LinkOffer) error {
	if link.Kind != "awg" || remote.LinkID != link.ID {
		return errors.New("link or offer does not match AWG provider")
	}
	_, spec, err := p.peerInterface(endpoint.ResourceID)
	if err != nil {
		return err
	}
	var remoteSpec awgLinkOffer
	if err := json.Unmarshal(remote.Data, &remoteSpec); err != nil {
		return err
	}
	remotePublicKey, err := models.ParseKey(remoteSpec.PublicKey)
	if err != nil {
		return err
	}
	role, err := localLinkRole(link, endpoint)
	if err != nil {
		return err
	}
	old, err := p.store.Get(spec.Interface)
	if err != nil {
		return err
	}
	cfg := *old
	cfg.Peers = append([]models.InterfacePeer(nil), old.Peers...)
	cfg.PreUp = append([]string(nil), old.PreUp...)
	cfg.PostUp = append([]string(nil), old.PostUp...)
	cfg.PreDown = append([]string(nil), old.PreDown...)
	cfg.PostDown = append([]string(nil), old.PostDown...)
	if previous, stateErr := p.links.get(link.ID); stateErr == nil {
		removeLinkArtifacts(&cfg, previous)
	} else if !storage.IsNotFound(stateErr) {
		return stateErr
	} else {
		removeLegacyLinkArtifacts(&cfg, remoteSpec.PublicKey)
	}

	sum := sha256.Sum256([]byte(link.ID))
	table := 100 + int(sum[0])%20000
	state := linkState{Interface: spec.Interface, Role: role, PeerPublicKey: remoteSpec.PublicKey, Table: table, Priority: table}
	peer := models.InterfacePeer{Key: remotePublicKey, Endpoint: remoteEndpoint(remote), KeepaliveInterval: 25 * time.Second}
	if role == "source" {
		peer.AllowedIPs = []string{"0.0.0.0/0"}
		cfg.Table = table
		state.PreUp = []string{
			"sysctl -w net.ipv4.ip_forward=1",
			fmt.Sprintf("ip rule del iif %%i table %d priority %d 2>/dev/null; ip rule add iif %%i table %d priority %d", table, table, table, table),
			fmt.Sprintf("ip route replace default dev %%i table %d", table),
		}
		state.PostDown = []string{
			fmt.Sprintf("ip route del default dev %%i table %d 2>/dev/null || true", table),
			fmt.Sprintf("ip rule del iif %%i table %d priority %d 2>/dev/null || true", table, table),
		}
	} else {
		_, subnet, err := net.ParseCIDR(remoteSpec.Interface.Address)
		if err != nil {
			return fmt.Errorf("remote interface address: %w", err)
		}
		peer.AllowedIPs = []string{subnet.String()}
		cfg.Table = 0
		copyLinkObfuscation(&cfg, remoteSpec.Interface.config())
		state.Mark = "0x" + hex.EncodeToString(sum[:4])
		state.PreUp = []string{
			"sysctl -w net.ipv4.ip_forward=1",
			fmt.Sprintf("iptables -t mangle -C PREROUTING -i %%i -j MARK --set-mark %s 2>/dev/null || iptables -t mangle -A PREROUTING -i %%i -j MARK --set-mark %s", state.Mark, state.Mark),
			"iptables -t nat -C POSTROUTING ! -o %i -m mark --mark " + state.Mark + " 2>/dev/null || iptables -t nat -A POSTROUTING ! -o %i -m mark --mark " + state.Mark + " -j MASQUERADE",
		}
		state.PostDown = []string{
			"iptables -t mangle -D PREROUTING -i %i -j MARK --set-mark " + state.Mark + " 2>/dev/null || true",
			"iptables -t nat -D POSTROUTING ! -o %i -m mark --mark " + state.Mark + " -j MASQUERADE 2>/dev/null || true",
		}
	}
	cfg.PreUp = append(cfg.PreUp, state.PreUp...)
	cfg.PostDown = append(cfg.PostDown, state.PostDown...)
	replaced := false
	for i := range cfg.Peers {
		if cfg.Peers[i].Key == remotePublicKey {
			peer.PresharedKey = cfg.Peers[i].PresharedKey
			cfg.Peers[i] = peer
			replaced = true
			break
		}
	}
	if !replaced {
		cfg.Peers = append(cfg.Peers, peer)
	}
	if err := p.network.One(old, cfg); err != nil {
		return err
	}
	if err := p.store.Set(&cfg); err != nil {
		return err
	}
	return p.links.put(link.ID, state)
}

func (p *Provider) DeleteLink(_ context.Context, link models.Link, endpoint models.LinkEndpoint, remote *models.LinkOffer) error {
	state, err := p.links.get(link.ID)
	legacy := false
	if err != nil {
		if !storage.IsNotFound(err) {
			return err
		}
		if remote == nil {
			return nil
		}
		_, spec, interfaceErr := p.peerInterface(endpoint.ResourceID)
		if interfaceErr != nil {
			return interfaceErr
		}
		var remoteSpec awgLinkOffer
		if err := json.Unmarshal(remote.Data, &remoteSpec); err != nil {
			return err
		}
		state = linkState{Interface: spec.Interface, PeerPublicKey: remoteSpec.PublicKey}
		legacy = true
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
	cfg.PostDown = append([]string(nil), old.PostDown...)
	if legacy {
		removeLegacyLinkArtifacts(&cfg, state.PeerPublicKey)
	} else {
		removeLinkArtifacts(&cfg, state)
	}
	if err := p.network.One(old, cfg); err != nil {
		return err
	}
	if err := p.store.Set(&cfg); err != nil {
		return err
	}
	return p.links.delete(link.ID)
}

func localLinkRole(link models.Link, endpoint models.LinkEndpoint) (string, error) {
	if endpoint.ServerID == link.Source.ServerID && endpoint.ResourceID == link.Source.ResourceID {
		return "source", nil
	}
	if endpoint.ServerID == link.Target.ServerID && endpoint.ResourceID == link.Target.ResourceID {
		return "target", nil
	}
	return "", errors.New("endpoint does not belong to link")
}

func remoteEndpoint(remote models.LinkOffer) string {
	var offer awgLinkOffer
	if err := json.Unmarshal(remote.Data, &offer); err != nil || remote.Endpoint.Address == "" || offer.Interface.ListenPort == 0 {
		return ""
	}
	return net.JoinHostPort(remote.Endpoint.Address, strconv.Itoa(int(offer.Interface.ListenPort)))
}

func removeLinkArtifacts(cfg *models.InterfaceConfig, state linkState) {
	peers := make([]models.InterfacePeer, 0, len(cfg.Peers))
	for _, peer := range cfg.Peers {
		if peer.Key.String() != state.PeerPublicKey {
			peers = append(peers, peer)
		}
	}
	cfg.Peers = peers
	cfg.PreUp = removeCommands(cfg.PreUp, state.PreUp)
	cfg.PostDown = removeCommands(cfg.PostDown, state.PostDown)
	if cfg.Table == state.Table {
		cfg.Table = 0
	}
}

func removeLegacyLinkArtifacts(cfg *models.InterfaceConfig, remotePublicKey string) {
	peers := make([]models.InterfacePeer, 0, len(cfg.Peers))
	for _, peer := range cfg.Peers {
		if peer.Key.String() != remotePublicKey {
			peers = append(peers, peer)
		}
	}
	cfg.Peers = peers
	cfg.PreUp = removeLegacyCommands(cfg.PreUp)
	cfg.PostUp = removeLegacyCommands(cfg.PostUp)
	cfg.PreDown = removeLegacyCommands(cfg.PreDown)
	cfg.PostDown = removeLegacyCommands(cfg.PostDown)
	cfg.Table = 0
}

func removeLegacyCommands(commands []string) []string {
	remaining := make([]string, 0, len(commands))
	for _, command := range commands {
		generated := command == "sysctl -w net.ipv4.ip_forward=1" ||
			strings.HasPrefix(command, "ip rule del iif %i") ||
			strings.HasPrefix(command, "ip route replace default dev %i") ||
			strings.HasPrefix(command, "ip route del default dev %i") ||
			(strings.HasPrefix(command, "iptables -t mangle") && strings.Contains(command, "MARK")) ||
			(strings.HasPrefix(command, "iptables -t nat") && strings.Contains(command, "MASQUERADE"))
		if !generated {
			remaining = append(remaining, command)
		}
	}
	return remaining
}

func removeCommands(current, remove []string) []string {
	remaining := append([]string(nil), current...)
	for _, command := range remove {
		for i := range remaining {
			if remaining[i] == command {
				remaining = append(remaining[:i], remaining[i+1:]...)
				break
			}
		}
	}
	return remaining
}

func copyLinkObfuscation(destination *models.InterfaceConfig, source models.InterfaceConfig) {
	destination.Jc, destination.Jmin, destination.Jmax = source.Jc, source.Jmin, source.Jmax
	destination.S1, destination.S2, destination.S3, destination.S4 = source.S1, source.S2, source.S3, source.S4
	destination.H1, destination.H2, destination.H3, destination.H4 = source.H1, source.H2, source.H3, source.H4
	destination.I1, destination.I2, destination.I3, destination.I4, destination.I5 = source.I1, source.I2, source.I3, source.I4, source.I5
}
