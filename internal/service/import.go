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
	"errors"
	"fmt"
	"net"
	"strings"

	agentmodels "github.com/ks-tool/awg-admin/agent/models"
	"github.com/ks-tool/awg-admin/internal/agentclient"
	"github.com/ks-tool/awg-admin/internal/wgquick"
	"github.com/ks-tool/awg-admin/models"
	"github.com/ks-tool/awg-admin/storage"

	"github.com/google/uuid"
)

// importPlan is everything PreviewImport reports and ImportInterfaceFromServer
// writes, computed once by planImport so the two can't diverge.
type importPlan struct {
	cfg     agentmodels.InterfaceConfig
	peers   []storage.ImportPeer
	preview *models.ImportPreview
}

// PreviewImport dry-runs importing the wg-quick/awg-quick interface ifaceName
// from serverID: it fetches the conf the server has on disk through the agent,
// parses and translates it (see internal/wgquick), applies the user→peer map,
// runs every validation the real import would, and reports what would be
// created plus the warnings — without writing anything. peerMap is the map
// text (see wgquick.PeerMap); it may be empty, in which case every peer is
// imported without a user. Validation problems come back as a ValidationError.
func (s *Service) PreviewImport(serverID, ifaceName, peerMap string) (*models.ImportPreview, error) {
	debugOp("PreviewImport").Str("server_id", serverID).Str("interface", ifaceName).Msg("previewing wg-quick import")
	plan, err := s.planImport(serverID, ifaceName, peerMap)
	if err != nil {
		return nil, err
	}
	return plan.preview, nil
}

// ImportInterfaceFromServer adopts the wg-quick/awg-quick interface ifaceName
// on serverID into awg-admin: the same fetch/parse/validate as PreviewImport,
// then ONE storage transaction (storage.Interfaces.Import) creating the
// interface with its full peer list, the users named in peerMap that don't
// exist yet, and their peer records — followed by one best-effort push of the
// complete config to the agent, exactly like every other mutation (a failed
// push is recorded on the interface's sync status and retried by Sync, never
// rolled back: the interface lives on the server regardless, and forgetting it
// again is the problem this feature exists to fix).
//
// On the agent the push goes down the update path when the link is live
// (adoption in place: the link keeps its type, no recreate, and — the conf
// being the source of truth — the device config is applied with ReplacePeers,
// so anything on the live device that isn't in the conf is removed), or the
// create path when the interface was taken down first.
func (s *Service) ImportInterfaceFromServer(serverID, ifaceName, peerMap string) (*models.Interface, error) {
	debugOp("ImportInterfaceFromServer").Str("server_id", serverID).Str("interface", ifaceName).Msg("importing wg-quick interface")
	plan, err := s.planImport(serverID, ifaceName, peerMap)
	if err != nil {
		return nil, err
	}
	sID, _ := uuid.Parse(serverID) // validated by planImport

	iface := &models.Interface{ID: uuid.New(), InterfaceConfig: plan.cfg}
	if err := s.store.Servers().Interfaces(sID).Import(iface, plan.peers); err != nil {
		return nil, err
	}
	s.pushInterface(sID, iface)
	return iface, nil
}

