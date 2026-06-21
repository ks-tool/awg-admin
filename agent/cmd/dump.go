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

package main

import (
	"fmt"
	"os"

	"github.com/Jipok/wgctrl-go"
	"github.com/Jipok/wgctrl-go/wgtypes"
)

func main() {
	iface := os.Args[1]
	awg, err := wgctrl.New()
	if err != nil {
		panic(err)
	}
	defer func() { _ = awg.Close() }()

	dev, err := awg.Device(iface)
	if err != nil {
		panic(err)
	}

	for _, peer := range dev.Peers {
		psk := peer.PresharedKey.String()
		if peer.PresharedKey == [wgtypes.KeyLen]byte{} {
			psk = "(none)"
		}

		var pkiStr string
		pki := peer.PersistentKeepaliveInterval
		if pki == 0 {
			pkiStr = "off"
		} else {
			pkiStr = fmt.Sprintf("%d", pki)
		}

		endpoint := "(none)"
		if peer.Endpoint != nil {
			endpoint = peer.Endpoint.String()
		}

		lastHandshakeTime := peer.LastHandshakeTime.Unix()
		if lastHandshakeTime < 0 {
			lastHandshakeTime = 0
		}

		fmt.Printf(
			"%s\t%s\t%s\t%s\t%d\t%d\t%d\t%s\n",
			peer.PublicKey,
			psk,
			endpoint,
			peer.AllowedIPs[0].String(),
			lastHandshakeTime,
			peer.ReceiveBytes,
			peer.TransmitBytes,
			pkiStr,
		)
	}
}
