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
	"fmt"
	"path/filepath"
	"testing"
	"uuid"

	"github.com/ks-tool/vpn-admin/internal/plugin"
	"github.com/ks-tool/vpn-admin/models"
	"github.com/ks-tool/vpn-admin/storage/boltdb"
)

type topologyGeocoderFunc func(context.Context, string) (float64, float64, error)

func (f topologyGeocoderFunc) Geocode(ctx context.Context, location string) (float64, float64, error) {
	return f(ctx, location)
}

type topologyPlugin struct {
	users      []models.PluginUser
	deployment plugin.Deployment
}

func (p *topologyPlugin) Describe(context.Context) (models.PluginDescriptor, error) {
	return models.PluginDescriptor{ID: "test-vpn", Name: "Test VPN", ConfigSchema: map[string]any{"type": "object"}, UserSchema: map[string]any{"type": "object"}}, nil
}

func (p *topologyPlugin) Apply(_ context.Context, deployment plugin.Deployment) (models.PluginStatus, error) {
	p.deployment = deployment
	return models.PluginStatus{State: "running"}, nil
}

func (p *topologyPlugin) Delete(context.Context, plugin.Deployment) error { return nil }
func (p *topologyPlugin) Status(context.Context, plugin.Deployment) (models.PluginStatus, error) {
	return models.PluginStatus{State: "running"}, nil
}
func (p *topologyPlugin) ListUsers(context.Context, plugin.Deployment) ([]models.PluginUser, error) {
	return p.users, nil
}
func (p *topologyPlugin) CreateUser(_ context.Context, _ plugin.Deployment, data map[string]any) (models.PluginUser, error) {
	user := models.PluginUser{ID: "user-1", Data: data}
	p.users = append(p.users, user)
	return user, nil
}
func (p *topologyPlugin) UpdateUser(_ context.Context, _ plugin.Deployment, id string, data map[string]any) (models.PluginUser, error) {
	for index := range p.users {
		if p.users[index].ID == id {
			p.users[index].Data = data
			return p.users[index], nil
		}
	}
	return models.PluginUser{}, nil
}
func (p *topologyPlugin) DeleteUser(_ context.Context, _ plugin.Deployment, id string) error {
	for index := range p.users {
		if p.users[index].ID == id {
			p.users = append(p.users[:index], p.users[index+1:]...)
		}
	}
	return nil
}

