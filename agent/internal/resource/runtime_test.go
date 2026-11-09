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

package resource

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/ks-tool/vpn-agent/models"
	"github.com/ks-tool/vpn-agent/storage"
)

type testProvider struct {
	mu            sync.Mutex
	reconcile     int
	deletes       int
	failReconcile int
	failDelete    int
	last          models.Resource
}

func (p *testProvider) Name() string    { return "test" }
func (p *testProvider) Version() string { return "1.0.0" }
func (p *testProvider) Definitions() []models.ResourceDefinition {
	return []models.ResourceDefinition{{
		APIVersion: "test/v1", Kind: "Service", SpecSchema: json.RawMessage(`{"type":"object"}`), SchemaHash: "hash",
		Actions: []models.ActionDefinition{{Name: "inspect", Title: "Inspect", ResultType: models.ActionResultJSON}},
	}}
}
func (p *testProvider) Validate(context.Context, models.Resource) error { return nil }
func (p *testProvider) Reconcile(_ context.Context, resource models.Resource) (json.RawMessage, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.reconcile++
	p.last = resource
	if p.reconcile <= p.failReconcile {
		return nil, errors.New("temporary apply error")
	}
	return json.RawMessage(`{"endpoint":"192.0.2.1"}`), nil
}
func (p *testProvider) Delete(_ context.Context, resource models.Resource) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.deletes++
	p.last = resource
	if p.deletes <= p.failDelete {
		return errors.New("temporary delete error")
	}
	return nil
}
func (p *testProvider) Action(_ context.Context, _ models.Resource, name string, arguments json.RawMessage) (models.ActionResult, error) {
	if name != "inspect" {
		return models.ActionResult{}, ErrUnknownAction
	}
	return models.ActionResult{Type: models.ActionResultJSON, Data: arguments}, nil
}
func (p *testProvider) counts() (int, int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.reconcile, p.deletes
}

func TestRuntimeLifecycleRestartAndResync(t *testing.T) {
	store, err := NewFileStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	agentID, err := store.AgentID()
	if err != nil {
		t.Fatal(err)
	}
	provider := &testProvider{failReconcile: 1, failDelete: 1}
	runtime := NewRuntime(store, agentID, time.Millisecond, time.Hour)
	if err := runtime.Register(provider); err != nil {
		t.Fatal(err)
	}
	runtime.Start(context.Background())

	desired := models.Resource{APIVersion: "test/v1", Spec: json.RawMessage(`{"enabled":true}`)}
	created, err := runtime.Put("test", "Service", "public", desired)
	if err != nil {
		t.Fatal(err)
	}
	if created.ID == "" || created.Generation != 1 {
		t.Fatalf("unexpected created resource: %+v", created)
	}
	waitForStatus(t, runtime, 1, models.ResourceReady)
	reconciles, _ := provider.counts()
	if reconciles != 2 {
		t.Fatalf("expected failed apply and retry, got %d applies", reconciles)
	}

	desired.ID = created.ID
	desired.Generation = created.Generation
	repeated, err := runtime.Put("test", "Service", "public", desired)
	if err != nil {
		t.Fatal(err)
	}
	if repeated.Generation != 1 {
		t.Fatalf("repeated revision changed generation: %d", repeated.Generation)
	}
	time.Sleep(10 * time.Millisecond)
	reconciles, _ = provider.counts()
	if reconciles != 2 {
		t.Fatalf("repeated revision triggered apply: %d", reconciles)
	}

	desired.Spec = json.RawMessage(`{"enabled":false}`)
	updated, err := runtime.Put("test", "Service", "public", desired)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Generation != 2 {
		t.Fatalf("expected generation 2, got %d", updated.Generation)
	}
	waitForStatus(t, runtime, 2, models.ResourceReady)

	result, err := runtime.Action(context.Background(), "test", "Service", "public", "inspect", json.RawMessage(`{"verbose":true}`))
	if err != nil {
		t.Fatal(err)
	}
	if string(result.Data) != `{"verbose":true}` {
		t.Fatalf("unexpected action result: %s", result.Data)
	}

	before, _ := provider.counts()
	runtime.Resync()
	waitForApplies(t, provider, before+1)
	runtime.Close()

	restartedProvider := &testProvider{failDelete: 1}
	restarted := NewRuntime(store, agentID, time.Millisecond, time.Hour)
	if err := restarted.Register(restartedProvider); err != nil {
		t.Fatal(err)
	}
	restarted.Start(context.Background())
	defer restarted.Close()
	waitForApplies(t, restartedProvider, 1)
	waitForStatus(t, restarted, 2, models.ResourceReady)
	if id, err := store.AgentID(); err != nil || id != agentID {
		t.Fatalf("agent id changed after restart: %q, %v", id, err)
	}

	if err := restarted.Delete("test", "Service", "public"); err != nil {
		t.Fatal(err)
	}
	waitForDelete(t, restartedProvider, 2)
	if _, err := restarted.Get("test", "Service", "public"); !storage.IsNotFound(err) {
		t.Fatalf("deleted resource is still available: %v", err)
	}
	if err := restarted.Delete("test", "Service", "public"); err != nil {
		t.Fatalf("repeated delete failed: %v", err)
	}
}

