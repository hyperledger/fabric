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
	"github.com/hyperledger/fabric/protos/common"
	pb "github.com/hyperledger/fabric/protos/peer"
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

// ValidateAndPrepareBatch implements method in Validator interface
func (v *Validator) ValidateAndPrepareBatch(block *common.Block, doMVCCValidation bool) (
	*common.Block, []*pb.InvalidTransaction, *statedb.UpdateBatch, error) {
	logger.Debugf("New block arrived for validation:%#v, doMVCCValidation=%t", block, doMVCCValidation)
	invalidTxs := []*pb.InvalidTransaction{}
	var valid bool
	updates := statedb.NewUpdateBatch()
	logger.Debugf("Validating a block with [%d] transactions", len(block.Data.Data))
	for txIndex, envBytes := range block.Data.Data {
		// extract actions from the envelope message
		respPayload, err := putils.GetActionFromEnvelope(envBytes)
		if err != nil {
			return nil, nil, nil, err
		}

		//preparation for extracting RWSet from transaction
		txRWSet := &rwset.TxReadWriteSet{}

		// Get the Result from the Action
		// and then Unmarshal it into a TxReadWriteSet using custom unmarshalling
		if err = txRWSet.Unmarshal(respPayload.Results); err != nil {
			return nil, nil, nil, err
		}

		// trace the first 2000 characters of RWSet only, in case it is huge
		if logger.IsEnabledFor(logging.DEBUG) {
			txRWSetString := txRWSet.String()
			if len(txRWSetString) < 2000 {
				logger.Debugf("validating txRWSet:[%s]", txRWSetString)
			} else {
				logger.Debugf("validating txRWSet:[%s...]", txRWSetString[0:2000])
			}
		}

		if !doMVCCValidation {
			valid = true
		} else if valid, err = v.validateTx(txRWSet, updates); err != nil {
			return nil, nil, nil, err
		}
		//TODO add the validation info to the bitmap in the metadata of the block
		if valid {
			committingTxHeight := version.NewHeight(block.Header.Number, uint64(txIndex+1))
			addWriteSetToBatch(txRWSet, committingTxHeight, updates)
		} else {
			invalidTxs = append(invalidTxs, &pb.InvalidTransaction{
				Transaction: &pb.Transaction{ /* FIXME */ }, Cause: pb.InvalidTransaction_RWConflictDuringCommit})
		}
	}
	return block, invalidTxs, updates, nil
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
		for _, kvRead := range nsRWSet.Reads {
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
		}
	}
	return true, nil
}
