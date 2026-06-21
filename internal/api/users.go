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

package api

import (
	"net/http"

	"github.com/ks-tool/awg-admin/internal/service"

	"github.com/rs/zerolog/log"
	"github.com/uptrace/bunrouter"
)

func (h *Handler) userList(w http.ResponseWriter, r bunrouter.Request) error {
	fields := map[string]any{"method": r.Method, "path": r.URL.Path}
	log.Debug().Fields(fields).Msg("listing users")

	userList, err := h.svc.ListUsers()
	if err != nil {
		return handleErr(err, fields)
	}

	return bunrouter.JSON(w, userList)
}

func (h *Handler) userCreate(w http.ResponseWriter, r bunrouter.Request) error {
	fields := map[string]any{"method": r.Method, "path": r.URL.Path}

	var user service.UserInput
	if err := decode(r, &user); err != nil {
		return badRequest(err)
	}
	log.Debug().Fields(fields).Msg("creating new user")

	newUser, err := h.svc.CreateUser(user)
	if err != nil {
		return handleErr(err, fields)
	}

	w.WriteHeader(http.StatusCreated)
	return bunrouter.JSON(w, newUser)
}

func (h *Handler) userGet(w http.ResponseWriter, r bunrouter.Request) error {
	fields := map[string]any{"method": r.Method, "path": r.URL.Path}

	uID, err := userID(r)
	if err != nil {
		return badRequest(err)
	}
	fields["user_id"] = uID
	log.Debug().Fields(fields).Msg("getting user")

	user, err := h.svc.GetUser(uID)
	if err != nil {
		return handleErr(err, fields)
	}

	return bunrouter.JSON(w, user)
}

func (h *Handler) userUpdate(w http.ResponseWriter, r bunrouter.Request) error {
	fields := map[string]any{"method": r.Method, "path": r.URL.Path}

	uID, err := userID(r)
	if err != nil {
		return badRequest(err)
	}
	fields["user_id"] = uID
	log.Debug().Fields(fields).Msg("updating user")

	var user service.UserInput
	if err = decode(r, &user); err != nil {
		return badRequest(err)
	}

	updatedUser, err := h.svc.UpdateUser(uID, user)
	if err != nil {
		return handleErr(err, fields)
	}

	w.WriteHeader(http.StatusAccepted)
	return bunrouter.JSON(w, updatedUser)
}

func (h *Handler) userDelete(w http.ResponseWriter, r bunrouter.Request) error {
	fields := map[string]any{"method": r.Method, "path": r.URL.Path}

	uID, err := userID(r)
	if err != nil {
		return badRequest(err)
	}
	fields["user_id"] = uID
	log.Debug().Fields(fields).Msg("deleting user")

	if err = h.svc.DeleteUser(uID); err != nil {
		return handleErr(err, fields)
	}

	w.WriteHeader(http.StatusNoContent)
	return nil
}
