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
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ks-tool/awg-admin/internal/plugin"
	"github.com/ks-tool/awg-admin/internal/plugin/awg"
	"github.com/ks-tool/awg-admin/models"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
)

const pluginCallTimeout = 30 * time.Second

func defaultPluginRegistry() *plugin.Registry {
	registry := plugin.NewRegistry()
	if err := registry.Register(awg.New()); err != nil {
		log.Error().Err(err).Msg("failed to register built-in AWG plugin")
	}
	dir := os.Getenv("AWG_ADMIN_PLUGINS_DIR")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err == nil {
			dir = filepath.Join(home, ".awg-admin-plugins")
		}
	}
	if dir != "" {
		if err := registry.LoadDir(dir); err != nil {
			log.Error().Err(err).Str("dir", dir).Msg("failed to load VPN plugins")
		}
	}
	return registry
}

type TopologyServerInput struct {
	Name        string                  `json:"name"`
	Address     string                  `json:"address"`
	Description string                  `json:"description"`
	Location    string                  `json:"location"`
	Latitude    *float64                `json:"latitude"`
	Longitude   *float64                `json:"longitude"`
	Interfaces  []NetworkInterfaceInput `json:"interfaces"`
}

type NetworkInterfaceInput struct {
	Name    string `json:"name"`
	Address string `json:"address"`
}

type VPNServiceInput struct {
	ServerID    string         `json:"serverId"`
	InterfaceID string         `json:"interfaceId"`
	PluginID    string         `json:"pluginId"`
	Name        string         `json:"name"`
	Config      map[string]any `json:"config"`
}

func (s *Service) GetTopology() (*models.Topology, error) {
	debugOp("GetTopology").Msg("getting VPN topology")
	s.topologyMu.Lock()
	defer s.topologyMu.Unlock()
	topology, err := s.loadTopology()
	if err != nil {
		return nil, err
	}
	return topology, s.resolveTopologyLocations(topology)
}

func (s *Service) ListVPNPlugins() ([]models.PluginDescriptor, error) {
	debugOp("ListVPNPlugins").Msg("listing VPN plugins")
	ctx, cancel := context.WithTimeout(context.Background(), pluginCallTimeout)
	defer cancel()
	return s.plugins.List(ctx), nil
}

func topologyServerIndex(topology *models.Topology, id uuid.UUID) int {
	for index := range topology.Servers {
		if topology.Servers[index].ID == id {
			return index
		}
	}
	return -1
}

func networkInterfaceIndex(topology *models.Topology, id uuid.UUID) int {
	for index := range topology.Interfaces {
		if topology.Interfaces[index].ID == id {
			return index
		}
	}
	return -1
}

func vpnServiceIndex(topology *models.Topology, id uuid.UUID) int {
	for index := range topology.Services {
		if topology.Services[index].ID == id {
			return index
		}
	}
	return -1
}

func parseTopologyID(value, field string) (uuid.UUID, error) {
	id, err := uuid.Parse(value)
	if err != nil {
		return uuid.Nil, invalidInput("invalid %s", field)
	}
	return id, nil
}

func (s *Service) CreateTopologyServer(in TopologyServerInput) (*models.Topology, error) {
	debugOp("CreateTopologyServer").Str("name", in.Name).Msg("creating topology server")
	if strings.TrimSpace(in.Name) == "" {
		return nil, invalidInput("server name is required")
	}
	location := strings.TrimSpace(in.Location)
	if location == "" {
		if err := validateServerCoordinates(in.Latitude, in.Longitude); err != nil {
			return nil, err
		}
	}
	s.topologyMu.Lock()
	defer s.topologyMu.Unlock()
	topology, err := s.loadTopology()
	if err != nil {
		return nil, err
	}
	server := models.TopologyServer{
		ID: uuid.New(), Name: strings.TrimSpace(in.Name), Address: strings.TrimSpace(in.Address), Description: strings.TrimSpace(in.Description), Location: location,
		X: 80, Y: 80 + float64(len(topology.Servers))*150,
	}
	if location == "" && in.Latitude != nil {
		server.Latitude = in.Latitude
		server.Longitude = in.Longitude
		server.CoordinatesManual = true
	}
	topology.Servers = append(topology.Servers, server)
	seen := make(map[string]struct{}, len(in.Interfaces))
	for index, input := range in.Interfaces {
		name := strings.TrimSpace(input.Name)
		if name == "" {
			return nil, invalidInput("interface %d name is required", index+1)
		}
		if _, exists := seen[name]; exists {
			return nil, invalidInput("interface name %q is duplicated", name)
		}
		seen[name] = struct{}{}
		topology.Interfaces = append(topology.Interfaces, models.NetworkInterface{
			ID: uuid.New(), ServerID: server.ID, Name: name, Address: strings.TrimSpace(input.Address),
			X: 420, Y: 80 + float64(len(topology.Interfaces))*120,
		})
	}
	return topology, s.store.Topology().Set(topology)
}

