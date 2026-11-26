/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package privdata

import (
	protosgossip "github.com/hyperledger/fabric-protos-go-apiv2/gossip"
	"github.com/hyperledger/fabric-protos-go-apiv2/ledger/rwset"
	"github.com/hyperledger/fabric-protos-go-apiv2/peer"
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

		configs := dr.newConfigCache()
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
			pvtRWSet, err := dr.fromTransientStore(dig, filter, configs)
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
	configs := dr.newConfigCache()

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

		configPackage, err := configs.configPackage(dig.GetNamespace(), dig.GetBlockSeq())
		if err != nil {
			return nil, errors.WithMessagef(err, "cannot obtain the collection config for collection <%s>"+
				" txID <%s>", dig.GetCollection(), dig.GetTxId())
		}
		if configPackage == nil {
			return nil, errors.Errorf("no collection config update below block sequence = <%d>"+
				" collection name = <%s> for chaincode <%s> is available ", dig.GetBlockSeq(), dig.GetCollection(), dig.GetNamespace())
		}
		collectionConfig := extractCollectionConfig(configPackage, dig.GetCollection())
		if collectionConfig == nil {
			return nil, errors.Errorf("no collection config was found for collection <%s>"+
				" namespace <%s> txID <%s>", dig.GetCollection(), dig.GetNamespace(), dig.GetTxId())
		}
		pvtRWSetWithConfig.CollectionConfig = collectionConfig
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

// configCache looks up collection configs in the ledger's config history, caching each
// (namespace, block sequence) lookup for the lifetime of a single CollectionRWSet call.
// The lookup is not only a config history read: for implicit collections the kvledger
// wrapper creates a query executor and reads the state DB. A pull request commonly carries
// several digests for the same namespace and block, which would otherwise repeat it.
type configCache struct {
	committer committer.Committer
	retriever ledger.ConfigHistoryRetriever
	retrErr   error
	retrieved bool
	packages  map[configCacheKey]cachedConfigPackage
}

type configCacheKey struct {
	namespace string
	blockSeq  uint64
}

type cachedConfigPackage struct {
	configPackage *peer.CollectionConfigPackage
	err           error
}

func (dr *dataRetriever) newConfigCache() *configCache {
	return &configCache{
		committer: dr.committer,
		packages:  make(map[configCacheKey]cachedConfigPackage),
	}
}

// configPackage returns the collection config package committed to the ledger most recently
// below the given block sequence, or nil if the ledger holds no config for the namespace below
// that block.
func (c *configCache) configPackage(namespace string, blockSeq uint64) (*peer.CollectionConfigPackage, error) {
	key := configCacheKey{namespace: namespace, blockSeq: blockSeq}
	if cached, ok := c.packages[key]; ok {
		return cached.configPackage, cached.err
	}

	configPackage, err := c.lookup(namespace, blockSeq)
	c.packages[key] = cachedConfigPackage{configPackage: configPackage, err: err}
	return configPackage, err
}

func (c *configCache) lookup(namespace string, blockSeq uint64) (*peer.CollectionConfigPackage, error) {
	if !c.retrieved {
		c.retriever, c.retrErr = c.committer.GetConfigHistoryRetriever()
		c.retrieved = true
	}
	if c.retrErr != nil {
		return nil, errors.Errorf("cannot obtain configuration history retriever, for chaincode <%s>"+
			" block sequence number <%d> due to <%s>", namespace, blockSeq, c.retrErr)
	}

	configInfo, err := c.retriever.MostRecentCollectionConfigBelow(blockSeq, namespace)
	if err != nil {
		return nil, errors.Errorf("cannot find recent collection config update below block sequence = %d,"+
			" for chaincode <%s>", blockSeq, namespace)
	}
	if configInfo == nil {
		return nil, nil
	}
	return configInfo.CollectionConfig, nil
}

func (dr *dataRetriever) fromTransientStore(dig *protosgossip.PvtDataDigest, filter map[string]ledger.PvtCollFilter, configs *configCache) (*util.PrivateRWSetWithConfig, error) {
	results := &util.PrivateRWSetWithConfig{}
	rwSets, err := dr.transientStoreRWSets(dig, filter)
	if err != nil {
		return nil, err
	}
	if len(rwSets) == 0 {
		return results, nil
	}

	// A transient store entry can be written by any peer in the channel through a private data
	// push, and the collection config it carries is stored as it was received, so it cannot
	// decide who the private data may be released to. The requester is checked against the
	// collection config committed to the ledger instead, the same lookup fromLedger uses, and
	// the write sets stored for the txID are returned under that config whatever config the
	// entries themselves carry.
	//
	// Without a config from the ledger there is nothing to check the requester against, so
	// nothing is served for the digest and filterNotEligible skips it. A collection the ledger
	// holds no config for is expected here, for instance when a peer pushed an entry for a
	// collection that does not exist, so that case is logged at debug level, while a failure of
	// the lookup itself is logged as a warning.
	configPackage, err := configs.configPackage(dig.GetNamespace(), dig.GetBlockSeq())
	if err != nil {
		dr.logger.Warningf("Not serving %d transient store write sets for txID <%s>, collection <%s>, namespace <%s>,"+
			" because the collection config could not be read from the ledger: %s",
			len(rwSets), dig.GetTxId(), dig.GetCollection(), dig.GetNamespace(), err)
		return results, nil
	}

	collectionConfig := extractCollectionConfig(configPackage, dig.GetCollection())
	if collectionConfig == nil {
		dr.logger.Debugf("Not serving %d transient store write sets for txID <%s>, collection <%s>, namespace <%s>,"+
			" because the ledger holds no config for that collection below block sequence %d",
			len(rwSets), dig.GetTxId(), dig.GetCollection(), dig.GetNamespace(), dig.GetBlockSeq())
		return results, nil
	}

	results.CollectionConfig = collectionConfig
	results.RWSet = rwSets
	return results, nil
}

// transientStoreRWSets returns the write sets the transient store holds for the digest's txID,
// namespace and collection. The iterator is closed before the caller looks up the ledger.
func (dr *dataRetriever) transientStoreRWSets(dig *protosgossip.PvtDataDigest, filter map[string]ledger.PvtCollFilter) ([]util.PrivateRWSet, error) {
	it, err := dr.store.GetTxPvtRWSetByTxid(dig.GetTxId(), filter)
	if err != nil {
		return nil, errors.Errorf("was not able to retrieve private data from transient store, namespace <%s>"+
			", collection name %s, txID <%s>, due to <%s>", dig.GetNamespace(), dig.GetCollection(), dig.GetTxId(), err)
	}
	defer it.Close()

	var rwSets []util.PrivateRWSet
	for {
		res, err := it.Next()
		if err != nil {
			return nil, errors.Errorf("error getting next element out of private data iterator, namespace <%s>"+
				", collection name <%s>, txID <%s>, due to <%s>", dig.GetNamespace(), dig.GetCollection(), dig.GetTxId(), err)
		}
		if res == nil {
			return rwSets, nil
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

		if extractCollectionConfig(colConfigs, dig.GetCollection()) == nil {
			dr.logger.Error("No collection config was found for collection", dig.GetCollection(),
				"namespace", dig.GetNamespace(), "txID", dig.GetTxId())
			continue
		}

		rwSets = append(rwSets, dr.extractPvtRWsets(txPvtRWSet.GetNsPvtRwset(), dig.GetNamespace(), dig.GetCollection())...)
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
