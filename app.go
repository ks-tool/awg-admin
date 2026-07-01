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

package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ks-tool/awg-admin/internal/service"
	"github.com/ks-tool/awg-admin/storage/boltdb"
	"github.com/rs/zerolog/log"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// App struct
type App struct {
	*service.Service
	ctx context.Context
}

// NewApp creates a new App application struct
func NewApp() *App {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}

	db, err := boltdb.Open(filepath.Join(home, ".awg-admin"))
	if err != nil {
		log.Fatal().Err(err).Send()
	}

	return &App{Service: service.New(db)}
}

// startup is called when the app starts. The context is saved
// so we can call the runtime methods
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.StartTunnels()
}

// shutdown is called when the app is closing.
func (a *App) shutdown(_ context.Context) {
	a.StopTunnels()
}

// SelectFile opens a native file picker and returns the chosen path, or an
// empty string if the dialog was cancelled. Only available when running as
// a Wails desktop app (a.ctx is set in startup).
func (a *App) SelectFile(title string) (string, error) {
	if a.ctx == nil {
		return "", nil
	}
	return runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{Title: title})
}

// SavePeerQRCode renders key's client-config QR code (see GetPeerQRCode) as a
// PNG and writes it to a file the user chooses via a native save dialog. This
// is the desktop path for "save QR as PNG": the Wails webview can't download a
// data: URL the way a browser tab can (an <a download> is a no-op there), so
// the file is written here in Go instead. Returns true if a file was written,
// false if the dialog was cancelled. Desktop-only (a.ctx is set in startup);
// in any other mode it returns false so the frontend falls back to a browser
// download.
func (a *App) SavePeerQRCode(userID, key, defaultName string) (bool, error) {
	if a.ctx == nil {
		return false, nil
	}

	b64, err := a.GetPeerQRCode(userID, key)
	if err != nil {
		return false, err
	}
	png, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return false, fmt.Errorf("decode QR code: %w", err)
	}

	if defaultName == "" {
		defaultName = "peer"
	}
	path, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		Title:           "Save QR code",
		DefaultFilename: defaultName + ".png",
		Filters: []runtime.FileFilter{{
			DisplayName: "PNG image (*.png)",
			Pattern:     "*.png",
		}},
	})
	if err != nil {
		return false, err
	}
	if path == "" {
		return false, nil // dialog cancelled
	}
	if filepath.Ext(path) == "" {
		path += ".png"
	}

	if err := os.WriteFile(path, png, 0o644); err != nil {
		return false, fmt.Errorf("write QR code: %w", err)
	}
	return true, nil
}
