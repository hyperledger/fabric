/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package privdata

import (
	protosgossip "github.com/hyperledger/fabric-protos-go-apiv2/gossip"
	"github.com/hyperledger/fabric-protos-go-apiv2/ledger/rwset"
	"github.com/hyperledger/fabric/core/committer"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/transientstore"
	"github.com/hyperledger/fabric/gossip/privdata/common"
	"github.com/hyperledger/fabric/gossip/util"
	"github.com/pkg/errors"
)

//go:generate mockery -dir . -name RWSetScanner -case underscore -output mocks/

// RWSetScanner is the local interface used to generate mocks for foreign interface.
type RWSetScanner interface {
	transientstore.RWSetScanner
}

// StorageDataRetriever defines an API to retrieve private data from the storage.
type StorageDataRetriever interface {
	// CollectionRWSet retrieves for give digest relevant private data if
	// available otherwise returns nil, bool which is true if data fetched from ledger and false if was fetched from transient store, and an error
	CollectionRWSet(dig []*protosgossip.PvtDataDigest, blockNum uint64) (Dig2PvtRWSetWithConfig, bool, error)
}

type dataRetriever struct {
	logger    util.Logger
	store     *transientstore.Store
	committer committer.Committer
}

// NewDataRetriever constructing function for implementation of the
// StorageDataRetriever interface
func NewDataRetriever(channelID string, store *transientstore.Store, committer committer.Committer) StorageDataRetriever {
	return &dataRetriever{
		logger:    logger.With("channel", channelID),
		store:     store,
		committer: committer,
	}
}

// CollectionRWSet retrieves for give digest relevant private data if
// available otherwise returns nil, bool which is true if data fetched from ledger and false if was fetched from transient store, and an error
func (dr *dataRetriever) CollectionRWSet(digests []*protosgossip.PvtDataDigest, blockNum uint64) (Dig2PvtRWSetWithConfig, bool, error) {
	height, err := dr.committer.LedgerHeight()
	if err != nil {
		// if there is an error getting info from the ledger, we need to try to read from transient store
		return nil, false, errors.Wrap(err, "wasn't able to read ledger height")
	}

	// The condition may be true for either commit or reconciliation case when another peer sends a request to retrieve private data.
	// For the commit case, get the private data from the transient store because the block has not been committed.
	// For the reconciliation case, this peer is further behind the ledger height than the peer that requested for the private data.
	// In this case, the ledger does not have the requested private data. Also, the data cannot be queried in the transient store,
	// as the txID in the digest will be missing.
	if height <= blockNum { // Check whenever current ledger height is equal or below block sequence num.
		dr.logger.Debug("Current ledger height ", height, "is below requested block sequence number",
			blockNum, "retrieving private data from transient store")

		results := make(Dig2PvtRWSetWithConfig)
		for _, dig := range digests {
			// skip retrieving from transient store if txid is not available
			if dig.GetTxId() == "" {
				dr.logger.Infof("Skip querying transient store for chaincode %s, collection name %s, block number %d, sequence in block %d, "+
					"as the txid is missing, perhaps because it is a reconciliation request",
					dig.GetNamespace(), dig.GetCollection(), blockNum, dig.GetSeqInBlock())

				continue
			}

			filter := map[string]ledger.PvtCollFilter{
				dig.GetNamespace(): map[string]bool{
					dig.GetCollection(): true,
				},
			}
			pvtRWSet, err := dr.fromTransientStore(dig, filter)
			if err != nil {
				dr.logger.Errorf("couldn't read from transient store private read-write set, "+
					"digest %+v, because of %s", dig, err)
				continue
			}
			results[common.DigKey{
				Namespace:  dig.GetNamespace(),
				Collection: dig.GetCollection(),
				TxId:       dig.GetTxId(),
				BlockSeq:   dig.GetBlockSeq(),
				SeqInBlock: dig.GetSeqInBlock(),
			}] = pvtRWSet
		}

		return results, false, nil
	}
	// Since ledger height is above block sequence number private data is might be available in the ledger
	results, err := dr.fromLedger(digests, blockNum)
	return results, true, err
}

