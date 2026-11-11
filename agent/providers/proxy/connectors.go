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
	"github.com/ks-tool/vpn-agent/models"
)

type ConnectorManager struct {
	processes *processManager
}

func NewConnectorManager(root string) (*ConnectorManager, error) {
	processes, err := newProcessManager("route-connector", root)
	if err != nil {
		return nil, err
	}
	return &ConnectorManager{processes: processes}, nil
}

func (m *ConnectorManager) Apply(linkID string, driver models.RouteDriver) error {
	return m.processes.apply("link:"+linkID, driverCoreConfig(driver))
}

func (m *ConnectorManager) Stop(linkID string) error {
	return m.processes.stop("link:" + linkID)
}

func (m *ConnectorManager) Close() error {
	return m.processes.close()
}
