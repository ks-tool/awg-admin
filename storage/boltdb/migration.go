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
	"sort"
	"strconv"
	"time"

	agentmodels "github.com/ks-tool/awg-admin/agent/models"
	"github.com/ks-tool/awg-admin/models"
	"github.com/ks-tool/awg-admin/storage/boltdb/dump"

	"github.com/google/uuid"
	bolt "go.etcd.io/bbolt"
	"golang.org/x/crypto/bcrypt"
)

const currentSchemaVersion = 2

var (
	bktResources     = []byte("resources")
	bktLinks         = []byte("links")
	bktProviders     = []byte("providers")
	bktMeta          = []byte("meta")
	schemaVersionKey = []byte("schemaVersion")
)

func (db *BoltDB) initialize(path string) error {
	legacy := false
	version := 0
	if err := db.db.View(func(tx *bolt.Tx) error {
		meta := tx.Bucket(bktMeta)
		if meta != nil && meta.Get(schemaVersionKey) != nil {
			parsed, err := strconv.Atoi(string(meta.Get(schemaVersionKey)))
			if err != nil {
				return err
			}
			version = parsed
		}
		legacy = version == 0 && tx.Bucket(bktServers) != nil
		return nil
	}); err != nil {
		return err
	}
	if version > currentSchemaVersion {
		return fmt.Errorf("database schema version %d is newer than supported version %d", version, currentSchemaVersion)
	}
	if legacy {
		if err := backupBeforeMigration(db.db, path); err != nil {
			return fmt.Errorf("backup before migration: %w", err)
		}
	}

	return db.db.Update(func(tx *bolt.Tx) error {
		for _, bucket := range [][]byte{bktServers, bktInterfaces, bktUsers, bktAgentSources, bktTopology, bktAuth, bktResources, bktLinks, bktProviders, bktMeta} {
			if _, err := tx.CreateBucketIfNotExists(bucket); err != nil {
				return err
			}
		}
		if legacy {
			if err := migrateLegacyRecords(tx); err != nil {
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
		return tx.Bucket(bktMeta).Put(schemaVersionKey, []byte(strconv.Itoa(currentSchemaVersion)))
	})
}

func backupBeforeMigration(db *bolt.DB, path string) error {
	file, err := os.OpenFile(path+".v1.backup", os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	complete := false
	defer func() {
		_ = file.Close()
		if !complete {
			_ = os.Remove(file.Name())
		}
	}()
	if err := dump.Export(db, file); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	complete = true
	return nil
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
	interfaceServers := make(map[uuid.UUID]uuid.UUID)
	tunnelMembers := make(map[uuid.UUID][]models.Interface)
	if err := tx.Bucket(bktServers).ForEach(func(_, value []byte) error {
		var server models.Server
		if err := json.Unmarshal(value, &server); err != nil {
			return err
		}
		for _, interfaceID := range server.Interfaces {
			var iface models.Interface
			if err := json.Unmarshal(tx.Bucket(bktInterfaces).Get(idKey(interfaceID)), &iface); err != nil {
				return err
			}
			interfaces[iface.ID] = iface
			interfaceServers[iface.ID] = server.ID
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
			resource, err := migratePeer(user, peer, iface, interfaceServers[iface.ID])
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
			linkID := uuid.NewSHA1(uuid.NameSpaceOID, []byte(tunnelID.String()+":"+strconv.Itoa(index)))
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
	id := uuid.NewSHA1(uuid.NameSpaceOID, []byte(user.ID.String()+":"+peer.InterfaceId.String()+":"+publicKeyValue))
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

func migrateLink(id uuid.UUID, source, target models.Interface, servers map[uuid.UUID]uuid.UUID, spec json.RawMessage) models.Link {
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

func migrateEndpoint(iface models.Interface, serverID uuid.UUID) models.LinkEndpoint {
	return models.LinkEndpoint{
		ServerID: serverID.String(), ResourceID: iface.ID.String(), APIVersion: "awg/v1alpha1",
		Kind: "Interface", Name: iface.Interface, Port: "tunnel",
	}
}

func migrationStatus(iface models.Interface) models.ResourceStatus {
	status := models.ResourceStatus{Phase: models.ResourcePending}
	if iface.InSync {
		status.ObservedGeneration = 1
		status.Phase = models.ResourceReady
	}
	condition := models.Condition{Type: "Ready", Status: models.ConditionUnknown, Reason: "Pending", Message: "waiting for agent reconciliation", ObservedGeneration: status.ObservedGeneration, LastTransitionTime: time.Now().UTC()}
	if iface.InSync {
		condition.Status = models.ConditionTrue
		condition.Reason = "Reconciled"
		condition.Message = "resource was synchronized before migration"
		condition.LastTransitionTime = iface.LastSyncedAt
	}
	if iface.LastSyncError != "" {
		status.Phase = models.ResourceDegraded
		condition.Status = models.ConditionFalse
		condition.Reason = "ApplyFailed"
		condition.Message = iface.LastSyncError
	}
	status.Conditions = []models.Condition{condition}
	return status
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
