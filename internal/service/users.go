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
	"fmt"

	"github.com/ks-tool/awg-admin/models"

	"github.com/google/uuid"
)

func (s *Service) ListUsers() ([]models.User, error) {
	users, err := s.store.Users().List()
	if err != nil {
		return nil, err
	}
	return sanitizeUsers(users), nil
}

func (s *Service) GetUser(id string) (*models.User, error) {
	uID, err := uuid.Parse(id)
	if err != nil {
		return nil, err
	}
	u, err := s.store.Users().Get(uID)
	if err != nil {
		return nil, err
	}
	sanitized := sanitizeUser(*u)
	return &sanitized, nil
}

type UserInput struct {
	Name        string
	Description string
	Disabled    bool
}

func (s *Service) CreateUser(in UserInput) (*models.User, error) {
	if len(in.Name) == 0 {
		return nil, fmt.Errorf("user name is required")
	}
	u := &models.User{ID: uuid.New(), Name: in.Name, Description: in.Description, Disabled: in.Disabled}
	if err := s.store.Users().Set(u); err != nil {
		return nil, err
	}
	sanitized := sanitizeUser(*u)
	return &sanitized, nil
}

func (s *Service) UpdateUser(id string, in UserInput) (*models.User, error) {
	uID, err := uuid.Parse(id)
	if err != nil {
		return nil, err
	}

	u, err := s.store.Users().Get(uID)
	if err != nil {
		return nil, err
	}
	u.Name = in.Name
	u.Description = in.Description
	u.Disabled = in.Disabled
	if err := s.store.Users().Set(u); err != nil {
		return nil, err
	}
	sanitized := sanitizeUser(*u)
	return &sanitized, nil
}

// DeleteUser removes the user and cascades peer removal from all interfaces.
func (s *Service) DeleteUser(id string) error {
	uID, err := uuid.Parse(id)
	if err != nil {
		return err
	}
	return s.store.Users().Delete(uID)
}
