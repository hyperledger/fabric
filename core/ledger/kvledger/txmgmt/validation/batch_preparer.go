/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package validation

import (
	"bytes"

	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric-protos-go/ledger/rwset"
	"github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/internal/version"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/privacyenabledstate"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/rwsetutil"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/statedb"
	"github.com/hyperledger/fabric/core/ledger/util"
	"github.com/hyperledger/fabric/internal/pkg/txflags"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
)

var logger = flogging.MustGetLogger("validation")

// PostOrderSimulatorProvider provides access to a tx simulator for executing post order non-endorser transactions
type PostOrderSimulatorProvider interface {
	NewTxSimulator(txid string) (ledger.TxSimulator, error)
}

// CommitBatchPreparer performs validation and prepares the final batch that is to be committed to the statedb
type CommitBatchPreparer struct {
	postOrderSimulatorProvider PostOrderSimulatorProvider
	db                         *privacyenabledstate.DB
	validator                  *validator
	customTxProcessors         map[common.HeaderType]ledger.CustomTxProcessor
}

// TxStatInfo encapsulates information about a transaction
type TxStatInfo struct {
	TxIDFromChannelHeader string
	ValidationCode        peer.TxValidationCode
	TxType                common.HeaderType
	ChaincodeID           *peer.ChaincodeID
	ChaincodeEventData    []byte
	NumCollections        int
}

// NewCommitBatchPreparer constructs a validator that internally manages statebased validator and in addition
// handles the tasks that are agnostic to a particular validation scheme such as parsing the block and handling the pvt data
func NewCommitBatchPreparer(
	postOrderSimulatorProvider PostOrderSimulatorProvider,
	db *privacyenabledstate.DB,
	customTxProcessors map[common.HeaderType]ledger.CustomTxProcessor,
	hashFunc rwsetutil.HashFunc,
) *CommitBatchPreparer {
	return &CommitBatchPreparer{
		postOrderSimulatorProvider,
		db,
		&validator{
			db:       db,
			hashFunc: hashFunc,
		},
		customTxProcessors,
	}
}

// ValidateAndPrepareBatch performs validation of transactions in the block and prepares the batch of final writes
func (p *CommitBatchPreparer) ValidateAndPrepareBatch(blockAndPvtdata *ledger.BlockAndPvtData,
	doMVCCValidation bool) (*privacyenabledstate.UpdateBatch, []*AppInitiatedPurgeUpdate, []*TxStatInfo, error) {
	blk := blockAndPvtdata.Block
	logger.Debugf("ValidateAndPrepareBatch() for block number = [%d]", blk.Header.Number)
	var internalBlock *block
	var txsStatInfo []*TxStatInfo
	var pubAndHashUpdates *publicAndHashUpdates
	var pvtUpdates *privacyenabledstate.PvtUpdateBatch
	var purgeUpdates []*AppInitiatedPurgeUpdate
	var err error

	logger.Debug("preprocessing ProtoBlock...")
	if internalBlock, txsStatInfo, err = preprocessProtoBlock(
		p.postOrderSimulatorProvider,
		p.db.ValidateKeyValue,
		blk,
		doMVCCValidation,
		p.customTxProcessors,
	); err != nil {
		return nil, nil, nil, err
	}

	if pubAndHashUpdates, purgeUpdates, err = p.validator.validateAndPrepareBatch(internalBlock, doMVCCValidation); err != nil {
		return nil, nil, nil, err
	}
	logger.Debug("validating rwset...")
	if pvtUpdates, err = validateAndPreparePvtBatch(
		internalBlock,
		p.db,
		pubAndHashUpdates,
		blockAndPvtdata.PvtData,
	); err != nil {
		return nil, nil, nil, err
	}
	logger.Debug("postprocessing ProtoBlock...")
	postprocessProtoBlock(blk, internalBlock)
	logger.Debug("ValidateAndPrepareBatch() complete")

	txsFilter := txflags.ValidationFlags(blk.Metadata.Metadata[common.BlockMetadataIndex_TRANSACTIONS_FILTER])
	for i := range txsFilter {
		txsStatInfo[i].ValidationCode = txsFilter.Flag(i)
	}
	return &privacyenabledstate.UpdateBatch{
		PubUpdates:  pubAndHashUpdates.publicUpdates,
		HashUpdates: pubAndHashUpdates.hashUpdates,
		PvtUpdates:  pvtUpdates,
	}, purgeUpdates, txsStatInfo, nil
}