// planImport does the shared work of PreviewImport and ImportInterfaceFromServer.
func (s *Service) planImport(serverID, ifaceName, peerMap string) (*importPlan, error) {
	sID, err := uuid.Parse(serverID)
	if err != nil {
		return nil, err
	}
	srv, err := s.store.Servers().Get(sID)
	if err != nil {
		return nil, err
	}
	ifaceName = strings.TrimSpace(ifaceName)
	if ifaceName == "" {
		return nil, invalidInput("interface name is required")
	}

	// The map is local input: fail on it before any agent round-trip.
	pm, err := wgquick.ParsePeerMap(peerMap)
	if err != nil {
		return nil, invalidInput("peer map: %v", err)
	}

	// One tunnel session for the three reads. The conf is required; the
	// unmanaged list and host info only feed warnings, so their failure (e.g. an
	// older agent) degrades to "unknown" instead of blocking.
	var (
		src       *models.ImportSource
		unmanaged []models.UnmanagedInterface
		hostInfo  *agentmodels.HostInfo
	)
	err = s.callAgent(srv, func(ctx context.Context, c *agentclient.Client) error {
		var sErr error
		if src, sErr = c.ImportSource(ctx, ifaceName); sErr != nil {
			return sErr
		}
		unmanaged, _ = c.ListUnmanaged(ctx)
		hostInfo, _ = c.Info(ctx)
		return nil
	})
	if err != nil {
		var notFound *agentclient.NotFoundError
		if errors.As(err, &notFound) {
			return nil, invalidInput("no wg-quick config for %q on the server (looked for /etc/amnezia/amneziawg/%s.conf and /etc/wireguard/%s.conf)", ifaceName, ifaceName, ifaceName)
		}
		return nil, fmt.Errorf("read %s config from agent: %w", ifaceName, err)
	}

	conf, err := wgquick.ParseConf(src.Content)
	if err != nil {
		return nil, invalidInput("%s: %v", src.Path, err)
	}
	tr, err := wgquick.Translate(ifaceName, conf)
	if err != nil {
		return nil, invalidInput("%s: %v", src.Path, err)
	}
	cfg := tr.Config

	if err := validateInterfaceConfig(cfg); err != nil {
		return nil, err
	}
	if err := s.validateInterfaceUnique(sID, cfg, uuid.Nil, nil); err != nil {
		return nil, err
	}
	if err := validateImportedPeerIPs(cfg); err != nil {
		return nil, err
	}

	// Match the map's private keys to the conf's peers via the derived public
	// key; whatever the map doesn't cover stays an embedded (user-less) peer.
	byPub := make(map[agentmodels.Key]int, len(cfg.Peers))
	for i, p := range cfg.Peers {
		byPub[p.Key] = i
	}
	existingUsers, err := s.store.Users().List()
	if err != nil {
		return nil, err
	}
	usersByName := map[string][]models.User{}
	peerOwner := map[agentmodels.Key]string{} // pubkey → user name, for "already registered"
	for _, u := range existingUsers {
		usersByName[u.Name] = append(usersByName[u.Name], u)
		for _, p := range u.Peers {
			peerOwner[p.PrivateKey.PublicKey()] = u.Name
		}
	}
	plan := &importPlan{cfg: cfg}
	preview := &models.ImportPreview{
		Interface:      ifaceName,
		Source:         src.Path,
		Address:        cfg.Address,
		ListenPort:     cfg.ListenPort,
		Amnezia:        cfg.IsAmnezia(),
		MTU:            cfg.MTU,
		DNS:            cfg.DNS,
		Table:          cfg.Table,
		Users:          []models.ImportPreviewUser{},
		EmbeddedPeers:  []models.ImportPreviewPeer{},
		Hooks:          models.ImportPreviewHooks{PreUp: conf.Interface.PreUp, PostUp: conf.Interface.PostUp, PreDown: conf.Interface.PreDown, PostDown: conf.Interface.PostDown},
		GeneratedHooks: models.ImportPreviewHooks{PostUp: tr.GeneratedPostUp, PreDown: tr.GeneratedPreDown},
		Warnings:       append([]string{}, tr.Warnings...),
	}
	claimed := make(map[agentmodels.Key]bool, len(cfg.Peers))
	for _, mu := range pm.Users {
		if len(usersByName[mu.Name]) > 1 {
			return nil, invalidInput("user name %q matches %d users in the database; rename one before importing", mu.Name, len(usersByName[mu.Name]))
		}
		pu := models.ImportPreviewUser{Name: mu.Name, Exists: len(usersByName[mu.Name]) == 1, Peers: []models.ImportPreviewPeer{}}
		for _, mp := range mu.Peers {
			pub := mp.PrivateKey.PublicKey()
			idx, ok := byPub[pub]
			if !ok {
				return nil, invalidInput("peer %q (user %q): its private key matches no [Peer] in %s (derived public key %s)", mp.Name, mu.Name, src.Path, pub.String())
			}
			if claimed[pub] {
				return nil, invalidInput("peer %q (user %q): [Peer] %s is claimed twice", mp.Name, mu.Name, shortKey(pub))
			}
			if owner, dup := peerOwner[pub]; dup {
				return nil, invalidInput("peer %q (user %q): a peer with this key already exists in the database (user %q)", mp.Name, mu.Name, owner)
			}
			claimed[pub] = true
			plan.peers = append(plan.peers, storage.ImportPeer{
				UserName: mu.Name,
				Peer:     models.Peer{Name: mp.Name, PrivateKey: mp.PrivateKey},
			})
			pu.Peers = append(pu.Peers, previewPeer(mp.Name, cfg.Peers[idx]))
		}
		preview.Users = append(preview.Users, pu)
	}
	var embeddedDesc []string
	for _, p := range cfg.Peers {
		if claimed[p.Key] {
			continue
		}
		preview.EmbeddedPeers = append(preview.EmbeddedPeers, previewPeer("", p))
		embeddedDesc = append(embeddedDesc, fmt.Sprintf("%s (%s)", shortKey(p.Key), strings.Join(p.AllowedIPs, ", ")))
	}
	if len(embeddedDesc) > 0 {
		preview.Warnings = append(preview.Warnings, fmt.Sprintf("%d peer(s) aren't in the peer map and will be imported without a user — they keep working, but no client config/QR can be rendered for them until re-issued: %s", len(embeddedDesc), strings.Join(embeddedDesc, "; ")))
	}

	// How the push will land: adoption of a live link vs. creation, and whether
	// the conf's flavour matches the link's.
	for _, u := range unmanaged {
		if u.Name == ifaceName {
			preview.Live = true
			preview.LiveKind = u.Kind
		}
	}
	switch {
	case unmanaged == nil:
		preview.Warnings = append(preview.Warnings, "could not ask the agent whether the interface is currently up (older agent?); the import adopts a live link in place or creates it if it's down")
	case !preview.Live:
		preview.Warnings = append(preview.Warnings, fmt.Sprintf("no live interface named %q on the server right now: the agent will create it from the imported config (fine after `wg-quick down`)", ifaceName))
	case preview.LiveKind == models.InterfaceKindWireGuard && cfg.IsAmnezia():
		preview.Warnings = append(preview.Warnings, fmt.Sprintf("the live %q is a plain WireGuard link but the config carries AmneziaWG parameters; applying them to it may be rejected — consider `wg-quick down %s` before importing so the agent recreates it as an AmneziaWG link", ifaceName, ifaceName))
	case preview.LiveKind == models.InterfaceKindAmnezia && !cfg.IsAmnezia():
		preview.Warnings = append(preview.Warnings, fmt.Sprintf("the live %q is an AmneziaWG link but the config has no AmneziaWG parameters; it will be adopted as-is and keep its link type", ifaceName))
	}
	if hostInfo != nil && hostInfo.Backend == "userspace" && preview.Live {
		preview.Warnings = append(preview.Warnings, fmt.Sprintf("the agent on this server is the userspace build, which can't adopt a kernel link it didn't create: the first push will fail while %q is up — bring it down (`wg-quick down %s`) and press Sync, and the agent will bring it up as its own userspace device", ifaceName, ifaceName))
	}

	plan.preview = preview
	return plan, nil
}

