/*
Copyright IBM Corp. 2016 All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

		 http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package statebasedval

import (
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/rwset"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/statedb"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/version"
	"github.com/hyperledger/fabric/core/ledger/util"
	"github.com/hyperledger/fabric/protos/common"
	putils "github.com/hyperledger/fabric/protos/utils"
	logging "github.com/op/go-logging"
)

var logger = logging.MustGetLogger("statevalidator")

// Validator validates a tx against the latest committed state
// and preceding valid transactions with in the same block
type Validator struct {
	db statedb.VersionedDB
}

// NewValidator constructs StateValidator
func NewValidator(db statedb.VersionedDB) *Validator {
	return &Validator{db}
}

//validate endorser transaction
func (v *Validator) validateEndorserTX(envBytes []byte, doMVCCValidation bool, updates *statedb.UpdateBatch) (*rwset.TxReadWriteSet, error) {
	// extract actions from the envelope message
	respPayload, err := putils.GetActionFromEnvelope(envBytes)
	if err != nil {
		return nil, err
	}

	//preparation for extracting RWSet from transaction
	txRWSet := &rwset.TxReadWriteSet{}

	// Get the Result from the Action
	// and then Unmarshal it into a TxReadWriteSet using custom unmarshalling
	if err = txRWSet.Unmarshal(respPayload.Results); err != nil {
		return nil, err
	}

	// trace the first 1000 characters of RWSet only, in case it is huge
	if logger.IsEnabledFor(logging.DEBUG) {
		txRWSetString := txRWSet.String()
		if len(txRWSetString) < 1000 {
			logger.Debugf("validating txRWSet:[%s]", txRWSetString)
		} else {
			logger.Debugf("validating txRWSet:[%s...]", txRWSetString[0:1000])
		}
	}

	//mvccvalidation, may invalidate transaction
	if doMVCCValidation {
		if valid, err := v.validateTx(txRWSet, updates); err != nil {
			return nil, err
		} else if !valid {
			txRWSet = nil
		}
	}

	return txRWSet, err
}

// TODO validate configuration transaction
func (v *Validator) validateConfigTX(env *common.Envelope) (bool, error) {
	return true, nil
}

// ValidateAndPrepareBatch implements method in Validator interface
func (v *Validator) ValidateAndPrepareBatch(block *common.Block, doMVCCValidation bool) (*statedb.UpdateBatch, error) {
	logger.Debugf("New block arrived for validation:%#v, doMVCCValidation=%t", block, doMVCCValidation)
	updates := statedb.NewUpdateBatch()
	logger.Debugf("Validating a block with [%d] transactions", len(block.Data.Data))
	txsFilter := util.NewFilterBitArrayFromBytes(block.Metadata.Metadata[common.BlockMetadataIndex_TRANSACTIONS_FILTER])
	for txIndex, envBytes := range block.Data.Data {
		if txsFilter.IsSet(uint(txIndex)) {
			// Skiping invalid transaction
			logger.Debug("Skipping transaction marked as invalid, txIndex=", txIndex)
			continue
		}

		env, err := putils.GetEnvelopeFromBlock(envBytes)
		if err != nil {
			return nil, err
		}

		payload, err := putils.GetPayload(env)
		if err != nil {
			return nil, err
		}

		valid := false
		if common.HeaderType(payload.Header.ChannelHeader.Type) == common.HeaderType_ENDORSER_TRANSACTION {
			txRWSet, err := v.validateEndorserTX(envBytes, doMVCCValidation, updates)
			if err != nil {
				return nil, err
			}
			//txRWSet != nil => t is valid
			if txRWSet != nil {
				committingTxHeight := version.NewHeight(block.Header.Number, uint64(txIndex+1))
				addWriteSetToBatch(txRWSet, committingTxHeight, updates)
				valid = true
			}
		} else if common.HeaderType(payload.Header.ChannelHeader.Type) == common.HeaderType_CONFIG {
			valid, err = v.validateConfigTX(env)
			if err != nil {
				return nil, err
			}
		} else {
			logger.Errorf("Skipping transaction %d that's not an endorsement or configuration %d", txIndex, payload.Header.ChannelHeader.Type)
			valid = false
		}

		if !valid {
			// Unset bit in byte array corresponded to the invalid transaction
			txsFilter.Set(uint(txIndex))
		}
	}
	block.Metadata.Metadata[common.BlockMetadataIndex_TRANSACTIONS_FILTER] = txsFilter.ToBytes()
	return updates, nil
}

func addWriteSetToBatch(txRWSet *rwset.TxReadWriteSet, txHeight *version.Height, batch *statedb.UpdateBatch) {
	for _, nsRWSet := range txRWSet.NsRWs {
		ns := nsRWSet.NameSpace
		for _, kvWrite := range nsRWSet.Writes {
			if kvWrite.IsDelete {
				batch.Delete(ns, kvWrite.Key, txHeight)
			} else {
				batch.Put(ns, kvWrite.Key, kvWrite.Value, txHeight)
			}
		}
	}
}

func (v *Validator) validateTx(txRWSet *rwset.TxReadWriteSet, updates *statedb.UpdateBatch) (bool, error) {
	for _, nsRWSet := range txRWSet.NsRWs {
		ns := nsRWSet.NameSpace
		//TODO introduce different Error codes for different causes of validation failure
		if valid, err := v.validateReadSet(ns, nsRWSet.Reads, updates); !valid || err != nil {
			return valid, err
		}
		if valid, err := v.validateRangeQueries(ns, nsRWSet.RangeQueriesInfo, updates); !valid || err != nil {
			return valid, err
		}
	}
	return true, nil
}

func (v *Validator) validateReadSet(ns string, kvReads []*rwset.KVRead, updates *statedb.UpdateBatch) (bool, error) {
	for _, kvRead := range kvReads {
		if valid, err := v.validateKVRead(ns, kvRead, updates); !valid || err != nil {
			return valid, err
		}
	}
	return true, nil
}

// validateKVRead performs mvcc check for a key read during transaction simulation.
// i.e., it checks whether a key/version combination is already updated in the statedb (by an already committed block)
// or in the updates (by a preceding valid transaction in the current block)
func (v *Validator) validateKVRead(ns string, kvRead *rwset.KVRead, updates *statedb.UpdateBatch) (bool, error) {
	if updates.Exists(ns, kvRead.Key) {
		return false, nil
	}
	versionedValue, err := v.db.GetState(ns, kvRead.Key)
	if err != nil {
		return false, nil
	}
	var committedVersion *version.Height
	if versionedValue != nil {
		committedVersion = versionedValue.Version
	}
	if !version.AreSame(committedVersion, kvRead.Version) {
		logger.Debugf("Version mismatch for key [%s:%s]. Committed version = [%s], Version in readSet [%s]",
			ns, kvRead.Key, committedVersion, kvRead.Version)
		return false, nil
	}
	return true, nil
}

func (v *Validator) validateRangeQueries(ns string, rangeQueriesInfo []*rwset.RangeQueryInfo, updates *statedb.UpdateBatch) (bool, error) {
	for _, rqi := range rangeQueriesInfo {
		if valid, err := v.validateRangeQuery(ns, rqi, updates); !valid || err != nil {
			return valid, err
		}
	}
	return true, nil
}

// validateRangeQuery performs a phatom read check i.e., it
// checks whether the results of the range query are still the same when executed on the
// statedb (latest state as of last committed block) + updates (prepared by the writes of preceding valid transactions
// in the current block and yet to be committed as part of group commit at the end of the validation of the block)
func (v *Validator) validateRangeQuery(ns string, rangeQueryInfo *rwset.RangeQueryInfo, updates *statedb.UpdateBatch) (bool, error) {
	logger.Debugf("validateRangeQuery: ns=%s, rangeQueryInfo=%s", ns, rangeQueryInfo)

	// If during simulation, the caller had not exhausted the iterator so
	// rangeQueryInfo.EndKey is not actual endKey given by the caller in the range query
	// but rather it is the last key seen by the caller and hence the combinedItr should include the endKey in the results.
	includeEndKey := !rangeQueryInfo.ItrExhausted

	combinedItr, err := newCombinedIterator(v.db, updates,
		ns, rangeQueryInfo.StartKey, rangeQueryInfo.EndKey, includeEndKey)
	if err != nil {
		return false, err
	}
	defer combinedItr.Close()
	var validator rangeQueryValidator
	if rangeQueryInfo.ResultHash != nil {
		logger.Debug(`Hashing results are present in the range query info hence, initiating hashing based validation`)
		validator = &rangeQueryHashValidator{}
	} else {
		logger.Debug(`Hashing results are not present in the range query info hence, initiating raw KVReads based validation`)
		validator = &rangeQueryResultsValidator{}
	}
	validator.init(rangeQueryInfo, combinedItr)
	return validator.validate()
}
