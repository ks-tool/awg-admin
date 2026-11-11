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
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/ks-tool/vpn-admin/models"
)

func TestOCIRegistryTagsAndCatalogWithBearerAuth(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/token" {
			username, password, ok := r.BasicAuth()
			if !ok || username != "reader" || password != "secret" {
				t.Errorf("unexpected token credentials: %q %q %v", username, password, ok)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(w, `{"token":"registry-token"}`)
			return
		}
		if r.Header.Get("Authorization") != "Bearer registry-token" {
			w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Bearer realm=%q,service="registry.test",scope="repository:team/agent:pull"`, server.URL+"/token"))
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v2/team/agent/tags/list":
			_, _ = fmt.Fprint(w, `{"tags":["v1.0.0-alpha.1","v1.0.0-rc.1","v1.0.0-alpha.1"]}`)
		case "/v2/_catalog":
			_, _ = fmt.Fprint(w, `{"repositories":["team/providers/awg","team/providers/xray","other/awg"]}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	svc := newTestService(t)
	registry := models.Registry{Username: "reader", Password: "secret", PlainHTTP: true}
	repository := "oci://" + server.Listener.Addr().String() + "/team/agent"
	tags, err := svc.ListArtifactTags(repository, registry)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"v1.0.0-rc.1", "v1.0.0-alpha.1"}; !reflect.DeepEqual(tags, want) {
		t.Fatalf("tags = %#v, want %#v", tags, want)
	}
	providers, err := svc.ListProviderRepositories("oci://"+server.Listener.Addr().String()+"/team/providers", registry)
	if err != nil {
		t.Fatal(err)
	}
	if len(providers) != 2 || providers[0].Name != "awg" || providers[1].Name != "xray" {
		t.Fatalf("providers = %#v", providers)
	}
}

func TestRegistrySourceLifecycleKeepsPasswordOutOfList(t *testing.T) {
	svc := newTestService(t)
	initialized, err := svc.ListRegistrySources()
	if err != nil {
		t.Fatal(err)
	}
	if len(initialized) != 2 || initialized[0].URL != officialAgentRepository || initialized[0].Type != models.RegistrySourceAgent || initialized[1].URL != officialProviderRepository || initialized[1].Type != models.RegistrySourceProvider {
		t.Fatalf("default sources = %#v", initialized)
	}
	for _, source := range initialized {
		if err := svc.DeleteRegistrySource(source.ID.String()); err != nil {
			t.Fatal(err)
		}
	}
	source, err := svc.CreateRegistrySource("oci://registry.example:5000/team/agent/", models.RegistrySourceAgent, "reader", "secret", true)
	if err != nil {
		t.Fatal(err)
	}
	if source.URL != "oci://registry.example:5000/team/agent" || source.Type != models.RegistrySourceAgent || !source.HasPassword || source.Password != "" {
		t.Fatalf("created source = %#v", source)
	}
	list, err := svc.ListRegistrySources()
	if err != nil || len(list) != 1 || list[0].Password != "" || !list[0].HasPassword {
		t.Fatalf("sources = %#v, %v", list, err)
	}
	updated, err := svc.UpdateRegistrySource(source.ID.String(), source.URL, source.Type, "reader", "", false)
	if err != nil || !updated.HasPassword || updated.PlainHTTP {
		t.Fatalf("updated source = %#v, %v", updated, err)
	}
	resolved, err := svc.resolveRegistry(models.Registry{SourceID: source.ID.String()})
	if err != nil || resolved.Password != "secret" || resolved.PlainHTTP {
		t.Fatalf("resolved registry = %#v, %v", resolved, err)
	}
	cleared, err := svc.UpdateRegistrySource(source.ID.String(), source.URL, source.Type, "", "", false)
	if err != nil || cleared.HasPassword {
		t.Fatalf("cleared source = %#v, %v", cleared, err)
	}
	if err := svc.DeleteRegistrySource(source.ID.String()); err != nil {
		t.Fatal(err)
	}
}

func TestRegistrySourceAcceptsLocalAgentBinaryOnly(t *testing.T) {
	svc := newTestService(t)
	source, err := svc.CreateRegistrySource("/opt/vpn-agent", models.RegistrySourceAgent, "", "", false)
	if err != nil || source.URL != "/opt/vpn-agent" {
		t.Fatalf("local source = %#v, %v", source, err)
	}
	if _, err := svc.CreateRegistrySource("relative/vpn-agent", models.RegistrySourceAgent, "", "", false); err == nil {
		t.Fatal("relative local path was accepted")
	}
	if _, err := svc.CreateRegistrySource("/opt/provider", models.RegistrySourceProvider, "", "", false); err == nil {
		t.Fatal("local provider source was accepted")
	}
}