// validateAndPreparePvtBatch pulls out the private write-set for the transactions that are marked as valid
// by the internal public data validator. Finally, it validates (if not already self-endorsed) the pvt rwset against the
// corresponding hash present in the public rwset
func validateAndPreparePvtBatch(
	blk *block,
	db *privacyenabledstate.DB,
	pubAndHashUpdates *publicAndHashUpdates,
	pvtdata map[uint64]*ledger.TxPvtData,
) (*privacyenabledstate.PvtUpdateBatch, error) {
	pvtUpdates := privacyenabledstate.NewPvtUpdateBatch()
	metadataUpdates := metadataUpdates{}
	for _, tx := range blk.txs {
		if tx.validationCode != peer.TxValidationCode_VALID {
			continue
		}
		if !tx.containsPvtWrites() {
			continue
		}
		txPvtdata := pvtdata[uint64(tx.indexInBlock)]
		if txPvtdata == nil {
			continue
		}
		if requiresPvtdataValidation(txPvtdata) {
			if err := validatePvtdata(tx, txPvtdata); err != nil {
				return nil, err
			}
		}
		var pvtRWSet *rwsetutil.TxPvtRwSet
		var err error
		if pvtRWSet, err = rwsetutil.TxPvtRwSetFromProtoMsg(txPvtdata.WriteSet); err != nil {
			return nil, err
		}
		addPvtRWSetToPvtUpdateBatch(pvtRWSet, pvtUpdates, version.NewHeight(blk.num, uint64(tx.indexInBlock)))
		addEntriesToMetadataUpdates(metadataUpdates, pvtRWSet)
	}
	if err := incrementPvtdataVersionIfNeeded(metadataUpdates, pvtUpdates, pubAndHashUpdates, db); err != nil {
		return nil, err
	}
	return pvtUpdates, nil
}

// requiresPvtdataValidation returns whether or not a hashes of the collection should be computed
// for the collections of present in the private data
// TODO for now always return true. Add capability of checking if this data was produced by
// the validating peer itself during simulation and in that case return false
func requiresPvtdataValidation(tx *ledger.TxPvtData) bool {
	return true
}

// validPvtdata returns true if hashes of all the collections writeset present in the pvt data
// match with the corresponding hashes present in the public read-write set
func validatePvtdata(tx *transaction, pvtdata *ledger.TxPvtData) error {
	if pvtdata.WriteSet == nil {
		return nil
	}

	for _, nsPvtdata := range pvtdata.WriteSet.NsPvtRwset {
		for _, collPvtdata := range nsPvtdata.CollectionPvtRwset {
			collPvtdataHash := util.ComputeHash(collPvtdata.Rwset)
			hashInPubdata := tx.retrieveHash(nsPvtdata.Namespace, collPvtdata.CollectionName)
			if !bytes.Equal(collPvtdataHash, hashInPubdata) {
				return errors.Errorf(`hash of pvt data for collection [%s:%s] does not match with the corresponding hash in the public data. public hash = [%#v], pvt data hash = [%#v]`,
					nsPvtdata.Namespace, collPvtdata.CollectionName, hashInPubdata, collPvtdataHash)
			}
		}
	}
	return nil
}

