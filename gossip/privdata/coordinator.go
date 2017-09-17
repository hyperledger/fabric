/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package privdata

import (
	"encoding/hex"
	"fmt"

	util2 "github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/core/committer"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/rwsetutil"
	"github.com/hyperledger/fabric/core/transientstore"
	"github.com/hyperledger/fabric/gossip/util"
	"github.com/hyperledger/fabric/protos/common"
	gossip2 "github.com/hyperledger/fabric/protos/gossip"
	"github.com/hyperledger/fabric/protos/ledger/rwset"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/op/go-logging"
	"github.com/pkg/errors"
)

var logger *logging.Logger // package-level logger

func init() {
	logger = util.GetLogger(util.LoggingPrivModule, "")
}

// TransientStore holds private data that the corresponding blocks haven't been committed yet into the ledger
type TransientStore interface {
	// Persist stores the private read-write set of a transaction in the transient store
	Persist(txid string, endorsementBlkHt uint64, privateSimulationResults *rwset.TxPvtReadWriteSet) error
	// GetTxPvtRWSetByTxid returns an iterator due to the fact that the txid may have multiple private
	// RWSets persisted from different endorsers (via Gossip)
	GetTxPvtRWSetByTxid(txid string, filter ledger.PvtNsCollFilter) (transientstore.RWSetScanner, error)
}

// PrivateDataDistributor distributes private data to peers
type PrivateDataDistributor interface {
	// Distribute distributes a given private data with a specific transactionID
	// to peers according policies that are derived from the given PolicyStore and PolicyParser
	Distribute(privateData *rwset.TxPvtReadWriteSet, txID string) error
}

// Coordinator orchestrates the flow of the new
// blocks arrival and in flight transient data, responsible
// to complete missing parts of transient data for given block.
type Coordinator interface {
	PrivateDataDistributor
	// StoreBlock deliver new block with underlined private data
	// returns missing transaction ids
	StoreBlock(block *common.Block, data util.PvtDataCollections) error

	// GetPvtDataAndBlockByNum get block by number and returns also all related private data
	// the order of private data in slice of PvtDataCollections doesn't implies the order of
	// transactions in the block related to these private data, to get the correct placement
	// need to read TxPvtData.SeqInBlock field
	GetPvtDataAndBlockByNum(seqNum uint64) (*common.Block, util.PvtDataCollections, error)

	// GetBlockByNum returns block and related to the block private data
	GetBlockByNum(seqNum uint64) (*common.Block, error)

	// Get recent block sequence number
	LedgerHeight() (uint64, error)

	// Close coordinator, shuts down coordinator service
	Close()
}

type fetcher interface {
	fetch(req *gossip2.RemotePvtDataRequest) ([]*gossip2.PvtDataElement, error)
}

type coordinator struct {
	committer.Committer
	TransientStore
	gossipFetcher fetcher
}

// NewCoordinator creates a new instance of coordinator
func NewCoordinator(committer committer.Committer, store TransientStore, gossipFetcher fetcher) Coordinator {
	return &coordinator{Committer: committer, TransientStore: store, gossipFetcher: gossipFetcher}
}

// Distribute distributes a given private data with a specific transactionID
// to peers according policies that are derived from the given PolicyStore and PolicyParser
func (c *coordinator) Distribute(privateData *rwset.TxPvtReadWriteSet, txID string) error {
	// TODO: also need to distribute the data...
	return c.TransientStore.Persist(txID, 0, privateData)
}

