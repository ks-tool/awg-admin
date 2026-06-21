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
	"errors"
	"net/http"

	"github.com/ks-tool/awg-admin/internal/service"

	"github.com/rs/zerolog/log"
	"github.com/uptrace/bunrouter"
)

func (h *Handler) authLogin(w http.ResponseWriter, r bunrouter.Request) error {
	fields := map[string]any{"method": r.Method, "path": r.URL.Path}

	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := decode(r, &req); err != nil {
		return badRequest(err)
	}

	if err := h.svc.Login(req.Username, req.Password); err != nil {
		log.Warn().Fields(fields).Str("username", req.Username).Msg("login failed")
		return &ErrorResponse{Err: err, Status: http.StatusUnauthorized}
	}

	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    h.sessions.create(),
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(sessionTTL.Seconds()),
	})
	log.Info().Fields(fields).Str("username", req.Username).Msg("login succeeded")
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (h *Handler) authLogout(w http.ResponseWriter, r bunrouter.Request) error {
	if c, err := r.Cookie(sessionCookieName); err == nil {
		h.sessions.revoke(c.Value)
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (h *Handler) authMe(w http.ResponseWriter, r bunrouter.Request) error {
	fields := map[string]any{"method": r.Method, "path": r.URL.Path}

	username, err := h.svc.CurrentUsername()
	if err != nil {
		return handleErr(err, fields)
	}
	basicAuthEnabled, err := h.svc.BasicAuthEnabled()
	if err != nil {
		return handleErr(err, fields)
	}
	return bunrouter.JSON(w, map[string]any{"username": username, "basicAuthEnabled": basicAuthEnabled})
}

func (h *Handler) authSetBasicAuth(w http.ResponseWriter, r bunrouter.Request) error {
	fields := map[string]any{"method": r.Method, "path": r.URL.Path}

	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := decode(r, &req); err != nil {
		return badRequest(err)
	}

	if err := h.svc.SetBasicAuthEnabled(req.Enabled); err != nil {
		return handleErr(err, fields)
	}
	log.Info().Fields(fields).Bool("enabled", req.Enabled).Msg("basic auth setting changed")
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (h *Handler) authChangeCredentials(w http.ResponseWriter, r bunrouter.Request) error {
	fields := map[string]any{"method": r.Method, "path": r.URL.Path}

	var req struct {
		CurrentPassword string `json:"currentPassword"`
		NewUsername     string `json:"newUsername,omitempty"`
		NewPassword     string `json:"newPassword,omitempty"`
	}
	if err := decode(r, &req); err != nil {
		return badRequest(err)
	}

	if err := h.svc.ChangeCredentials(req.CurrentPassword, req.NewUsername, req.NewPassword); err != nil {
		if errors.Is(err, service.ErrInvalidCredentials) {
			return &ErrorResponse{Err: err, Status: http.StatusUnauthorized}
		}
		return handleErr(err, fields)
	}
	log.Info().Fields(fields).Msg("admin credentials changed")
	w.WriteHeader(http.StatusNoContent)
	return nil
}

// requireAuth rejects any request without a valid session cookie. Mounted
// on every route group except /auth/login and /auth/logout (see Handler.New)
// — there's no session to check against before logging in.
func (h *Handler) requireAuth(next bunrouter.HandlerFunc) bunrouter.HandlerFunc {
	return func(w http.ResponseWriter, r bunrouter.Request) error {
		c, err := r.Cookie(sessionCookieName)
		if err != nil || !h.sessions.valid(c.Value) {
			return &ErrorResponse{Err: errors.New("authentication required"), Status: http.StatusUnauthorized}
		}
		return next(w, r)
	}
}
