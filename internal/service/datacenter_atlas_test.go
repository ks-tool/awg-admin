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
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
)

func TestDatacenterAtlasGeocodeAndCache(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[
			{"name":"Equinix AM5 Amsterdam IBX","country":"Netherlands","latitude":52.3,"longitude":4.9},
			{"name":"Global Switch Amsterdam campus","country":"Netherlands","latitude":52.4,"longitude":4.8},
			{"name":"Telehouse Canada, Toronto","country":"Canada","latitude":43.64,"longitude":-79.38}
		]`))
	}))
	defer server.Close()

	cachePath := filepath.Join(t.TempDir(), "atlas.json")
	atlas := &datacenterAtlas{endpoint: server.URL, cachePath: cachePath, client: server.Client()}
	latitude, longitude, err := atlas.Geocode(context.Background(), "Амстердам, Нидерланды")
	if err != nil {
		t.Fatal(err)
	}
	if latitude != 52.3 || longitude != 4.9 {
		t.Fatalf("unexpected city coordinates: %f, %f", latitude, longitude)
	}
	for _, location := range []string{"Moscow", "Moscow, Russia", "Москва, Россия"} {
		latitude, longitude, err = atlas.Geocode(context.Background(), location)
		if err != nil {
			t.Fatalf("geocode %q: %v", location, err)
		}
		if latitude != 55.7558 || longitude != 37.6173 {
			t.Fatalf("unexpected Moscow coordinates for %q: %f, %f", location, latitude, longitude)
		}
	}
	if _, _, err := atlas.Geocode(context.Background(), "Unknown city"); err == nil {
		t.Fatal("expected an unknown data center city to be rejected")
	}
	if requests.Load() != 1 {
		t.Fatalf("dataset was downloaded %d times", requests.Load())
	}

	cached := &datacenterAtlas{endpoint: server.URL, cachePath: cachePath, client: server.Client()}
	latitude, longitude, err = cached.Geocode(context.Background(), "Toronto, Canada")
	if err != nil {
		t.Fatal(err)
	}
	if latitude != 43.64 || longitude != -79.38 || requests.Load() != 1 {
		t.Fatalf("cached dataset was not used: %f, %f, requests=%d", latitude, longitude, requests.Load())
	}
}
