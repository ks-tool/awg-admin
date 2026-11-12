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
	"uuid"

	"github.com/ks-tool/vpn-admin/models"
	"github.com/ks-tool/vpn-admin/storage"
	agentmodels "github.com/ks-tool/vpn-agent/v2/models"
)

// loadTopology projects the existing AWG inventory into the neutral graph.
// Legacy servers and interfaces keep their IDs and remain authoritative for
// names/addresses; the topology store adds positions, services and connections.
func (s *Service) loadTopology() (*models.Topology, error) {
	stored, err := s.store.Topology().Get()
	if err != nil {
		return nil, err
	}
	legacyServers, err := s.store.Servers().List()
	if err != nil {
		return nil, err
	}

	serverPositions := make(map[uuid.UUID]models.TopologyServer, len(stored.Servers))
	interfaceMetadata := make(map[uuid.UUID]models.NetworkInterface, len(stored.Interfaces))
	for _, server := range stored.Servers {
		serverPositions[server.ID] = server
	}
	for _, iface := range stored.Interfaces {
		interfaceMetadata[iface.ID] = iface
	}

	legacyInterfaceIDs := make(map[uuid.UUID]struct{})
	type legacyProjection struct {
		server     models.TopologyServer
		interfaces []models.NetworkInterface
	}
	projections := make([]legacyProjection, 0, len(legacyServers))
	for _, legacy := range legacyServers {
		server := models.TopologyServer{
			ID: legacy.ID, Name: legacy.Name, Address: legacy.SSH.Host, Description: legacy.Info.Description, Location: legacy.Info.Location, Legacy: true,
			X: 80, Y: 80,
		}
		if previous, ok := serverPositions[legacy.ID]; ok {
			server.X, server.Y = previous.X, previous.Y
			if previous.Location == server.Location {
				server.Latitude, server.Longitude = previous.Latitude, previous.Longitude
				server.CoordinatesManual = previous.CoordinatesManual
			}
		}
		projection := legacyProjection{server: server}
		interfaces, err := s.store.Servers().Interfaces(legacy.ID).List()
		if err != nil {
			return nil, err
		}
		for _, legacyIface := range interfaces {
			legacyInterfaceIDs[legacyIface.ID] = struct{}{}
			iface := models.NetworkInterface{
				ID: legacyIface.ID, ServerID: legacy.ID, Name: legacyIface.Interface, Address: legacyIface.Address, Legacy: true,
				X: 420, Y: 80,
			}
			if previous, ok := interfaceMetadata[legacyIface.ID]; ok {
				iface.X, iface.Y = previous.X, previous.Y
				iface.ServiceID = previous.ServiceID
			}
			projection.interfaces = append(projection.interfaces, iface)
		}
		projections = append(projections, projection)
	}

	result := &models.Topology{
		Servers:     make([]models.TopologyServer, 0, len(stored.Servers)+len(legacyServers)),
		Interfaces:  make([]models.NetworkInterface, 0, len(stored.Interfaces)+len(legacyInterfaceIDs)),
		Services:    make([]models.VPNService, 0, len(stored.Services)),
		Connections: make([]models.NetworkConnection, 0, len(stored.Connections)),
	}
	for _, server := range stored.Servers {
		if !server.Legacy {
			result.Servers = append(result.Servers, server)
		}
	}
	for _, iface := range stored.Interfaces {
		if !iface.Legacy {
			result.Interfaces = append(result.Interfaces, iface)
		}
	}
	for _, projection := range projections {
		if _, positioned := serverPositions[projection.server.ID]; !positioned {
			projection.server.Y += float64(len(result.Servers)) * 150
		}
		result.Servers = append(result.Servers, projection.server)
		for _, iface := range projection.interfaces {
			if _, positioned := interfaceMetadata[iface.ID]; !positioned {
				iface.Y += float64(len(result.Interfaces)) * 120
			}
			result.Interfaces = append(result.Interfaces, iface)
		}
	}

	serverIDs := make(map[uuid.UUID]struct{}, len(result.Servers))
	interfaceIDs := make(map[uuid.UUID]struct{}, len(result.Interfaces))
	for _, server := range result.Servers {
		serverIDs[server.ID] = struct{}{}
	}
	for _, iface := range result.Interfaces {
		interfaceIDs[iface.ID] = struct{}{}
	}
	for _, service := range stored.Services {
		if _, exists := serverIDs[service.ServerID]; !exists {
			continue
		}
		if service.InterfaceID != nil {
			if _, exists := interfaceIDs[*service.InterfaceID]; !exists {
				continue
			}
		}
		result.Services = append(result.Services, service)
	}
	for _, connection := range stored.Connections {
		_, sourceExists := interfaceIDs[connection.SourceID]
		_, targetExists := interfaceIDs[connection.TargetID]
		if sourceExists && targetExists {
			result.Connections = append(result.Connections, connection)
		}
	}
	return result, nil
}

func (s *Service) legacyServer(id uuid.UUID) (*models.Server, bool, error) {
	server, err := s.store.Servers().Get(id)
	if storage.IsNotFound(err) {
		return nil, false, nil
	}
	return server, err == nil, err
}

func (s *Service) updateLegacyServer(id uuid.UUID, in TopologyServerInput) (bool, error) {
	server, legacy, err := s.legacyServer(id)
	if err != nil || !legacy {
		return legacy, err
	}
	server.Name = in.Name
	server.SSH.Host = in.Address
	server.Info.Description = in.Description
	server.Info.Location = in.Location
	return true, s.store.Servers().Set(server)
}

func (s *Service) deleteLegacyServer(id uuid.UUID) (bool, error) {
	_, legacy, err := s.legacyServer(id)
	if err != nil || !legacy {
		return legacy, err
	}
	return true, s.DeleteServer(id.String())
}

func (s *Service) createLegacyInterface(serverID uuid.UUID, in NetworkInterfaceInput) (*models.Interface, bool, error) {
	_, legacy, err := s.legacyServer(serverID)
	if err != nil || !legacy {
		return nil, legacy, err
	}
	iface, err := s.CreateInterface(serverID.String(), agentmodels.InterfaceConfig{Interface: in.Name, Address: in.Address})
	return iface, true, err
}

func (s *Service) updateLegacyInterface(serverID, ifaceID uuid.UUID, in NetworkInterfaceInput) (*models.Interface, bool, error) {
	iface, err := s.store.Servers().Interfaces(serverID).Get(ifaceID)
	if storage.IsNotFound(err) {
		return nil, false, nil
	}
	if err != nil {
		return nil, true, err
	}
	cfg := iface.InterfaceConfig
	cfg.Interface = in.Name
	cfg.Address = in.Address
	updated, err := s.UpdateInterfaceConfig(serverID.String(), ifaceID.String(), cfg)
	return updated, true, err
}

func (s *Service) deleteLegacyInterface(serverID, ifaceID uuid.UUID) (bool, error) {
	_, err := s.store.Servers().Interfaces(serverID).Get(ifaceID)
	if storage.IsNotFound(err) {
		return false, nil
	}
	if err != nil {
		return true, err
	}
	return true, s.DeleteInterface(serverID.String(), ifaceID.String())
}
