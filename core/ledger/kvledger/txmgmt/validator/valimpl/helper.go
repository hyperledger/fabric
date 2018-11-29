/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package valimpl

import (
	"bytes"
	"fmt"

	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/customtx"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/privacyenabledstate"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/rwsetutil"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/statedb"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/txmgr"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/validator"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/validator/internal"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/version"
	"github.com/hyperledger/fabric/core/ledger/util"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/ledger/rwset"
	"github.com/hyperledger/fabric/protos/peer"
	"github.com/hyperledger/fabric/protos/utils"
)

// validateAndPreparePvtBatch pulls out the private write-set for the transactions that are marked as valid
// by the internal public data validator. Finally, it validates (if not already self-endorsed) the pvt rwset against the
// corresponding hash present in the public rwset
func validateAndPreparePvtBatch(block *internal.Block, db privacyenabledstate.DB,
	pubAndHashUpdates *internal.PubAndHashUpdates, pvtdata map[uint64]*ledger.TxPvtData) (*privacyenabledstate.PvtUpdateBatch, error) {
	pvtUpdates := privacyenabledstate.NewPvtUpdateBatch()
	metadataUpdates := metadataUpdates{}
	for _, tx := range block.Txs {
		if tx.ValidationCode != peer.TxValidationCode_VALID {
			continue
		}
		if !tx.ContainsPvtWrites() {
			continue
		}
		txPvtdata := pvtdata[uint64(tx.IndexInBlock)]
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
		addPvtRWSetToPvtUpdateBatch(pvtRWSet, pvtUpdates, version.NewHeight(block.Num, uint64(tx.IndexInBlock)))
		addEntriesToMetadataUpdates(metadataUpdates, pvtRWSet)
	}
	if err := incrementPvtdataVersionIfNeeded(metadataUpdates, pvtUpdates, pubAndHashUpdates, db); err != nil {
		return nil, err
	}
	return pvtUpdates, nil
}

// requiresPvtdataValidation returns whether or not a hashes of the collection should be computed
// for the collections of present in the private data
// TODO for now always return true. Add capabilty of checking if this data was produced by
// the validating peer itself during similation and in that case return false
func requiresPvtdataValidation(tx *ledger.TxPvtData) bool {
	return true
}

// validPvtdata returns true if hashes of all the collections writeset present in the pvt data
// match with the corresponding hashes present in the public read-write set
func validatePvtdata(tx *internal.Transaction, pvtdata *ledger.TxPvtData) error {
	if pvtdata.WriteSet == nil {
		return nil
	}

	for _, nsPvtdata := range pvtdata.WriteSet.NsPvtRwset {
		for _, collPvtdata := range nsPvtdata.CollectionPvtRwset {
			collPvtdataHash := util.ComputeHash(collPvtdata.Rwset)
			hashInPubdata := tx.RetrieveHash(nsPvtdata.Namespace, collPvtdata.CollectionName)
			if !bytes.Equal(collPvtdataHash, hashInPubdata) {
				return &validator.ErrPvtdataHashMissmatch{
					Msg: fmt.Sprintf(`Hash of pvt data for collection [%s:%s] does not match with the corresponding hash in the public data.
					public hash = [%#v], pvt data hash = [%#v]`, nsPvtdata.Namespace, collPvtdata.CollectionName, hashInPubdata, collPvtdataHash),
				}
			}
		}
	}
	return nil
}