func (s *Service) UpdateTopologyServer(id string, in TopologyServerInput) (*models.Topology, error) {
	serverID, err := parseTopologyID(id, "server id")
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(in.Name) == "" {
		return nil, invalidInput("server name is required")
	}
	location := strings.TrimSpace(in.Location)
	if location == "" {
		if err := validateServerCoordinates(in.Latitude, in.Longitude); err != nil {
			return nil, err
		}
	}
	s.topologyMu.Lock()
	defer s.topologyMu.Unlock()
	topology, err := s.loadTopology()
	if err != nil {
		return nil, err
	}
	index := topologyServerIndex(topology, serverID)
	if index < 0 {
		return nil, invalidInput("topology server not found")
	}
	if _, err := s.updateLegacyServer(serverID, in); err != nil {
		return nil, err
	}
	server := &topology.Servers[index]
	server.Name = strings.TrimSpace(in.Name)
	server.Address = strings.TrimSpace(in.Address)
	server.Description = strings.TrimSpace(in.Description)
	if server.Location != location {
		server.Latitude = nil
		server.Longitude = nil
	}
	server.Location = location
	server.LocationError = ""
	server.CoordinatesManual = false
	if location == "" {
		server.Latitude = in.Latitude
		server.Longitude = in.Longitude
		server.CoordinatesManual = in.Latitude != nil
	}
	return topology, s.store.Topology().Set(topology)
}

func (s *Service) DeleteTopologyServer(id string) error {
	serverID, err := parseTopologyID(id, "server id")
	if err != nil {
		return err
	}
	s.topologyMu.Lock()
	defer s.topologyMu.Unlock()
	topology, err := s.loadTopology()
	if err != nil {
		return err
	}
	index := topologyServerIndex(topology, serverID)
	if index < 0 {
		return invalidInput("topology server not found")
	}
	for _, service := range topology.Services {
		if service.ServerID == serverID {
			return invalidInput("remove VPN services from server %q first", topology.Servers[index].Name)
		}
	}
	if _, err := s.deleteLegacyServer(serverID); err != nil {
		return err
	}
	removedInterfaces := make(map[uuid.UUID]struct{})
	interfaces := topology.Interfaces[:0]
	for _, iface := range topology.Interfaces {
		if iface.ServerID == serverID {
			removedInterfaces[iface.ID] = struct{}{}
			continue
		}
		interfaces = append(interfaces, iface)
	}
	topology.Interfaces = interfaces
	topology.Connections = filterConnections(topology.Connections, removedInterfaces)
	topology.Servers = append(topology.Servers[:index], topology.Servers[index+1:]...)
	return s.store.Topology().Set(topology)
}

func filterConnections(connections []models.NetworkConnection, removed map[uuid.UUID]struct{}) []models.NetworkConnection {
	result := connections[:0]
	for _, connection := range connections {
		_, sourceRemoved := removed[connection.SourceID]
		_, targetRemoved := removed[connection.TargetID]
		if !sourceRemoved && !targetRemoved {
			result = append(result, connection)
		}
	}
	return result
}

