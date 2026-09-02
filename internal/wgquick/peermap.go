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

package wgquick

import (
	"fmt"
	"strings"

	agentmodels "github.com/ks-tool/awg-admin/agent/models"
)

// PeerMap is the operator-supplied assignment of a conf's peers to awg-admin
// users. A wg-quick server conf only holds each peer's public key; awg-admin's
// model needs the private key (it renders client configs/QR codes from it), and
// only the operator has those — from the client configs handed out earlier. The
// map is a two-level, YAML-like text:
//
//	alice:
//	  <private-key>: laptop
//	  <private-key>: phone
//	"Bob Smith":
//	  <private-key>: bob-pc
//
// Users are matched by exact name (created when missing); a peer's private key
// is matched against the conf's [Peer] sections through its derived public
// key. Peer names are display-only and needn't be unique; private keys must be.
// Peers the map doesn't mention are imported without a user (see the import's
// "embedded peers").
type PeerMap struct {
	Users []PeerMapUser
}

// PeerMapUser is one user block of a PeerMap.
type PeerMapUser struct {
	Name  string
	Peers []PeerMapPeer
}

// PeerMapPeer is one `private-key: name` line of a PeerMap.
type PeerMapPeer struct {
	PrivateKey agentmodels.Key
	Name       string
}

// ParsePeerMap parses the user→peer map text (format: see PeerMap). An empty
// or comment-only text is a valid, empty map. Lines starting with `#` are
// comments; a user header is an unindented `name:` line, its peers the
// indented `private-key: peer-name` lines that follow. Names may be quoted.
// A private key listed twice, a peer line before any user, or a malformed
// line is an error.
func ParsePeerMap(text string) (*PeerMap, error) {
	pm := &PeerMap{}
	byName := map[string]int{} // user name → index in pm.Users (repeated blocks merge)
	seenKeys := map[agentmodels.Key]string{}
	current := -1

	for i, raw := range strings.Split(text, "\n") {
		lineNo := i + 1
		line := strings.TrimRight(raw, " \t\r")
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		indented := line != trimmed // leading whitespace → a peer line

		if !indented {
			name, rest, ok := strings.Cut(trimmed, ":")
			if !ok {
				return nil, fmt.Errorf("line %d: expected a user header like `name:`, got %q", lineNo, trimmed)
			}
			if strings.TrimSpace(rest) != "" {
				return nil, fmt.Errorf("line %d: a user header must be `name:` on its own; peers go on indented lines below it", lineNo)
			}
			name = unquote(strings.TrimSpace(name))
			if name == "" {
				return nil, fmt.Errorf("line %d: user name is empty", lineNo)
			}
			idx, exists := byName[name]
			if !exists {
				pm.Users = append(pm.Users, PeerMapUser{Name: name})
				idx = len(pm.Users) - 1
				byName[name] = idx
			}
			current = idx
			continue
		}

		if current < 0 {
			return nil, fmt.Errorf("line %d: peer line before any user header", lineNo)
		}
		keyText, name, ok := strings.Cut(trimmed, ":")
		if !ok {
			return nil, fmt.Errorf("line %d: expected `private-key: peer-name`, got %q", lineNo, trimmed)
		}
		key, err := agentmodels.ParseKey(strings.TrimSpace(keyText))
		if err != nil {
			return nil, fmt.Errorf("line %d: %q is not a valid base64 WireGuard private key", lineNo, strings.TrimSpace(keyText))
		}
		name = unquote(strings.TrimSpace(name))
		if name == "" {
			return nil, fmt.Errorf("line %d: peer name is required after the private key", lineNo)
		}
		if owner, dup := seenKeys[key]; dup {
			return nil, fmt.Errorf("line %d: private key listed twice (first under user %q)", lineNo, owner)
		}
		seenKeys[key] = pm.Users[current].Name
		pm.Users[current].Peers = append(pm.Users[current].Peers, PeerMapPeer{PrivateKey: key, Name: name})
	}
	return pm, nil
}

// unquote strips one pair of matching surrounding quotes, so a name with
// spaces or a colon can be written YAML-style as "John Doe".
func unquote(s string) string {
	if len(s) >= 2 && (s[0] == '"' || s[0] == '\'') && s[len(s)-1] == s[0] {
		return s[1 : len(s)-1]
	}
	return s
}
