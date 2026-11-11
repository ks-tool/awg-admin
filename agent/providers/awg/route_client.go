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
	"context"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/netip"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/amnezia-vpn/amneziawg-go/conn"
	"github.com/amnezia-vpn/amneziawg-go/device"
	"github.com/amnezia-vpn/amneziawg-go/tun/netstack"
)

func runRouteClient(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("route-client", flag.ContinueOnError)
	configPath := flags.String("config", "", "route client configuration")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if len(*configPath) == 0 {
		return errors.New("route-client --config is required")
	}
	data, err := os.ReadFile(*configPath)
	if err != nil {
		return err
	}
	var config routeClientConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return err
	}
	prefix, err := netip.ParsePrefix(config.Address)
	if err != nil {
		return err
	}
	tun, network, err := netstack.CreateNetTUN([]netip.Addr{prefix.Addr()}, nil, 1380)
	if err != nil {
		return err
	}
	logger := device.NewLogger(device.LogLevelError, "[route-client] ")
	awg := device.NewDevice(tun, conn.NewDefaultBind(), logger)
	defer awg.Close()
	if err := awg.IpcSet(routeClientUAPI(config)); err != nil {
		return err
	}
	if err := awg.Up(); err != nil {
		return err
	}
	server := &socksServer{network: network, sessions: make(map[string]*socksUDPSession)}
	if err := server.listen(ctx, config.SOCKS5); err != nil {
		return err
	}
	return nil
}

func routeClientUAPI(config routeClientConfig) string {
	var b strings.Builder
	fmt.Fprintf(&b, "private_key=%s\n", hex.EncodeToString(config.PrivateKey[:]))
	fmt.Fprintf(&b, "public_key=%s\n", hex.EncodeToString(config.Peer.Key[:]))
	if config.Peer.PresharedKey != nil {
		fmt.Fprintf(&b, "preshared_key=%s\n", hex.EncodeToString(config.Peer.PresharedKey[:]))
	}
	if len(config.Peer.Endpoint) > 0 {
		fmt.Fprintf(&b, "endpoint=%s\n", config.Peer.Endpoint)
	}
	fmt.Fprintln(&b, "allowed_ip=0.0.0.0/0")
	if config.Peer.KeepaliveInterval > 0 {
		fmt.Fprintf(&b, "persistent_keepalive_interval=%d\n", int(config.Peer.KeepaliveInterval.Seconds()))
	}
	writeInt := func(name string, value *int) {
		if value != nil {
			fmt.Fprintf(&b, "%s=%d\n", name, *value)
		}
	}
	writeString := func(name string, value *string) {
		if value != nil {
			fmt.Fprintf(&b, "%s=%s\n", name, *value)
		}
	}
	writeInt("jc", config.Interface.Jc)
	writeInt("jmin", config.Interface.Jmin)
	writeInt("jmax", config.Interface.Jmax)
	writeInt("s1", config.Interface.S1)
	writeInt("s2", config.Interface.S2)
	writeInt("s3", config.Interface.S3)
	writeInt("s4", config.Interface.S4)
	writeString("h1", config.Interface.H1)
	writeString("h2", config.Interface.H2)
	writeString("h3", config.Interface.H3)
	writeString("h4", config.Interface.H4)
	writeString("i1", config.Interface.I1)
	writeString("i2", config.Interface.I2)
	writeString("i3", config.Interface.I3)
	writeString("i4", config.Interface.I4)
	writeString("i5", config.Interface.I5)
	return b.String()
}

type socksNetwork interface {
	DialContext(context.Context, string, string) (net.Conn, error)
}

type socksServer struct {
	network  socksNetwork
	mu       sync.Mutex
	sessions map[string]*socksUDPSession
}

type socksUDPSession struct {
	conn     net.Conn
	lastUsed time.Time
}

func (s *socksServer) listen(ctx context.Context, address string) error {
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return err
	}
	defer listener.Close()
	go func() {
		<-ctx.Done()
		_ = listener.Close()
		s.mu.Lock()
		for _, session := range s.sessions {
			_ = session.conn.Close()
		}
		s.mu.Unlock()
	}()
	for {
		client, err := listener.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		go s.handle(ctx, client)
	}
}

func (s *socksServer) handle(ctx context.Context, client net.Conn) {
	defer client.Close()
	if err := socksHandshake(client); err != nil {
		return
	}
	command, address, err := readSocksRequest(client)
	if err != nil {
		return
	}
	switch command {
	case 1:
		remote, err := s.network.DialContext(ctx, "tcp", address)
		if err != nil {
			_ = writeSocksReply(client, 5)
			return
		}
		defer remote.Close()
		if err := writeSocksReply(client, 0); err != nil {
			return
		}
		done := make(chan struct{})
		go func() {
			_, _ = io.Copy(remote, client)
			close(done)
		}()
		_, _ = io.Copy(client, remote)
		<-done
	case 3:
		if err := writeSocksReply(client, 0); err != nil {
			return
		}
		s.serveUDP(ctx, client)
	default:
		_ = writeSocksReply(client, 7)
	}
}