// preprocessProtoBlock parses the proto instance of block into 'Block' structure.
// The returned 'Block' structure contains only transactions that are endorser transactions and are not already marked as invalid
func preprocessProtoBlock(postOrderSimulatorProvider PostOrderSimulatorProvider,
	validateKVFunc func(key string, value []byte) error,
	blk *common.Block, doMVCCValidation bool,
	customTxProcessors map[common.HeaderType]ledger.CustomTxProcessor,
) (*block, []*TxStatInfo, error) {
	b := &block{num: blk.Header.Number}
	txsStatInfo := []*TxStatInfo{}
	// Committer validator has already set validation flags based on well formed tran checks
	txsFilter := txflags.ValidationFlags(blk.Metadata.Metadata[common.BlockMetadataIndex_TRANSACTIONS_FILTER])
	for txIndex, envBytes := range blk.Data.Data {
		var env *common.Envelope
		var chdr *common.ChannelHeader
		var payload *common.Payload
		var err error
		txStatInfo := &TxStatInfo{TxType: -1}
		txsStatInfo = append(txsStatInfo, txStatInfo)
		if env, err = protoutil.GetEnvelopeFromBlock(envBytes); err == nil {
			if payload, err = protoutil.UnmarshalPayload(env.Payload); err == nil {
				chdr, err = protoutil.UnmarshalChannelHeader(payload.Header.ChannelHeader)
			}
		}
		txStatInfo.TxIDFromChannelHeader = chdr.GetTxId()
		if txsFilter.IsInvalid(txIndex) {
			// Skipping invalid transaction
			logger.Warningf("Channel [%s]: Block [%d] Transaction index [%d] TxId [%s]"+
				" marked as invalid by committer. Reason code [%s]",
				chdr.GetChannelId(), blk.Header.Number, txIndex, chdr.GetTxId(),
				txsFilter.Flag(txIndex).String())
			continue
		}
		if err != nil {
			return nil, nil, err
		}

		var txRWSet *rwsetutil.TxRwSet
		var containsPostOrderWrites bool
		txType := common.HeaderType(chdr.Type)
		logger.Debugf("txType=%s", txType)
		txStatInfo.TxType = txType
		if txType == common.HeaderType_ENDORSER_TRANSACTION {
			// extract actions from the envelope message
			respPayload, err := protoutil.GetActionFromEnvelope(envBytes)
			if err != nil {
				txsFilter.SetFlag(txIndex, peer.TxValidationCode_NIL_TXACTION)
				continue
			}
			txStatInfo.ChaincodeID = respPayload.ChaincodeId
			txStatInfo.ChaincodeEventData = respPayload.Events
			txRWSet = &rwsetutil.TxRwSet{}
			if err = txRWSet.FromProtoBytes(respPayload.Results); err != nil {
				txsFilter.SetFlag(txIndex, peer.TxValidationCode_INVALID_OTHER_REASON)
				continue
			}
		} else {
			rwsetProto, err := processNonEndorserTx(
				env,
				chdr.TxId,
				txType,
				postOrderSimulatorProvider,
				!doMVCCValidation,
				customTxProcessors,
			)
			if _, ok := err.(*ledger.InvalidTxError); ok {
				txsFilter.SetFlag(txIndex, peer.TxValidationCode_INVALID_OTHER_REASON)
				continue
			}
			if err != nil {
				return nil, nil, err
			}
			if rwsetProto != nil {
				if txRWSet, err = rwsetutil.TxRwSetFromProtoMsg(rwsetProto); err != nil {
					return nil, nil, err
				}
			}
			containsPostOrderWrites = true
		}
		if txRWSet != nil {
			txStatInfo.NumCollections = txRWSet.NumCollections()
			if err := validateWriteset(txRWSet, validateKVFunc); err != nil {
				logger.Warningf("Channel [%s]: Block [%d] Transaction index [%d] TxId [%s]"+
					" marked as invalid. Reason code [%s]",
					chdr.GetChannelId(), blk.Header.Number, txIndex, chdr.GetTxId(), peer.TxValidationCode_INVALID_WRITESET)
				txsFilter.SetFlag(txIndex, peer.TxValidationCode_INVALID_WRITESET)
				continue
			}
			b.txs = append(b.txs, &transaction{
				indexInBlock:            txIndex,
				id:                      chdr.TxId,
				rwset:                   txRWSet,
				containsPostOrderWrites: containsPostOrderWrites,
			})
		}
	}
	return b, txsStatInfo, nil
}

func processNonEndorserTx(
	txEnv *common.Envelope,
	txid string,
	txType common.HeaderType,
	postOrderSimulatorProvider PostOrderSimulatorProvider,
	synchingState bool,
	customTxProcessors map[common.HeaderType]ledger.CustomTxProcessor,
) (*rwset.TxReadWriteSet, error) {
	logger.Debugf("Performing custom processing for transaction [txid=%s], [txType=%s]", txid, txType)
	processor := customTxProcessors[txType]
	logger.Debugf("Processor for custom tx processing:%#v", processor)
	if processor == nil {
		return nil, nil
	}

	var err error
	var sim ledger.TxSimulator
	var simRes *ledger.TxSimulationResults
	if sim, err = postOrderSimulatorProvider.NewTxSimulator(txid); err != nil {
		return nil, err
	}
	defer sim.Done()
	if err = processor.GenerateSimulationResults(txEnv, sim, synchingState); err != nil {
		return nil, err
	}
	if simRes, err = sim.GetTxSimulationResults(); err != nil {
		return nil, err
	}
	return simRes.PubSimulationResults, nil
}

func validateWriteset(txRWSet *rwsetutil.TxRwSet, validateKVFunc func(key string, value []byte) error) error {
	for _, nsRwSet := range txRWSet.NsRwSets {
		pubWriteset := nsRwSet.KvRwSet
		if pubWriteset == nil {
			continue
		}
		for _, kvwrite := range pubWriteset.Writes {
			if err := validateKVFunc(kvwrite.Key, kvwrite.Value); err != nil {
				return err
			}
		}
	}
	return nil
}