func TestRuntimeRetainsDesiredStateAndErrorStatus(t *testing.T) {
	store, err := NewFileStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	provider := &testProvider{failReconcile: 100}
	runtime := NewRuntime(store, "agent", time.Hour, time.Hour)
	if err := runtime.Register(provider); err != nil {
		t.Fatal(err)
	}
	runtime.Start(context.Background())
	defer runtime.Close()
	created, err := runtime.Put("test", "Service", "broken", models.Resource{APIVersion: "test/v1", Spec: json.RawMessage(`{"value":1}`)})
	if err != nil {
		t.Fatal(err)
	}
	waitForStatus(t, runtime, 1, models.ResourceDegraded)
	current, err := runtime.Get("test", "Service", "broken")
	if err != nil {
		t.Fatalf("desired state was removed after failure: %v", err)
	}
	if current.ID != created.ID || string(current.Spec) != `{"value":1}` {
		t.Fatalf("desired state changed: %+v", current)
	}
	condition := current.Status.Conditions[0]
	if condition.Reason != "ApplyFailed" || condition.Message != "temporary apply error" || condition.ObservedGeneration != 1 {
		t.Fatalf("unexpected failure condition: %+v", condition)
	}
}

func waitForStatus(t *testing.T, runtime *Runtime, generation int64, phase models.ResourcePhase) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		items, err := runtime.List("test")
		if err == nil && len(items) == 1 && items[0].Status.ObservedGeneration == generation && items[0].Status.Phase == phase {
			return
		}
		time.Sleep(time.Millisecond)
	}
	items, _ := runtime.List("test")
	t.Fatalf("status did not reach %s at generation %d: %+v", phase, generation, items)
}

func waitForApplies(t *testing.T, provider *testProvider, count int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		applies, _ := provider.counts()
		if applies >= count {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("provider did not reach %d applies", count)
}

func waitForDelete(t *testing.T, provider *testProvider, count int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		_, deletes := provider.counts()
		if deletes >= count {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("provider did not reach %d deletes", count)
}

func TestRuntimeFailedDeleteRemainsVisibleAndCanBeRetried(t *testing.T) {
	store, err := NewFileStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	provider := &testProvider{failDelete: 1}
	runtime := NewRuntime(store, "agent", time.Hour, time.Hour)
	if err := runtime.Register(provider); err != nil {
		t.Fatal(err)
	}
	runtime.Start(context.Background())
	defer runtime.Close()
	if _, err := runtime.Put("test", "Service", "public", models.Resource{APIVersion: "test/v1", Spec: json.RawMessage(`{}`)}); err != nil {
		t.Fatal(err)
	}
	waitForStatus(t, runtime, 1, models.ResourceReady)
	if err := runtime.Delete("test", "Service", "public"); err != nil {
		t.Fatal(err)
	}
	waitForDelete(t, provider, 1)
	waitForStatus(t, runtime, 2, models.ResourceDegraded)
	items, err := runtime.List("test")
	if err != nil || len(items) != 1 || items[0].Status.Phase != models.ResourceDegraded {
		t.Fatalf("failed deletion disappeared from inventory: %+v, %v", items, err)
	}
	if err := runtime.Delete("test", "Service", "public"); err != nil {
		t.Fatal(err)
	}
	waitForDelete(t, provider, 2)
	items, err = runtime.List("test")
	if err != nil || len(items) != 0 {
		t.Fatalf("resource survived successful deletion: %+v, %v", items, err)
	}
}
