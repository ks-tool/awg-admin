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

package models

import (
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"net"
	"time"

	"github.com/Jipok/wgctrl-go/wgtypes"
)

type Key wgtypes.Key

func IsEmpty(key Key) bool {
	return key == [wgtypes.KeyLen]byte{}
}

func GenerateKey() (Key, error) {
	k, err := wgtypes.GenerateKey()
	if err != nil {
		return Key{}, err
	}
	return Key(k), nil
}

func GeneratePrivateKey() (Key, error) {
	k, err := wgtypes.GeneratePrivateKey()
	if err != nil {
		return Key{}, err
	}
	return Key(k), nil
}

func ParseKey(s string) (Key, error) {
	k, err := wgtypes.ParseKey(s)
	if err != nil {
		return Key{}, err
	}
	return Key(k), nil
}

func (k *Key) String() string {
	return k.WGKey().String()
}

func (k *Key) WGKey() wgtypes.Key {
	return wgtypes.Key(*k)
}

func (k *Key) PublicKey() Key {
	return Key(k.WGKey().PublicKey())
}

func (k *Key) UnmarshalJSON(data []byte) error {
	var dataBytes []byte
	if err := json.Unmarshal(data, &dataBytes); err != nil {
		return err
	}
	if len(dataBytes) == 0 {
		return nil
	}
	*k = Key(dataBytes)
	return nil
}

func (k *Key) MarshalJSON() ([]byte, error) {
	return json.Marshal((*k)[:])
}

type InterfaceConfig struct {
	Interface string `json:"iface"`
	// PrivateKey specifies a private key configuration, if not nil.
	PrivateKey Key `json:"pk"`
	// ListenPort specifies a device's listening port, if not nil.
	ListenPort uint16 `json:"listen"`

	Address string   `json:"addr"`
	MTU     int      `json:"mtu,omitempty"`
	DNS     []string `json:"dns,omitempty"`

	// Disabled deactivates the interface on the agent: its link is torn down
	// (or never brought up at startup) and its device config is not applied,
	// while the config itself stays stored. Zero value (false) keeps the
	// interface active, so configs written before this field existed stay up.
	Disabled bool `json:"disabled,omitempty"`

	// Table the routing table number
	Table int `json:"table,omitempty"`
	// FirewallMark specifies a device's firewall mark, if not nil.
	//
	// If non-nil and set to 0, the firewall mark will be cleared.
	FirewallMark *int `json:"fwMark,omitempty"`

	PreUp    []string `json:"preUp,omitempty"`    // action that is executed before the device is up
	PostUp   []string `json:"postUp,omitempty"`   // action that is executed after the device is up
	PreDown  []string `json:"preDown,omitempty"`  // action that is executed before the device is down
	PostDown []string `json:"postDown,omitempty"` // action that is executed after the device is down

	// Peers specifies a list of peer configurations to apply to a device.
	Peers []InterfacePeer `json:"peers,omitempty"`

	// --- AmneziaWG Specific Configuration ---
	// All fields are pointers to handle "optional update" semantics.

	// Junk Packet parameters
	Jc   *int `json:"jc,omitempty"`   // Count
	Jmin *int `json:"jmin,omitempty"` // Min size
	Jmax *int `json:"jmax,omitempty"` // Max size

	// Message Padding parameters (bytes)
	S1 *int `json:"s1,omitempty"` // Init
	S2 *int `json:"s2,omitempty"` // Response
	S3 *int `json:"s3,omitempty"` // Cookie
	S4 *int `json:"s4,omitempty"` // Transport

	// Message Magic Headers
	// In AmneziaWG 2.0 these should be ranges (e.g., "123456-123999")
	H1 *string `json:"h1,omitempty"` // Init
	H2 *string `json:"h2,omitempty"` // Response
	H3 *string `json:"h3,omitempty"` // Cookie
	H4 *string `json:"h4,omitempty"` // Transport

	// Init Packet Magic / Custom Signature (obfuscation)
	// For client-side explicit setups only.
	I1 *string `json:"i1,omitempty"`
	I2 *string `json:"i2,omitempty"`
	I3 *string `json:"i3,omitempty"`
	I4 *string `json:"i4,omitempty"`
	I5 *string `json:"i5,omitempty"`
}

