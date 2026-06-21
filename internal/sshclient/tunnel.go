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
	"context"
	"net"
	"net/http"

	"golang.org/x/crypto/ssh"
)

// TunnelClient is an *http.Client whose connections to the agent are
// carried over an existing SSH connection, plus the underlying SSH client
// so the caller can close it once done.
type TunnelClient struct {
	*http.Client
	ssh  *ssh.Client
	dead chan struct{}
}

// Close tears down the underlying SSH connection.
func (t *TunnelClient) Close() error {
	return t.ssh.Close()
}

// Alive reports whether the underlying SSH connection is still up, purely
// from in-memory state — it never does network I/O itself.
//
// An earlier version of this method round-tripped a throwaway global
// request (ssh.Client.SendRequest) to probe the connection live. That's
// unsafe: golang.org/x/crypto/ssh's mux.SendRequest drains its
// globalResponses channel in a `select { case <-ch: default: break }` loop
// before sending — once the connection has actually died, that channel is
// left permanently closed, and reading from a closed channel never blocks
// and is always "ready", so the select's case fires every iteration and
// `default` (the only exit) is never reached: SendRequest livelocks the
// calling goroutine forever instead of returning an error. So calling it
// *after* a connection is already known-dead — exactly the case Ensure
// needs to detect — could hang Ensure permanently rather than letting it
// reconnect.
//
// Instead, NewTunnelClient starts a background goroutine that blocks on
// ssh.Client.Wait() (which returns as soon as the connection's read loop
// errors out — clean close, EOF, or reset) and closes dead when it does.
// Alive just checks whether that's happened yet.
func (t *TunnelClient) Alive() bool {
	select {
	case <-t.dead:
		return false
	default:
		return true
	}
}

// NewTunnelClient dials sshCfg and returns an http.Client that reaches
// agentAddr (typically the agent's loopback address, e.g. 127.0.0.1:8080,
// on the remote server) through that SSH connection — no local listening
// port or separate port-forward process is needed since ssh.Client.Dial
// opens the remote-side connection directly per request.
func NewTunnelClient(client *ssh.Client, agentAddr string) *TunnelClient {
	transport := &http.Transport{
		DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
			return client.Dial("tcp", agentAddr)
		},
	}
	t := &TunnelClient{Client: &http.Client{Transport: transport}, ssh: client, dead: make(chan struct{})}
	go func() {
		_ = client.Wait()
		close(t.dead)
	}()
	return t
}
