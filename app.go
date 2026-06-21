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
