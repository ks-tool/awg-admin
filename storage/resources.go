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

package storage

import (
	"uuid"

	"github.com/ks-tool/vpn-admin/models"
)

type Resources interface {
	List() ([]models.Resource, error)
	Get(id string) (*models.Resource, error)
	Set(resource *models.Resource) error
	Delete(id string) error
}

type Links interface {
	List() ([]models.Link, error)
	Get(id string) (*models.Link, error)
	Set(link *models.Link) error
	Delete(id string) error
}

type Providers interface {
	List(serverID uuid.UUID) ([]models.ProviderRecord, error)
	Get(serverID uuid.UUID, name string) (*models.ProviderRecord, error)
	Set(record *models.ProviderRecord) error
	Delete(serverID uuid.UUID, name string) error
}
