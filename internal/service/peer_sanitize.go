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
	"github.com/ks-tool/awg-admin/models"
)

// sanitizePeer returns a copy of p with PrivateKey replaced by its derived
// public key. models.Peer.PrivateKey is tagged json:"pk" for storage
// compatibility (storage/boltdb marshals this exact struct), but every
// caller outside this package — the HTTP API and the Wails bindings, which
// both serialize whatever a *Service method returns — only ever needs the
// peer's *public* key to identify it (GetPeer/DeletePeer/GetPeerConfig all
// look a peer up by public key). Sending the real private key in "pk" let
// the frontend round-trip it back as if it were the public key (e.g. in a
// QR-code request URL), which always failed lookup with "not found" since
// the stored peer's actual public key never matched. Real private keys
// never leave storage: GetPeerConfig/GetPeerQRCode read them fresh via
// s.store.Users().Peers(uID).Get(...), not through a sanitized copy.
func sanitizePeer(p models.Peer) models.Peer {
	p.PrivateKey = p.PrivateKey.PublicKey()
	return p
}

func sanitizePeers(peers []models.Peer) []models.Peer {
	out := make([]models.Peer, len(peers))
	for i, p := range peers {
		out[i] = sanitizePeer(p)
	}
	return out
}

func sanitizeUser(u models.User) models.User {
	u.Peers = sanitizePeers(u.Peers)
	return u
}

func sanitizeUsers(users []models.User) []models.User {
	out := make([]models.User, len(users))
	for i, u := range users {
		out[i] = sanitizeUser(u)
	}
	return out
}