func (dr *dataRetriever) fromLedger(digests []*protosgossip.PvtDataDigest, blockNum uint64) (Dig2PvtRWSetWithConfig, error) {
	filter := make(map[string]ledger.PvtCollFilter)
	for _, dig := range digests {
		if _, ok := filter[dig.GetNamespace()]; !ok {
			filter[dig.GetNamespace()] = make(ledger.PvtCollFilter)
		}
		filter[dig.GetNamespace()][dig.GetCollection()] = true
	}

	pvtData, err := dr.committer.GetPvtDataByNum(blockNum, filter)
	if err != nil {
		return nil, errors.Errorf("wasn't able to obtain private data, block sequence number %d, due to %s", blockNum, err)
	}

	results := make(Dig2PvtRWSetWithConfig)
	for _, dig := range digests {
		pvtRWSetWithConfig := &util.PrivateRWSetWithConfig{}
		for _, data := range pvtData {
			if data.WriteSet == nil {
				dr.logger.Warning("Received nil write set for collection tx in block", data.SeqInBlock, "block number", blockNum)
				continue
			}

			// private data doesn't hold rwsets for namespace and collection or
			// belongs to different transaction
			if !data.Has(dig.GetNamespace(), dig.GetCollection()) || data.SeqInBlock != dig.GetSeqInBlock() {
				continue
			}

			pvtRWSet := dr.extractPvtRWsets(data.WriteSet.GetNsPvtRwset(), dig.GetNamespace(), dig.GetCollection())
			pvtRWSetWithConfig.RWSet = append(pvtRWSetWithConfig.RWSet, pvtRWSet...)
		}

		confHistoryRetriever, err := dr.committer.GetConfigHistoryRetriever()
		if err != nil {
			return nil, errors.Errorf("cannot obtain configuration history retriever, for collection <%s>"+
				" txID <%s> block sequence number <%d> due to <%s>", dig.GetCollection(), dig.GetTxId(), dig.GetBlockSeq(), err)
		}

		configInfo, err := confHistoryRetriever.MostRecentCollectionConfigBelow(dig.GetBlockSeq(), dig.GetNamespace())
		if err != nil {
			return nil, errors.Errorf("cannot find recent collection config update below block sequence = %d,"+
				" collection name = <%s> for chaincode <%s>", dig.GetBlockSeq(), dig.GetCollection(), dig.GetNamespace())
		}

		if configInfo == nil {
			return nil, errors.Errorf("no collection config update below block sequence = <%d>"+
				" collection name = <%s> for chaincode <%s> is available ", dig.GetBlockSeq(), dig.GetCollection(), dig.GetNamespace())
		}
		configs := extractCollectionConfig(configInfo.CollectionConfig, dig.GetCollection())
		if configs == nil {
			return nil, errors.Errorf("no collection config was found for collection <%s>"+
				" namespace <%s> txID <%s>", dig.GetCollection(), dig.GetNamespace(), dig.GetTxId())
		}
		pvtRWSetWithConfig.CollectionConfig = configs
		results[common.DigKey{
			Namespace:  dig.GetNamespace(),
			Collection: dig.GetCollection(),
			TxId:       dig.GetTxId(),
			BlockSeq:   dig.GetBlockSeq(),
			SeqInBlock: dig.GetSeqInBlock(),
		}] = pvtRWSetWithConfig
	}

	return results, nil
}

func (dr *dataRetriever) fromTransientStore(dig *protosgossip.PvtDataDigest, filter map[string]ledger.PvtCollFilter) (*util.PrivateRWSetWithConfig, error) {
	results := &util.PrivateRWSetWithConfig{}
	it, err := dr.store.GetTxPvtRWSetByTxid(dig.GetTxId(), filter)
	if err != nil {
		return nil, errors.Errorf("was not able to retrieve private data from transient store, namespace <%s>"+
			", collection name %s, txID <%s>, due to <%s>", dig.GetNamespace(), dig.GetCollection(), dig.GetTxId(), err)
	}
	defer it.Close()

	maxEndorsedAt := uint64(0)
	for {
		res, err := it.Next()
		if err != nil {
			return nil, errors.Errorf("error getting next element out of private data iterator, namespace <%s>"+
				", collection name <%s>, txID <%s>, due to <%s>", dig.GetNamespace(), dig.GetCollection(), dig.GetTxId(), err)
		}
		if res == nil {
			return results, nil
		}
		rws := res.PvtSimulationResultsWithConfig
		if rws == nil {
			dr.logger.Debug("Skipping nil PvtSimulationResultsWithConfig received at block height", res.ReceivedAtBlockHeight)
			continue
		}
		txPvtRWSet := rws.GetPvtRwset()
		if txPvtRWSet == nil {
			dr.logger.Debug("Skipping empty PvtRwset of PvtSimulationResultsWithConfig received at block height", res.ReceivedAtBlockHeight)
			continue
		}

		colConfigs, found := rws.GetCollectionConfigs()[dig.GetNamespace()]
		if !found {
			dr.logger.Error("No collection config was found for chaincode", dig.GetNamespace(), "collection name",
				dig.GetCollection(), "txID", dig.GetTxId())
			continue
		}

		configs := extractCollectionConfig(colConfigs, dig.GetCollection())
		if configs == nil {
			dr.logger.Error("No collection config was found for collection", dig.GetCollection(),
				"namespace", dig.GetNamespace(), "txID", dig.GetTxId())
			continue
		}

		pvtRWSet := dr.extractPvtRWsets(txPvtRWSet.GetNsPvtRwset(), dig.GetNamespace(), dig.GetCollection())
		if rws.GetEndorsedAt() >= maxEndorsedAt {
			maxEndorsedAt = rws.GetEndorsedAt()
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
		if nsws.GetNamespace() != namespace {
			dr.logger.Debug("Received private data namespace ", nsws.GetNamespace(), " instead of ", namespace, " skipping...")
			continue
		}
		for _, col := range nsws.GetCollectionPvtRwset() {
			// This isn't the collection we're looking for
			if col.GetCollectionName() != collectionName {
				dr.logger.Debug("Received private data collection ", col.GetCollectionName(), " instead of ", collectionName, " skipping...")
				continue
			}
			// Add the collection pRWset to the accumulated set
			pRWsets = append(pRWsets, col.GetRwset())
		}
	}

	return pRWsets
}
