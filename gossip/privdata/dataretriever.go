/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package privdata

import (
	"errors"
	"fmt"

	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/transientstore"
	"github.com/hyperledger/fabric/gossip/util"
	"github.com/hyperledger/fabric/protos/common"
	gossip2 "github.com/hyperledger/fabric/protos/gossip"
	"github.com/hyperledger/fabric/protos/ledger/rwset"
)

// StorageDataRetriever defines an API to retrieve private date from the storage
type StorageDataRetriever interface {
	// CollectionRWSet retrieves for give digest relevant private data if
	// available otherwise returns nil
	CollectionRWSet(dig *gossip2.PvtDataDigest) (*util.PrivateRWSetWithConfig, error)
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

	// GetConfigHistoryRetriever returns the ConfigHistoryRetriever
	GetConfigHistoryRetriever() (ledger.ConfigHistoryRetriever, error)

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
func (dr *dataRetriever) CollectionRWSet(dig *gossip2.PvtDataDigest) (*util.PrivateRWSetWithConfig, error) {
	filter := map[string]ledger.PvtCollFilter{
		dig.Namespace: map[string]bool{
			dig.Collection: true,
		},
	}

	height, err := dr.store.LedgerHeight()
	if err != nil {
		// if there is an error getting info from the ledger, we need to try to read from transient store
		return nil, errors.New(fmt.Sprint("Wasn't able to read ledger height, due to", err, "trying to lookup "+
			"private data from transient store, namespace", dig.Namespace, "collection name", dig.Collection, "txID", dig.TxId))
	}
	if height <= dig.BlockSeq {
		logger.Debug("Current ledger height ", height, "is below requested block sequence number",
			dig.BlockSeq, "retrieving private data from transient store, namespace", dig.Namespace, "collection name",
			dig.Collection, "txID", dig.TxId)
	}
	if height <= dig.BlockSeq { // Check whenever current ledger height is equal or above block sequence num.
		return dr.fromTransientStore(dig, filter)
	}
	// Since ledger height is above block sequence number private data is might be available in the ledger
	return dr.fromLedger(dig, filter)
}

func (dr *dataRetriever) fromLedger(dig *gossip2.PvtDataDigest, filter map[string]ledger.PvtCollFilter) (*util.PrivateRWSetWithConfig, error) {
	results := &util.PrivateRWSetWithConfig{}
	pvtData, err := dr.store.GetPvtDataByNum(dig.BlockSeq, filter)
	if err != nil {
		return nil, errors.New(fmt.Sprint("wasn't able to obtain private data for collection", dig.Collection,
			"txID", dig.TxId, "block sequence number", dig.BlockSeq, "due to", err))
	}

	for _, data := range pvtData {
		if data.WriteSet == nil {
			logger.Warning("Received nil write set for collection", dig.Collection, "namespace", dig.Namespace)
			continue
		}
		pvtRWSet := dr.extractPvtRWsets(data.WriteSet.NsPvtRwset, dig.Namespace, dig.Collection)
		results.RWSet = append(results.RWSet, pvtRWSet...)
	}

	confHistoryRetriever, err := dr.store.GetConfigHistoryRetriever()
	if err != nil {
		return nil, errors.New(fmt.Sprint("cannot obtain configuration history retriever, for collection,", dig.Collection,
			"txID", dig.TxId, "block sequence number", dig.BlockSeq, "due to", err))
	}

	configInfo, err := confHistoryRetriever.MostRecentCollectionConfigBelow(dig.BlockSeq, dig.Namespace)
	if err != nil {
		return nil, errors.New(fmt.Sprint("cannot find recent collection config update below block sequence = ", dig.BlockSeq,
			"collection name =", dig.Collection, "for chaincode", dig.Namespace))
	}

	if configInfo == nil {
		return nil, errors.New(fmt.Sprint("no collection config update below block sequence = ", dig.BlockSeq,
			"collection name =", dig.Collection, "for chaincode", dig.Namespace, "is available"))
	}
	configs := dr.extractCollectionConfigs(configInfo.CollectionConfig, dig)
	if configs == nil {
		return nil, errors.New(fmt.Sprint("no collection config was found for collection", dig.Collection,
			"namespace", dig.Namespace, "txID", dig.TxId))
	}
	results.CollectionConfig = configs
	return results, nil
}

func (dr *dataRetriever) fromTransientStore(dig *gossip2.PvtDataDigest, filter map[string]ledger.PvtCollFilter) (*util.PrivateRWSetWithConfig, error) {
	results := &util.PrivateRWSetWithConfig{}
	it, err := dr.store.GetTxPvtRWSetByTxid(dig.TxId, filter)
	if err != nil {
		return nil, errors.New(fmt.Sprint("was not able to retrieve private data from transient store, namespace", dig.Namespace,
			", collection name", dig.Collection, ", txID", dig.TxId, ", due to", err))
	}
	defer it.Close()

	maxEndorsedAt := uint64(0)
	for {
		res, err := it.NextWithConfig()
		if err != nil {
			return nil, errors.New(fmt.Sprint("error getting next element out of private data iterator, namespace", dig.Namespace,
				", collection name", dig.Collection, ", txID", dig.TxId, ", due to", err))
		}
		if res == nil {
			return results, nil
		}
		rws := res.PvtSimulationResultsWithConfig
		if rws == nil {
			logger.Debug("Skipping nil PvtSimulationResultsWithConfig received at block height", res.ReceivedAtBlockHeight)
			continue
		}
		txPvtRWSet := rws.PvtRwset
		if txPvtRWSet == nil {
			logger.Debug("Skipping empty PvtRwset of PvtSimulationResultsWithConfig received at block height", res.ReceivedAtBlockHeight)
			continue
		}

		colConfigs, found := rws.CollectionConfigs[dig.Namespace]
		if !found {
			logger.Error("No collection config was found for chaincode", dig.Namespace, "collection name",
				dig.Namespace, "txID", dig.TxId)
			continue
		}
		configs := dr.extractCollectionConfigs(colConfigs, dig)
		if configs == nil {
			logger.Error("No collection config was found for collection", dig.Collection,
				"namespace", dig.Namespace, "txID", dig.TxId)
			continue
		}

		pvtRWSet := dr.extractPvtRWsets(txPvtRWSet.NsPvtRwset, dig.Namespace, dig.Collection)
		if rws.EndorsedAt >= maxEndorsedAt {
			maxEndorsedAt = rws.EndorsedAt
			results.CollectionConfig = configs
		}
		results.RWSet = append(results.RWSet, pvtRWSet...)
	}
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

func (dr *dataRetriever) extractCollectionConfigs(configPackage *common.CollectionConfigPackage, digest *gossip2.PvtDataDigest) *common.CollectionConfig {
	for _, config := range configPackage.Config {
		switch cconf := config.Payload.(type) {
		case *common.CollectionConfig_StaticCollectionConfig:
			if cconf.StaticCollectionConfig.Name == digest.Collection {
				return config
			}
		default:
			return nil
		}
	}
	return nil
}
