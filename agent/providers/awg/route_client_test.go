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

package awg

import (
	"bytes"
	"context"
	"io"
	"net"
	"testing"
	"time"
)

type directSocksNetwork struct {
	dialed chan string
}

func (n directSocksNetwork) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	var dialer net.Dialer
	conn, err := dialer.DialContext(ctx, network, address)
	if err == nil {
		n.dialed <- address
	}
	return conn, err
}

func TestSocksUDPAssociateReturnsUsableRelay(t *testing.T) {
	echo, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
	if err != nil {
		t.Fatal(err)
	}
	defer echo.Close()
	echoResult := make(chan error, 1)
	go func() {
		buffer := make([]byte, 64)
		n, source, err := echo.ReadFromUDP(buffer)
		if err == nil {
			_, err = echo.WriteToUDP(buffer[:n], source)
		}
		echoResult <- err
	}()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	network := directSocksNetwork{dialed: make(chan string, 1)}
	server := &socksServer{network: network, sessions: make(map[string]*socksUDPSession)}
	done := make(chan struct{})
	go func() {
		defer close(done)
		client, err := listener.Accept()
		if err == nil {
			server.handle(ctx, client)
		}
	}()

	client, err := net.DialTimeout("tcp", listener.Addr().String(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if err := client.SetDeadline(time.Now().Add(3 * time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Write([]byte{5, 1, 0}); err != nil {
		t.Fatal(err)
	}
	greeting := make([]byte, 2)
	if _, err := io.ReadFull(client, greeting); err != nil || !bytes.Equal(greeting, []byte{5, 0}) {
		t.Fatalf("SOCKS handshake = %v, %v", greeting, err)
	}
	if _, err := client.Write([]byte{5, 3, 0, 1, 0, 0, 0, 0, 0, 0}); err != nil {
		t.Fatal(err)
	}
	header := make([]byte, 4)
	if _, err := io.ReadFull(client, header); err != nil {
		t.Fatal(err)
	}
	if header[0] != 5 || header[1] != 0 {
		t.Fatalf("UDP Associate failed: %v", header)
	}
	relayAddress, err := readSocksAddress(client, header[3])
	if err != nil {
		t.Fatal(err)
	}
	relay, err := net.ResolveUDPAddr("udp", relayAddress)
	if err != nil {
		t.Fatal(err)
	}
	if relay.Port == 0 || relay.IP == nil || relay.IP.IsUnspecified() {
		t.Fatalf("unusable relay address: %s", relay)
	}

	udpClient, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
	if err != nil {
		t.Fatal(err)
	}
	defer udpClient.Close()
	payload := []byte("udp-through-socks")
	packet, err := encodeUDPPacket(echo.LocalAddr().String(), payload)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := udpClient.WriteToUDP(packet, relay); err != nil {
		t.Fatal(err)
	}
	select {
	case address := <-network.dialed:
		if address != echo.LocalAddr().String() {
			t.Fatalf("relay dialed %s, want %s", address, echo.LocalAddr())
		}
	case <-time.After(3 * time.Second):
		t.Fatal("relay did not dial the remote UDP endpoint")
	}
	select {
	case err := <-echoResult:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("remote echo server did not receive the relayed datagram")
	}
	if err := udpClient.SetReadDeadline(time.Now().Add(3 * time.Second)); err != nil {
		t.Fatal(err)
	}
	buffer := make([]byte, 1024)
	n, _, err := udpClient.ReadFromUDP(buffer)
	if err != nil {
		t.Fatal(err)
	}
	_, echoed, err := decodeUDPPacket(buffer[:n])
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(echoed, payload) {
		t.Fatalf("echoed payload = %q, want %q", echoed, payload)
	}
	cancel()
	client.Close()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("UDP relay did not stop")
	}
}
