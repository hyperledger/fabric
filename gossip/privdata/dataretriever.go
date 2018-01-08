/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package privdata

import (
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/transientstore"
	"github.com/hyperledger/fabric/gossip/util"
	gossip2 "github.com/hyperledger/fabric/protos/gossip"
	"github.com/hyperledger/fabric/protos/ledger/rwset"
)

// StorageDataRetriever defines an API to retrieve private date from the storage
type StorageDataRetriever interface {
	// CollectionRWSet retrieves for give digest relevant private data if
	// available otherwise returns nil
	CollectionRWSet(dig *gossip2.PvtDataDigest) []util.PrivateRWSet
}

// DataStore defines set of APIs need to get private data
// from underlined data store
type DataStore interface {
	// GetTxPvtRWSetByTxid returns an iterator due to the fact that the txid may have multiple private
	// RWSets persisted from different endorsers (via Gossip)
	GetTxPvtRWSetByTxid(txid string, filter ledger.PvtNsCollFilter) (transientstore.RWSetScanner, error)

	// GetPvtDataByNum returns a slice of the private data from the ledger
	// for given block and based on the filter which indicates a map of
	// collections and namespaces of private data to retrieve
	GetPvtDataByNum(blockNum uint64, filter ledger.PvtNsCollFilter) ([]*ledger.TxPvtData, error)

	// Get recent block sequence number
	LedgerHeight() (uint64, error)
}

type dataRetriever struct {
	store DataStore
}

// NewDataRetriever constructing function for implementation of the
// StorageDataRetriever interface
func NewDataRetriever(store DataStore) StorageDataRetriever {
	return &dataRetriever{store: store}
}

// CollectionRWSet retrieves for give digest relevant private data if
// available otherwise returns nil
func (dr *dataRetriever) CollectionRWSet(dig *gossip2.PvtDataDigest) []util.PrivateRWSet {
	filter := map[string]ledger.PvtCollFilter{
		dig.Namespace: map[string]bool{
			dig.Collection: true,
		},
	}

	pRWsets := []util.PrivateRWSet{}

	height, err := dr.store.LedgerHeight()
	if err != nil {
		// if there is an error getting info from the ledger, we need to try to read from transient store
		logger.Warning("Wasn't able to read ledger height, due to", err, "trying to lookup "+
			"private data from transient store, namespace", dig.Namespace, "collection name", dig.Collection, "txID", dig.TxId)
	}
	if height <= dig.BlockSeq {
		logger.Debug("Current ledger height ", height, "is below requested block sequence number",
			dig.BlockSeq, "retrieving private data from transient store, namespace", dig.Namespace, "collection name",
			dig.Collection, "txID", dig.TxId)
	}
	if err != nil || height <= dig.BlockSeq { // Check whenever current ledger height is equal or above block sequence num.

		it, err := dr.store.GetTxPvtRWSetByTxid(dig.TxId, filter)
		if err != nil {
			logger.Error("Was not able to retrieve private data from transient store, namespace", dig.Namespace,
				", collection name", dig.Collection, ", txID", dig.TxId, ", due to", err)
			return nil
		}
		defer it.Close()

		for {
			res, err := it.Next()
			if err != nil {
				logger.Error("Error getting next element out of private data iterator, namespace", dig.Namespace,
					", collection name", dig.Collection, ", txID", dig.TxId, ", due to", err)
				return nil
			}
			if res == nil {
				return pRWsets
			}
			rws := res.PvtSimulationResults
			if rws == nil {
				logger.Debug("Skipping empty PvtSimulationResults received at block height", res.ReceivedAtBlockHeight)
				continue
			}
			pRWsets = append(pRWsets, dr.extractPvtRWsets(rws.NsPvtRwset, dig.Namespace, dig.Collection)...)
		}
	} else { // Since ledger height is above block sequence number private data is available in the ledger
		pvtData, err := dr.store.GetPvtDataByNum(dig.BlockSeq, filter)
		if err != nil {
			logger.Error("Wasn't able to obtain private data for collection", dig.Collection,
				"txID", dig.TxId, "block sequence number", dig.BlockSeq, "due to", err)
		}
		for _, data := range pvtData {
			if data.WriteSet == nil {
				logger.Warning("Received nil write set for collection", dig.Collection, "namespace", dig.Namespace)
				continue
			}
			pRWsets = append(pRWsets, dr.extractPvtRWsets(data.WriteSet.NsPvtRwset, dig.Namespace, dig.Collection)...)
		}
	}

	return pRWsets
}

func (dr *dataRetriever) extractPvtRWsets(pvtRWSets []*rwset.NsPvtReadWriteSet, namespace string, collectionName string) []util.PrivateRWSet {
	pRWsets := []util.PrivateRWSet{}

	// Iterate over all namespaces
	for _, nsws := range pvtRWSets {
		// and in each namespace - iterate over all collections
		if nsws.Namespace != namespace {
			logger.Warning("Received private data namespace", nsws.Namespace, "instead of", namespace, "skipping...")
			continue
		}
		for _, col := range nsws.CollectionPvtRwset {
			// This isn't the collection we're looking for
			if col.CollectionName != collectionName {
				logger.Warning("Received private data collection", col.CollectionName, "instead of", collectionName, "skipping...")
				continue
			}
			// Add the collection pRWset to the accumulated set
			pRWsets = append(pRWsets, col.Rwset)
		}
	}

	return pRWsets
}
