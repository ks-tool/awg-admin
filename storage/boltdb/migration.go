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

package boltdb

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"uuid"

	"github.com/ks-tool/vpn-admin/models"
	agentmodels "github.com/ks-tool/vpn-agent/models"

	bolt "go.etcd.io/bbolt"
	"golang.org/x/crypto/bcrypt"
)

const currentSchemaVersion = 2

var (
	bktResources                 = []byte("resources")
	bktLinks                     = []byte("links")
	bktProviders                 = []byte("providers")
	bktMeta                      = []byte("meta")
	schemaVersionKey             = []byte("schemaVersion")
	registrySourcesInitializeKey = []byte("registrySourcesInitialized")
)

func (db *BoltDB) initialize(path string) error {
	version, legacy, _, err := readSchemaVersion(db.db)
	if err != nil {
		return err
	}
	if version > currentSchemaVersion {
		return fmt.Errorf("database schema version %d is newer than supported version %d", version, currentSchemaVersion)
	}
	if legacy {
		return fmt.Errorf("database %s uses schema v1; run awg-migrate v2 before starting vpn-admin", path)
	}
	return db.db.Update(initializeSchema)
}

func readSchemaVersion(db *bolt.DB) (int, bool, bool, error) {
	version := 0
	legacy := false
	current := false
	err := db.View(func(tx *bolt.Tx) error {
		meta := tx.Bucket(bktMeta)
		if meta != nil && meta.Get(schemaVersionKey) != nil {
			parsed, err := strconv.Atoi(string(meta.Get(schemaVersionKey)))
			if err != nil {
				return err
			}
			version = parsed
		}
		legacy = tx.Bucket(bktServers) != nil && tx.Bucket(bktResources) == nil && tx.Bucket(bktLinks) == nil
		current = tx.Bucket(bktResources) != nil && tx.Bucket(bktLinks) != nil
		return nil
	})
	return version, legacy, current, err
}

func initializeSchema(tx *bolt.Tx) error {
	for _, bucket := range [][]byte{bktServers, bktInterfaces, bktUsers, bktAgentSources, bktRegistrySources, bktTopology, bktAuth, bktResources, bktLinks, bktProviders, bktMeta} {
		if _, err := tx.CreateBucketIfNotExists(bucket); err != nil {
			return err
		}
	}
	authBucket := tx.Bucket(bktAuth)
	if authBucket.Get(authRecordKey) == nil {
		hash, err := bcrypt.GenerateFromPassword([]byte(defaultAdminPassword), bcrypt.DefaultCost)
		if err != nil {
			return err
		}
		if err := put(authBucket, authRecordKey, &models.AuthCredentials{Username: defaultAdminUsername, PasswordHash: string(hash)}); err != nil {
			return err
		}
	}
	if err := migrateRegistrySourceRecords(tx); err != nil {
		return err
	}
	meta := tx.Bucket(bktMeta)
	if meta.Get(registrySourcesInitializeKey) == nil {
		sources := []*models.RegistrySource{
			{ID: uuid.New(), URL: "oci://ghcr.io/ks-tool/vpn-agent", Type: models.RegistrySourceAgent},
			{ID: uuid.New(), URL: "oci://ghcr.io/ks-tool/vpn-providers", Type: models.RegistrySourceProvider},
		}
		for _, source := range sources {
			if err := put(tx.Bucket(bktRegistrySources), idKey(source.ID), source); err != nil {
				return err
			}
		}
		if err := meta.Put(registrySourcesInitializeKey, []byte("1")); err != nil {
			return err
		}
	}
	return meta.Put(schemaVersionKey, []byte(strconv.Itoa(currentSchemaVersion)))
}

