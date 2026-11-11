//go:build linux

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

package metrics

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"sync/atomic"
	"syscall"
	"time"

	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
)

var pingSequence atomic.Uint32

func probeLatency(ctx context.Context, iface, address string) (time.Duration, error) {
	ip := net.ParseIP(address).To4()
	if ip == nil {
		return 0, fmt.Errorf("probe address %q is not IPv4", address)
	}
	listen := net.ListenConfig{Control: func(_, _ string, raw syscall.RawConn) error {
		var controlErr error
		if err := raw.Control(func(fd uintptr) {
			controlErr = syscall.SetsockoptString(int(fd), syscall.SOL_SOCKET, syscall.SO_BINDTODEVICE, iface)
		}); err != nil {
			return err
		}
		return controlErr
	}}
	conn, err := listen.ListenPacket(ctx, "ip4:icmp", "0.0.0.0")
	if err != nil {
		return 0, err
	}
	defer func() { _ = conn.Close() }()

	id := os.Getpid() & 0xffff
	sequence := int(pingSequence.Add(1) & 0xffff)
	message := icmp.Message{Type: ipv4.ICMPTypeEcho, Body: &icmp.Echo{ID: id, Seq: sequence, Data: []byte("vpn-agent")}}
	packet, err := message.Marshal(nil)
	if err != nil {
		return 0, err
	}
	started := time.Now()
	if _, err := conn.WriteTo(packet, &net.IPAddr{IP: ip}); err != nil {
		return 0, err
	}
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(latencyProbeTimeout)
	}
	if err := conn.SetReadDeadline(deadline); err != nil {
		return 0, err
	}
	buffer := make([]byte, 1500)
	for {
		n, _, err := conn.ReadFrom(buffer)
		if err != nil {
			return 0, err
		}
		reply, err := icmp.ParseMessage(1, buffer[:n])
		if err != nil || reply.Type != ipv4.ICMPTypeEchoReply {
			continue
		}
		echo, ok := reply.Body.(*icmp.Echo)
		if ok && echo.ID == id && echo.Seq == sequence {
			return time.Since(started), nil
		}
		if ctx.Err() != nil {
			return 0, errors.Join(context.Cause(ctx), ctx.Err())
		}
	}
}