func (s *Service) CreateNetworkInterface(serverID string, in NetworkInterfaceInput) (*models.Topology, error) {
	id, err := parseTopologyID(serverID, "server id")
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(in.Name) == "" {
		return nil, invalidInput("interface name is required")
	}
	s.topologyMu.Lock()
	defer s.topologyMu.Unlock()
	topology, err := s.loadTopology()
	if err != nil {
		return nil, err
	}
	if topologyServerIndex(topology, id) < 0 {
		return nil, invalidInput("topology server not found")
	}
	for _, iface := range topology.Interfaces {
		if iface.ServerID == id && iface.Name == strings.TrimSpace(in.Name) {
			return nil, invalidInput("interface %q already exists on this server", in.Name)
		}
	}
	in.Name = strings.TrimSpace(in.Name)
	in.Address = strings.TrimSpace(in.Address)
	if created, legacy, err := s.createLegacyInterface(id, in); legacy {
		if err != nil {
			return nil, err
		}
		topology.Interfaces = append(topology.Interfaces, models.NetworkInterface{
			ID: created.ID, ServerID: id, Name: created.Interface, Address: created.Address, Legacy: true,
			X: 420, Y: 80 + float64(len(topology.Interfaces))*120,
		})
		return topology, s.store.Topology().Set(topology)
	}
	topology.Interfaces = append(topology.Interfaces, models.NetworkInterface{
		ID: uuid.New(), ServerID: id, Name: in.Name, Address: in.Address,
		X: 420, Y: 80 + float64(len(topology.Interfaces))*120,
	})
	return topology, s.store.Topology().Set(topology)
}

func (s *Service) UpdateNetworkInterface(id string, in NetworkInterfaceInput) (*models.Topology, error) {
	ifaceID, err := parseTopologyID(id, "interface id")
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(in.Name) == "" {
		return nil, invalidInput("interface name is required")
	}
	s.topologyMu.Lock()
	defer s.topologyMu.Unlock()
	topology, err := s.loadTopology()
	if err != nil {
		return nil, err
	}
	index := networkInterfaceIndex(topology, ifaceID)
	if index < 0 {
		return nil, invalidInput("network interface not found")
	}
	iface := &topology.Interfaces[index]
	for _, other := range topology.Interfaces {
		if other.ID != ifaceID && other.ServerID == iface.ServerID && other.Name == strings.TrimSpace(in.Name) {
			return nil, invalidInput("interface %q already exists on this server", in.Name)
		}
	}
	in.Name = strings.TrimSpace(in.Name)
	in.Address = strings.TrimSpace(in.Address)
	if updated, legacy, err := s.updateLegacyInterface(iface.ServerID, ifaceID, in); legacy {
		if err != nil {
			return nil, err
		}
		iface.Name = updated.Interface
		iface.Address = updated.Address
	} else {
		iface.Name = in.Name
		iface.Address = in.Address
	}
	return topology, s.store.Topology().Set(topology)
}

func (s *Service) DeleteNetworkInterface(id string) error {
	ifaceID, err := parseTopologyID(id, "interface id")
	if err != nil {
		return err
	}
	s.topologyMu.Lock()
	defer s.topologyMu.Unlock()
	topology, err := s.loadTopology()
	if err != nil {
		return err
	}
	index := networkInterfaceIndex(topology, ifaceID)
	if index < 0 {
		return invalidInput("network interface not found")
	}
	if topology.Interfaces[index].ServiceID != nil {
		return invalidInput("remove the VPN service attached to interface %q first", topology.Interfaces[index].Name)
	}
	iface := topology.Interfaces[index]
	if _, err := s.deleteLegacyInterface(iface.ServerID, ifaceID); err != nil {
		return err
	}
	removed := map[uuid.UUID]struct{}{ifaceID: {}}
	topology.Connections = filterConnections(topology.Connections, removed)
	topology.Interfaces = append(topology.Interfaces[:index], topology.Interfaces[index+1:]...)
	return s.store.Topology().Set(topology)
}

func validateServiceInterface(topology *models.Topology, serverID uuid.UUID, interfaceID string, serviceID uuid.UUID) (*uuid.UUID, error) {
	if interfaceID == "" {
		return nil, nil
	}
	id, err := parseTopologyID(interfaceID, "interface id")
	if err != nil {
		return nil, err
	}
	index := networkInterfaceIndex(topology, id)
	if index < 0 || topology.Interfaces[index].ServerID != serverID {
		return nil, invalidInput("interface must belong to the service server")
	}
	attached := topology.Interfaces[index].ServiceID
	if attached != nil && *attached != serviceID {
		return nil, invalidInput("interface %q already has a VPN service", topology.Interfaces[index].Name)
	}
	return &id, nil
}