func migrateRegistrySourceRecords(tx *bolt.Tx) error {
	type legacySource struct {
		ID                 uuid.UUID `json:"id"`
		URL                string    `json:"url"`
		Type               string    `json:"type"`
		AgentRepository    string    `json:"agentRepository"`
		ProviderRepository string    `json:"providerRepository"`
		Username           string    `json:"username"`
		Password           string    `json:"password"`
		PlainHTTP          bool      `json:"plainHttp"`
	}
	bucket := tx.Bucket(bktRegistrySources)
	return bucket.ForEach(func(key, value []byte) error {
		if value == nil {
			return nil
		}
		var legacy legacySource
		if err := json.Unmarshal(value, &legacy); err != nil {
			return err
		}
		if legacy.Type == models.RegistrySourceAgent || legacy.Type == models.RegistrySourceProvider {
			return nil
		}
		agentRepository := legacy.AgentRepository
		providerRepository := legacy.ProviderRepository
		if len(agentRepository) == 0 {
			agentRepository = "oci://ghcr.io/ks-tool/vpn-agent"
			if legacy.URL != "oci://ghcr.io" {
				agentRepository = strings.TrimSuffix(legacy.URL, "/") + "/ks-tool/vpn-agent"
			}
		}
		if len(providerRepository) == 0 {
			providerRepository = "oci://ghcr.io/ks-tool/vpn-providers"
			if legacy.URL != "oci://ghcr.io" {
				providerRepository = strings.TrimSuffix(legacy.URL, "/") + "/vpn-providers"
			}
		}
		sources := []*models.RegistrySource{
			{ID: legacy.ID, URL: agentRepository, Type: models.RegistrySourceAgent, Username: legacy.Username, Password: legacy.Password, PlainHTTP: legacy.PlainHTTP},
			{ID: uuid.New(), URL: providerRepository, Type: models.RegistrySourceProvider, Username: legacy.Username, Password: legacy.Password, PlainHTTP: legacy.PlainHTTP},
		}
		for _, source := range sources {
			if err := put(bucket, idKey(source.ID), source); err != nil {
				return err
			}
		}
		return nil
	})
}

func MigrateV2(sourcePath, destinationPath string, force bool) error {
	sourceAbs, err := filepath.Abs(sourcePath)
	if err != nil {
		return err
	}
	destinationAbs, err := filepath.Abs(destinationPath)
	if err != nil {
		return err
	}
	if sourceAbs == destinationAbs {
		return errors.New("source and destination must be different files")
	}
	if _, err := os.Stat(destinationAbs); err == nil && !force {
		return fmt.Errorf("destination %s already exists (use -force to replace it)", destinationAbs)
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}

	source, err := bolt.Open(sourceAbs, 0600, &bolt.Options{ReadOnly: true, Timeout: 5 * time.Second})
	if err != nil {
		return fmt.Errorf("open v1 source: %w", err)
	}
	defer func() { _ = source.Close() }()
	version, legacy, current, err := readSchemaVersion(source)
	if err != nil {
		return err
	}
	if current {
		return errors.New("source is already a v2 database; copy it to the v2 destination instead of migrating")
	}
	if !legacy {
		return fmt.Errorf("source schema version %d is not a v1 database", version)
	}

	if err := os.MkdirAll(filepath.Dir(destinationAbs), 0700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(destinationAbs), ".v2-migration-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	if err := temporary.Close(); err != nil {
		_ = os.Remove(temporaryPath)
		return err
	}
	defer func() { _ = os.Remove(temporaryPath) }()

	destination, err := bolt.Open(temporaryPath, 0600, nil)
	if err != nil {
		return err
	}
	err = source.View(func(sourceTx *bolt.Tx) error {
		return destination.Update(func(destinationTx *bolt.Tx) error {
			if err := sourceTx.ForEach(func(name []byte, sourceBucket *bolt.Bucket) error {
				destinationBucket, err := destinationTx.CreateBucketIfNotExists(name)
				if err != nil {
					return err
				}
				return sourceBucket.ForEach(func(key, value []byte) error {
					if value == nil {
						return nil
					}
					return destinationBucket.Put(key, value)
				})
			}); err != nil {
				return err
			}
			if err := initializeSchema(destinationTx); err != nil {
				return err
			}
			return migrateLegacyRecords(destinationTx)
		})
	})
	if err != nil {
		_ = destination.Close()
		return err
	}
	if err := destination.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryPath, destinationAbs)
}

func (db *BoltDB) SchemaVersion() (int, error) {
	version := 0
	err := db.db.View(func(tx *bolt.Tx) error {
		value := tx.Bucket(bktMeta).Get(schemaVersionKey)
		if value == nil {
			return errors.New("schema version is missing")
		}
		parsed, err := strconv.Atoi(string(value))
		version = parsed
		return err
	})
	return version, err
}

