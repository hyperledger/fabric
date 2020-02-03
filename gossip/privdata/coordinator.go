/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package privdata

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/channelconfig"
	vsccErrors "github.com/hyperledger/fabric/common/errors"
	util2 "github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/core/committer"
	"github.com/hyperledger/fabric/core/committer/txvalidator"
	"github.com/hyperledger/fabric/core/common/privdata"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/rwsetutil"
	"github.com/hyperledger/fabric/core/transientstore"
	"github.com/hyperledger/fabric/gossip/metrics"
	privdatacommon "github.com/hyperledger/fabric/gossip/privdata/common"
	"github.com/hyperledger/fabric/gossip/util"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/ledger/rwset"
	"github.com/hyperledger/fabric/protos/msp"
	"github.com/hyperledger/fabric/protos/peer"
	transientstore2 "github.com/hyperledger/fabric/protos/transientstore"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/pkg/errors"
)

const pullRetrySleepInterval = time.Second

var logger = util.GetLogger(util.PrivateDataLogger, "")

//go:generate mockery -dir ../../core/common/privdata/ -name CollectionStore -case underscore -output mocks/
//go:generate mockery -dir ../../core/committer/ -name Committer -case underscore -output mocks/

// TransientStore holds private data that the corresponding blocks haven't been committed yet into the ledger
type TransientStore interface {
	// PersistWithConfig stores the private write set of a transaction along with the collection config
	// in the transient store based on txid and the block height the private data was received at
	PersistWithConfig(txid string, blockHeight uint64, privateSimulationResultsWithConfig *transientstore2.TxPvtReadWriteSetWithConfigInfo) error

	// Persist stores the private write set of a transaction in the transient store
	Persist(txid string, blockHeight uint64, privateSimulationResults *rwset.TxPvtReadWriteSet) error
	// GetTxPvtRWSetByTxid returns an iterator due to the fact that the txid may have multiple private
	// write sets persisted from different endorsers (via Gossip)
	GetTxPvtRWSetByTxid(txid string, filter ledger.PvtNsCollFilter) (transientstore.RWSetScanner, error)

	// PurgeByTxids removes private read-write sets for a given set of transactions from the
	// transient store
	PurgeByTxids(txids []string) error

	// PurgeByHeight removes private write sets at block height lesser than
	// a given maxBlockNumToRetain. In other words, Purge only retains private write sets
	// that were persisted at block height of maxBlockNumToRetain or higher. Though the private
	// write sets stored in transient store is removed by coordinator using PurgebyTxids()
	// after successful block commit, PurgeByHeight() is still required to remove orphan entries (as
	// transaction that gets endorsed may not be submitted by the client for commit)
	PurgeByHeight(maxBlockNumToRetain uint64) error
}

// Coordinator orchestrates the flow of the new
// blocks arrival and in flight transient data, responsible
// to complete missing parts of transient data for given block.
type Coordinator interface {
	// StoreBlock deliver new block with underlined private data
	// returns missing transaction ids
	StoreBlock(block *common.Block, data util.PvtDataCollections) error

	// StorePvtData used to persist private data into transient store
	StorePvtData(txid string, privData *transientstore2.TxPvtReadWriteSetWithConfigInfo, blckHeight uint64) error

	// GetPvtDataAndBlockByNum get block by number and returns also all related private data
	// the order of private data in slice of PvtDataCollections doesn't implies the order of
	// transactions in the block related to these private data, to get the correct placement
	// need to read TxPvtData.SeqInBlock field
	GetPvtDataAndBlockByNum(seqNum uint64, peerAuth common.SignedData) (*common.Block, util.PvtDataCollections, error)

	// Get recent block sequence number
	LedgerHeight() (uint64, error)

	// Close coordinator, shuts down coordinator service
	Close()
}

type dig2sources map[privdatacommon.DigKey][]*peer.Endorsement

func (d2s dig2sources) keys() []privdatacommon.DigKey {
	res := make([]privdatacommon.DigKey, 0, len(d2s))
	for dig := range d2s {
		res = append(res, dig)
	}
	return res
}

// Fetcher interface which defines API to fetch missing
// private data elements
type Fetcher interface {
	fetch(dig2src dig2sources) (*privdatacommon.FetchedPvtDataContainer, error)
}

