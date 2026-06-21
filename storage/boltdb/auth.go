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

package boltdb

import (
	"github.com/ks-tool/awg-admin/models"

	bolt "go.etcd.io/bbolt"
)

type auth struct {
	db *bolt.DB
}

func (a *auth) Get() (*models.AuthCredentials, error) {
	var creds models.AuthCredentials
	err := a.db.View(func(tx *bolt.Tx) error {
		var err error
		creds, err = get[models.AuthCredentials](tx.Bucket(bktAuth), authRecordKey)
		return err
	})
	if err != nil {
		return nil, err
	}
	return &creds, nil
}

func (a *auth) Set(creds *models.AuthCredentials) error {
	return a.db.Update(func(tx *bolt.Tx) error {
		return put(tx.Bucket(bktAuth), authRecordKey, creds)
	})
}