func migrateLegacyRecords(tx *bolt.Tx) error {
	interfaces := make(map[uuid.UUID]models.Interface)
	interfaceServers := make(map[uuid.UUID]models.Server)
	tunnelMembers := make(map[uuid.UUID][]models.Interface)
	if err := tx.Bucket(bktServers).ForEach(func(key, value []byte) error {
		var server models.Server
		if err := json.Unmarshal(value, &server); err != nil {
			return err
		}
		server.Unmanaged = true
		if err := put(tx.Bucket(bktServers), key, &server); err != nil {
			return err
		}
		for _, interfaceID := range server.Interfaces {
			var iface models.Interface
			if err := json.Unmarshal(tx.Bucket(bktInterfaces).Get(idKey(interfaceID)), &iface); err != nil {
				return err
			}
			interfaces[iface.ID] = iface
			interfaceServers[iface.ID] = server
			if iface.Tunnel != nil {
				tunnelMembers[*iface.Tunnel] = append(tunnelMembers[*iface.Tunnel], iface)
			}
			resource, err := migrateInterface(server.ID, iface)
			if err != nil {
				return err
			}
			if err := put(tx.Bucket(bktResources), []byte(resource.ID), resource); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return err
	}

	if err := tx.Bucket(bktUsers).ForEach(func(_, value []byte) error {
		var user models.User
		if err := json.Unmarshal(value, &user); err != nil {
			return err
		}
		for _, peer := range user.Peers {
			iface, ok := interfaces[peer.InterfaceId]
			if !ok {
				continue
			}
			resource, err := migratePeer(user, peer, iface, interfaceServers[iface.ID].ID)
			if err != nil {
				return err
			}
			if err := put(tx.Bucket(bktResources), []byte(resource.ID), resource); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return err
	}

	pairs := make(map[string]struct{})
	for tunnelID, members := range tunnelMembers {
		sort.SliceStable(members, func(i, j int) bool {
			if (members[i].Table == 0) != (members[j].Table == 0) {
				return members[i].Table != 0
			}
			return members[i].ID.String() < members[j].ID.String()
		})
		for index := 0; index+1 < len(members); index++ {
			source, target := members[index], members[index+1]
			linkID := models.NewDeterministicUUID(tunnelID.String() + ":" + strconv.Itoa(index))
			if index == 0 {
				linkID = tunnelID
			}
			link := migrateLink(linkID, source, target, interfaceServers, json.RawMessage(fmt.Sprintf(`{"tunnelId":%q}`, tunnelID.String())))
			if err := put(tx.Bucket(bktLinks), []byte(link.ID), link); err != nil {
				return err
			}
			pairs[source.ID.String()+target.ID.String()] = struct{}{}
		}
	}

	var topology models.Topology
	if value := tx.Bucket(bktTopology).Get(topologyRecordKey); len(value) > 0 {
		if err := json.Unmarshal(value, &topology); err != nil {
			return err
		}
		for _, connection := range topology.Connections {
			source, sourceOK := interfaces[connection.SourceID]
			target, targetOK := interfaces[connection.TargetID]
			if !sourceOK || !targetOK {
				continue
			}
			pair := source.ID.String() + target.ID.String()
			if _, exists := pairs[pair]; exists {
				continue
			}
			link := migrateLink(connection.ID, source, target, interfaceServers, json.RawMessage(`{}`))
			if err := put(tx.Bucket(bktLinks), []byte(link.ID), link); err != nil {
				return err
			}
			pairs[pair] = struct{}{}
		}
	}
	return nil
}

func migrateInterface(serverID uuid.UUID, iface models.Interface) (models.Resource, error) {
	data, err := json.Marshal(iface.InterfaceConfig)
	if err != nil {
		return models.Resource{}, err
	}
	var spec map[string]any
	if err := json.Unmarshal(data, &spec); err != nil {
		return models.Resource{}, err
	}
	for _, field := range []string{"pk", "peers", "preUp", "postUp", "preDown", "postDown"} {
		delete(spec, field)
	}
	rawSpec, err := json.Marshal(spec)
	if err != nil {
		return models.Resource{}, err
	}
	status := migrationStatus(iface)
	provider, _ := json.Marshal(map[string]any{
		"interface": iface.Interface, "publicKey": publicKey(iface.PrivateKey), "secretMigrationPending": true,
	})
	status.Provider = provider
	status.Conditions = append(status.Conditions, secretMigrationCondition())
	return models.Resource{
		ID: iface.ID.String(), ServerID: serverID.String(), APIVersion: "awg/v1alpha1", Kind: "Interface",
		Name: iface.Interface, Generation: 1, Spec: rawSpec, Status: status,
	}, nil
}

func migratePeer(user models.User, peer models.Peer, iface models.Interface, serverID uuid.UUID) (models.Resource, error) {
	allowedIPs := make([]string, 0)
	keepaliveSeconds := 0
	publicKeyValue := publicKey(peer.PrivateKey)
	for _, configured := range iface.Peers {
		if configured.Key.String() == publicKeyValue {
			allowedIPs = append(allowedIPs, configured.AllowedIPs...)
			keepaliveSeconds = int(configured.KeepaliveInterval.Seconds())
			break
		}
	}
	id := models.NewDeterministicUUID(user.ID.String() + ":" + peer.InterfaceId.String() + ":" + publicKeyValue)
	spec, err := json.Marshal(map[string]any{
		"name": peer.Name, "interfaceRef": peer.InterfaceId.String(), "userRef": user.ID.String(), "publicKey": publicKeyValue,
		"allowedIPs": allowedIPs, "dns": peer.DNS, "disabled": peer.Disabled || user.Disabled,
		"keepaliveSeconds": keepaliveSeconds,
	})
	if err != nil {
		return models.Resource{}, err
	}
	status := migrationStatus(iface)
	provider, _ := json.Marshal(map[string]any{"publicKey": publicKeyValue, "secretMigrationPending": true})
	status.Provider = provider
	status.Conditions = append(status.Conditions, secretMigrationCondition())
	return models.Resource{
		ID: id.String(), ServerID: serverID.String(), APIVersion: "awg/v1alpha1", Kind: "Peer",
		Name: user.Name + "-" + peer.Name + "-" + id.String()[:8], Generation: 1, Spec: spec, Status: status,
	}, nil
}

func migrateLink(id uuid.UUID, source, target models.Interface, servers map[uuid.UUID]models.Server, spec json.RawMessage) models.Link {
	status := migrationStatus(source)
	targetStatus := migrationStatus(target)
	if status.Phase != models.ResourceDegraded && targetStatus.Phase == models.ResourceDegraded {
		status = targetStatus
	} else if status.Phase == models.ResourceReady && targetStatus.Phase != models.ResourceReady {
		status = targetStatus
	}
	return models.Link{
		ID: id.String(), Kind: "awg", Generation: 1,
		Source: migrateEndpoint(source, servers[source.ID]), Target: migrateEndpoint(target, servers[target.ID]),
		Spec:   spec,
		Status: models.LinkStatus{ObservedGeneration: status.ObservedGeneration, Phase: models.LinkPhase(status.Phase), Conditions: status.Conditions},
	}
}

func migrateEndpoint(iface models.Interface, server models.Server) models.LinkEndpoint {
	return models.LinkEndpoint{
		ServerID: server.ID.String(), ResourceID: iface.ID.String(), APIVersion: "awg/v1alpha1",
		Kind: "Interface", Name: iface.Interface, Port: "tunnel", Address: server.SSH.Host,
	}
}

func migrationStatus(_ models.Interface) models.ResourceStatus {
	now := time.Now().UTC()
	return models.ResourceStatus{
		Phase: models.ResourcePending,
		Conditions: []models.Condition{
			{Type: "Ready", Status: models.ConditionUnknown, Reason: "Unmanaged", Message: "waiting for vpn-agent and AWG provider migration", LastTransitionTime: now},
			{Type: "Managed", Status: models.ConditionFalse, Reason: "Unmanaged", Message: "resource has not been adopted by an agent provider", LastTransitionTime: now},
		},
	}
}

func secretMigrationCondition() models.Condition {
	return models.Condition{
		Type: "Secrets", Status: models.ConditionFalse, Reason: "SecretMigrationPending",
		Message:            "private key remains in legacy storage until the agent confirms migration",
		LastTransitionTime: time.Now().UTC(),
	}
}

func publicKey(privateKey agentmodels.Key) string {
	publicKey := privateKey.PublicKey()
	return publicKey.String()
}