//go:generate mockery -dir ./ -name CapabilityProvider -case underscore -output mocks/

// CapabilityProvider contains functions to retrieve capability information for a channel
type CapabilityProvider interface {
	// Capabilities defines the capabilities for the application portion of this channel
	Capabilities() channelconfig.ApplicationCapabilities
}

// Support encapsulates set of interfaces to
// aggregate required functionality by single struct
type Support struct {
	ChainID string
	privdata.CollectionStore
	txvalidator.Validator
	committer.Committer
	TransientStore
	Fetcher
	CapabilityProvider
}

type coordinator struct {
	mspID          string
	selfSignedData common.SignedData
	Support
	transientBlockRetention        uint64
	metrics                        *metrics.PrivdataMetrics
	pullRetryThreshold             time.Duration
	skipPullingInvalidTransactions bool
}

type CoordinatorConfig struct {
	TransientBlockRetention        uint64
	PullRetryThreshold             time.Duration
	SkipPullingInvalidTransactions bool
}

// NewCoordinator creates a new instance of coordinator
func NewCoordinator(mspID string, support Support, selfSignedData common.SignedData, metrics *metrics.PrivdataMetrics,
	config CoordinatorConfig) Coordinator {
	return &coordinator{Support: support,
		mspID:                          mspID,
		selfSignedData:                 selfSignedData,
		transientBlockRetention:        config.TransientBlockRetention,
		metrics:                        metrics,
		pullRetryThreshold:             config.PullRetryThreshold,
		skipPullingInvalidTransactions: config.SkipPullingInvalidTransactions}
}

// StorePvtData used to persist private date into transient store
func (c *coordinator) StorePvtData(txID string, privData *transientstore2.TxPvtReadWriteSetWithConfigInfo, blkHeight uint64) error {
	return c.TransientStore.PersistWithConfig(txID, blkHeight, privData)
}