func (awg InterfaceConfig) ToAmneziaConfig() *wgtypes.Config {
	peers := make([]wgtypes.PeerConfig, 0, len(awg.Peers))
	for _, peer := range awg.Peers {
		peers = append(peers, peer.PeerConfig())
	}

	return &wgtypes.Config{
		PrivateKey:   new(awg.PrivateKey.WGKey()),
		ListenPort:   new(int(awg.ListenPort)),
		FirewallMark: awg.FirewallMark,
		Peers:        peers,
		Jc:           awg.Jc,
		Jmin:         awg.Jmin,
		Jmax:         awg.Jmax,
		S1:           awg.S1,
		S2:           awg.S2,
		S3:           awg.S3,
		S4:           awg.S4,
		H1:           awg.H1,
		H2:           awg.H2,
		H3:           awg.H3,
		H4:           awg.H4,
		I1:           awg.I1,
		I2:           awg.I2,
		I3:           awg.I3,
		I4:           awg.I4,
		I5:           awg.I5,
	}
}

// IsAmnezia reports whether this is an AmneziaWG interface (as opposed to a
// plain WireGuard one), i.e. any obfuscation parameter is set. The admin's
// "Amnezia Interface" toggle drives this: when on it sends a generated param
// set, when off it sends none. The backend uses it to pick the kernel link kind
// ("amneziawg" vs "wireguard").
func (awg InterfaceConfig) IsAmnezia() bool {
	return awg.Jc != nil || awg.Jmin != nil || awg.Jmax != nil ||
		awg.S1 != nil || awg.S2 != nil || awg.S3 != nil || awg.S4 != nil ||
		awg.H1 != nil || awg.H2 != nil || awg.H3 != nil || awg.H4 != nil ||
		awg.I1 != nil || awg.I2 != nil || awg.I3 != nil || awg.I4 != nil || awg.I5 != nil
}

// GenerateAmneziaParams populates the config with obfuscation values optimized for AWG 2.0.
func GenerateAmneziaParams(cfg *InterfaceConfig) {
	// A missing private key is generated unconditionally, independent of
	// the Jc/Jmin/Jmax early-return below — those only gate the AWG
	// obfuscation params (skip if the caller already set any of them), an
	// unset/zero private key should never be left as-is regardless.
	if IsEmpty(cfg.PrivateKey) {
		var err error
		cfg.PrivateKey, err = GeneratePrivateKey()
		if err != nil {
			panic(err)
		}
	}

	// ==========================================
	// 1. PRE-SESSION JUNK PACKETS (Jc, Jmin, Jmax)
	// Doc limits: Jc 0-10, Jmin/Jmax 64-1024 bytes.
	// We avoid packets smaller than 64 bytes so DPI doesn't flag them as anomalies.
	// ==========================================

	if cfg.Jc != nil || cfg.Jmin != nil || cfg.Jmax != nil {
		return
	}

	cfg.Jc = new(3 + rand.IntN(4)) // 3 to 6 packets

	// AWG Go core easily handles up to 1024. We wanna look like UDP app traffic.
	cfg.Jmin = new(64 + rand.IntN(50))              // 64-113 bytes
	cfg.Jmax = new(*cfg.Jmin + 50 + rand.IntN(100)) // Jmin + (50-149 bytes)

	// ==========================================
	// 2. PACKET PADDING (S1, S2, S3, S4)
	// Doc limits: S1-S3: 0-64 bytes. S4: 0-32 bytes.
	// Base standard WG sizes: Init=148, Resp=92, Cookie=64.
	// Random garbage bytes prepended to the START of WireGuard packets.
	// ==========================================

	for {
		// Strict limits to prevent high overhead while breaking WG signature
		s1 := 15 + rand.IntN(49) // 15-63 bytes
		s2 := 15 + rand.IntN(49) // 15-63 bytes
		s3 := 10 + rand.IntN(54) // 10-63 bytes
		s4 := 1 + rand.IntN(15)  // 1-15 bytes (keep Transport small to save MTU)

		// Rule A: All padding values must be unique
		if s1 == s2 || s1 == s3 || s1 == s4 || s2 == s3 || s2 == s4 || s3 == s4 {
			continue
		}

		// Rule B: Total resulting packet sizes must NEVER be equal.
		// NOTE: We do not check S4 against control packets because Transport
		// packets have variable payload sizes. The AWG core handles Transport
		// size alignment dynamically using inner MsgType validation.
		if s1+148 == s2+92 || s3+64 == s1+148 || s3+64 == s2+92 {
			continue
		}

		// Apply values and break the loop
		cfg.S1, cfg.S2, cfg.S3, cfg.S4 = new(s1), new(s2), new(s3), new(s4)
		break
	}

	// ==========================================
	// 3. MAGIC HEADERS RANGES (H1 - H4)
	// We generate 4 non-overlapping mathematical ranges (to satisfy the Go parser).
	// Then we shuffle them so that H1 < H2 < H3 < H4 is mathematically destroyed,
	// preventing heuristic DPI signature matching.
	// We keep max value below math.MaxInt32 to prevent integer
	// overflow crashes on legacy C++ clients which parse strings into signed ints.
	// ==========================================

	currentOffset := 150_000_000 + rand.IntN(50_000_000)
	ranges := make([]*string, 4)

	// Step 3.1: Generate strictly increasing non-overlapping ranges
	for i := 0; i < 4; i++ {
		rangeStart := currentOffset
		rangeEnd := rangeStart + 50_000_000 + rand.IntN(100_000_000)
		ranges[i] = new(fmt.Sprintf("%d-%d", rangeStart, rangeEnd))

		// Add a guaranteed gap to prevent overlap
		currentOffset = rangeEnd + 10_000_000 + rand.IntN(20_000_000)
	}

	// Step 3.2: SHUFFLE THE RANGES (The Anti-Heuristic Magic)
	rand.Shuffle(len(ranges), func(i, j int) {
		ranges[i], ranges[j] = ranges[j], ranges[i]
	})

	// Step 3.3: Assign shuffled ranges to packet types
	cfg.H1 = ranges[0] // Handshake Initiation
	cfg.H2 = ranges[1] // Handshake Response
	cfg.H3 = ranges[2] // Cookie Reply
	cfg.H4 = ranges[3] // Transport Data

	// Init-packets (I1..I5) are usually for specific protocol emulation (TLS/DTLS).
	// From Doc:
	// If the parameter I1 is missing, the entire chain (I2-I5) is skipped, and AmneziaWG behaves as AmneziaWG 1.0, simplifying compatibility.
	i1Length := 15 + rand.IntN(26)
	cfg.I1 = new(fmt.Sprintf("<r %d>", i1Length))

	// Got it from amnezia desktop client. Without it some dpi drops packets
	cfg.I1 = new("<r 2><b 0x858000010001000000000669636c6f756403636f6d0000010001c00c000100010000105a00044d583737>")
}