func (s *Service) CreateVPNService(in VPNServiceInput) (*models.VPNService, error) {
	debugOp("CreateVPNService").Str("name", in.Name).Str("plugin_id", in.PluginID).Msg("creating VPN service")
	serverID, err := parseTopologyID(in.ServerID, "server id")
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(in.Name) == "" {
		return nil, invalidInput("service name is required")
	}
	if _, err := s.plugins.Get(in.PluginID); err != nil {
		return nil, invalidInput("%s", err)
	}
	s.topologyMu.Lock()
	defer s.topologyMu.Unlock()
	topology, err := s.loadTopology()
	if err != nil {
		return nil, err
	}
	if topologyServerIndex(topology, serverID) < 0 {
		return nil, invalidInput("topology server not found")
	}
	service := models.VPNService{ID: uuid.New(), ServerID: serverID, PluginID: in.PluginID, Name: strings.TrimSpace(in.Name), Config: in.Config, Status: "draft"}
	if service.Config == nil {
		service.Config = make(map[string]any)
	}
	service.InterfaceID, err = validateServiceInterface(topology, serverID, in.InterfaceID, service.ID)
	if err != nil {
		return nil, err
	}
	if service.InterfaceID != nil {
		index := networkInterfaceIndex(topology, *service.InterfaceID)
		topology.Interfaces[index].ServiceID = &service.ID
	}
	topology.Services = append(topology.Services, service)
	if err := s.store.Topology().Set(topology); err != nil {
		return nil, err
	}
	result := topology.Services[len(topology.Services)-1]
	return &result, nil
}

func (s *Service) UpdateVPNService(id string, in VPNServiceInput) (*models.VPNService, error) {
	serviceID, err := parseTopologyID(id, "service id")
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(in.Name) == "" {
		return nil, invalidInput("service name is required")
	}
	s.topologyMu.Lock()
	defer s.topologyMu.Unlock()
	topology, err := s.loadTopology()
	if err != nil {
		return nil, err
	}
	index := vpnServiceIndex(topology, serviceID)
	if index < 0 {
		return nil, invalidInput("VPN service not found")
	}
	service := &topology.Services[index]
	interfaceID, err := validateServiceInterface(topology, service.ServerID, in.InterfaceID, serviceID)
	if err != nil {
		return nil, err
	}
	for ifaceIndex := range topology.Interfaces {
		iface := &topology.Interfaces[ifaceIndex]
		if iface.ServiceID != nil && *iface.ServiceID == serviceID {
			iface.ServiceID = nil
		}
	}
	if interfaceID != nil {
		ifaceIndex := networkInterfaceIndex(topology, *interfaceID)
		topology.Interfaces[ifaceIndex].ServiceID = &service.ID
	}
	service.InterfaceID = interfaceID
	service.Name = strings.TrimSpace(in.Name)
	service.Config = in.Config
	if service.Config == nil {
		service.Config = make(map[string]any)
	}
	service.Status = "pending"
	service.StatusMessage = "Configuration changed; apply it to update the running service"
	if err := s.store.Topology().Set(topology); err != nil {
		return nil, err
	}
	result := *service
	return &result, nil
}

func deploymentFor(topology *models.Topology, service models.VPNService) (plugin.Deployment, error) {
	deployment := plugin.Deployment{Service: service, Connections: make([]models.NetworkConnection, 0)}
	serverIndex := topologyServerIndex(topology, service.ServerID)
	if serverIndex < 0 {
		return deployment, fmt.Errorf("service server not found")
	}
	deployment.Server = topology.Servers[serverIndex]
	if service.InterfaceID != nil {
		index := networkInterfaceIndex(topology, *service.InterfaceID)
		if index < 0 {
			return deployment, fmt.Errorf("service interface not found")
		}
		iface := topology.Interfaces[index]
		deployment.Interface = &iface
		for _, connection := range topology.Connections {
			if connection.SourceID == iface.ID || connection.TargetID == iface.ID {
				deployment.Connections = append(deployment.Connections, connection)
			}
		}
	}
	return deployment, nil
}

