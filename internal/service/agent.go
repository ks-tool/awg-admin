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
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"

	agentmodels "github.com/ks-tool/awg-admin/agent/models"
	"github.com/ks-tool/awg-admin/internal/agentclient"
	"github.com/ks-tool/awg-admin/models"

	"github.com/google/uuid"
)

// agentClientFor picks the transport for srv's agent and wraps it in a
// agentclient.Client: a direct mTLS *http.Client when srv.Agent.TLS is
// configured (the agent is reachable on its own, typically a public IP),
// otherwise the SSH tunnel opened by StartTunnels/Manager.Open for that
// server ID.
func (s *Service) agentClientFor(srv *models.Server) (*agentclient.Client, error) {
	if !srv.Agent.TLS.IsZero() {
		httpClient, err := mtlsHTTPClient(srv.Agent.TLS)
		if err != nil {
			return nil, fmt.Errorf("build mTLS client: %w", err)
		}
		return agentclient.New(httpClient, "https://"+srv.Agent.Address), nil
	}

	tunnel, err := s.tunnels.Ensure(srv.ID, srv.SSH, srv.Agent.Address)
	if err != nil {
		return nil, fmt.Errorf("no SSH tunnel open for server %s (agent unreachable): %w", srv.ID, err)
	}
	return agentclient.New(tunnel.Client, "http://"+srv.Agent.Address), nil
}

// callAgent runs an idempotent op against srv's agent, owning transport setup,
// the request timeout, and — for tunnelled servers — a single retry after
// force-reopening a tunnel that turned out to be dead.
//
// TunnelClient.Alive() reports liveness from an in-memory flag that a
// background goroutine only flips once ssh.Client.Wait() returns, so it can
// briefly lag a connection that has just died (agent host rebooted, sshd
// restarted, network dropped the session). In that window agentClientFor →
// Manager.Ensure hands back a tunnel whose very next request fails with a bare
// io.EOF from the dead SSH mux — surfacing as e.g.
// `PUT /interfaces: Put "http://127.0.0.1:8080/interfaces": EOF` — instead of
// being transparently reconnected. Every mutation routed through here
// (PUT/DELETE /interfaces, PATCH /metrics) is idempotent, so we recover by
// forcing a fresh dial (Manager.Open, which closes the stale tunnel first) and
// retrying once; by then Alive() has flipped and Ensure yields a live tunnel.
//
// mTLS clients aren't retried here: their only comparable failure is a stale
// keep-alive connection, which net/http already retries on its own via the
// Idempotency-Key set in agentclient.markIdempotent.
func (s *Service) callAgent(srv *models.Server, op func(context.Context, *agentclient.Client) error) error {
	ctx, cancel := context.WithTimeout(context.Background(), pushTimeout)
	defer cancel()

	client, err := s.agentClientFor(srv)
	if err != nil {
		return err
	}

	err = op(ctx, client)
	if err == nil || !srv.Agent.TLS.IsZero() || !tunnelDropped(err) {
		return err
	}

	// The tunnel died underneath the request but Ensure didn't notice in
	// time. Force a fresh one and try again; keep the original error if the
	// reconnect itself fails so the caller sees the real push failure.
	if reopenErr := s.tunnels.Open(srv.ID, srv.SSH, srv.Agent.Address); reopenErr != nil {
		return err
	}
	client, reErr := s.agentClientFor(srv)
	if reErr != nil {
		return err
	}
	return op(ctx, client)
}

// tunnelDropped reports whether err looks like the SSH tunnel (or a pooled
// connection carried over it) died underneath the request, as opposed to the
// agent itself answering with an error — the cases callAgent can recover by
// reconnecting. The wrapped io.EOF is what golang.org/x/crypto/ssh's mux
// returns from a Dial once the connection is gone; net.ErrClosed covers a
// connection torn down concurrently.
func tunnelDropped(err error) bool {
	return errors.Is(err, io.EOF) ||
		errors.Is(err, io.ErrUnexpectedEOF) ||
		errors.Is(err, net.ErrClosed)
}

// GetServerMetrics fetches the latest CPU/RAM/load/network/peer snapshot
// from serverID's agent, for display on the frontend Dashboard.
func (s *Service) GetServerMetrics(serverID string) (*agentmodels.MetricsSnapshot, error) {
	sID, err := uuid.Parse(serverID)
	if err != nil {
		return nil, err
	}
	srv, err := s.store.Servers().Get(sID)
	if err != nil {
		return nil, err
	}

	var snap *agentmodels.MetricsSnapshot
	err = s.callAgent(srv, func(ctx context.Context, c *agentclient.Client) error {
		var mErr error
		snap, mErr = c.Metrics(ctx)
		return mErr
	})
	if err != nil {
		return nil, err
	}
	return snap, nil
}

// GetServerMetricsHistory fetches every host-level sample still retained in
// serverID's agent's in-memory ring buffer (up to 48h), for the Dashboard's
// per-server metrics chart modal.
func (s *Service) GetServerMetricsHistory(serverID string) (*agentmodels.SystemHistory, error) {
	sID, err := uuid.Parse(serverID)
	if err != nil {
		return nil, err
	}
	srv, err := s.store.Servers().Get(sID)
	if err != nil {
		return nil, err
	}

	var history *agentmodels.SystemHistory
	err = s.callAgent(srv, func(ctx context.Context, c *agentclient.Client) error {
		var mErr error
		history, mErr = c.MetricsHistory(ctx)
		return mErr
	})
	if err != nil {
		return nil, err
	}
	return history, nil
}

// SetServerMonitoring enables/disables the agent's metrics collection for
// serverID and persists the desired state on the server record so it's
// re-applied by SyncServer (e.g. after a redeploy, which starts the agent's
// collector back up enabled by default).
func (s *Service) SetServerMonitoring(serverID string, enabled bool) (*models.Server, error) {
	sID, err := uuid.Parse(serverID)
	if err != nil {
		return nil, err
	}
	srv, err := s.store.Servers().Get(sID)
	if err != nil {
		return nil, err
	}

	if err := s.callAgent(srv, func(ctx context.Context, c *agentclient.Client) error {
		return c.SetMetricsEnabled(ctx, enabled)
	}); err != nil {
		return nil, err
	}

	srv.Agent.MonitoringDisabled = !enabled
	if err := s.store.Servers().Set(srv); err != nil {
		return nil, err
	}
	return srv, nil
}

// mtlsHTTPClient builds an *http.Client that presents tlsCfg.Client's
// certificate and trusts tlsCfg.CA, mirroring the server side built by
// agent/cmd/awg-agent.go's buildTLSConfig.
func mtlsHTTPClient(tlsCfg *models.AgentTLS) (*http.Client, error) {
	clientCert, err := tls.X509KeyPair([]byte(tlsCfg.Client.Certificate), []byte(tlsCfg.Client.PrivateKey))
	if err != nil {
		return nil, fmt.Errorf("parse client cert/key: %w", err)
	}

	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM([]byte(tlsCfg.CA.Certificate)) {
		return nil, fmt.Errorf("CA cert contains no valid certificates")
	}

	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			Certificates: []tls.Certificate{clientCert},
			RootCAs:      pool,
			MinVersion:   tls.VersionTLS12,
		},
	}
	return &http.Client{Transport: transport}, nil
}