func socksHandshake(conn net.Conn) error {
	header := make([]byte, 2)
	if _, err := io.ReadFull(conn, header); err != nil {
		return err
	}
	if header[0] != 5 || header[1] == 0 {
		return errors.New("invalid SOCKS5 greeting")
	}
	methods := make([]byte, int(header[1]))
	if _, err := io.ReadFull(conn, methods); err != nil {
		return err
	}
	_, err := conn.Write([]byte{5, 0})
	return err
}

func readSocksRequest(conn net.Conn) (byte, string, error) {
	header := make([]byte, 4)
	if _, err := io.ReadFull(conn, header); err != nil {
		return 0, "", err
	}
	if header[0] != 5 {
		return 0, "", errors.New("invalid SOCKS5 request")
	}
	address, err := readSocksAddress(conn, header[3])
	return header[1], address, err
}

func readSocksAddress(conn io.Reader, addressType byte) (string, error) {
	var host string
	switch addressType {
	case 1:
		data := make([]byte, 4)
		if _, err := io.ReadFull(conn, data); err != nil {
			return "", err
		}
		host = net.IP(data).String()
	case 3:
		length := make([]byte, 1)
		if _, err := io.ReadFull(conn, length); err != nil {
			return "", err
		}
		data := make([]byte, int(length[0]))
		if _, err := io.ReadFull(conn, data); err != nil {
			return "", err
		}
		host = string(data)
	case 4:
		data := make([]byte, 16)
		if _, err := io.ReadFull(conn, data); err != nil {
			return "", err
		}
		host = net.IP(data).String()
	default:
		return "", errors.New("unsupported SOCKS5 address type")
	}
	portData := make([]byte, 2)
	if _, err := io.ReadFull(conn, portData); err != nil {
		return "", err
	}
	return net.JoinHostPort(host, strconv.Itoa(int(binary.BigEndian.Uint16(portData)))), nil
}

func writeSocksReply(conn net.Conn, status byte) error {
	_, err := conn.Write([]byte{5, status, 0, 1, 0, 0, 0, 0, 0, 0})
	return err
}

func (s *socksServer) serveUDP(ctx context.Context, client net.Conn) {
	clientAddress, err := net.ResolveUDPAddr("udp", client.RemoteAddr().String())
	if err != nil {
		return
	}
	listener, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
	if err != nil {
		return
	}
	defer listener.Close()
	go func() {
		<-ctx.Done()
		_ = listener.Close()
	}()
	buffer := make([]byte, 65535)
	for {
		if err := listener.SetReadDeadline(time.Now().Add(2 * time.Minute)); err != nil {
			return
		}
		n, source, err := listener.ReadFromUDP(buffer)
		if err != nil {
			return
		}
		if source.IP.String() != clientAddress.IP.String() {
			continue
		}
		address, payload, err := decodeUDPPacket(buffer[:n])
		if err != nil {
			continue
		}
		session, err := s.udpSession(ctx, address)
		if err != nil {
			continue
		}
		if _, err := session.conn.Write(payload); err != nil {
			continue
		}
		go s.relayUDP(listener, source, address, session)
	}
}

func (s *socksServer) udpSession(ctx context.Context, address string) (*socksUDPSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if session := s.sessions[address]; session != nil {
		session.lastUsed = time.Now()
		return session, nil
	}
	remote, err := s.network.DialContext(ctx, "udp", address)
	if err != nil {
		return nil, err
	}
	session := &socksUDPSession{conn: remote, lastUsed: time.Now()}
	s.sessions[address] = session
	return session, nil
}

func (s *socksServer) relayUDP(listener *net.UDPConn, client *net.UDPAddr, address string, session *socksUDPSession) {
	buffer := make([]byte, 65535)
	n, err := session.conn.Read(buffer)
	if err != nil {
		return
	}
	packet, err := encodeUDPPacket(address, buffer[:n])
	if err != nil {
		return
	}
	_, _ = listener.WriteToUDP(packet, client)
}

func decodeUDPPacket(data []byte) (string, []byte, error) {
	if len(data) < 4 || data[0] != 0 || data[1] != 0 || data[2] != 0 {
		return "", nil, errors.New("invalid SOCKS5 UDP packet")
	}
	reader := &packetReader{data: data[3:], conn: nil}
	address, err := readSocksAddress(reader, data[3])
	if err != nil {
		return "", nil, err
	}
	return address, data[len(data)-reader.remaining():], nil
}

type packetReader struct {
	data []byte
	conn net.Conn
}

func (r *packetReader) Read(p []byte) (int, error) {
	if len(r.data) == 0 {
		return 0, io.EOF
	}
	n := copy(p, r.data)
	r.data = r.data[n:]
	return n, nil
}

func (r *packetReader) remaining() int { return len(r.data) }

func encodeUDPPacket(address string, payload []byte) ([]byte, error) {
	host, portText, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		return nil, err
	}
	packet := []byte{0, 0, 0}
	ip := net.ParseIP(host)
	if ip == nil {
		packet = append(packet, 3, byte(len(host)))
		packet = append(packet, []byte(host)...)
	} else if ipv4 := ip.To4(); ipv4 != nil {
		packet = append(packet, 1)
		packet = append(packet, ipv4...)
	} else {
		packet = append(packet, 4)
		packet = append(packet, ip.To16()...)
	}
	packet = binary.BigEndian.AppendUint16(packet, uint16(port))
	return append(packet, payload...), nil
}
