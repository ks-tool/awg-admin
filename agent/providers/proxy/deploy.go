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

package proxy

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/ks-tool/vpn-agent/models"
)

type Deployment struct {
	Port      int
	CorePath  string
	EnvPrefix string
	Args      []string
	StatePath string
	Packages  []string
}

type Deployer interface {
	Deploy(context.Context, map[string]any) (Deployment, error)
}

type DeployConfig struct {
	AgentDB string         `json:"agentDB"`
	Options map[string]any `json:"options"`
}

func DeployCommand(args []string, deploy func(context.Context, string) error) (bool, error) {
	if len(args) == 0 || args[0] != "deploy" {
		return false, nil
	}
	if len(args) != 3 || args[1] != "--config" {
		return true, errors.New("usage: provider deploy --config path")
	}
	return true, deploy(context.Background(), args[2])
}

func DeployProvider(ctx context.Context, adapter Adapter, configPath string) error {
	deployer, ok := adapter.(Deployer)
	if !ok {
		return fmt.Errorf("provider %s does not implement deploy", adapter.Name())
	}
	data, err := os.ReadFile(configPath)
	if err != nil {
		return err
	}
	var config DeployConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return err
	}
	if len(config.AgentDB) == 0 {
		return errors.New("agentDB is required")
	}
	deployment, err := deployer.Deploy(ctx, config.Options)
	if err != nil {
		return err
	}
	if len(deployment.Packages) > 0 {
		arguments := append([]string{"install", "-y"}, deployment.Packages...)
		command := exec.CommandContext(ctx, "apt-get", arguments...)
		command.Stdout, command.Stderr = os.Stdout, os.Stderr
		if err := command.Run(); err != nil {
			return fmt.Errorf("install %s dependencies: %w", adapter.Name(), err)
		}
	}
	return WriteRegistration(adapter.Name(), config.AgentDB, deployment)
}

func WriteRegistration(name, agentDB string, deployment Deployment) error {
	if deployment.Port == 0 || len(deployment.EnvPrefix) == 0 {
		return errors.New("provider deployment port and env prefix are required")
	}
	statePath := deployment.StatePath
	if len(statePath) == 0 {
		statePath = "/var/lib/vpn-provider-" + name
	}
	if err := os.MkdirAll(statePath, 0o700); err != nil {
		return err
	}
	registrationDir := filepath.Join(agentDB, "providers.d")
	if err := os.MkdirAll(registrationDir, 0o700); err != nil {
		return err
	}
	env := map[string]string{
		deployment.EnvPrefix + "_DB":   statePath,
		deployment.EnvPrefix + "_ADDR": fmt.Sprintf("127.0.0.1:%d", deployment.Port),
	}
	if len(deployment.CorePath) > 0 {
		env[deployment.EnvPrefix+"_CORE"] = deployment.CorePath
	}
	registration := map[string]any{
		"protocolVersion": models.ProtocolVersion,
		"providers": []any{map[string]any{
			"address": fmt.Sprintf("http://127.0.0.1:%d", deployment.Port), "discover": true,
			"command": "/usr/local/bin/vpn-provider-" + name,
			"args":    deployment.Args, "env": env,
			"provider": map[string]any{"name": name},
		}},
	}
	data, err := json.MarshalIndent(registration, "", "  ")
	if err != nil {
		return err
	}
	return writePrivate(filepath.Join(registrationDir, name+".conf"), data)
}

func DownloadArtifact(ctx context.Context, artifact, expectedSHA256, destination, archiveEntry string) error {
	if len(expectedSHA256) == 0 {
		return errors.New("coreSHA256 is required for a custom coreArtifact")
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, artifact, nil)
	if err != nil {
		return err
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("download core: HTTP %s", response.Status)
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(destination), ".download-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer func() { _ = os.Remove(temporaryPath) }()
	hash := sha256.New()
	written, copyErr := io.Copy(io.MultiWriter(temporary, hash), io.LimitReader(response.Body, 129<<20))
	closeErr := temporary.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	if written > 128<<20 {
		return errors.New("core artifact exceeds 128 MiB")
	}
	if actual := hex.EncodeToString(hash.Sum(nil)); !strings.EqualFold(actual, strings.TrimPrefix(expectedSHA256, "sha256:")) {
		return errors.New("core artifact checksum mismatch")
	}
	if len(archiveEntry) > 0 {
		if err := extractZipEntry(temporaryPath, archiveEntry, destination); err != nil {
			return err
		}
	} else if err := os.Rename(temporaryPath, destination); err != nil {
		return err
	}
	return os.Chmod(destination, 0o755)
}

func extractZipEntry(archivePath, entry, destination string) error {
	archive, err := zip.OpenReader(archivePath)
	if err != nil {
		return err
	}
	defer func() { _ = archive.Close() }()
	for _, file := range archive.File {
		if file.Name != entry {
			continue
		}
		source, err := file.Open()
		if err != nil {
			return err
		}
		defer func() { _ = source.Close() }()
		target, err := os.OpenFile(destination, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(target, source)
		closeErr := target.Close()
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	}
	return fmt.Errorf("core archive does not contain %s", entry)
}
