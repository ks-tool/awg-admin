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
	"encoding/hex"
	"sort"
	"sync"
	"uuid"

	"github.com/ks-tool/vpn-admin/models"
)

type tunnelMetricTarget struct {
	connectionID string
	peerKey      string
}

type tunnelMetricSource struct {
	serverID    uuid.UUID
	interfaceID uuid.UUID
	targets     []tunnelMetricTarget
}

func (s *Service) GetTunnelMetrics() ([]models.TunnelMetric, error) {
	s.topologyMu.Lock()
	topology, err := s.loadTopology()
	s.topologyMu.Unlock()
	if err != nil {
		return nil, err
	}
	links, err := s.store.Links().List()
	if err != nil {
		return nil, err
	}

	interfaces := make(map[uuid.UUID]models.NetworkInterface, len(topology.Interfaces))
	for _, iface := range topology.Interfaces {
		interfaces[iface.ID] = iface
	}
	sources := make(map[string]*tunnelMetricSource)
	add := func(connectionID string, sourceID, targetID uuid.UUID) {
		source, sourceOK := interfaces[sourceID]
		target, targetOK := interfaces[targetID]
		if !sourceOK || !targetOK || !source.Legacy || !target.Legacy || source.ServerID == target.ServerID {
			return
		}
		targetConfig, err := s.store.Servers().Interfaces(target.ServerID).Get(target.ID)
		if err != nil {
			return
		}
		key := targetConfig.PrivateKey.PublicKey()
		sourceKey := source.ServerID.String() + "/" + source.ID.String()
		group := sources[sourceKey]
		if group == nil {
			group = &tunnelMetricSource{serverID: source.ServerID, interfaceID: source.ID}
			sources[sourceKey] = group
		}
		group.targets = append(group.targets, tunnelMetricTarget{connectionID: connectionID, peerKey: hex.EncodeToString(key[:])})
	}
	for _, connection := range topology.Connections {
		add(connection.ID.String(), connection.SourceID, connection.TargetID)
	}
	for _, link := range links {
		sourceID, sourceErr := uuid.Parse(link.Source.ResourceID)
		targetID, targetErr := uuid.Parse(link.Target.ResourceID)
		if sourceErr == nil && targetErr == nil {
			add("link:"+link.ID, sourceID, targetID)
		}
	}

	resultCount := 0
	for _, source := range sources {
		resultCount += len(source.targets)
	}
	results := make(chan models.TunnelMetric, resultCount)
	var wg sync.WaitGroup
	for _, source := range sources {
		wg.Add(1)
		go func(source *tunnelMetricSource) {
			defer wg.Done()
			snapshot, err := s.GetServerInterfaceMetrics(source.serverID.String(), source.interfaceID.String())
			for _, target := range source.targets {
				metric := models.TunnelMetric{ConnectionID: target.connectionID}
				if err != nil {
					metric.Error = err.Error()
				} else {
					for _, peer := range snapshot.Peers {
						if peer.PublicKey == target.peerKey {
							metric.LatencyP95Ms = peer.LatencyP95Ms
							break
						}
					}
				}
				results <- metric
			}
		}(source)
	}
	wg.Wait()
	close(results)

	metrics := make([]models.TunnelMetric, 0)
	for metric := range results {
		metrics = append(metrics, metric)
	}
	sort.Slice(metrics, func(i, j int) bool { return metrics[i].ConnectionID < metrics[j].ConnectionID })
	return metrics, nil
}
