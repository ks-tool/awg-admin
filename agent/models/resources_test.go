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

package models

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestResourceWireFormatPreservesOpaqueSpecAndStatus(t *testing.T) {
	transition := time.Date(2026, time.November, 9, 12, 0, 0, 0, time.UTC)
	resource := Resource{
		ID:         "resource-id",
		ServerID:   "server-id",
		APIVersion: "example.ks-tool.dev/v1",
		Kind:       "Service",
		Name:       "public",
		Generation: 3,
		Spec:       json.RawMessage(`{"nested":{"enabled":true},"items":[1,2]}`),
		Status: ResourceStatus{
			ObservedGeneration: 2,
			Phase:              ResourceDegraded,
			Conditions: []Condition{{
				Type:               "Ready",
				Status:             ConditionFalse,
				Reason:             "ApplyFailed",
				Message:            "temporary provider error",
				ObservedGeneration: 2,
				LastTransitionTime: transition,
			}},
			Provider: json.RawMessage(`{"publicEndpoint":"192.0.2.1:1234"}`),
		},
	}

	data, err := json.Marshal(resource)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{
		`"id":"resource-id"`, `"serverId":"server-id"`, `"apiVersion":"example.ks-tool.dev/v1"`,
		`"kind":"Service"`, `"generation":3`, `"observedGeneration":2`,
		`"phase":"Degraded"`, `"reason":"ApplyFailed"`, `"message":"temporary provider error"`,
	} {
		if !strings.Contains(string(data), field) {
			t.Errorf("wire format does not contain %s: %s", field, data)
		}
	}

	var decoded Resource
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decoded, resource) {
		t.Fatalf("round trip changed resource:\nwant: %#v\n got: %#v", resource, decoded)
	}
}

func TestManifestAndDefinitionsWireFormat(t *testing.T) {
	manifest := Manifest{
		ProtocolVersion: ProtocolVersion,
		AgentID:         "agent-id",
		Providers: []ProviderDefinition{{
			Name:    "example",
			Version: "1.2.3",
			Resources: []ResourceDefinition{{
				APIVersion: "example.ks-tool.dev/v1",
				Kind:       "Service",
				SpecSchema: json.RawMessage(`{"type":"object"}`),
				UISchema:   json.RawMessage(`{"ui:order":["name"]}`),
				SchemaHash: "sha256:abc123",
				Ports: []PortDefinition{{
					Name:        "transport",
					Direction:   "source",
					LinkKinds:   []string{"example-tunnel"},
					RemotePorts: []string{"transport"},
				}},
				Actions: []ActionDefinition{{
					Name:                 "restart",
					Title:                "Restart service",
					ArgumentsSchema:      json.RawMessage(`{"type":"object","properties":{}}`),
					ResultType:           ActionResultAsync,
					TimeoutSeconds:       30,
					Mutating:             true,
					RequiresConfirmation: true,
				}},
			}},
		}},
	}

	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	var decoded Manifest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decoded, manifest) {
		t.Fatalf("round trip changed manifest:\nwant: %#v\n got: %#v", manifest, decoded)
	}
	if decoded.Providers[0].Resources[0].SchemaHash != "sha256:abc123" {
		t.Fatal("schema hash was not preserved")
	}
	if decoded.Providers[0].Resources[0].Actions[0].ResultType != ActionResultAsync {
		t.Fatal("action result type was not preserved")
	}
}

func TestLinkAndOpaqueOfferWireFormat(t *testing.T) {
	source := LinkEndpoint{
		ServerID: "server-a", ResourceID: "resource-a", APIVersion: "example.ks-tool.dev/v1",
		Kind: "Service", Name: "a", Port: "transport",
	}
	target := LinkEndpoint{
		ServerID: "server-b", ResourceID: "resource-b", APIVersion: "example.ks-tool.dev/v1",
		Kind: "Service", Name: "b", Port: "transport",
	}
	link := Link{
		ID: "link-id", Kind: "example-tunnel", Generation: 1,
		Source: source, Target: target,
		Spec:   json.RawMessage(`{"mtu":1420}`),
		Status: LinkStatus{ObservedGeneration: 1, Phase: LinkPreparing},
	}
	offer := LinkOffer{
		LinkID: "link-id", Endpoint: source,
		Data: json.RawMessage(`{"providerOpaque":{"token":"public-value"}}`),
	}

	linkData, err := json.Marshal(link)
	if err != nil {
		t.Fatal(err)
	}
	var decodedLink Link
	if err := json.Unmarshal(linkData, &decodedLink); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decodedLink, link) {
		t.Fatalf("round trip changed link:\nwant: %#v\n got: %#v", link, decodedLink)
	}

	offerData, err := json.Marshal(offer)
	if err != nil {
		t.Fatal(err)
	}
	var decodedOffer LinkOffer
	if err := json.Unmarshal(offerData, &decodedOffer); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decodedOffer, offer) {
		t.Fatalf("round trip changed offer:\nwant: %#v\n got: %#v", offer, decodedOffer)
	}
}

func TestOptionalWireFieldsAreOmitted(t *testing.T) {
	data, err := json.Marshal(ResourceDefinition{
		APIVersion: "example.ks-tool.dev/v1",
		Kind:       "Service",
		SpecSchema: json.RawMessage(`{"type":"object"}`),
		SchemaHash: "hash",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"uiSchema", "ports", "actions"} {
		if strings.Contains(string(data), `"`+field+`"`) {
			t.Errorf("empty %s was not omitted: %s", field, data)
		}
	}
}
