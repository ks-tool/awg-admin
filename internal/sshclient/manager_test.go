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

package sshclient

import (
	"crypto/ed25519"
	"crypto/rand"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/ks-tool/awg-admin/models"

	"github.com/google/uuid"
	"golang.org/x/crypto/ssh"
)

// fakeSSHServer is a minimal in-process sshd stand-in: it accepts any
// password, performs the handshake, and then just discards everything —
// enough to exercise dial/keepalive/reconnect without needing a real host.
type fakeSSHServer struct {
	listener net.Listener
	cfg      *ssh.ServerConfig

	mu    sync.Mutex
	conns []*ssh.ServerConn
}

func newFakeSSHServer(t *testing.T) *fakeSSHServer {
	t.Helper()

	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate host key: %v", err)
	}
	signer, err := ssh.NewSignerFromSigner(priv)
	if err != nil {
		t.Fatalf("signer from host key: %v", err)
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	srv := &fakeSSHServer{
		listener: ln,
		cfg: &ssh.ServerConfig{
			PasswordCallback: func(conn ssh.ConnMetadata, password []byte) (*ssh.Permissions, error) {
				return nil, nil
			},
		},
	}
	srv.cfg.AddHostKey(signer)

	go srv.acceptLoop(t)
	t.Cleanup(func() { _ = ln.Close() })
	return srv
}

func (srv *fakeSSHServer) acceptLoop(t *testing.T) {
	for {
		conn, err := srv.listener.Accept()
		if err != nil {
			return // listener closed, test is done
		}
		go func() {
			sc, chans, reqs, err := ssh.NewServerConn(conn, srv.cfg)
			if err != nil {
				return
			}
			srv.mu.Lock()
			srv.conns = append(srv.conns, sc)
			srv.mu.Unlock()

			go ssh.DiscardRequests(reqs)
			go func() {
				for ch := range chans {
					_ = ch.Reject(ssh.UnknownChannelType, "not supported")
				}
			}()
		}()
	}
}

func (srv *fakeSSHServer) addr() string {
	return srv.listener.Addr().String()
}

// killNewestConn severs the most recently accepted connection from the
// server side, simulating the remote sshd/agent host dropping the
// connection (restart, network blip) without the client-side ssh.Client
// ever being told explicitly.
func (srv *fakeSSHServer) killNewestConn(t *testing.T) {
	t.Helper()
	srv.mu.Lock()
	defer srv.mu.Unlock()
	if len(srv.conns) == 0 {
		t.Fatal("no server-side connections accepted yet")
	}
	if err := srv.conns[len(srv.conns)-1].Close(); err != nil {
		t.Fatalf("close server-side conn: %v", err)
	}
}

func sshCfgFor(srv *fakeSSHServer) models.SSHConfig {
	host, port := splitHostPort(srv.addr())
	return models.SSHConfig{Host: host, Port: port, User: "root", Password: "anything"}
}

func splitHostPort(addr string) (string, uint16) {
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		panic(err)
	}
	var port uint16
	for _, c := range portStr {
		port = port*10 + uint16(c-'0')
	}
	return host, port
}

func TestManagerEnsureReconnectsAfterDeadConnection(t *testing.T) {
	srv := newFakeSSHServer(t)
	cfg := sshCfgFor(srv)
	serverID := uuid.New()

	m := NewManager()
	t.Cleanup(func() { _ = m.Close() })

	if err := m.Open(serverID, cfg, "127.0.0.1:0"); err != nil {
		t.Fatalf("Open: %v", err)
	}
	tunnel, ok := m.Get(serverID)
	if !ok {
		t.Fatal("expected a tunnel after Open")
	}
	if !tunnel.Alive() {
		t.Fatal("freshly opened tunnel should be alive")
	}

	srv.killNewestConn(t)

	// Give the client side's connection-closed notification a moment to
	// propagate before asserting on it — closing the server-side conn is
	// async from the client's perspective.
	deadline := time.Now().Add(2 * time.Second)
	for tunnel.Alive() && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if tunnel.Alive() {
		t.Fatal("expected tunnel to be dead after server-side connection was closed")
	}

	reconnected, err := m.Ensure(serverID, cfg, "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	if !reconnected.Alive() {
		t.Fatal("expected Ensure to return a live tunnel")
	}

	got, ok := m.Get(serverID)
	if !ok || got != reconnected {
		t.Fatal("expected Ensure to have replaced the cached tunnel with the reconnected one")
	}
}

func TestManagerEnsureReusesLiveConnection(t *testing.T) {
	srv := newFakeSSHServer(t)
	cfg := sshCfgFor(srv)
	serverID := uuid.New()

	m := NewManager()
	t.Cleanup(func() { _ = m.Close() })

	if err := m.Open(serverID, cfg, "127.0.0.1:0"); err != nil {
		t.Fatalf("Open: %v", err)
	}
	original, _ := m.Get(serverID)

	reused, err := m.Ensure(serverID, cfg, "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	if reused != original {
		t.Fatal("expected Ensure to reuse the existing live tunnel instead of reconnecting")
	}
}
