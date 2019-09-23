/*
Copyright IBM Corp. All Rights Reserved.
SPDX-License-Identifier: Apache-2.0
*/

package privacyenabledstate

import (
	"sort"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/statedb"
	"github.com/hyperledger/fabric/core/ledger/util"
	"github.com/pkg/errors"
)

type UpdatesBytesBuilder struct {
}

// DeterministicBytesForPubAndHashUpdates constructs the bytes for a given UpdateBatch
// while constructing the bytes, it considers only public writes and hashed writes for
// the collections. For achieving the determinism, it constructs a slice of proto messages
// of type 'KVWriteProto'. In the slice, all the writes for a namespace "ns1" appear before
// the writes for another namespace "ns2" if "ns1" < "ns2" (lexicographically). Within a
// namespace, all the public writes appear before the collection writes. Like namespaces,
// the collections writes within a namespace appear in the order of lexicographical order.
// If an entry has the same namespace as its preceding entry, the namespace field is skipped.
// A Similar treatment is given to the repetitive entries for a collection within a namespace.
// For illustration, see the corresponding unit tests
func (bb *UpdatesBytesBuilder) DeterministicBytesForPubAndHashUpdates(u *UpdateBatch) ([]byte, error) {
	pubUpdates := u.PubUpdates
	hashUpdates := u.HashUpdates.UpdateMap
	namespaces := dedupAndSort(
		pubUpdates.GetUpdatedNamespaces(),
		hashUpdates.getUpdatedNamespaces(),
	)

	kvWritesProto := []*KVWriteProto{}
	for _, ns := range namespaces {
		if ns == "" {
			// an empty namespace is used for persisting the channel config
			// skipping the channel config from including into commit hash computation
			// as this proto uses maps and hence is non deterministic
			continue
		}
		p := bb.buildForKeys(pubUpdates.GetUpdates(ns))
		collsForNs, ok := hashUpdates[ns]
		if ok {
			p = append(p, bb.buildForColls(collsForNs)...)
		}
		p[0].Namespace = ns
		kvWritesProto = append(kvWritesProto, p...)
	}
	batchProto := &KVWritesBatchProto{
		Kvwrites: kvWritesProto,
	}

	batchBytes, err := proto.Marshal(batchProto)
	return batchBytes, errors.Wrap(err, "error constructing deterministic bytes from update batch")
}

func (bb *UpdatesBytesBuilder) buildForColls(colls nsBatch) []*KVWriteProto {
	collNames := colls.GetCollectionNames()
	sort.Strings(collNames)
	collsProto := []*KVWriteProto{}
	for _, collName := range collNames {
		collUpdates := colls.getCollectionUpdates(collName)
		p := bb.buildForKeys(collUpdates)
		p[0].Collection = collName
		collsProto = append(collsProto, p...)
	}
	return collsProto
}

func (bb *UpdatesBytesBuilder) buildForKeys(kv map[string]*statedb.VersionedValue) []*KVWriteProto {
	keys := util.GetSortedKeys(kv)
	p := []*KVWriteProto{}
	for _, key := range keys {
		val := kv[key]
		p = append(
			p,
			&KVWriteProto{
				Key:          []byte(key),
				Value:        val.Value,
				IsDelete:     val.Value == nil,
				VersionBytes: val.Version.ToBytes(),
			},
		)
	}
	return p
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