func (s *Service) serviceDeployment(id string) (plugin.Deployment, Plugin, error) {
	serviceID, err := parseTopologyID(id, "service id")
	if err != nil {
		return plugin.Deployment{}, nil, err
	}
	topology, err := s.loadTopology()
	if err != nil {
		return plugin.Deployment{}, nil, err
	}
	index := vpnServiceIndex(topology, serviceID)
	if index < 0 {
		return plugin.Deployment{}, nil, invalidInput("VPN service not found")
	}
	service := topology.Services[index]
	p, err := s.plugins.Get(service.PluginID)
	if err != nil {
		return plugin.Deployment{}, nil, err
	}
	deployment, err := deploymentFor(topology, service)
	return deployment, p, err
}

// Plugin is the small service-facing subset, kept as an alias to avoid exposing
// registry internals to callers.
type Plugin = plugin.Plugin

func (s *Service) ApplyVPNService(id string) (*models.VPNService, error) {
	debugOp("ApplyVPNService").Str("service_id", id).Msg("applying VPN service")
	s.topologyMu.Lock()
	deployment, p, err := s.serviceDeployment(id)
	s.topologyMu.Unlock()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), pluginCallTimeout)
	defer cancel()
	status, applyErr := p.Apply(ctx, deployment)
	s.topologyMu.Lock()
	defer s.topologyMu.Unlock()
	topology, err := s.loadTopology()
	if err != nil {
		return nil, err
	}
	index := vpnServiceIndex(topology, deployment.Service.ID)
	if index < 0 {
		return nil, invalidInput("VPN service not found")
	}
	service := &topology.Services[index]
	if applyErr != nil {
		service.Status = "error"
		service.StatusMessage = applyErr.Error()
	} else {
		service.Status = status.State
		if service.Status == "" {
			service.Status = "running"
		}
		service.StatusMessage = status.Message
		now := time.Now()
		service.LastAppliedAt = &now
	}
	if err := s.store.Topology().Set(topology); err != nil {
		return nil, err
	}
	result := *service
	if applyErr != nil {
		return &result, applyErr
	}
	return &result, nil
}

func (s *Service) RefreshVPNServiceStatus(id string) (*models.VPNService, error) {
	s.topologyMu.Lock()
	deployment, p, err := s.serviceDeployment(id)
	s.topologyMu.Unlock()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), pluginCallTimeout)
	defer cancel()
	status, statusErr := p.Status(ctx, deployment)
	if statusErr != nil {
		return nil, statusErr
	}
	s.topologyMu.Lock()
	defer s.topologyMu.Unlock()
	topology, err := s.loadTopology()
	if err != nil {
		return nil, err
	}
	index := vpnServiceIndex(topology, deployment.Service.ID)
	if index < 0 {
		return nil, invalidInput("VPN service not found")
	}
	service := &topology.Services[index]
	service.Status = status.State
	service.StatusMessage = status.Message
	if err := s.store.Topology().Set(topology); err != nil {
		return nil, err
	}
	result := *service
	return &result, nil
}

func (s *Service) DeleteVPNService(id string) error {
	debugOp("DeleteVPNService").Str("service_id", id).Msg("deleting VPN service")
	s.topologyMu.Lock()
	deployment, p, err := s.serviceDeployment(id)
	s.topologyMu.Unlock()
	if err != nil {
		return err
	}
	if deployment.Service.Status != "draft" {
		ctx, cancel := context.WithTimeout(context.Background(), pluginCallTimeout)
		defer cancel()
		if err := p.Delete(ctx, deployment); err != nil {
			return fmt.Errorf("remove deployed service: %w", err)
		}
	}
	s.topologyMu.Lock()
	defer s.topologyMu.Unlock()
	topology, err := s.loadTopology()
	if err != nil {
		return err
	}
	index := vpnServiceIndex(topology, deployment.Service.ID)
	if index < 0 {
		return nil
	}
	topology.Services = append(topology.Services[:index], topology.Services[index+1:]...)
	for ifaceIndex := range topology.Interfaces {
		attached := topology.Interfaces[ifaceIndex].ServiceID
		if attached != nil && *attached == deployment.Service.ID {
			topology.Interfaces[ifaceIndex].ServiceID = nil
		}
	}
	return s.store.Topology().Set(topology)
}

