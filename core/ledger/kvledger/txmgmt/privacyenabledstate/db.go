/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package privacyenabledstate

import (
	"github.com/hyperledger/fabric/core/ledger/cceventmgmt"
	"github.com/hyperledger/fabric/core/ledger/internal/state"
	"github.com/hyperledger/fabric/core/ledger/internal/version"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/statedb"
)

// DBProvider provides handle to a PvtVersionedDB
type DBProvider interface {
	// GetDBHandle returns a handle to a PvtVersionedDB
	GetDBHandle(id string) (DB, error)
	// Close closes all the PvtVersionedDB instances and releases any resources held by VersionedDBProvider
	Close()
}

// DB extends VersionedDB interface. This interface provides additional functions for managing private data state
type DB interface {
	statedb.VersionedDB
	IsBulkOptimizable() bool
	LoadCommittedVersionsOfPubAndHashedKeys(pubKeys []*state.CompositeKey, hashedKeys []*state.HashedCompositeKey) error
	GetCachedKeyHashVersion(namespace, collection string, keyHash []byte) (*version.Height, bool)
	ClearCachedVersions()
	GetChaincodeEventListener() cceventmgmt.ChaincodeLifecycleEventListener
	GetPrivateData(namespace, collection, key string) (*state.VersionedValue, error)
	GetPrivateDataHash(namespace, collection, key string) (*state.VersionedValue, error)
	GetValueHash(namespace, collection string, keyHash []byte) (*state.VersionedValue, error)
	GetKeyHashVersion(namespace, collection string, keyHash []byte) (*version.Height, error)
	GetPrivateDataMultipleKeys(namespace, collection string, keys []string) ([]*state.VersionedValue, error)
	GetPrivateDataRangeScanIterator(namespace, collection, startKey, endKey string) (state.ResultsIterator, error)
	GetStateMetadata(namespace, key string) ([]byte, error)
	GetPrivateDataMetadataByHash(namespace, collection string, keyHash []byte) ([]byte, error)
	ExecuteQueryOnPrivateData(namespace, collection, query string) (state.ResultsIterator, error)
	ApplyPrivacyAwareUpdates(updates *state.PubHashPvtUpdateBatch, height *version.Height) error
}
