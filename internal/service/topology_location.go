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
	"math"
	"strings"
	"time"

	"github.com/ks-tool/vpn-admin/models"
	"github.com/rs/zerolog/log"
)

const locationResolveTimeout = 3 * time.Second

type locationGeocoder interface {
	Geocode(ctx context.Context, location string) (latitude, longitude float64, err error)
}

func validateServerCoordinates(latitude, longitude *float64) error {
	if (latitude == nil) != (longitude == nil) {
		return invalidInput("both latitude and longitude are required")
	}
	if latitude != nil && (*latitude < -90 || *latitude > 90 || math.IsNaN(*latitude) || math.IsInf(*latitude, 0)) {
		return invalidInput("latitude must be between -90 and 90")
	}
	if longitude != nil && (*longitude < -180 || *longitude > 180 || math.IsNaN(*longitude) || math.IsInf(*longitude, 0)) {
		return invalidInput("longitude must be between -180 and 180")
	}
	return nil
}

func (s *Service) resolveTopologyLocations(topology *models.Topology) error {
	changed := false
	ctx, cancel := context.WithTimeout(context.Background(), locationResolveTimeout)
	defer cancel()
	for index := range topology.Servers {
		server := &topology.Servers[index]
		server.LocationError = ""
		location := strings.TrimSpace(server.Location)
		if location == "" && server.CoordinatesManual && server.Latitude != nil && server.Longitude != nil {
			continue
		}
		if server.CoordinatesManual {
			server.CoordinatesManual = false
			server.Latitude = nil
			server.Longitude = nil
			changed = true
		}
		if location == "" {
			if server.Latitude != nil || server.Longitude != nil {
				server.Latitude = nil
				server.Longitude = nil
				changed = true
			}
			continue
		}
		if server.Latitude != nil && server.Longitude != nil {
			continue
		}
		latitude, longitude, err := s.geocoder.Geocode(ctx, location)
		if err != nil {
			server.LocationError = err.Error()
			log.Warn().Err(err).Str("server_id", server.ID.String()).Str("location", location).Msg("failed to resolve server location")
			continue
		}
		server.Latitude = &latitude
		server.Longitude = &longitude
		changed = true
	}
	if changed {
		return s.store.Topology().Set(topology)
	}
	return nil
}