func (s *Service) CreateNetworkConnection(sourceID, targetID string) (*models.NetworkConnection, error) {
	source, err := parseTopologyID(sourceID, "source interface id")
	if err != nil {
		return nil, err
	}
	target, err := parseTopologyID(targetID, "target interface id")
	if err != nil {
		return nil, err
	}
	if source == target {
		return nil, invalidInput("an interface cannot connect to itself")
	}
	s.topologyMu.Lock()
	defer s.topologyMu.Unlock()
	topology, err := s.loadTopology()
	if err != nil {
		return nil, err
	}
	if networkInterfaceIndex(topology, source) < 0 || networkInterfaceIndex(topology, target) < 0 {
		return nil, invalidInput("both connection interfaces must exist")
	}
	for _, connection := range topology.Connections {
		if connection.SourceID == source && connection.TargetID == target {
			return nil, invalidInput("this connection already exists")
		}
	}
	if connectionCreatesCycle(topology.Connections, source, target) {
		return nil, invalidInput("this connection would create a cycle")
	}
	connection := models.NetworkConnection{ID: uuid.New(), SourceID: source, TargetID: target}
	topology.Connections = append(topology.Connections, connection)
	if err := s.store.Topology().Set(topology); err != nil {
		return nil, err
	}
	return &connection, nil
}

func connectionCreatesCycle(connections []models.NetworkConnection, source, target uuid.UUID) bool {
	next := make(map[uuid.UUID][]uuid.UUID, len(connections))
	for _, connection := range connections {
		next[connection.SourceID] = append(next[connection.SourceID], connection.TargetID)
	}
	visited := make(map[uuid.UUID]struct{}, len(next))
	pending := []uuid.UUID{target}
	for len(pending) > 0 {
		current := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		if current == source {
			return true
		}
		if _, seen := visited[current]; seen {
			continue
		}
		visited[current] = struct{}{}
		pending = append(pending, next[current]...)
	}
	return false
}

func (s *Service) DeleteNetworkConnection(id string) error {
	connectionID, err := parseTopologyID(id, "connection id")
	if err != nil {
		return err
	}
	s.topologyMu.Lock()
	defer s.topologyMu.Unlock()
	topology, err := s.loadTopology()
	if err != nil {
		return err
	}
	for index, connection := range topology.Connections {
		if connection.ID == connectionID {
			topology.Connections = append(topology.Connections[:index], topology.Connections[index+1:]...)
			return s.store.Topology().Set(topology)
		}
	}
	return invalidInput("network connection not found")
}

func (s *Service) MoveTopologyNodes(positions []models.NodePosition) error {
	s.topologyMu.Lock()
	defer s.topologyMu.Unlock()
	topology, err := s.loadTopology()
	if err != nil {
		return err
	}
	for _, position := range positions {
		if index := topologyServerIndex(topology, position.ID); index >= 0 {
			topology.Servers[index].X = position.X
			topology.Servers[index].Y = position.Y
		}
		if index := networkInterfaceIndex(topology, position.ID); index >= 0 {
			topology.Interfaces[index].X = position.X
			topology.Interfaces[index].Y = position.Y
		}
	}
	return s.store.Topology().Set(topology)
}

func (s *Service) ListVPNServiceUsers(id string) ([]models.PluginUser, error) {
	deployment, p, err := s.serviceDeployment(id)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), pluginCallTimeout)
	defer cancel()
	return p.ListUsers(ctx, deployment)
}

func (s *Service) CreateVPNServiceUser(id string, input map[string]any) (*models.PluginUser, error) {
	deployment, p, err := s.serviceDeployment(id)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), pluginCallTimeout)
	defer cancel()
	user, err := p.CreateUser(ctx, deployment, input)
	return &user, err
}

func (s *Service) UpdateVPNServiceUser(id, userID string, input map[string]any) (*models.PluginUser, error) {
	deployment, p, err := s.serviceDeployment(id)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), pluginCallTimeout)
	defer cancel()
	user, err := p.UpdateUser(ctx, deployment, userID, input)
	return &user, err
}

func (s *Service) DeleteVPNServiceUser(id, userID string) error {
	deployment, p, err := s.serviceDeployment(id)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), pluginCallTimeout)
	defer cancel()
	return p.DeleteUser(ctx, deployment, userID)
}
