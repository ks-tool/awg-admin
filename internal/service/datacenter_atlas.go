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
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/ks-tool/vpn-admin/internal/config"

	"github.com/rs/zerolog/log"
)

const (
	datacenterAtlasURL       = "https://kiankyars.github.io/datacenter-atlas/api/v1/sites.json"
	datacenterAtlasCacheName = "datacenter-atlas.json"
	datacenterAtlasCacheTTL  = 24 * time.Hour
)

type atlasSite struct {
	Name      string  `json:"name"`
	Country   string  `json:"country"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

type datacenterAtlas struct {
	endpoint  string
	cachePath string
	client    *http.Client

	mu     sync.Mutex
	loaded bool
	sites  []atlasSite
}

func defaultLocationGeocoder() locationGeocoder {
	var cachePath string
	if home, err := os.UserHomeDir(); err == nil {
		cachePath = filepath.Join(config.AdminCachePath(home), datacenterAtlasCacheName)
	}
	return &datacenterAtlas{
		endpoint:  datacenterAtlasURL,
		cachePath: cachePath,
		client:    &http.Client{Timeout: locationResolveTimeout},
	}
}

var datacenterAtlasCityFallbacks = []atlasSite{
	{Name: "Moscow (Москва)", Country: "Russia (Россия)", Latitude: 55.7558, Longitude: 37.6173},
}

func (a *datacenterAtlas) Geocode(ctx context.Context, location string) (float64, float64, error) {
	sites, err := a.loadSites(ctx)
	if err != nil {
		return 0, 0, err
	}
	locationParts := strings.SplitN(location, ",", 2)
	query := atlasNormalize(locationParts[0])
	if query == "" {
		return 0, 0, fmt.Errorf("location is empty")
	}
	var countryQuery string
	if len(locationParts) == 2 {
		countryQuery = atlasNormalize(locationParts[1])
	}
	site := findAtlasSite(sites, query, countryQuery)
	if site == nil {
		site = findAtlasSite(datacenterAtlasCityFallbacks, query, countryQuery)
	}
	if site == nil {
		return 0, 0, fmt.Errorf("no data center city matches %q", location)
	}
	return site.Latitude, site.Longitude, nil
}

func findAtlasSite(sites []atlasSite, query, countryQuery string) *atlasSite {
	var firstMatch *atlasSite
	for index := range sites {
		site := &sites[index]
		haystack := atlasNormalize(site.Name + " " + site.Country)
		if !atlasContainsAll(haystack, query) {
			continue
		}
		if firstMatch == nil {
			firstMatch = site
		}
		if countryQuery != "" && atlasContainsAll(atlasNormalize(site.Country), countryQuery) {
			return site
		}
	}
	return firstMatch
}

func (a *datacenterAtlas) loadSites(ctx context.Context) ([]atlasSite, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.loaded {
		return a.sites, nil
	}
	if cached, modTime, err := readAtlasCache(a.cachePath); err == nil {
		a.sites = cached
		a.loaded = true
		if time.Since(modTime) < datacenterAtlasCacheTTL {
			return a.sites, nil
		}
	}
	sites, err := a.downloadSites(ctx)
	if err != nil {
		if a.loaded {
			log.Warn().Err(err).Msg("failed to refresh Data Center Atlas; using cached data")
			return a.sites, nil
		}
		return nil, fmt.Errorf("download Data Center Atlas: %w", err)
	}
	a.sites = sites
	a.loaded = true
	if err := writeAtlasCache(a.cachePath, sites); err != nil {
		log.Warn().Err(err).Msg("failed to cache Data Center Atlas")
	}
	return a.sites, nil
}

func (a *datacenterAtlas) downloadSites(ctx context.Context) ([]atlasSite, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "vpn-admin/2 (https://github.com/ks-tool/vpn-admin)")
	req.Header.Set("Accept", "application/json")
	response, err := a.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected HTTP status %s", response.Status)
	}
	var sites []atlasSite
	if err := json.NewDecoder(response.Body).Decode(&sites); err != nil {
		return nil, err
	}
	if len(sites) == 0 {
		return nil, fmt.Errorf("dataset is empty")
	}
	return sites, nil
}

func readAtlasCache(path string) ([]atlasSite, time.Time, error) {
	if path == "" {
		return nil, time.Time{}, os.ErrNotExist
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, time.Time{}, err
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, time.Time{}, err
	}
	defer func() { _ = file.Close() }()
	var sites []atlasSite
	if err := json.NewDecoder(file).Decode(&sites); err != nil {
		return nil, time.Time{}, err
	}
	if len(sites) == 0 {
		return nil, time.Time{}, fmt.Errorf("cached dataset is empty")
	}
	return sites, info.ModTime(), nil
}

func writeAtlasCache(path string, sites []atlasSite) error {
	if path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".datacenter-atlas-*")
	if err != nil {
		return err
	}
	temporaryPath := file.Name()
	defer func() { _ = os.Remove(temporaryPath) }()
	if err := json.NewEncoder(file).Encode(sites); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryPath, path)
}

var cyrillicTransliteration = map[rune]string{
	'а': "a", 'б': "b", 'в': "v", 'г': "g", 'д': "d", 'е': "e", 'ё': "e", 'ж': "zh",
	'з': "z", 'и': "i", 'й': "y", 'к': "k", 'л': "l", 'м': "m", 'н': "n", 'о': "o",
	'п': "p", 'р': "r", 'с': "s", 'т': "t", 'у': "u", 'ф': "f", 'х': "kh", 'ц': "ts",
	'ч': "ch", 'ш': "sh", 'щ': "sch", 'ъ': "", 'ы': "y", 'ь': "", 'э': "e", 'ю': "yu", 'я': "ya",
}

func atlasNormalize(value string) string {
	var result strings.Builder
	for _, current := range strings.ToLower(value) {
		if replacement, ok := cyrillicTransliteration[current]; ok {
			result.WriteString(replacement)
		} else if unicode.IsLetter(current) || unicode.IsDigit(current) {
			result.WriteRune(current)
		} else {
			result.WriteByte(' ')
		}
	}
	return strings.Join(strings.Fields(result.String()), " ")
}

func atlasContainsAll(value, query string) bool {
	for token := range strings.FieldsSeq(query) {
		if !strings.Contains(value, token) {
			return false
		}
	}
	return query != ""
}
