/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package txmgr

import (
	"sort"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/privacyenabledstate"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/statedb"
	"github.com/hyperledger/fabric/core/ledger/util"
	"github.com/pkg/errors"
)

// deterministicBytesForPubAndHashUpdates constructs the bytes for a given UpdateBatch
// while constructing the bytes, it considers only public writes and hashed writes for
// the collections. For achieving the determinism, it constructs a slice of proto messages
// of type 'KVWrite'. In the slice, all the writes for a namespace "ns1" appear before
// the writes for another namespace "ns2" if "ns1" < "ns2" (lexicographically). Within a
// namespace, all the public writes appear before the collection writes. Like namespaces,
// the collections writes within a namespace appear in the order of lexicographical order.
// If an entry has the same namespace as its preceding entry, the namespace field is skipped.
// A Similar treatment is given to the repetitive entries for a collection within a namespace.
// For illustration, see the corresponding unit tests
func deterministicBytesForPubAndHashUpdates(u *privacyenabledstate.UpdateBatch) ([]byte, error) {
	pubUpdates := u.PubUpdates
	hashUpdates := u.HashUpdates.UpdateMap
	namespaces := dedupAndSort(
		pubUpdates.GetUpdatedNamespaces(),
		hashUpdates.GetUpdatedNamespaces(),
	)

	kvWrites := []*KVWrite{}
	for _, ns := range namespaces {
		if ns == "" {
			// an empty namespace is used for persisting the channel config
			// skipping the channel config from including into commit hash computation
			// as this proto uses maps and hence is non deterministic
			continue
		}
		kvs := genKVsFromNsUpdates(pubUpdates.GetUpdates(ns))
		collsForNs, ok := hashUpdates[ns]
		if ok {
			kvs = append(kvs, genKVsFromCollsUpdates(collsForNs)...)
		}
		kvs[0].Namespace = ns
		kvWrites = append(kvWrites, kvs...)
	}
	updates := &Updates{
		Kvwrites: kvWrites,
	}

	batchBytes, err := proto.Marshal(updates)
	return batchBytes, errors.Wrap(err, "error constructing deterministic bytes from update batch")
}

func genKVsFromNsUpdates(nsUpdates map[string]*statedb.VersionedValue) []*KVWrite {
	return genKVs(nsUpdates)
}

func genKVsFromCollsUpdates(collsUpdates privacyenabledstate.NsBatch) []*KVWrite {
	collNames := collsUpdates.GetCollectionNames()
	sort.Strings(collNames)
	collsKVWrites := []*KVWrite{}
	for _, collName := range collNames {
		collUpdates := collsUpdates.GetCollectionUpdates(collName)
		kvs := genKVs(collUpdates)
		kvs[0].Collection = collName
		collsKVWrites = append(collsKVWrites, kvs...)
	}
	return collsKVWrites
}

func genKVs(updates map[string]*statedb.VersionedValue) []*KVWrite {
	keys := util.GetSortedKeys(updates)
	kvWrites := []*KVWrite{}
	for _, key := range keys {
		val := updates[key]
		kvWrites = append(
			kvWrites,
			&KVWrite{
				Key:          []byte(key),
				Value:        val.Value,
				IsDelete:     val.Value == nil,
				VersionBytes: val.Version.ToBytes(),
			},
		)
	}
	return kvWrites
}

func dedupAndSort(a, b []string) []string {
	m := map[string]struct{}{}
	for _, entry := range a {
		m[entry] = struct{}{}
	}
	for _, entry := range b {
		m[entry] = struct{}{}
	}
	return util.GetSortedKeys(m)
}