// preprocessProtoBlock parses the proto instance of block into 'Block' structure.
// The retuned 'Block' structure contains only transactions that are endorser transactions and are not alredy marked as invalid
func preprocessProtoBlock(txMgr txmgr.TxMgr,
	validateKVFunc func(key string, value []byte) error,
	block *common.Block, doMVCCValidation bool,
) (*internal.Block, []*txmgr.TxStatInfo, error) {
	b := &internal.Block{Num: block.Header.Number}
	txsStatInfo := []*txmgr.TxStatInfo{}
	// Committer validator has already set validation flags based on well formed tran checks
	txsFilter := util.TxValidationFlags(block.Metadata.Metadata[common.BlockMetadataIndex_TRANSACTIONS_FILTER])
	for txIndex, envBytes := range block.Data.Data {
		var env *common.Envelope
		var chdr *common.ChannelHeader
		var payload *common.Payload
		var err error
		txStatInfo := &txmgr.TxStatInfo{TxType: -1}
		txsStatInfo = append(txsStatInfo, txStatInfo)
		if env, err = utils.GetEnvelopeFromBlock(envBytes); err == nil {
			if payload, err = utils.GetPayload(env); err == nil {
				chdr, err = utils.UnmarshalChannelHeader(payload.Header.ChannelHeader)
			}
		}
		if txsFilter.IsInvalid(txIndex) {
			// Skipping invalid transaction
			logger.Warningf("Channel [%s]: Block [%d] Transaction index [%d] TxId [%s]"+
				" marked as invalid by committer. Reason code [%s]",
				chdr.GetChannelId(), block.Header.Number, txIndex, chdr.GetTxId(),
				txsFilter.Flag(txIndex).String())
			continue
		}
		if err != nil {
			return nil, nil, err
		}

		var txRWSet *rwsetutil.TxRwSet
		txType := common.HeaderType(chdr.Type)
		logger.Debugf("txType=%s", txType)
		txStatInfo.TxType = txType
		if txType == common.HeaderType_ENDORSER_TRANSACTION {
			// extract actions from the envelope message
			respPayload, err := utils.GetActionFromEnvelope(envBytes)
			if err != nil {
				txsFilter.SetFlag(txIndex, peer.TxValidationCode_NIL_TXACTION)
				continue
			}
			txStatInfo.ChaincodeID = respPayload.ChaincodeId
			txRWSet = &rwsetutil.TxRwSet{}
			if err = txRWSet.FromProtoBytes(respPayload.Results); err != nil {
				txsFilter.SetFlag(txIndex, peer.TxValidationCode_INVALID_OTHER_REASON)
				continue
			}
		} else {
			rwsetProto, err := processNonEndorserTx(env, chdr.TxId, txType, txMgr, !doMVCCValidation)
			if _, ok := err.(*customtx.InvalidTxError); ok {
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
		}
		if txRWSet != nil {
			txStatInfo.NumCollections = txRWSet.NumCollections()
			if err := validateWriteset(txRWSet, validateKVFunc); err != nil {
				logger.Warningf("Channel [%s]: Block [%d] Transaction index [%d] TxId [%s]"+
					" marked as invalid. Reason code [%s]",
					chdr.GetChannelId(), block.Header.Number, txIndex, chdr.GetTxId(), peer.TxValidationCode_INVALID_WRITESET)
				txsFilter.SetFlag(txIndex, peer.TxValidationCode_INVALID_WRITESET)
				continue
			}
			b.Txs = append(b.Txs, &internal.Transaction{IndexInBlock: txIndex, ID: chdr.TxId, RWSet: txRWSet})
		}
	}
	return b, txsStatInfo, nil
}

func processNonEndorserTx(txEnv *common.Envelope, txid string, txType common.HeaderType, txmgr txmgr.TxMgr, synchingState bool) (*rwset.TxReadWriteSet, error) {
	logger.Debugf("Performing custom processing for transaction [txid=%s], [txType=%s]", txid, txType)
	processor := customtx.GetProcessor(txType)
	logger.Debugf("Processor for custom tx processing:%#v", processor)
	if processor == nil {
		return nil, nil
	}

	var err error
	var sim ledger.TxSimulator
	var simRes *ledger.TxSimulationResults
	if sim, err = txmgr.NewTxSimulator(txid); err != nil {
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
func postprocessProtoBlock(block *common.Block, validatedBlock *internal.Block) {
	txsFilter := util.TxValidationFlags(block.Metadata.Metadata[common.BlockMetadataIndex_TRANSACTIONS_FILTER])
	for _, tx := range validatedBlock.Txs {
		txsFilter.SetFlag(tx.IndexInBlock, tx.ValidationCode)
	}
	block.Metadata.Metadata[common.BlockMetadataIndex_TRANSACTIONS_FILTER] = txsFilter
}

func addPvtRWSetToPvtUpdateBatch(pvtRWSet *rwsetutil.TxPvtRwSet, pvtUpdateBatch *privacyenabledstate.PvtUpdateBatch, ver *version.Height) {
	for _, ns := range pvtRWSet.NsPvtRwSet {
		for _, coll := range ns.CollPvtRwSets {
			for _, kvwrite := range coll.KvRwSet.Writes {
				if !kvwrite.IsDelete {
					pvtUpdateBatch.Put(ns.NameSpace, coll.CollectionName, kvwrite.Key, kvwrite.Value, ver)
				} else {
					pvtUpdateBatch.Delete(ns.NameSpace, coll.CollectionName, kvwrite.Key, ver)
				}
			}
		}
	}
}

// incrementPvtdataVersionIfNeeded changes the versions of the private data keys if the version of the corresponding hashed key has
// been upgrded. A metadata-update-only type of transaction may have caused the version change of the existing value in the hashed space.
// Iterate through all the metadata writes and try to get these keys and increment the version in the private writes to be the same as of the hashed key version - if the latest
// value of the key is available. Otherwise, in this scenario, we end up having the latest value in the private state but the version
// gets left as stale and will cause simulation failure because of wrongly assuming that we have stale value
func incrementPvtdataVersionIfNeeded(
	metadataUpdates metadataUpdates,
	pvtUpdateBatch *privacyenabledstate.PvtUpdateBatch,
	pubAndHashUpdates *internal.PubAndHashUpdates,
	db privacyenabledstate.DB) error {

	for collKey := range metadataUpdates {
		ns, coll, key := collKey.ns, collKey.coll, collKey.key
		keyHash := util.ComputeStringHash(key)
		hashedVal := pubAndHashUpdates.HashUpdates.Get(ns, coll, string(keyHash))
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
	db privacyenabledstate.DB) (val *statedb.VersionedValue, err error) {
	val = pvtUpdateBatch.Get(ns, coll, key)
	if val == nil {
		val, err = db.GetPrivateData(ns, coll, key)
	}
	return
}