// StoreBlock stores block with private data into the ledger
func (c *coordinator) StoreBlock(block *common.Block, privateDataSets util.PvtDataCollections) error {
	if block.Data == nil {
		return errors.New("Block data is empty")
	}
	if block.Header == nil {
		return errors.New("Block header is nil")
	}

	logger.Infof("[%s] Received block [%d] from buffer", c.ChainID, block.Header.Number)

	logger.Debugf("[%s] Validating block [%d]", c.ChainID, block.Header.Number)

	validationStart := time.Now()
	err := c.Validator.Validate(block)
	c.reportValidationDuration(time.Since(validationStart))
	if err != nil {
		logger.Errorf("Validation failed: %+v", err)
		return err
	}

	blockAndPvtData := &ledger.BlockAndPvtData{
		Block:          block,
		PvtData:        make(ledger.TxPvtDataMap),
		MissingPvtData: make(ledger.TxMissingPvtDataMap),
	}

	exist, err := c.DoesPvtDataInfoExistInLedger(block.Header.Number)
	if err != nil {
		return err
	}
	if exist {
		commitOpts := &ledger.CommitOptions{FetchPvtDataFromLedger: true}
		return c.CommitWithPvtData(blockAndPvtData, commitOpts)
	}

	listMissingStart := time.Now()
	ownedRWsets, err := computeOwnedRWsets(block, privateDataSets)
	if err != nil {
		logger.Warning("Failed computing owned RWSets", err)
		return err
	}

	privateInfo, err := c.listMissingPrivateData(block, ownedRWsets)
	if err != nil {
		logger.Warning(err)
		return err
	}

	// if the peer is configured to not pull private rwset of invalid
	// transaction during block commit, we need to delete those
	// missing entries from the missingKeys list (to be used for pulling rwset
	// from other peers). Instead add them to the block's private data
	// missing list so that the private data reconciler can pull them later.
	if c.skipPullingInvalidTransactions {
		txsFilter := txValidationFlags(block.Metadata.Metadata[common.BlockMetadataIndex_TRANSACTIONS_FILTER])
		for missingRWS := range privateInfo.missingKeys {
			if txsFilter[missingRWS.seqInBlock] != uint8(peer.TxValidationCode_VALID) {
				blockAndPvtData.MissingPvtData.Add(missingRWS.seqInBlock, missingRWS.namespace, missingRWS.collection, true)
				delete(privateInfo.missingKeys, missingRWS)
			}
		}
	}

	c.reportListMissingPrivateDataDuration(time.Since(listMissingStart))

	retryThresh := c.pullRetryThreshold
	var bFetchFromPeers bool // defaults to false
	if len(privateInfo.missingKeys) == 0 {
		logger.Debugf("[%s] No missing collection private write sets to fetch from remote peers", c.ChainID)
	} else {
		bFetchFromPeers = true
		logger.Debugf("[%s] Could not find all collection private write sets in local peer transient store for block [%d].", c.ChainID, block.Header.Number)
		logger.Debugf("[%s] Fetching %d collection private write sets from remote peers for a maximum duration of %s", c.ChainID, len(privateInfo.missingKeys), retryThresh)
	}
	startPull := time.Now()
	limit := startPull.Add(retryThresh)
	for len(privateInfo.missingKeys) > 0 && time.Now().Before(limit) {
		c.fetchFromPeers(block.Header.Number, ownedRWsets, privateInfo)
		// If succeeded to fetch everything, no need to sleep before
		// retry
		if len(privateInfo.missingKeys) == 0 {
			break
		}
		time.Sleep(pullRetrySleepInterval)
	}
	elapsedPull := int64(time.Since(startPull) / time.Millisecond) // duration in ms

	c.reportFetchDuration(time.Since(startPull))

	// Only log results if we actually attempted to fetch
	if bFetchFromPeers {
		if len(privateInfo.missingKeys) == 0 {
			logger.Infof("[%s] Fetched all missing collection private write sets from remote peers for block [%d] (%dms)", c.ChainID, block.Header.Number, elapsedPull)
		} else {
			logger.Warningf("[%s] Could not fetch all missing collection private write sets from remote peers. Will commit block [%d] with missing private write sets:[%v]",
				c.ChainID, block.Header.Number, privateInfo.missingKeys)
		}
	}

	// populate the private RWSets passed to the ledger
	for seqInBlock, nsRWS := range ownedRWsets.bySeqsInBlock() {
		rwsets := nsRWS.toRWSet()
		logger.Debugf("[%s] Added %d namespace private write sets for block [%d], tran [%d]", c.ChainID, len(rwsets.NsPvtRwset), block.Header.Number, seqInBlock)
		blockAndPvtData.PvtData[seqInBlock] = &ledger.TxPvtData{
			SeqInBlock: seqInBlock,
			WriteSet:   rwsets,
		}
	}

	// populate missing RWSets to be passed to the ledger
	for missingRWS := range privateInfo.missingKeys {
		blockAndPvtData.MissingPvtData.Add(missingRWS.seqInBlock, missingRWS.namespace, missingRWS.collection, true)
	}

	// populate missing RWSets for ineligible collections to be passed to the ledger
	for _, missingRWS := range privateInfo.missingRWSButIneligible {
		blockAndPvtData.MissingPvtData.Add(missingRWS.seqInBlock, missingRWS.namespace, missingRWS.collection, false)
	}

	// commit block and private data
	commitStart := time.Now()
	err = c.CommitWithPvtData(blockAndPvtData, &ledger.CommitOptions{})
	c.reportCommitDuration(time.Since(commitStart))
	if err != nil {
		return errors.Wrap(err, "commit failed")
	}

	purgeStart := time.Now()

	if len(blockAndPvtData.PvtData) > 0 {
		// Finally, purge all transactions in block - valid or not valid.
		if err := c.PurgeByTxids(privateInfo.txns); err != nil {
			logger.Error("Purging transactions", privateInfo.txns, "failed:", err)
		}
	}

	seq := block.Header.Number
	if seq%c.transientBlockRetention == 0 && seq > c.transientBlockRetention {
		err := c.PurgeByHeight(seq - c.transientBlockRetention)
		if err != nil {
			logger.Error("Failed purging data from transient store at block", seq, ":", err)
		}
	}

	c.reportPurgeDuration(time.Since(purgeStart))

	return nil
}