// StoreBlock stores block with private data into the ledger
func (c *coordinator) StoreBlock(block *common.Block, privateDataSets util.PvtDataCollections) error {
	if block.Data == nil {
		return errors.New("Block data is empty")
	}
	if block.Header == nil {
		return errors.New("Block header is nil")
	}
	blockAndPvtData := &ledger.BlockAndPvtData{
		Block:        block,
		BlockPvtData: make(map[uint64]*ledger.TxPvtData),
	}

	ownedRWsets, err := computeOwnedRWsets(block, privateDataSets)
	if err != nil {
		logger.Warning("Failed computing owned RWSets", err)
		return err
	}
	logger.Info("Got block", block.Header.Number, "with", len(privateDataSets), "rwsets")

	missing, err := c.listMissingPrivateData(block, ownedRWsets)
	if err != nil {
		logger.Warning(err)
		return err
	}
	logger.Debug("Missing", len(missing), "rwsets")

	// Put into ownedRWsets RW sets that are missing and found in the transient store
	c.fetchMissingFromTransientStore(missing, ownedRWsets)

	missingKeys := missing.flatten()
	// Remove all keys we already own
	missingKeys.exclude(func(key rwSetKey) bool {
		_, exists := ownedRWsets[key]
		return exists
	})

	logger.Debug("Fetching", len(missingKeys), "rwsets from peers")
	for len(missingKeys) > 0 {
		c.fetchFromPeers(block.Header.Number, missingKeys, ownedRWsets)
	}
	logger.Debug("Fetched all missing rwsets from peers")

	for seqInBlock, nsRWS := range ownedRWsets.BySeqsInBlock() {
		rwsets := nsRWS.toRWSet()
		logger.Debug("Added", len(rwsets.NsPvtRwset), "rwsets to sequence", seqInBlock, "for block", block.Header.Number)
		blockAndPvtData.BlockPvtData[seqInBlock] = &ledger.TxPvtData{
			SeqInBlock: seqInBlock,
			WriteSet:   rwsets,
		}
	}

	// commit block and private data
	return c.CommitWithPvtData(blockAndPvtData)
}

func (c *coordinator) fetchFromPeers(blockSeq uint64, missingKeys rwsetKeys, ownedRWsets map[rwSetKey][]byte) {
	req := &gossip2.RemotePvtDataRequest{}
	missingKeys.foreach(func(k rwSetKey) {
		req.Digests = append(req.Digests, &gossip2.PvtDataDigest{
			TxId:       k.txID,
			SeqInBlock: k.seqInBlock,
			Collection: k.collection,
			Namespace:  k.namespace,
			BlockSeq:   blockSeq,
		})
	})

	fetchedData, err := c.gossipFetcher.fetch(req)
	if err != nil {
		logger.Warning("Failed fetching private data for block", blockSeq, "from peers:", err)
		return
	}

	// Iterate over data fetched from peers
	for _, element := range fetchedData {
		dig := element.Digest
		for _, rws := range element.Payload {
			hash := hex.EncodeToString(util2.ComputeSHA256(rws))
			key := rwSetKey{
				txID:       dig.TxId,
				namespace:  dig.Namespace,
				collection: dig.Collection,
				seqInBlock: dig.SeqInBlock,
				hash:       hash,
			}
			if _, isMissing := missingKeys[key]; !isMissing {
				logger.Debug("Ignoring", key, "because it wasn't found in the block")
				continue
			}
			ownedRWsets[key] = rws
			delete(missingKeys, key)
			c.TransientStore.Persist(dig.TxId, 0, key.toTxPvtReadWriteSet(rws))
			logger.Debug("Fetched", key)
		}
	}
}

func (c *coordinator) fetchMissingFromTransientStore(missing rwSetKeysByTxIDs, ownedRWsets map[rwSetKey][]byte) {
	// Check transient store
	for txAndSeq, filter := range missing.FiltersByTxIDs() {
		c.fetchFromTransientStore(txAndSeq, filter, ownedRWsets)
	}
}

func (c *coordinator) fetchFromTransientStore(txAndSeq txAndSeqInBlock, filter ledger.PvtNsCollFilter, ownedRWsets map[rwSetKey][]byte) {
	iterator, err := c.TransientStore.GetTxPvtRWSetByTxid(txAndSeq.txID, filter)
	if err != nil {
		logger.Warning("Failed obtaining iterator from transient store:", err)
		return
	}
	defer iterator.Close()
	for {
		res, err := iterator.Next()
		if err != nil {
			logger.Warning("Failed iterating:", err)
			break
		}
		if res == nil {
			// End of iteration
			break
		}
		if res.PvtSimulationResults == nil {
			logger.Warning("Resultset's PvtSimulationResults for", txAndSeq.txID, "is nil, skipping")
			continue
		}
		for _, ns := range res.PvtSimulationResults.NsPvtRwset {
			for _, col := range ns.CollectionPvtRwset {
				key := rwSetKey{
					txID:       txAndSeq.txID,
					seqInBlock: txAndSeq.seqInBlock,
					collection: col.CollectionName,
					namespace:  ns.Namespace,
					hash:       hex.EncodeToString(util2.ComputeSHA256(col.Rwset)),
				}
				// populate the ownedRWsets with the RW set from the transient store
				ownedRWsets[key] = col.Rwset
			} // iterating over all collections
		} // iterating over all namespaces
	} // iterating over the TxPvtRWSet results
}