func TestTopologyServiceLifecycle(t *testing.T) {
	db, err := boltdb.Open(filepath.Join(t.TempDir(), "admin.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	adapter := &topologyPlugin{}
	registry := plugin.NewRegistry()
	if err := registry.Register(adapter); err != nil {
		t.Fatal(err)
	}
	svc := New(db, WithPluginRegistry(registry))

	_, err = svc.CreateTopologyServer(TopologyServerInput{Name: "entry", Interfaces: []NetworkInterfaceInput{{Name: "vpn0", Address: "10.0.0.1/24"}}})
	if err != nil {
		t.Fatal(err)
	}
	topology, err := svc.CreateTopologyServer(TopologyServerInput{Name: "exit", Interfaces: []NetworkInterfaceInput{{Name: "vpn0"}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(topology.Servers) != 2 || len(topology.Interfaces) != 2 {
		t.Fatalf("unexpected topology: %#v", topology)
	}
	source := topology.Interfaces[0]
	target := topology.Interfaces[1]
	if _, err := svc.CreateNetworkConnection(source.ID.String(), source.ID.String()); err == nil {
		t.Fatal("expected self-connection to be rejected")
	}
	connection, err := svc.CreateNetworkConnection(source.ID.String(), target.ID.String())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreateNetworkConnection(target.ID.String(), source.ID.String()); err == nil {
		t.Fatal("expected reverse connection to be rejected as a cycle")
	}

	service, err := svc.CreateVPNService(VPNServiceInput{
		ServerID: source.ServerID.String(), InterfaceID: source.ID.String(), PluginID: "test-vpn", Name: "Entry VPN",
		Config: map[string]any{"protocolSpecific": true},
	})
	if err != nil {
		t.Fatal(err)
	}
	service, err = svc.ApplyVPNService(service.ID.String())
	if err != nil {
		t.Fatal(err)
	}
	if service.Status != "running" || adapter.deployment.Interface == nil || len(adapter.deployment.Connections) != 1 {
		t.Fatalf("plugin received incomplete deployment: %#v", adapter.deployment)
	}
	if adapter.deployment.Service.Config["protocolSpecific"] != true {
		t.Fatal("opaque service config was not passed to plugin")
	}

	if _, err := svc.CreateVPNServiceUser(service.ID.String(), map[string]any{"name": "alice"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.UpdateVPNServiceUser(service.ID.String(), "user-1", map[string]any{"name": "alice2"}); err != nil {
		t.Fatal(err)
	}
	users, err := svc.ListVPNServiceUsers(service.ID.String())
	if err != nil || len(users) != 1 || users[0].Data["name"] != "alice2" {
		t.Fatalf("unexpected users: %#v, %v", users, err)
	}
	if err := svc.DeleteVPNServiceUser(service.ID.String(), "user-1"); err != nil {
		t.Fatal(err)
	}

	service, err = svc.UpdateVPNService(service.ID.String(), VPNServiceInput{InterfaceID: source.ID.String(), Name: "Entry VPN", Config: map[string]any{"version": 2}})
	if err != nil {
		t.Fatal(err)
	}
	if service.Status != "pending" {
		t.Fatalf("updated service status = %q", service.Status)
	}
	if err := svc.DeleteNetworkInterface(source.ID.String()); err == nil {
		t.Fatal("expected attached interface deletion to be rejected")
	}
	if err := svc.DeleteVPNService(service.ID.String()); err != nil {
		t.Fatal(err)
	}
	if err := svc.DeleteNetworkInterface(source.ID.String()); err != nil {
		t.Fatal(err)
	}
	stored, err := svc.GetTopology()
	if err != nil {
		t.Fatal(err)
	}
	for _, current := range stored.Connections {
		if current.ID == connection.ID {
			t.Fatal("connection was not removed with its interface")
		}
	}
}

func TestTopologyServerLocation(t *testing.T) {
	db, err := boltdb.Open(filepath.Join(t.TempDir(), "admin.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	calls := 0
	geocoder := topologyGeocoderFunc(func(_ context.Context, location string) (float64, float64, error) {
		calls++
		if location != "Amsterdam" {
			return 0, 0, fmt.Errorf("unknown location")
		}
		return 52.37, 4.9, nil
	})
	svc := New(db, WithPluginRegistry(plugin.NewRegistry()), withGeocoder(geocoder))
	if _, err := svc.CreateTopologyServer(TopologyServerInput{Name: "NL", Location: "Amsterdam"}); err != nil {
		t.Fatal(err)
	}
	topology, err := svc.GetTopology()
	if err != nil {
		t.Fatal(err)
	}
	server := topology.Servers[0]
	if server.Latitude == nil || *server.Latitude != 52.37 || server.Longitude == nil || *server.Longitude != 4.9 || calls != 1 {
		t.Fatalf("location was not resolved: %#v, calls=%d", server, calls)
	}
	if _, err := svc.GetTopology(); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("resolved location was requested again: calls=%d", calls)
	}

	if _, err := svc.UpdateTopologyServer(server.ID.String(), TopologyServerInput{Name: "NL", Location: "Unknown"}); err != nil {
		t.Fatal(err)
	}
	topology, err = svc.GetTopology()
	if err != nil {
		t.Fatal(err)
	}
	server = topology.Servers[0]
	if server.Latitude != nil || server.Longitude != nil || server.LocationError == "" {
		t.Fatalf("changed location kept stale coordinates: %#v", server)
	}

	latitude, longitude := 55.7558, 37.6173
	if _, err := svc.UpdateTopologyServer(server.ID.String(), TopologyServerInput{Name: "Manual", Latitude: &latitude, Longitude: &longitude}); err != nil {
		t.Fatal(err)
	}
	topology, err = svc.GetTopology()
	if err != nil {
		t.Fatal(err)
	}
	server = topology.Servers[0]
	if !server.CoordinatesManual || server.Latitude == nil || *server.Latitude != latitude || server.Longitude == nil || *server.Longitude != longitude {
		t.Fatalf("manual coordinates were not preserved: %#v", server)
	}
	if _, err := svc.UpdateTopologyServer(server.ID.String(), TopologyServerInput{Name: "NL", Location: "Amsterdam", Latitude: &latitude, Longitude: &longitude}); err != nil {
		t.Fatal(err)
	}
	topology, err = svc.GetTopology()
	if err != nil {
		t.Fatal(err)
	}
	server = topology.Servers[0]
	if server.CoordinatesManual || server.Latitude == nil || *server.Latitude != 52.37 || server.Longitude == nil || *server.Longitude != 4.9 {
		t.Fatalf("location did not take priority over manual coordinates: %#v", server)
	}
}

func TestTopologyProjectsExistingInventory(t *testing.T) {
	db, err := boltdb.Open(filepath.Join(t.TempDir(), "admin.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	serverID := uuid.New()
	interfaceID := uuid.New()
	legacy := &models.Server{
		ID: serverID, Name: "Existing server",
		Info: models.ServerInfo{Description: "Already in BoltDB", Location: "Yekaterinburg"},
		SSH:  models.SSHConfig{Host: "old.example.com", User: "root", Password: "secret"},
	}
	if err := db.Servers().Set(legacy); err != nil {
		t.Fatal(err)
	}
	if err := db.Servers().Interfaces(serverID).Set(&models.Interface{
		ID:        interfaceID,
		Interface: "awg0", Address: "10.20.0.1/24",
	}); err != nil {
		t.Fatal(err)
	}

	latitude, longitude := 56.8389, 60.6057
	geocoder := topologyGeocoderFunc(func(_ context.Context, location string) (float64, float64, error) {
		if location != "Yekaterinburg" {
			return 0, 0, fmt.Errorf("unknown location")
		}
		return latitude, longitude, nil
	})
	svc := New(db, WithPluginRegistry(plugin.NewRegistry()), withGeocoder(geocoder))
	topology, err := svc.GetTopology()
	if err != nil {
		t.Fatal(err)
	}
	if len(topology.Servers) != 1 || len(topology.Interfaces) != 1 {
		t.Fatalf("existing inventory is missing: %#v", topology)
	}
	server := topology.Servers[0]
	iface := topology.Interfaces[0]
	if server.ID != serverID || server.Address != "old.example.com" || !server.Legacy {
		t.Fatalf("unexpected legacy server projection: %#v", server)
	}
	if iface.ID != interfaceID || iface.Name != "awg0" || !iface.Legacy {
		t.Fatalf("unexpected legacy interface projection: %#v", iface)
	}

	if err := svc.MoveTopologyNodes([]models.NodePosition{{ID: serverID, X: 321, Y: 654}}); err != nil {
		t.Fatal(err)
	}
	topology, err = svc.UpdateTopologyServer(serverID.String(), TopologyServerInput{Name: "Renamed", Address: "new.example.com", Description: "Updated", Location: "Yekaterinburg"})
	if err != nil {
		t.Fatal(err)
	}
	if topology.Servers[0].X != 321 || topology.Servers[0].Y != 654 {
		t.Fatalf("legacy position was not preserved: %#v", topology.Servers[0])
	}
	topology, err = svc.GetTopology()
	if err != nil {
		t.Fatal(err)
	}
	if topology.Servers[0].Latitude == nil || *topology.Servers[0].Latitude != latitude || topology.Servers[0].Longitude == nil || *topology.Servers[0].Longitude != longitude {
		t.Fatalf("legacy coordinates were not preserved: %#v", topology.Servers[0])
	}
	storedLegacy, err := db.Servers().Get(serverID)
	if err != nil {
		t.Fatal(err)
	}
	if storedLegacy.Name != "Renamed" || storedLegacy.SSH.Host != "new.example.com" || storedLegacy.Info.Description != "Updated" || storedLegacy.Info.Location != "Yekaterinburg" {
		t.Fatalf("legacy record was not updated: %#v", storedLegacy)
	}
	if storedLegacy.SSH.Password != "secret" {
		t.Fatal("legacy credentials were not preserved")
	}

	if err := svc.DeleteTopologyServer(serverID.String()); err != nil {
		t.Fatal(err)
	}
	topology, err = svc.GetTopology()
	if err != nil {
		t.Fatal(err)
	}
	if len(topology.Servers) != 0 || len(topology.Interfaces) != 0 {
		t.Fatalf("deleted legacy inventory reappeared: %#v", topology)
	}
}

func TestConnectionCreatesCycle(t *testing.T) {
	a, b, c, d := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	connections := []models.NetworkConnection{
		{SourceID: a, TargetID: b},
		{SourceID: b, TargetID: c},
	}
	for _, test := range []struct {
		name           string
		source, target uuid.UUID
		want           bool
	}{
		{name: "direct cycle", source: b, target: a, want: true},
		{name: "indirect cycle", source: c, target: a, want: true},
		{name: "new path", source: c, target: d, want: false},
		{name: "incoming path", source: d, target: a, want: false},
		{name: "shortcut", source: a, target: c, want: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := connectionCreatesCycle(connections, test.source, test.target); got != test.want {
				t.Fatalf("connectionCreatesCycle() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestMoveNetworkConnection(t *testing.T) {
	svc := newTestService(t)
	for _, name := range []string{"a", "b", "c"} {
		if _, err := svc.CreateTopologyServer(TopologyServerInput{Name: name, Interfaces: []NetworkInterfaceInput{{Name: "vpn0"}}}); err != nil {
			t.Fatal(err)
		}
	}
	topology, err := svc.GetTopology()
	if err != nil {
		t.Fatal(err)
	}
	a, b, c := topology.Interfaces[0], topology.Interfaces[1], topology.Interfaces[2]
	connection, err := svc.CreateNetworkConnection(a.ID.String(), b.ID.String())
	if err != nil {
		t.Fatal(err)
	}
	connection, err = svc.MoveNetworkConnection(connection.ID.String(), a.ID.String(), c.ID.String())
	if err != nil {
		t.Fatalf("move target: %v", err)
	}
	if connection.SourceID != a.ID || connection.TargetID != c.ID {
		t.Fatalf("unexpected target move: %+v", connection)
	}
	connection, err = svc.MoveNetworkConnection(connection.ID.String(), b.ID.String(), c.ID.String())
	if err != nil {
		t.Fatalf("move source: %v", err)
	}
	if connection.SourceID != b.ID || connection.TargetID != c.ID {
		t.Fatalf("unexpected source move: %+v", connection)
	}
	if _, err := svc.MoveNetworkConnection(connection.ID.String(), c.ID.String(), c.ID.String()); err == nil {
		t.Fatal("self-connection was accepted")
	}
}
