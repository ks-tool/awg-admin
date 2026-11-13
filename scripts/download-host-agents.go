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
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

const releaseURL = "https://github.com/horchestra/targets/releases/download/host/v0.4.3/"

var agents = []struct {
	name   string
	sha256 string
}{
	{name: "pack-host-agent-linux-amd64", sha256: "6f36ccaa26bdb2adb40291742234368982ca0afe966b778a4a2ccf608d6e49d5"},
	{name: "pack-host-agent-linux-arm64", sha256: "1116e5311a77ff09f1bf7f701594949117bd0e6886cdb80fb311075190e15a71"},
}

func main() {
	const directory = "internal/deploy/agentbin"
	if err := os.MkdirAll(directory, 0o755); err != nil {
		fatal(err)
	}
	client := &http.Client{Timeout: 2 * time.Minute}
	for _, agent := range agents {
		path := filepath.Join(directory, agent.name)
		if validFile(path, agent.sha256) {
			_, _ = fmt.Printf("%s: up to date\n", agent.name)
			continue
		}
		if err := download(client, agent, path); err != nil {
			fatal(err)
		}
		_, _ = fmt.Printf("%s: downloaded and verified\n", agent.name)
	}
}

func validFile(path, expected string) bool {
	file, err := os.Open(path)
	if err != nil {
		return false
	}
	defer func() { _ = file.Close() }()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return false
	}
	return hex.EncodeToString(hash.Sum(nil)) == expected
}

func download(client *http.Client, agent struct {
	name   string
	sha256 string
}, path string) error {
	response, err := client.Get(releaseURL + agent.name)
	if err != nil {
		return fmt.Errorf("download %s: %w", agent.name, err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("download %s: %s", agent.name, response.Status)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, 32<<20))
	if err != nil {
		return fmt.Errorf("read %s: %w", agent.name, err)
	}
	sum := sha256.Sum256(data)
	if hex.EncodeToString(sum[:]) != agent.sha256 {
		return fmt.Errorf("%s: SHA-256 mismatch", agent.name)
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), agent.name+"-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer func() { _ = os.Remove(temporaryPath) }()
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Chmod(0o755); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return os.Rename(temporaryPath, path)
}

func fatal(err error) {
	_, _ = fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