func computeOwnedRWsets(block *common.Block, data util.PvtDataCollections) (rwsetByKeys, error) {
	lastBlockSeq := len(block.Data.Data) - 1

	ownedRWsets := make(map[rwSetKey][]byte)
	for _, item := range data {
		if lastBlockSeq < int(item.SeqInBlock) {
			logger.Warningf("Claimed SeqInBlock %d but block has only %d transactions", item.SeqInBlock, len(block.Data.Data))
			continue
		}
		env, err := utils.GetEnvelopeFromBlock(block.Data.Data[item.SeqInBlock])
		if err != nil {
			return nil, err
		}
		payload, err := utils.GetPayload(env)
		if err != nil {
			return nil, err
		}

		chdr, err := utils.UnmarshalChannelHeader(payload.Header.ChannelHeader)
		if err != nil {
			return nil, err
		}
		for _, ns := range item.WriteSet.NsPvtRwset {
			for _, col := range ns.CollectionPvtRwset {
				computedHash := hex.EncodeToString(util2.ComputeSHA256(col.Rwset))
				ownedRWsets[rwSetKey{
					txID:       chdr.TxId,
					seqInBlock: item.SeqInBlock,
					collection: col.CollectionName,
					namespace:  ns.Namespace,
					hash:       computedHash,
				}] = col.Rwset
			} // iterate over collections in the namespace
		} // iterate over the namespaces in the WSet
	} // iterate over the transactions in the block
	return ownedRWsets, nil
}

type readWriteSets []readWriteSet

func (s readWriteSets) toRWSet() *rwset.TxPvtReadWriteSet {
	namespaces := make(map[string]*rwset.NsPvtReadWriteSet)
	dataModel := rwset.TxReadWriteSet_KV
	for _, rws := range s {
		if _, exists := namespaces[rws.namespace]; !exists {
			namespaces[rws.namespace] = &rwset.NsPvtReadWriteSet{
				Namespace: rws.namespace,
			}
		}
		col := &rwset.CollectionPvtReadWriteSet{
			CollectionName: rws.collection,
			Rwset:          rws.rws,
		}
		namespaces[rws.namespace].CollectionPvtRwset = append(namespaces[rws.namespace].CollectionPvtRwset, col)
	}

	var namespaceSlice []*rwset.NsPvtReadWriteSet
	for _, nsRWset := range namespaces {
		namespaceSlice = append(namespaceSlice, nsRWset)
	}

	return &rwset.TxPvtReadWriteSet{
		DataModel:  dataModel,
		NsPvtRwset: namespaceSlice,
	}
}

type readWriteSet struct {
	rwSetKey
	rws []byte
}

type rwsetByKeys map[rwSetKey][]byte

func (s rwsetByKeys) BySeqsInBlock() map[uint64]readWriteSets {
	res := make(map[uint64]readWriteSets)
	for k, rws := range s {
		res[k.seqInBlock] = append(res[k.seqInBlock], readWriteSet{
			rws:      rws,
			rwSetKey: k,
		})
	}
	return res
}

type rwsetKeys map[rwSetKey]struct{}

// foreach invokes given function in each key
func (s rwsetKeys) foreach(f func(key rwSetKey)) {
	for k := range s {
		f(k)
	}
}

// exclude removes all keys accepted by the given predicate.
func (s rwsetKeys) exclude(exists func(key rwSetKey) bool) {
	for k := range s {
		if exists(k) {
			delete(s, k)
		}
	}
}

type txAndSeqInBlock struct {
	txID       string
	seqInBlock uint64
}

type rwSetKeysByTxIDs map[txAndSeqInBlock][]rwSetKey

func (s rwSetKeysByTxIDs) flatten() rwsetKeys {
	m := make(map[rwSetKey]struct{})
	for _, keys := range s {
		for _, k := range keys {
			m[k] = struct{}{}
		}
	}
	return m
}

func (s rwSetKeysByTxIDs) FiltersByTxIDs() map[txAndSeqInBlock]ledger.PvtNsCollFilter {
	filters := make(map[txAndSeqInBlock]ledger.PvtNsCollFilter)
	for txAndSeq, rwsKeys := range s {
		filter := ledger.NewPvtNsCollFilter()
		for _, rwskey := range rwsKeys {
			filter.Add(rwskey.namespace, rwskey.collection)
		}
		filters[txAndSeq] = filter
	}

	return filters
}