func (awg InterfaceConfig) ToWireguardPeers() []wgtypes.PeerConfig {
	peers := make([]wgtypes.PeerConfig, 0, len(awg.Peers))
	for _, peer := range awg.Peers {
		peers = append(peers, peer.PeerConfig())
	}
	return peers
}

type InterfacePeer struct {
	Key          Key      `json:"key"`
	PresharedKey *Key     `json:"psk,omitempty"`
	AllowedIPs   []string `json:"ips"`
	Endpoint     string   `json:"endpoint,omitempty"`

	KeepaliveInterval time.Duration `json:"keepalive"`
}

func (p InterfacePeer) PeerConfig() wgtypes.PeerConfig {
	cfg := wgtypes.PeerConfig{
		PublicKey:                   p.Key.WGKey(),
		PersistentKeepaliveInterval: &p.KeepaliveInterval,
	}

	if p.PresharedKey != nil {
		cfg.PresharedKey = new(p.PresharedKey.WGKey())
	}

	if len(p.Endpoint) > 0 {
		cfg.Endpoint, _ = net.ResolveUDPAddr("udp", p.Endpoint)
	}

	for _, ip := range p.AllowedIPs {
		if _, ipnet, _ := net.ParseCIDR(ip); ipnet != nil {
			cfg.AllowedIPs = append(cfg.AllowedIPs, *ipnet)
		}
	}

	return cfg
}

type DeviceInfo struct {
	Name         string             `json:"name"`
	PublicKey    Key                `json:"publicKey"`
	Type         wgtypes.DeviceType `json:"type"`
	ListenPort   int                `json:"listenPort"`
	FirewallMark int                `json:"firewallMark"`
	IsAmnezia    bool               `json:"isAmnezia"`
	Peers        []PeerInfo         `json:"peers"`
}

type PeerInfo struct {
	PublicKey       Key       `json:"publicKey"`
	Endpoint        string    `json:"endpoint"`
	LatestHandshake time.Time `json:"lastHandshake"`
	RxBytes         int64     `json:"rx"`
	TxBytes         int64     `json:"tx"`
	Online          bool      `json:"online"`
	UpdatedAt       time.Time `json:"updated_at"`
}
