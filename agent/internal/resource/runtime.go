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
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"time"

	"github.com/ks-tool/awg-admin/agent/models"
	"github.com/ks-tool/awg-admin/agent/storage"
)

var (
	ErrUnknownProvider = errors.New("unknown provider")
	ErrUnknownResource = errors.New("unknown resource type")
	ErrUnknownAction   = errors.New("unknown action")
)

type queueKey struct {
	provider string
	kind     string
	name     string
}

type Runtime struct {
	store          Store
	agentID        string
	retryDelay     time.Duration
	resyncInterval time.Duration

	mu        sync.RWMutex
	providers map[string]Provider
	pending   map[queueKey]bool
	attempts  map[queueKey]int
	subs      map[chan Event]struct{}
	queue     chan queueKey
	ctx       context.Context
	cancel    context.CancelFunc
	started   bool
	wg        sync.WaitGroup
}

func NewRuntime(store Store, agentID string, retryDelay, resyncInterval time.Duration) *Runtime {
	return &Runtime{
		store: store, agentID: agentID, retryDelay: retryDelay, resyncInterval: resyncInterval,
		providers: make(map[string]Provider), pending: make(map[queueKey]bool),
		attempts: make(map[queueKey]int), subs: make(map[chan Event]struct{}),
		queue: make(chan queueKey, 1024),
	}
}