type rwSetKey struct {
	txID       string
	seqInBlock uint64
	namespace  string
	collection string
	hash       string
}

func (k *rwSetKey) toTxPvtReadWriteSet(rws []byte) *rwset.TxPvtReadWriteSet {
	return &rwset.TxPvtReadWriteSet{
		DataModel: rwset.TxReadWriteSet_KV,
		NsPvtRwset: []*rwset.NsPvtReadWriteSet{
			{
				Namespace: k.namespace,
				CollectionPvtRwset: []*rwset.CollectionPvtReadWriteSet{
					{
						CollectionName: k.collection,
						Rwset:          rws,
					},
				},
			},
		},
	}
}

func (c *coordinator) listMissingPrivateData(block *common.Block, ownedRWsets map[rwSetKey][]byte) (rwSetKeysByTxIDs, error) {
	privateRWsetsInBlock := make(map[rwSetKey]struct{})
	missing := make(rwSetKeysByTxIDs)

	for seqInBlock, envBytes := range block.Data.Data {
		env, err := utils.GetEnvelopeFromBlock(envBytes)
		if err != nil {
			return nil, err
		}

		payload, err := utils.GetPayload(env)
		if err != nil {
			return nil, err
		}

		chdr, err := utils.UnmarshalChannelHeader(payload.Header.ChannelHeader)
		if err != nil {
			return nil, err
		}

		if chdr.Type != int32(common.HeaderType_ENDORSER_TRANSACTION) {
			continue
		}

		respPayload, err := utils.GetActionFromEnvelope(envBytes)
		if err != nil {
			logger.Warning("Failed obtaining action from envelope", err)
			continue
		}

		txRWSet := &rwsetutil.TxRwSet{}
		if err = txRWSet.FromProtoBytes(respPayload.Results); err != nil {
			logger.Warning("Failed obtaining TxRwSet from ChaincodeAction's results", err)
			continue
		}

		for _, ns := range txRWSet.NsRwSets {
			for _, hashed := range ns.CollHashedRwSets {
				key := rwSetKey{
					txID:       chdr.TxId,
					seqInBlock: uint64(seqInBlock),
					hash:       hex.EncodeToString(hashed.PvtRwSetHash),
					namespace:  ns.NameSpace,
					collection: hashed.CollectionName,
				}
				privateRWsetsInBlock[key] = struct{}{}
				if _, exists := ownedRWsets[key]; !exists {
					txAndSeq := txAndSeqInBlock{
						txID:       chdr.TxId,
						seqInBlock: uint64(seqInBlock),
					}
					missing[txAndSeq] = append(missing[txAndSeq], key)
				}
			} // for all hashed RW sets
		} // for all RW sets
	} // for all transactions in block

	// In the end, iterate over the ownedRWsets, and if the key doesn't exist in
	// the privateRWsetsInBlock - delete it from the ownedRWsets
	for k := range ownedRWsets {
		if _, exists := privateRWsetsInBlock[k]; !exists {
			logger.Warning("Removed", k.namespace, k.collection, "hash", k.hash, "from the data passed to the ledger")
			delete(ownedRWsets, k)
		}
	}

	return missing, nil
}

// GetPvtDataAndBlockByNum get block by number and returns also all related private data
// the order of private data in slice of PvtDataCollections doesn't implies the order of
// transactions in the block related to these private data, to get the correct placement
// need to read TxPvtData.SeqInBlock field
func (c *coordinator) GetPvtDataAndBlockByNum(seqNum uint64) (*common.Block, util.PvtDataCollections, error) {
	blockAndPvtData, err := c.Committer.GetPvtDataAndBlockByNum(seqNum)
	if err != nil {
		return nil, nil, fmt.Errorf("cannot retrieve block number %d, due to %s", seqNum, err)
	}

	var blockPvtData util.PvtDataCollections

	for _, item := range blockAndPvtData.BlockPvtData {
		blockPvtData = append(blockPvtData, item)
	}

	return blockAndPvtData.Block, blockPvtData, nil
}

// GetBlockByNum returns block by sequence number
func (c *coordinator) GetBlockByNum(seqNum uint64) (*common.Block, error) {
	blocks := c.GetBlocks([]uint64{seqNum})
	if len(blocks) == 0 {
		return nil, fmt.Errorf("cannot retrieve block number %d", seqNum)
	}
	return blocks[0], nil
}
