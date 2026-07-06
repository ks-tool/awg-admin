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
	"encoding/json"
	"errors"
	"net/http"

	"github.com/ks-tool/awg-admin/internal/service"
	"github.com/ks-tool/awg-admin/storage"

	"github.com/rs/zerolog/log"
	"github.com/uptrace/bunrouter"
)

type Handler struct {
	svc      *service.Service
	sessions *SessionStore
}

// New registers every route of the standalone web server's JSON API
// (cmd/awg-admin.go) on rootGroup. The Wails desktop app (app.go) never
// calls this — it binds *service.Service directly to the frontend, so none
// of this — including the login/session requirement — applies there.
//
// /auth/login and /auth/logout are deliberately left outside the
// requireAuth-protected group: there's no session yet to check before
// logging in, and logging out must work even with a stale/invalid cookie.
//
// Returns the session store it created so the caller can wire it into
// BasicAuthMiddleware, letting an already-logged-in session bypass that
// gate (see BasicAuthMiddleware's doc comment).
func New(svc *service.Service, rootGroup *bunrouter.Group) *SessionStore {
	h := &Handler{svc: svc, sessions: newSessionStore()}

	rootGroup.WithGroup("/auth", func(gr *bunrouter.Group) {
		gr.POST("/login", h.authLogin)
		gr.POST("/logout", h.authLogout)
	})

	// Group.Use returns a *new* Group with the middleware stacked rather
	// than mutating the receiver — the result must be captured, or the
	// middleware silently never applies (every route would stay open).
	protected := rootGroup.NewGroup("").Use(h.requireAuth)

	protected.WithGroup("/auth", func(gr *bunrouter.Group) {
		gr.GET("/me", h.authMe)
		gr.POST("/change-credentials", h.authChangeCredentials)
		gr.PATCH("/basic-auth", h.authSetBasicAuth)
	})

	protected.GET("/backup", h.backupDownload)
	protected.GET("/version", h.appVersion)

	protected.WithGroup("/agent-sources", func(gr *bunrouter.Group) {
		gr.POST("", h.agentSourceCreate)
		gr.GET("/", h.agentSourceList)
		gr.GET("/releases", h.agentSourceReleases)
		gr.PUT("/:sourceID", h.agentSourceUpdate)
		gr.POST("/:sourceID/refresh", h.agentSourceRefresh)
		gr.DELETE("/:sourceID", h.agentSourceDelete)
	})

	protected.WithGroup("/users", func(gr *bunrouter.Group) {
		gr.POST("", h.userCreate)
		gr.GET("/", h.userList)

		gr.WithGroup("/:userID", func(gr *bunrouter.Group) {
			gr.GET("", h.userGet)
			gr.PUT("", h.userUpdate)
			gr.DELETE("", h.userDelete)

			gr.WithGroup("/peers", func(gr *bunrouter.Group) {
				gr.POST("", h.peerAdd)
				gr.POST("/migrate", h.peerMigrate)
				gr.POST("/disabled", h.peerSetDisabled)
				gr.GET("/*pubkey", h.peerGet)
				gr.DELETE("/*pubkey", h.peerDelete)
			})
		})
	})

	protected.WithGroup("/tunnels", func(gr *bunrouter.Group) {
		gr.POST("", h.tunnelBuild)
		gr.GET("/", h.tunnelList)
		gr.DELETE("/:tunnelID", h.tunnelDelete)
	})

	// Server-independent: generated defaults for a new interface form.
	protected.GET("/interfaces/defaults", h.interfaceDefaults)

	protected.WithGroup("/servers", func(gr *bunrouter.Group) {
		gr.POST("", h.serverCreate)
		gr.GET("/", h.serverList)

		gr.WithGroup("/:serverID", func(gr *bunrouter.Group) {
			gr.GET("", h.serverGet)
			gr.PUT("", h.serverUpdate)
			gr.DELETE("", h.serverDelete)
			gr.POST("/tls", h.serverGenerateTLS)
			gr.POST("/deploy", h.serverDeployAgent)
			gr.GET("/deploy/status", h.serverDeployStatus)
			gr.POST("/sync", h.serverSync)
			gr.GET("/reconcile", h.serverReconcile)
			gr.POST("/reconcile/import", h.serverImportInterface)
			gr.POST("/reconcile/delete-agent", h.serverDeleteAgentInterface)
			gr.GET("/metrics", h.serverMetrics)
			gr.GET("/metrics/history", h.serverMetricsHistory)
			gr.GET("/agent-status", h.serverAgentStatus)
			gr.GET("/host-info", h.serverHostInfo)
			gr.PATCH("/monitoring", h.serverSetMonitoring)
			gr.PATCH("/profiling", h.serverSetProfiling)
			gr.GET("/profile", h.serverProfileDownload)
			gr.POST("/ssh/unlock", h.serverUnlockSSH)

			gr.WithGroup("/interfaces", func(gr *bunrouter.Group) {
				gr.POST("", h.interfaceCreate)
				gr.GET("/", h.interfaceList)

				gr.WithGroup("/:ifaceID", func(gr *bunrouter.Group) {
					gr.GET("", h.interfaceGet)
					gr.PUT("/config", h.interfaceConfigUpdate)
					gr.DELETE("", h.interfaceDelete)
				})
			})
		})
	})

	return h.sessions
}

// ─────────────────────────────── helpers ───────────────────────────────────

func decode(r bunrouter.Request, v any) error {
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	return d.Decode(v)
}

func serverID(r bunrouter.Request) (string, error) {
	sID := r.Param("serverID")
	if len(sID) == 0 {
		return "", errors.New("serverID required")
	}
	return sID, nil
}

func ifaceID(r bunrouter.Request) (string, error) {
	iID := r.Param("ifaceID")
	if len(iID) == 0 {
		return "", errors.New("ifaceID required")
	}
	return iID, nil
}

func userID(r bunrouter.Request) (string, error) {
	uID := r.Param("userID")
	if len(uID) == 0 {
		return "", errors.New("userID required")
	}
	return uID, nil
}

func pubKey(r bunrouter.Request) (string, error) {
	pubkey := r.Param("pubkey")
	if len(pubkey) == 0 {
		return "", errors.New("pubkey required")
	}
	return pubkey, nil
}

func handleErr(err error, fields map[string]any) error {
	var status int
	var validationErr *service.ValidationError
	switch {
	case storage.IsNotFound(err):
		status = http.StatusNotFound
	case storage.IsAlreadyExists(err):
		status = http.StatusConflict
	case errors.As(err, &validationErr):
		// Bad user input (malformed field or a uniqueness conflict) — a client
		// error, not an internal fault; return 400 with the message and don't
		// log it at error level.
		status = http.StatusBadRequest
	default:
		log.Error().Fields(fields).Err(err).Send()
		return err
	}
	return &ErrorResponse{Err: err, Status: status}
}

func badRequest(err error) error {
	return &ErrorResponse{Err: err, Status: http.StatusBadRequest}
}
