/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package privacyenabledstate

import (
	"github.com/hyperledger/fabric/common/ledger/util/leveldbhelper"
)

type metadataHint struct {
	cache      map[string]bool
	bookkeeper *leveldbhelper.DBHandle
}

func newMetadataHint(bookkeeper *leveldbhelper.DBHandle) *metadataHint {
	cache := map[string]bool{}
	itr := bookkeeper.GetIterator(nil, nil)
	defer itr.Release()
	for itr.Next() {
		namespace := string(itr.Key())
		cache[namespace] = true
	}
	return &metadataHint{cache, bookkeeper}
}

func (h *metadataHint) metadataEverUsedFor(namespace string) bool {
	return h.cache[namespace]
}

func (h *metadataHint) setMetadataUsedFlag(updates *UpdateBatch) {
	batch := leveldbhelper.NewUpdateBatch()
	for ns := range filterNamespacesThatHasMetadata(updates) {
		if h.cache[ns] {
			continue
		}
		h.cache[ns] = true
		batch.Put([]byte(ns), []byte{})
	}
	h.bookkeeper.WriteBatch(batch, true)
}

func filterNamespacesThatHasMetadata(updates *UpdateBatch) map[string]bool {
	namespaces := map[string]bool{}
	pubUpdates, hashUpdates := updates.PubUpdates, updates.HashUpdates
	// add ns for public data
	for _, ns := range pubUpdates.GetUpdatedNamespaces() {
		for _, vv := range updates.PubUpdates.GetUpdates(ns) {
			if vv.Metadata == nil {
				continue
			}
			namespaces[ns] = true
		}
	}
	// add ns for private hashes
	for ns, nsBatch := range hashUpdates.UpdateMap {
		for _, coll := range nsBatch.GetCollectionNames() {
			for _, vv := range nsBatch.GetUpdates(coll) {
				if vv.Metadata == nil {
					continue
				}
				namespaces[ns] = true
			}
		}
	}
	return namespaces
}