// postprocessProtoBlock updates the proto block's validation flags (in metadata) by the results of validation process
func postprocessProtoBlock(blk *common.Block, validatedBlock *block) {
	txsFilter := txflags.ValidationFlags(blk.Metadata.Metadata[common.BlockMetadataIndex_TRANSACTIONS_FILTER])
	for _, tx := range validatedBlock.txs {
		txsFilter.SetFlag(tx.indexInBlock, tx.validationCode)
	}
	blk.Metadata.Metadata[common.BlockMetadataIndex_TRANSACTIONS_FILTER] = txsFilter
}

func addPvtRWSetToPvtUpdateBatch(pvtRWSet *rwsetutil.TxPvtRwSet, pvtUpdateBatch *privacyenabledstate.PvtUpdateBatch, ver *version.Height) {
	for _, ns := range pvtRWSet.NsPvtRwSet {
		for _, coll := range ns.CollPvtRwSets {
			for _, kvwrite := range coll.KvRwSet.Writes {
				if !rwsetutil.IsKVWriteDelete(kvwrite) {
					pvtUpdateBatch.Put(ns.NameSpace, coll.CollectionName, kvwrite.Key, kvwrite.Value, ver)
				} else {
					pvtUpdateBatch.Delete(ns.NameSpace, coll.CollectionName, kvwrite.Key, ver)
				}
			}
		}
	}
}

// incrementPvtdataVersionIfNeeded changes the versions of the private data keys if the version of the corresponding hashed key has
// been upgraded. A metadata-update-only type of transaction may have caused the version change of the existing value in the hashed space.
// Iterate through all the metadata writes and try to get these keys and increment the version in the private writes to be the same as of the hashed key version - if the latest
// value of the key is available. Otherwise, in this scenario, we end up having the latest value in the private state but the version
// gets left as stale and will cause simulation failure because of wrongly assuming that we have stale value
func incrementPvtdataVersionIfNeeded(
	metadataUpdates metadataUpdates,
	pvtUpdateBatch *privacyenabledstate.PvtUpdateBatch,
	pubAndHashUpdates *publicAndHashUpdates,
	db *privacyenabledstate.DB) error {
	for collKey := range metadataUpdates {
		ns, coll, key := collKey.ns, collKey.coll, collKey.key
		keyHash := util.ComputeStringHash(key)
		hashedVal := pubAndHashUpdates.hashUpdates.Get(ns, coll, string(keyHash))
		if hashedVal == nil {
			// This key is finally not getting updated in the hashed space by this block -
			// either the metadata update was on a non-existing key or the key gets deleted by a latter transaction in the block
			// ignore the metadata update for this key
			continue
		}
		latestVal, err := retrieveLatestVal(ns, coll, key, pvtUpdateBatch, db)
		if err != nil {
			return err
		}
		if latestVal == nil || // latest value not found either in db or in the pvt updates (caused by commit with missing data)
			version.AreSame(latestVal.Version, hashedVal.Version) { // version is already same as in hashed space - No version increment because of metadata-only transaction took place
			continue
		}
		// TODO - computing hash could be avoided. In the hashed updates, we can augment additional info that
		// which original version has been renewed
		latestValHash := util.ComputeHash(latestVal.Value)
		if bytes.Equal(latestValHash, hashedVal.Value) { // since we allow block commits with missing pvt data, the private value available may be stale.
			// upgrade the version only if the pvt value matches with corresponding hash in the hashed space
			pvtUpdateBatch.Put(ns, coll, key, latestVal.Value, hashedVal.Version)
		}
	}
	return nil
}

type collKey struct {
	ns, coll, key string
}

type metadataUpdates map[collKey]bool

func addEntriesToMetadataUpdates(metadataUpdates metadataUpdates, pvtRWSet *rwsetutil.TxPvtRwSet) {
	for _, ns := range pvtRWSet.NsPvtRwSet {
		for _, coll := range ns.CollPvtRwSets {
			for _, metadataWrite := range coll.KvRwSet.MetadataWrites {
				ns, coll, key := ns.NameSpace, coll.CollectionName, metadataWrite.Key
				metadataUpdates[collKey{ns, coll, key}] = true
			}
		}
	}
}

func retrieveLatestVal(ns, coll, key string, pvtUpdateBatch *privacyenabledstate.PvtUpdateBatch,
	db *privacyenabledstate.DB) (val *statedb.VersionedValue, err error) {
	val = pvtUpdateBatch.Get(ns, coll, key)
	if val == nil {
		val, err = db.GetPrivateData(ns, coll, key)
	}
	return
}