func (r *Runtime) Register(provider Provider) error {
	if provider == nil || provider.Name() == "" {
		return errors.New("provider name is required")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.started {
		return errors.New("provider registration is closed after runtime start")
	}
	if _, exists := r.providers[provider.Name()]; exists {
		return fmt.Errorf("provider %q is already registered", provider.Name())
	}
	r.providers[provider.Name()] = provider
	return nil
}

func (r *Runtime) Start(ctx context.Context) {
	r.mu.Lock()
	if r.started {
		r.mu.Unlock()
		return
	}
	r.ctx, r.cancel = context.WithCancel(ctx)
	r.started = true
	r.wg.Add(2)
	r.mu.Unlock()

	go func() {
		defer r.wg.Done()
		r.run()
	}()
	go func() {
		defer r.wg.Done()
		r.resync()
	}()
	r.Resync()
}

func (r *Runtime) Close() {
	r.mu.Lock()
	if r.cancel != nil {
		r.cancel()
	}
	for ch := range r.subs {
		close(ch)
		delete(r.subs, ch)
	}
	r.mu.Unlock()
	r.wg.Wait()
}

func (r *Runtime) Manifest() models.Manifest {
	r.mu.RLock()
	defer r.mu.RUnlock()
	manifest := models.Manifest{ProtocolVersion: models.ProtocolVersion, AgentID: r.agentID, Providers: make([]models.ProviderDefinition, 0, len(r.providers))}
	for _, provider := range r.providers {
		manifest.Providers = append(manifest.Providers, models.ProviderDefinition{
			Name: provider.Name(), Version: provider.Version(), Resources: provider.Definitions(),
		})
	}
	return manifest
}

func (r *Runtime) Schema(name string) (models.ProviderDefinition, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	provider, ok := r.providers[name]
	if !ok {
		return models.ProviderDefinition{}, fmt.Errorf("%w: %s", ErrUnknownProvider, name)
	}
	return models.ProviderDefinition{Name: provider.Name(), Version: provider.Version(), Resources: provider.Definitions()}, nil
}

func (r *Runtime) List(providerName string) ([]models.Resource, error) {
	if _, err := r.Schema(providerName); err != nil {
		return nil, err
	}
	items, err := r.store.List()
	if err != nil {
		return nil, err
	}
	result := make([]models.Resource, 0)
	for _, item := range items {
		if item.Provider == providerName && !item.Deleted {
			result = append(result, item.Resource)
		}
	}
	return result, nil
}

func (r *Runtime) Get(providerName, kind, name string) (models.Resource, error) {
	item, err := r.store.Get(providerName, kind, name)
	if err != nil {
		return models.Resource{}, err
	}
	if item.Deleted {
		return models.Resource{}, storage.ErrNotFound
	}
	return item.Resource, nil
}

func (r *Runtime) Put(providerName, kind, name string, desired models.Resource) (models.Resource, error) {
	provider, definition, err := r.definition(providerName, kind)
	if err != nil {
		return models.Resource{}, err
	}
	_ = provider
	if desired.APIVersion != definition.APIVersion {
		return models.Resource{}, fmt.Errorf("apiVersion must be %s", definition.APIVersion)
	}
	if desired.Kind != "" && desired.Kind != kind {
		return models.Resource{}, errors.New("kind does not match request path")
	}
	if desired.Name != "" && desired.Name != name {
		return models.Resource{}, errors.New("name does not match request path")
	}
	if len(desired.Spec) == 0 || !json.Valid(desired.Spec) {
		return models.Resource{}, errors.New("spec must be valid JSON")
	}

	desired.Kind = kind
	desired.Name = name
	key := queueKey{provider: providerName, kind: kind, name: name}
	existing, err := r.store.Get(providerName, kind, name)
	if err != nil && !storage.IsNotFound(err) {
		return models.Resource{}, err
	}
	if err == nil && !existing.Deleted {
		if desired.ID != "" && desired.ID != existing.Resource.ID {
			return models.Resource{}, errors.New("resource id is immutable")
		}
		desired.ID = existing.Resource.ID
		if desired.Generation == existing.Resource.Generation && reflect.DeepEqual(desired.Spec, existing.Resource.Spec) {
			return existing.Resource, nil
		}
		desired.Generation = existing.Resource.Generation + 1
		desired.Status = existing.Resource.Status
		desired.Status.Phase = models.ResourcePending
	} else {
		if desired.ID == "" {
			desired.ID, err = randomID()
			if err != nil {
				return models.Resource{}, err
			}
		}
		desired.Generation = 1
		desired.Status = models.ResourceStatus{Phase: models.ResourcePending}
	}
	if err := r.store.Put(StoredResource{Provider: providerName, Resource: desired}); err != nil {
		return models.Resource{}, err
	}
	r.publish("MODIFIED", providerName, desired)
	r.enqueue(key)
	return desired, nil
}

func (r *Runtime) Delete(providerName, kind, name string) error {
	if _, _, err := r.definition(providerName, kind); err != nil {
		return err
	}
	item, err := r.store.Get(providerName, kind, name)
	if err != nil {
		if storage.IsNotFound(err) {
			return nil
		}
		return err
	}
	if !item.Deleted {
		item.Deleted = true
		item.Resource.Generation++
		item.Resource.Status.Phase = models.ResourceDeleting
		if err := r.store.Put(item); err != nil {
			return err
		}
		r.publish("MODIFIED", providerName, item.Resource)
	}
	r.enqueue(queueKey{provider: providerName, kind: kind, name: name})
	return nil
}

func (r *Runtime) Action(ctx context.Context, providerName, kind, name, actionName string, arguments json.RawMessage) (models.ActionResult, error) {
	provider, definition, err := r.definition(providerName, kind)
	if err != nil {
		return models.ActionResult{}, err
	}
	found := false
	mutating := false
	for _, action := range definition.Actions {
		if action.Name == actionName {
			found = true
			mutating = action.Mutating
			break
		}
	}
	if !found {
		return models.ActionResult{}, fmt.Errorf("%w: %s", ErrUnknownAction, actionName)
	}
	actions, ok := provider.(ActionProvider)
	if !ok {
		return models.ActionResult{}, fmt.Errorf("%w: %s", ErrUnknownAction, actionName)
	}
	item, err := r.store.Get(providerName, kind, name)
	if err != nil {
		return models.ActionResult{}, err
	}
	if item.Deleted {
		return models.ActionResult{}, storage.ErrNotFound
	}
	result, err := actions.Action(ctx, item.Resource, actionName, arguments)
	if err == nil && mutating {
		r.enqueue(queueKey{provider: providerName, kind: kind, name: name})
	}
	return result, err
}

func (r *Runtime) PrepareLink(ctx context.Context, providerName string, link models.Link, endpoint models.LinkEndpoint) (models.LinkOffer, error) {
	provider, err := r.linkProvider(providerName)
	if err != nil {
		return models.LinkOffer{}, err
	}
	return provider.PrepareLink(ctx, link, endpoint)
}

func (r *Runtime) CommitLink(ctx context.Context, providerName string, link models.Link, endpoint models.LinkEndpoint, remote models.LinkOffer) error {
	provider, err := r.linkProvider(providerName)
	if err != nil {
		return err
	}
	return provider.CommitLink(ctx, link, endpoint, remote)
}

func (r *Runtime) DeleteLink(ctx context.Context, providerName string, link models.Link, endpoint models.LinkEndpoint, remote *models.LinkOffer) error {
	provider, err := r.linkProvider(providerName)
	if err != nil {
		return err
	}
	return provider.DeleteLink(ctx, link, endpoint, remote)
}

func (r *Runtime) linkProvider(name string) (LinkProvider, error) {
	r.mu.RLock()
	provider := r.providers[name]
	r.mu.RUnlock()
	if provider == nil {
		return nil, fmt.Errorf("%w: %s", ErrUnknownProvider, name)
	}
	links, ok := provider.(LinkProvider)
	if !ok {
		return nil, errors.New("provider does not support links")
	}
	return links, nil
}

func (r *Runtime) Resync() {
	items, err := r.store.List()
	if err != nil {
		return
	}
	for _, item := range items {
		r.enqueue(queueKey{provider: item.Provider, kind: item.Resource.Kind, name: item.Resource.Name})
	}
}

func (r *Runtime) Subscribe() (<-chan Event, func()) {
	ch := make(chan Event, 64)
	r.mu.Lock()
	if r.started && r.ctx.Err() != nil {
		close(ch)
		r.mu.Unlock()
		return ch, func() {}
	}
	r.subs[ch] = struct{}{}
	r.mu.Unlock()
	return ch, func() {
		r.mu.Lock()
		if _, ok := r.subs[ch]; ok {
			close(ch)
			delete(r.subs, ch)
		}
		r.mu.Unlock()
	}
}

func (r *Runtime) definition(providerName, kind string) (Provider, models.ResourceDefinition, error) {
	r.mu.RLock()
	provider, ok := r.providers[providerName]
	r.mu.RUnlock()
	if !ok {
		return nil, models.ResourceDefinition{}, fmt.Errorf("%w: %s", ErrUnknownProvider, providerName)
	}
	for _, definition := range provider.Definitions() {
		if definition.Kind == kind {
			return provider, definition, nil
		}
	}
	return nil, models.ResourceDefinition{}, fmt.Errorf("%w: %s/%s", ErrUnknownResource, providerName, kind)
}

func (r *Runtime) run() {
	for {
		select {
		case <-r.ctx.Done():
			return
		case key := <-r.queue:
			r.mu.Lock()
			delete(r.pending, key)
			r.mu.Unlock()
			r.reconcile(key)
		}
	}
}

func (r *Runtime) reconcile(key queueKey) {
	r.mu.RLock()
	provider := r.providers[key.provider]
	r.mu.RUnlock()
	if provider == nil {
		return
	}
	item, err := r.store.Get(key.provider, key.kind, key.name)
	if err != nil {
		return
	}
	generation := item.Resource.Generation
	var providerStatus json.RawMessage
	if item.Deleted {
		err = provider.Delete(r.ctx, item.Resource)
	} else {
		if err = provider.Validate(r.ctx, item.Resource); err == nil {
			providerStatus, err = provider.Reconcile(r.ctx, item.Resource)
		}
	}

	current, getErr := r.store.Get(key.provider, key.kind, key.name)
	if getErr != nil {
		return
	}
	if current.Resource.Generation != generation || current.Deleted != item.Deleted {
		r.enqueue(key)
		return
	}
	if err == nil && item.Deleted {
		if deleteErr := r.store.Delete(key.provider, key.kind, key.name); deleteErr == nil || storage.IsNotFound(deleteErr) {
			r.publish("DELETED", key.provider, item.Resource)
			r.clearAttempts(key)
		}
		return
	}

	if err != nil {
		current.Resource.Status = statusFor(current.Resource.Status, generation, models.ResourceDegraded, models.ConditionFalse, "ApplyFailed", err.Error(), current.Resource.Status.Provider)
		r.retry(key)
	} else {
		current.Resource.Status = statusFor(current.Resource.Status, generation, models.ResourceReady, models.ConditionTrue, "Reconciled", "resource is reconciled", providerStatus)
		r.clearAttempts(key)
	}
	if putErr := r.store.Put(current); putErr == nil {
		r.publish("MODIFIED", key.provider, current.Resource)
	}
}

func statusFor(previous models.ResourceStatus, generation int64, phase models.ResourcePhase, conditionStatus models.ConditionStatus, reason, message string, providerStatus json.RawMessage) models.ResourceStatus {
	condition := models.Condition{Type: "Ready", Status: conditionStatus, Reason: reason, Message: message, ObservedGeneration: generation, LastTransitionTime: time.Now().UTC()}
	if len(previous.Conditions) > 0 {
		last := previous.Conditions[0]
		if last.Status == conditionStatus && last.Reason == reason && last.Message == message {
			condition.LastTransitionTime = last.LastTransitionTime
		}
	}
	return models.ResourceStatus{ObservedGeneration: generation, Phase: phase, Conditions: []models.Condition{condition}, Provider: providerStatus}
}

func (r *Runtime) retry(key queueKey) {
	r.mu.Lock()
	attempt := r.attempts[key] + 1
	r.attempts[key] = attempt
	delay := r.retryDelay * time.Duration(1<<min(attempt-1, 6))
	r.mu.Unlock()
	time.AfterFunc(delay, func() { r.enqueue(key) })
}

func (r *Runtime) clearAttempts(key queueKey) {
	r.mu.Lock()
	delete(r.attempts, key)
	r.mu.Unlock()
}

func (r *Runtime) enqueue(key queueKey) {
	r.mu.Lock()
	if r.pending[key] || (r.started && r.ctx.Err() != nil) {
		r.mu.Unlock()
		return
	}
	r.pending[key] = true
	r.mu.Unlock()
	select {
	case r.queue <- key:
	default:
		r.mu.Lock()
		delete(r.pending, key)
		r.mu.Unlock()
	}
}

func (r *Runtime) resync() {
	ticker := time.NewTicker(r.resyncInterval)
	defer ticker.Stop()
	for {
		select {
		case <-r.ctx.Done():
			return
		case <-ticker.C:
			r.Resync()
		}
	}
}

func (r *Runtime) publish(eventType, providerName string, resource models.Resource) {
	event := Event{Type: eventType, Provider: providerName, APIVersion: resource.APIVersion, Kind: resource.Kind, Name: resource.Name, Generation: resource.Generation, Status: resource.Status}
	r.mu.RLock()
	defer r.mu.RUnlock()
	for ch := range r.subs {
		select {
		case ch <- event:
		default:
		}
	}
}

func sha256Sum(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