func (c *coordinator) fetchFromPeers(blockSeq uint64, ownedRWsets map[rwSetKey][]byte, privateInfo *privateDataInfo) {
	dig2src := make(map[privdatacommon.DigKey][]*peer.Endorsement)
	privateInfo.missingKeys.foreach(func(k rwSetKey) {
		logger.Debug("Fetching", k, "from peers")
		dig := privdatacommon.DigKey{
			TxId:       k.txID,
			SeqInBlock: k.seqInBlock,
			Collection: k.collection,
			Namespace:  k.namespace,
			BlockSeq:   blockSeq,
		}
		dig2src[dig] = privateInfo.sources[k]
	})
	fetchedData, err := c.fetch(dig2src)
	if err != nil {
		logger.Warning("Failed fetching private data for block", blockSeq, "from peers:", err)
		return
	}

	// Iterate over data fetched from peers
	for _, element := range fetchedData.AvailableElements {
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
			if _, isMissing := privateInfo.missingKeys[key]; !isMissing {
				logger.Debug("Ignoring", key, "because it wasn't found in the block")
				continue
			}
			ownedRWsets[key] = rws
			delete(privateInfo.missingKeys, key)
			// If we fetch private data that is associated to block i, then our last block persisted must be i-1
			// so our ledger height is i, since blocks start from 0.
			c.TransientStore.Persist(dig.TxId, blockSeq, key.toTxPvtReadWriteSet(rws))
			logger.Debug("Fetched", key)
		}
	}
	// Iterate over purged data
	for _, dig := range fetchedData.PurgedElements {
		// delete purged key from missing keys
		for missingPvtRWKey := range privateInfo.missingKeys {
			if missingPvtRWKey.namespace == dig.Namespace &&
				missingPvtRWKey.collection == dig.Collection &&
				missingPvtRWKey.txID == dig.TxId {
				delete(privateInfo.missingKeys, missingPvtRWKey)
				logger.Warningf("Missing key because was purged or will soon be purged, "+
					"continue block commit without [%+v] in private rwset", missingPvtRWKey)
			}
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
		res, err := iterator.NextWithConfig()
		if err != nil {
			logger.Error("Failed iterating:", err)
			break
		}
		if res == nil {
			// End of iteration
			break
		}
		if res.PvtSimulationResultsWithConfig == nil {
			logger.Warning("Resultset's PvtSimulationResultsWithConfig for", txAndSeq.txID, "is nil, skipping")
			continue
		}
		simRes := res.PvtSimulationResultsWithConfig
		if simRes.PvtRwset == nil {
			logger.Warning("The PvtRwset of PvtSimulationResultsWithConfig for", txAndSeq.txID, "is nil, skipping")
			continue
		}
		for _, ns := range simRes.PvtRwset.NsPvtRwset {
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

// computeOwnedRWsets identifies which block private data we already have
func computeOwnedRWsets(block *common.Block, blockPvtData util.PvtDataCollections) (rwsetByKeys, error) {
	lastBlockSeq := len(block.Data.Data) - 1

	ownedRWsets := make(map[rwSetKey][]byte)
	for _, txPvtData := range blockPvtData {
		if lastBlockSeq < int(txPvtData.SeqInBlock) {
			logger.Warningf("Claimed SeqInBlock %d but block has only %d transactions", txPvtData.SeqInBlock, len(block.Data.Data))
			continue
		}
		env, err := utils.GetEnvelopeFromBlock(block.Data.Data[txPvtData.SeqInBlock])
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
		for _, ns := range txPvtData.WriteSet.NsPvtRwset {
			for _, col := range ns.CollectionPvtRwset {
				computedHash := hex.EncodeToString(util2.ComputeSHA256(col.Rwset))
				ownedRWsets[rwSetKey{
					txID:       chdr.TxId,
					seqInBlock: txPvtData.SeqInBlock,
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

func (s rwsetByKeys) bySeqsInBlock() map[uint64]readWriteSets {
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

// String returns a string representation of the rwsetKeys
func (s rwsetKeys) String() string {
	var buffer bytes.Buffer
	for k := range s {
		buffer.WriteString(fmt.Sprintf("%s\n", k.String()))
	}
	return buffer.String()
}

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

// String returns a string representation of the rwSetKey
func (k *rwSetKey) String() string {
	return fmt.Sprintf("txID: %s, seq: %d, namespace: %s, collection: %s, hash: %s", k.txID, k.seqInBlock, k.namespace, k.collection, k.hash)
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

type txns []string
type blockData [][]byte
type blockConsumer func(seqInBlock uint64, chdr *common.ChannelHeader, txRWSet *rwsetutil.TxRwSet, endorsers []*peer.Endorsement) error

func (data blockData) forEachTxn(storePvtDataOfInvalidTx bool, txsFilter txValidationFlags, consumer blockConsumer) (txns, error) {
	var txList []string
	for seqInBlock, envBytes := range data {
		env, err := utils.GetEnvelopeFromBlock(envBytes)
		if err != nil {
			logger.Warning("Invalid envelope:", err)
			continue
		}

		payload, err := utils.GetPayload(env)
		if err != nil {
			logger.Warning("Invalid payload:", err)
			continue
		}

		chdr, err := utils.UnmarshalChannelHeader(payload.Header.ChannelHeader)
		if err != nil {
			logger.Warning("Invalid channel header:", err)
			continue
		}

		if chdr.Type != int32(common.HeaderType_ENDORSER_TRANSACTION) {
			continue
		}

		txList = append(txList, chdr.TxId)

		if txsFilter[seqInBlock] != uint8(peer.TxValidationCode_VALID) && !storePvtDataOfInvalidTx {
			logger.Debugf("Skipping Tx", seqInBlock, "because it's invalid. Status is", txsFilter[seqInBlock])
			continue
		}

		respPayload, err := utils.GetActionFromEnvelope(envBytes)
		if err != nil {
			logger.Warning("Failed obtaining action from envelope", err)
			continue
		}

		tx, err := utils.GetTransaction(payload.Data)
		if err != nil {
			logger.Warning("Invalid transaction in payload data for tx ", chdr.TxId, ":", err)
			continue
		}

		ccActionPayload, err := utils.GetChaincodeActionPayload(tx.Actions[0].Payload)
		if err != nil {
			logger.Warning("Invalid chaincode action in payload for tx", chdr.TxId, ":", err)
			continue
		}

		if ccActionPayload.Action == nil {
			logger.Warning("Action in ChaincodeActionPayload for", chdr.TxId, "is nil")
			continue
		}

		txRWSet := &rwsetutil.TxRwSet{}
		if err = txRWSet.FromProtoBytes(respPayload.Results); err != nil {
			logger.Warning("Failed obtaining TxRwSet from ChaincodeAction's results", err)
			continue
		}
		err = consumer(uint64(seqInBlock), chdr, txRWSet, ccActionPayload.Action.Endorsements)
		if err != nil {
			return txList, err
		}
	}
	return txList, nil
}

func endorsersFromOrgs(ns string, col string, endorsers []*peer.Endorsement, orgs []string) []*peer.Endorsement {
	var res []*peer.Endorsement
	for _, e := range endorsers {
		sID := &msp.SerializedIdentity{}
		err := proto.Unmarshal(e.Endorser, sID)
		if err != nil {
			logger.Warning("Failed unmarshalling endorser:", err)
			continue
		}
		if !util.Contains(sID.Mspid, orgs) {
			logger.Debug(sID.Mspid, "isn't among the collection's orgs:", orgs, "for namespace", ns, ",collection", col)
			continue
		}
		res = append(res, e)
	}
	return res
}

type privateDataInfo struct {
	sources                 map[rwSetKey][]*peer.Endorsement
	missingKeysByTxIDs      rwSetKeysByTxIDs
	missingKeys             rwsetKeys
	txns                    txns
	missingRWSButIneligible []rwSetKey
}

// listMissingPrivateData identifies missing private write sets and attempts to retrieve them from local transient store
func (c *coordinator) listMissingPrivateData(block *common.Block, ownedRWsets map[rwSetKey][]byte) (*privateDataInfo, error) {
	if block.Metadata == nil || len(block.Metadata.Metadata) <= int(common.BlockMetadataIndex_TRANSACTIONS_FILTER) {
		return nil, errors.New("Block.Metadata is nil or Block.Metadata lacks a Tx filter bitmap")
	}
	txsFilter := txValidationFlags(block.Metadata.Metadata[common.BlockMetadataIndex_TRANSACTIONS_FILTER])
	if len(txsFilter) != len(block.Data.Data) {
		return nil, errors.Errorf("Block data size(%d) is different from Tx filter size(%d)", len(block.Data.Data), len(txsFilter))
	}

	sources := make(map[rwSetKey][]*peer.Endorsement)
	privateRWsetsInBlock := make(map[rwSetKey]struct{})
	missing := make(rwSetKeysByTxIDs)
	data := blockData(block.Data.Data)
	bi := &transactionInspector{
		sources:              sources,
		missingKeys:          missing,
		ownedRWsets:          ownedRWsets,
		privateRWsetsInBlock: privateRWsetsInBlock,
		coordinator:          c,
	}
	storePvtDataOfInvalidTx := c.Support.CapabilityProvider.Capabilities().StorePvtDataOfInvalidTx()
	txList, err := data.forEachTxn(storePvtDataOfInvalidTx, txsFilter, bi.inspectTransaction)
	if err != nil {
		return nil, err
	}

	privateInfo := &privateDataInfo{
		sources:                 sources,
		missingKeysByTxIDs:      missing,
		txns:                    txList,
		missingRWSButIneligible: bi.missingRWSButIneligible,
	}

	logger.Debug("Retrieving private write sets for", len(privateInfo.missingKeysByTxIDs), "transactions from transient store")

	// Put into ownedRWsets RW sets that are missing and found in the transient store
	c.fetchMissingFromTransientStore(privateInfo.missingKeysByTxIDs, ownedRWsets)

	// In the end, iterate over the ownedRWsets, and if the key doesn't exist in
	// the privateRWsetsInBlock - delete it from the ownedRWsets
	for k := range ownedRWsets {
		if _, exists := privateRWsetsInBlock[k]; !exists {
			logger.Warning("Removed", k.namespace, k.collection, "hash", k.hash, "from the data passed to the ledger")
			delete(ownedRWsets, k)
		}
	}

	privateInfo.missingKeys = privateInfo.missingKeysByTxIDs.flatten()
	// Remove all keys we already own
	privateInfo.missingKeys.exclude(func(key rwSetKey) bool {
		_, exists := ownedRWsets[key]
		return exists
	})

	return privateInfo, nil
}

type transactionInspector struct {
	*coordinator
	privateRWsetsInBlock    map[rwSetKey]struct{}
	missingKeys             rwSetKeysByTxIDs
	sources                 map[rwSetKey][]*peer.Endorsement
	ownedRWsets             map[rwSetKey][]byte
	missingRWSButIneligible []rwSetKey
}

func (bi *transactionInspector) inspectTransaction(seqInBlock uint64, chdr *common.ChannelHeader, txRWSet *rwsetutil.TxRwSet, endorsers []*peer.Endorsement) error {
	for _, ns := range txRWSet.NsRwSets {
		for _, hashedCollection := range ns.CollHashedRwSets {
			if !containsWrites(chdr.TxId, ns.NameSpace, hashedCollection) {
				continue
			}

			// If an error occurred due to the unavailability of database, we should stop committing
			// blocks for the associated chain. The policy can never be nil for a valid collection.
			// For collections which were never defined, the policy would be nil and we can safely
			// move on to the next collection.
			policy, err := bi.accessPolicyForCollection(chdr, ns.NameSpace, hashedCollection.CollectionName)
			if err != nil {
				return &vsccErrors.VSCCExecutionFailureError{Err: err}
			}

			if policy == nil {
				logger.Errorf("Failed to retrieve collection config for channel [%s], chaincode [%s], collection name [%s] for txID [%s]. Skipping.",
					chdr.ChannelId, ns.NameSpace, hashedCollection.CollectionName, chdr.TxId)
				continue
			}

			key := rwSetKey{
				txID:       chdr.TxId,
				seqInBlock: seqInBlock,
				hash:       hex.EncodeToString(hashedCollection.PvtRwSetHash),
				namespace:  ns.NameSpace,
				collection: hashedCollection.CollectionName,
			}

			if !bi.isEligible(policy, ns.NameSpace, hashedCollection.CollectionName) {
				logger.Debugf("Peer is not eligible for collection, channel [%s], chaincode [%s], "+
					"collection name [%s], txID [%s] the policy is [%#v]. Skipping.",
					chdr.ChannelId, ns.NameSpace, hashedCollection.CollectionName, chdr.TxId, policy)
				bi.missingRWSButIneligible = append(bi.missingRWSButIneligible, key)
				continue
			}

			bi.privateRWsetsInBlock[key] = struct{}{}
			if _, exists := bi.ownedRWsets[key]; !exists {
				txAndSeq := txAndSeqInBlock{
					txID:       chdr.TxId,
					seqInBlock: seqInBlock,
				}
				bi.missingKeys[txAndSeq] = append(bi.missingKeys[txAndSeq], key)
				bi.sources[key] = endorsersFromOrgs(ns.NameSpace, hashedCollection.CollectionName, endorsers, policy.MemberOrgs())
			}
		} // for all hashed RW sets
	} // for all RW sets
	return nil
}

// accessPolicyForCollection retrieves a CollectionAccessPolicy for a given namespace, collection name
// that corresponds to a given ChannelHeader
func (c *coordinator) accessPolicyForCollection(chdr *common.ChannelHeader, namespace string, col string) (privdata.CollectionAccessPolicy, error) {
	cp := common.CollectionCriteria{
		Channel:    chdr.ChannelId,
		Namespace:  namespace,
		Collection: col,
		TxId:       chdr.TxId,
	}
	sp, err := c.CollectionStore.RetrieveCollectionAccessPolicy(cp)

	if _, isNoSuchCollectionError := err.(privdata.NoSuchCollectionError); err != nil && !isNoSuchCollectionError {
		logger.Warning("Failed obtaining policy for", cp, "due to database unavailability:", err)
		return nil, err
	}
	return sp, nil
}

// isEligible checks if this peer is eligible for a given CollectionAccessPolicy
// It is used upon commit to determine if the peer is eligible to retrieve/persist
// the private data collection data for a given transaction.
func (c *coordinator) isEligible(ap privdata.CollectionAccessPolicy, namespace string, col string) bool {
	// Simple check to see if mspID part of collection's MemberOrgs list - FAB-17059
	if util.Contains(c.mspID, ap.MemberOrgs()) {
		return true
	}

	// If not part of list fall back to default policy evaluation logic
	filt := ap.AccessFilter()
	eligible := filt(c.selfSignedData)
	if !eligible {
		logger.Debug("Skipping namespace", namespace, "collection", col, "because we're not eligible for the private data")
	}
	return eligible
}

type seqAndDataModel struct {
	seq       uint64
	dataModel rwset.TxReadWriteSet_DataModel
}

// map from seqAndDataModel to:
//     maap from namespace to []*rwset.CollectionPvtReadWriteSet
type aggregatedCollections map[seqAndDataModel]map[string][]*rwset.CollectionPvtReadWriteSet

func (ac aggregatedCollections) addCollection(seqInBlock uint64, dm rwset.TxReadWriteSet_DataModel, namespace string, col *rwset.CollectionPvtReadWriteSet) {
	seq := seqAndDataModel{
		dataModel: dm,
		seq:       seqInBlock,
	}
	if _, exists := ac[seq]; !exists {
		ac[seq] = make(map[string][]*rwset.CollectionPvtReadWriteSet)
	}
	ac[seq][namespace] = append(ac[seq][namespace], col)
}

func (ac aggregatedCollections) asPrivateData() []*ledger.TxPvtData {
	var data []*ledger.TxPvtData
	for seq, ns := range ac {
		txPrivateData := &ledger.TxPvtData{
			SeqInBlock: seq.seq,
			WriteSet: &rwset.TxPvtReadWriteSet{
				DataModel: seq.dataModel,
			},
		}
		for namespaceName, cols := range ns {
			txPrivateData.WriteSet.NsPvtRwset = append(txPrivateData.WriteSet.NsPvtRwset, &rwset.NsPvtReadWriteSet{
				Namespace:          namespaceName,
				CollectionPvtRwset: cols,
			})
		}
		data = append(data, txPrivateData)
	}
	return data
}

// GetPvtDataAndBlockByNum get block by number and returns also all related private data
// the order of private data in slice of PvtDataCollections doesn't implies the order of
// transactions in the block related to these private data, to get the correct placement
// need to read TxPvtData.SeqInBlock field
func (c *coordinator) GetPvtDataAndBlockByNum(seqNum uint64, peerAuthInfo common.SignedData) (*common.Block, util.PvtDataCollections, error) {
	blockAndPvtData, err := c.Committer.GetPvtDataAndBlockByNum(seqNum)
	if err != nil {
		return nil, nil, err
	}

	seqs2Namespaces := aggregatedCollections(make(map[seqAndDataModel]map[string][]*rwset.CollectionPvtReadWriteSet))
	data := blockData(blockAndPvtData.Block.Data.Data)
	storePvtDataOfInvalidTx := c.Support.CapabilityProvider.Capabilities().StorePvtDataOfInvalidTx()
	data.forEachTxn(storePvtDataOfInvalidTx, make(txValidationFlags, len(data)),
		func(seqInBlock uint64, chdr *common.ChannelHeader, txRWSet *rwsetutil.TxRwSet, _ []*peer.Endorsement) error {
			item, exists := blockAndPvtData.PvtData[seqInBlock]
			if !exists {
				return nil
			}

			for _, ns := range item.WriteSet.NsPvtRwset {
				for _, col := range ns.CollectionPvtRwset {
					cc := common.CollectionCriteria{
						Channel:    chdr.ChannelId,
						TxId:       chdr.TxId,
						Namespace:  ns.Namespace,
						Collection: col.CollectionName,
					}
					sp, err := c.CollectionStore.RetrieveCollectionAccessPolicy(cc)
					if err != nil {
						logger.Warning("Failed obtaining policy for", cc, ":", err)
						continue
					}
					isAuthorized := sp.AccessFilter()
					if isAuthorized == nil {
						logger.Warning("Failed obtaining filter for", cc)
						continue
					}
					if !isAuthorized(peerAuthInfo) {
						logger.Debug("Skipping", cc, "because peer isn't authorized")
						continue
					}
					seqs2Namespaces.addCollection(seqInBlock, item.WriteSet.DataModel, ns.Namespace, col)
				}
			}
			return nil
		})

	return blockAndPvtData.Block, seqs2Namespaces.asPrivateData(), nil
}

func (c *coordinator) reportValidationDuration(time time.Duration) {
	c.metrics.ValidationDuration.With("channel", c.ChainID).Observe(time.Seconds())
}

func (c *coordinator) reportListMissingPrivateDataDuration(time time.Duration) {
	c.metrics.ListMissingPrivateDataDuration.With("channel", c.ChainID).Observe(time.Seconds())
}

func (c *coordinator) reportFetchDuration(time time.Duration) {
	c.metrics.FetchDuration.With("channel", c.ChainID).Observe(time.Seconds())
}

func (c *coordinator) reportCommitDuration(time time.Duration) {
	c.metrics.CommitPrivateDataDuration.With("channel", c.ChainID).Observe(time.Seconds())
}

func (c *coordinator) reportPurgeDuration(time time.Duration) {
	c.metrics.PurgeDuration.With("channel", c.ChainID).Observe(time.Seconds())
}

// containsWrites checks whether the given CollHashedRwSet contains writes
func containsWrites(txID string, namespace string, colHashedRWSet *rwsetutil.CollHashedRwSet) bool {
	if colHashedRWSet.HashedRwSet == nil {
		logger.Warningf("HashedRWSet of tx %s, namespace %s, collection %s is nil", txID, namespace, colHashedRWSet.CollectionName)
		return false
	}
	if len(colHashedRWSet.HashedRwSet.HashedWrites) == 0 && len(colHashedRWSet.HashedRwSet.MetadataWrites) == 0 {
		logger.Debugf("HashedRWSet of tx %s, namespace %s, collection %s doesn't contain writes", txID, namespace, colHashedRWSet.CollectionName)
		return false
	}
	return true
}
