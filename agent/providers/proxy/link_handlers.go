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

package proxy

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"

	"github.com/ks-tool/vpn-agent/models"
)

func (p *Provider) PrepareLink(_ context.Context, link models.Link, endpoint models.LinkEndpoint) (models.LinkOffer, error) {
	if err := p.validateLinkKind(link.Kind); err != nil {
		return models.LinkOffer{}, err
	}
	interfaceItem, _, err := p.interfaceResource(endpoint.ResourceID)
	if err != nil {
		return models.LinkOffer{}, err
	}
	offer := models.LinkOffer{LinkID: link.ID, Endpoint: endpoint, Data: json.RawMessage(`{}`)}
	if link.Target.ResourceID == endpoint.ResourceID && link.Target.ServerID == endpoint.ServerID {
		adapter, ok := p.adapter.(LinkAdapter)
		if !ok {
			return models.LinkOffer{}, errors.New("provider does not support outbound links")
		}
		records, err := p.links.list()
		if err != nil {
			return models.LinkOffer{}, err
		}
		routeID, err := FindRouteID(link.ID, func(candidate string) bool {
			socksAddress, tunName, _, _ := RouteIdentifiers(candidate)
			for id, record := range records {
				if id == link.ID || record.Driver == nil {
					continue
				}
				if record.Driver.SOCKS5 == socksAddress || record.Driver.TUN == tunName {
					return true
				}
			}
			return false
		})
		if err != nil {
			return models.LinkOffer{}, err
		}
		secret, err := p.adapter.NewSecret()
		if err != nil {
			return models.LinkOffer{}, err
		}
		peer, driver, err := adapter.PrepareLink(interfaceItem.Spec, link, endpoint, secret, routeID)
		if err != nil {
			return models.LinkOffer{}, err
		}
		driver.RouteID = routeID
		peer.Spec.Name = "link-" + link.ID
		peer.Spec.InterfaceRef = interfaceItem.ID
		data, err := json.Marshal(driver)
		if err != nil {
			return models.LinkOffer{}, err
		}
		offer.Data = data
		return offer, p.links.put(link.ID, linkRecord{Role: "target", Offer: offer, Peer: &peer, Driver: &driver})
	}
	if link.Source.ResourceID != endpoint.ResourceID || link.Source.ServerID != endpoint.ServerID {
		return models.LinkOffer{}, errors.New("endpoint does not belong to link")
	}
	records, err := p.links.list()
	if err != nil {
		return models.LinkOffer{}, err
	}
	for id, record := range records {
		if id != link.ID && record.Role == "source" && record.Offer.Endpoint.ResourceID == endpoint.ResourceID {
			return models.LinkOffer{}, errors.New("proxy interface already has an outgoing link")
		}
	}
	return offer, p.links.put(link.ID, linkRecord{Role: "source", Offer: offer})
}

func (p *Provider) CommitLink(_ context.Context, link models.Link, endpoint models.LinkEndpoint, remote models.LinkOffer) error {
	if err := p.validateLinkKind(link.Kind); err != nil {
		return err
	}
	if remote.LinkID != link.ID {
		return errors.New("remote offer does not match link")
	}
	record, err := p.links.get(link.ID)
	if err != nil {
		return err
	}
	if record.Role == "target" {
		if err := p.applyInterfaceRecord(record.Offer.Endpoint.ResourceID); err != nil {
			return err
		}
		return nil
	}
	var driver models.RouteDriver
	if err := json.Unmarshal(remote.Data, &driver); err != nil {
		return err
	}
	if len(driver.Binary) == 0 {
		var wrapped struct {
			Driver *models.RouteDriver `json:"driver"`
		}
		if err := json.Unmarshal(remote.Data, &wrapped); err != nil {
			return err
		}
		if wrapped.Driver != nil {
			driver = *wrapped.Driver
		}
	}
	if len(driver.Binary) == 0 || len(driver.Config) == 0 {
		return errors.New("remote offer has no route connector")
	}
	record.Driver = &driver
	if err := p.processes.apply("link:"+link.ID, driverCoreConfig(driver)); err != nil {
		return err
	}
	if err := p.links.put(link.ID, record); err != nil {
		return err
	}
	return p.applyInterfaceRecord(endpoint.ResourceID)
}

func (p *Provider) DeleteLink(_ context.Context, link models.Link, _ models.LinkEndpoint, _ *models.LinkOffer) error {
	record, err := p.links.get(link.ID)
	if err != nil {
		if osIsNotExist(err) {
			return nil
		}
		return err
	}
	if record.Role == "source" {
		if err := p.processes.stop("link:" + link.ID); err != nil {
			return err
		}
	}
	if err := p.links.delete(link.ID); err != nil {
		return err
	}
	return p.applyInterfaceRecord(record.Offer.Endpoint.ResourceID)
}

func (p *Provider) RestoreLinks() error {
	records, err := p.links.list()
	if err != nil {
		return err
	}
	for id, record := range records {
		if record.Role == "source" && record.Driver != nil {
			if err := p.processes.apply("link:"+id, driverCoreConfig(*record.Driver)); err != nil {
				return err
			}
		}
		if err := p.applyInterfaceRecord(record.Offer.Endpoint.ResourceID); err != nil {
			return err
		}
	}
	return nil
}

func (p *Provider) applyInterfaceRecord(resourceID string) error {
	item, _, err := p.interfaceResource(resourceID)
	if err != nil {
		return err
	}
	return p.applyInterface(item)
}

func (p *Provider) validateLinkKind(kind string) error {
	if kind != p.adapter.Name() && kind != models.RouteLinkKind {
		return errors.New("unsupported link kind")
	}
	return nil
}

func (p *Provider) interfaceRoutes(resourceID string) ([]Route, error) {
	records, err := p.links.list()
	if err != nil {
		return nil, err
	}
	routes := make([]Route, 0)
	for id, record := range records {
		if record.Role == "source" && record.Driver != nil && record.Offer.Endpoint.ResourceID == resourceID {
			routes = append(routes, Route{LinkID: id, SOCKS5: record.Driver.SOCKS5})
		}
	}
	return routes, nil
}

func driverCoreConfig(driver models.RouteDriver) CoreConfig {
	return CoreConfig{
		Data: driver.Config, Binary: driver.Binary,
		Arguments: func(path string) []string {
			arguments := make([]string, len(driver.Args))
			for index, argument := range driver.Args {
				arguments[index] = strings.ReplaceAll(argument, "{{config}}", path)
			}
			return arguments
		},
	}
}

func osIsNotExist(err error) bool {
	return errors.Is(err, os.ErrNotExist)
}
