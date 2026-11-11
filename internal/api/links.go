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

	"github.com/uptrace/bunrouter"
)

func (h *Handler) providerLinkList(w http.ResponseWriter, r bunrouter.Request) error {
	links, err := h.svc.ListProviderLinks()
	if err != nil {
		return handleErr(err, resourceFields(r))
	}
	return bunrouter.JSON(w, links)
}

func (h *Handler) providerLinkCreate(w http.ResponseWriter, r bunrouter.Request) error {
	var input struct {
		SourceID string `json:"sourceId"`
		TargetID string `json:"targetId"`
	}
	if err := decode(r, &input); err != nil {
		return badRequest(err)
	}
	link, err := h.svc.CreateProviderLink(input.SourceID, input.TargetID)
	if err != nil {
		return handleErr(err, resourceFields(r))
	}
	w.WriteHeader(http.StatusCreated)
	return bunrouter.JSON(w, link)
}

func (h *Handler) providerLinkMove(w http.ResponseWriter, r bunrouter.Request) error {
	var input struct {
		SourceID string `json:"sourceId"`
		TargetID string `json:"targetId"`
	}
	if err := decode(r, &input); err != nil {
		return badRequest(err)
	}
	link, err := h.svc.MoveProviderLink(r.Param("linkID"), input.SourceID, input.TargetID)
	if err != nil {
		return handleErr(err, resourceFields(r))
	}
	return bunrouter.JSON(w, link)
}

func (h *Handler) providerLinkApply(w http.ResponseWriter, r bunrouter.Request) error {
	link, err := h.svc.ApplyProviderLink(r.Param("linkID"))
	if err != nil {
		return handleErr(err, resourceFields(r))
	}
	return bunrouter.JSON(w, link)
}

func (h *Handler) providerLinkDelete(w http.ResponseWriter, r bunrouter.Request) error {
	if err := h.svc.DeleteProviderLink(r.Param("linkID")); err != nil {
		return handleErr(err, resourceFields(r))
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}
