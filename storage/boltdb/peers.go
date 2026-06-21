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

package boltdb

import (
	"fmt"

	"github.com/google/uuid"
	agentmodels "github.com/ks-tool/awg-admin/agent/models"
	"github.com/ks-tool/awg-admin/models"
	"github.com/ks-tool/awg-admin/storage"
	bolt "go.etcd.io/bbolt"
)

type peers struct {
	db     *bolt.DB
	userID uuid.UUID
}

func (p *peers) List() ([]models.Peer, error) {
	u, err := (&users{db: p.db}).Get(p.userID)
	if err != nil {
		return nil, err
	}
	return u.Peers, nil
}

func (p *peers) Get(publicKey agentmodels.Key) (*models.Peer, error) {
	u, err := (&users{db: p.db}).Get(p.userID)
	if err != nil {
		return nil, err
	}
	for _, peer := range u.Peers {
		if peer.PrivateKey.PublicKey() == publicKey {
			return &peer, nil
		}
	}
	return nil, storage.ErrNotFound
}

func (p *peers) Set(peer *models.Peer) error {
	ifacePeer := agentmodels.InterfacePeer{
		Key: peer.PrivateKey.PublicKey(),
	}
	return p.setWithIfacePeer(peer, ifacePeer)
}

func (p *peers) Delete(key agentmodels.Key) error {
	return p.db.Update(func(tx *bolt.Tx) error {
		ub := tx.Bucket(bktUsers)
		u, err := get[models.User](ub, idKey(p.userID))
		if err != nil {
			return err
		}

		var ifaceID uuid.UUID
		found := false
		newPeers := make([]models.Peer, 0, len(u.Peers))
		for _, peer := range u.Peers {
			// key is the peer's public key (as used by Get/SetWithIfacePeer),
			// not its private key — compare against the derived public key.
			if peer.PrivateKey.PublicKey() == key {
				ifaceID = peer.InterfaceId
				found = true
				continue
			}
			newPeers = append(newPeers, peer)
		}
		if !found {
			return storage.ErrNotFound
		}
		u.Peers = newPeers
		if err = put(ub, idKey(p.userID), u); err != nil {
			return err
		}

		return removePeerFromInterface(tx.Bucket(bktInterfaces), ifaceID, key)
	})
}

// SetWithIfacePeer is the preferred variant for callers that have the full
// InterfacePeer available (AllowedIPs, PSK, keepalive, endpoint).
func (p *peers) SetWithIfacePeer(peer *models.Peer, ifacePeer agentmodels.InterfacePeer) error {
	return p.setWithIfacePeer(peer, ifacePeer)
}

func (p *peers) setWithIfacePeer(peer *models.Peer, ifacePeer agentmodels.InterfacePeer) error {
	return p.db.Update(func(tx *bolt.Tx) error {
		ub := tx.Bucket(bktUsers)
		u, err := get[models.User](ub, idKey(p.userID))
		if err != nil {
			return err
		}

		ib := tx.Bucket(bktInterfaces)
		if ib.Get(idKey(peer.InterfaceId)) == nil {
			return fmt.Errorf("interface %s: %w", peer.InterfaceId, storage.ErrNotFound)
		}

		pubKey := peer.PrivateKey.PublicKey()

		// Upsert in user peer list
		found := false
		for i := range u.Peers {
			if u.Peers[i].PrivateKey.PublicKey() == pubKey {
				u.Peers[i] = *peer
				found = true
				break
			}
		}
		if !found {
			u.Peers = append(u.Peers, *peer)
		}
		if err := put(ub, idKey(p.userID), u); err != nil {
			return err
		}

		// Upsert InterfacePeer on the interface (ensure public key matches)
		ifacePeer.Key = pubKey
		return upsertIfacePeer(ib, peer.InterfaceId, ifacePeer)
	})
}

// ─────────────────────── helpers ────────────────────────────

// upsertIfacePeer adds or replaces an InterfacePeer by public key.
func upsertIfacePeer(ib *bolt.Bucket, ifaceID uuid.UUID, ip agentmodels.InterfacePeer) error {
	iface, err := get[models.Interface](ib, idKey(ifaceID))
	if err != nil {
		return fmt.Errorf("interface %s: %w", ifaceID, err)
	}
	replaced := false
	for i := range iface.Peers {
		if iface.Peers[i].Key == ip.Key {
			iface.Peers[i] = ip
			replaced = true
			break
		}
	}
	if !replaced {
		iface.Peers = append(iface.Peers, ip)
	}
	return put(ib, idKey(ifaceID), iface)
}

// removePeerFromInterface removes the peer with the given public key from an
// interface's peer list.  A missing interface is treated as a no-op.
func removePeerFromInterface(ib *bolt.Bucket, ifaceID uuid.UUID, pubKey agentmodels.Key) error {
	iface, err := get[models.Interface](ib, idKey(ifaceID))
	if err != nil {
		if storage.IsNotFound(err) {
			return nil // interface already gone — nothing to clean up
		}
		return err
	}
	filtered := iface.Peers[:0]
	for _, p := range iface.Peers {
		if p.Key != pubKey {
			filtered = append(filtered, p)
		}
	}
	iface.Peers = filtered
	return put(ib, idKey(ifaceID), iface)
}
