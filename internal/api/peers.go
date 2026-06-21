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

func (h *Handler) peerAdd(w http.ResponseWriter, r bunrouter.Request) error {
	fields := map[string]any{"method": r.Method, "path": r.URL.Path}

	uID, err := userID(r)
	if err != nil {
		return badRequest(err)
	}
	fields["user_id"] = uID

	var peer service.AddPeerInput
	if err = decode(r, &peer); err != nil {
		return badRequest(err)
	}
	log.Debug().Fields(fields).Msg("creating new peer")

	if _, err = h.svc.AddPeer(uID, peer); err != nil {
		return handleErr(err, fields)
	}

	w.WriteHeader(http.StatusCreated)
	return nil
}

func (h *Handler) peerGet(w http.ResponseWriter, r bunrouter.Request) error {
	fields := map[string]any{"method": r.Method, "path": r.URL.Path}

	uID, err := userID(r)
	if err != nil {
		return badRequest(err)
	}
	fields["user_id"] = uID

	pub, err := pubKey(r)
	if err != nil {
		log.Debug().Fields(fields).Msg("listing peers")

		peerList, err := h.svc.ListPeers(uID)
		if err != nil {
			return handleErr(err, fields)
		}

		return bunrouter.JSON(w, peerList)
	}

	fields["pub_key"] = pub

	switch r.URL.Query().Get("format") {
	case "config":
		log.Debug().Fields(fields).Msg("getting peer config")

		cfg, err := h.svc.GetPeerConfig(uID, pub)
		if err != nil {
			return handleErr(err, fields)
		}

		return bunrouter.JSON(w, map[string]string{"config": cfg})
	case "qrcode":
		log.Debug().Fields(fields).Msg("getting peer QR code")

		qr, err := h.svc.GetPeerQRCode(uID, pub)
		if err != nil {
			return handleErr(err, fields)
		}

		return bunrouter.JSON(w, map[string]string{"qrcode": qr})
	}

	log.Debug().Fields(fields).Msg("getting peer")

	peer, err := h.svc.GetPeer(uID, pub)
	if err != nil {
		log.Error().Fields(fields).Err(err).Send()
		return err
	}

	return bunrouter.JSON(w, peer)
}

func (h *Handler) peerDelete(w http.ResponseWriter, r bunrouter.Request) error {
	fields := map[string]any{"method": r.Method, "path": r.URL.Path}

	uID, err := userID(r)
	if err != nil {
		return badRequest(err)
	}
	fields["user_id"] = uID

	pub, err := pubKey(r)
	if err != nil {
		return badRequest(err)
	}
	fields["pub_key"] = pub
	log.Debug().Fields(fields).Msg("deleting peer")

	users, err := h.svc.DeletePeer(uID, pub)
	if err != nil {
		log.Error().Fields(fields).Err(err).Send()
		return handleErr(err, fields)
	}

	w.WriteHeader(http.StatusNoContent)
	return bunrouter.JSON(w, users.Peers)
}