// previewPeer projects an InterfacePeer onto the preview shape (public key only).
func previewPeer(name string, p agentmodels.InterfacePeer) models.ImportPreviewPeer {
	return models.ImportPreviewPeer{
		Name:         name,
		PublicKey:    p.Key.String(),
		AllowedIPs:   p.AllowedIPs,
		Endpoint:     p.Endpoint,
		Keepalive:    int(p.KeepaliveInterval.Seconds()),
		PresharedKey: p.PresharedKey != nil,
	}
}

// validateImportedPeerIPs applies validatePeerAllowedIPs' rules to a config
// that isn't stored yet: every peer's host address must lie inside the
// interface subnet and be unique across the peers and the interface's own
// address; routed CIDRs (a LAN behind a peer) pass. Without this an imported
// interface could carry a collision the normal AddPeer path never allows.
func validateImportedPeerIPs(cfg agentmodels.InterfaceConfig) error {
	ifaceIP, network, err := net.ParseCIDR(cfg.Address)
	if err != nil {
		return invalidInput("interface address %q must be a CIDR like 10.0.0.1/24", cfg.Address)
	}
	used := map[string]string{ifaceIP.String(): "the interface itself"}
	for _, p := range cfg.Peers {
		for _, raw := range p.AllowedIPs {
			ip, isHost := parseAllowedIP(raw)
			if ip == nil {
				return invalidInput("peer %s: allowed IP %q is not a valid IP address or CIDR", shortKey(p.Key), raw)
			}
			if !isHost {
				continue
			}
			if !network.Contains(ip) {
				return invalidInput("peer %s: allowed IP %s is not in the interface subnet %s", shortKey(p.Key), ip, network)
			}
			if owner, taken := used[ip.String()]; taken {
				return invalidInput("peer %s: allowed IP %s is already used by %s", shortKey(p.Key), ip, owner)
			}
			used[ip.String()] = "peer " + shortKey(p.Key)
		}
	}
	return nil
}

// shortKey abbreviates a key for messages the way the UI does.
func shortKey(k agentmodels.Key) string {
	s := k.String()
	if len(s) > 8 {
		return s[:8] + "…"
	}
	return s
}
