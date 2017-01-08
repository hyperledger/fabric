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

package txvalidator

import (
	"context"
	"fmt"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/core/chaincode"
	"github.com/hyperledger/fabric/core/ledger"
	ledgerUtil "github.com/hyperledger/fabric/core/ledger/util"
	"github.com/hyperledger/fabric/core/peer"
	coreUtil "github.com/hyperledger/fabric/core/util"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/op/go-logging"
)

//Validator interface which defines API to validate block transactions
// and return the bit array mask indicating invalid transactions which
// didn't pass validation.
type Validator interface {
	Validate(block *common.Block)
}

// private interface to decouple tx validator
// and vscc execution, in order to increase
// testability of txValidator
type vsccValidator interface {
	VSCCValidateTx(payload *common.Payload, envBytes []byte) error
}

// vsccValidator implementation which used to call
// vscc chaincode and validate block transactions
type vsccValidatorImpl struct {
	ledger ledger.ValidatedLedger
}

// implementation of Validator interface, keeps
// reference to the ledger to enable tx simulation
// and execution of vscc
type txValidator struct {
	ledger ledger.ValidatedLedger
	vscc   vsccValidator
}

var logger *logging.Logger // package-level logger

func init() {
	// Init logger with module name
	logger = logging.MustGetLogger("txvalidator")
}

// NewTxValidator creates new transactions validator
func NewTxValidator(ledger ledger.ValidatedLedger) Validator {
	// Encapsulates interface implementation
	return &txValidator{ledger, &vsccValidatorImpl{ledger}}
}

func (v *txValidator) Validate(block *common.Block) {
	logger.Debug("START Block Validation")
	defer logger.Debug("END Block Validation")
	txsfltr := ledgerUtil.NewFilterBitArray(uint(len(block.Data.Data)))
	for tIdx, d := range block.Data.Data {
		// Start by marking transaction as invalid, before
		// doing any validation checks.
		txsfltr.Set(uint(tIdx))
		if d != nil {
			if env, err := utils.GetEnvelopeFromBlock(d); err != nil {
				logger.Warningf("Error getting tx from block(%s)", err)
			} else if env != nil {
				// validate the transaction: here we check that the transaction
				// is properly formed, properly signed and that the security
				// chain binding proposal to endorsements to tx holds. We do
				// NOT check the validity of endorsements, though. That's a
				// job for VSCC below
				logger.Debug("Validating transaction peer.ValidateTransaction()")
				if payload, _, err := peer.ValidateTransaction(env); err != nil {
					logger.Errorf("Invalid transaction with index %d, error %s", tIdx, err)
				} else {
					// Check duplicate transactions
					txID := payload.Header.ChainHeader.TxID
					if _, err := v.ledger.GetTransactionByID(txID); err == nil {
						logger.Warning("Duplicate transaction found, ", txID, ", skipping")
						continue
					}

					//the payload is used to get headers
					logger.Debug("Validating transaction vscc tx validate")
					if err = v.vscc.VSCCValidateTx(payload, d); err != nil {
						txID := txID
						logger.Errorf("VSCCValidateTx for transaction txId = %s returned error %s", txID, err)
						continue
					}

					if _, err := proto.Marshal(env); err != nil {
						logger.Warningf("Cannot marshal transaction due to %s", err)
						continue
					}
					// Succeeded to pass down here, transaction is valid,
					// just unset the filter bit array flag.
					txsfltr.Unset(uint(tIdx))
				}
			} else {
				logger.Warning("Nil tx from block")
			}
		}
	}
	// Initialize metadata structure
	utils.InitBlockMetadata(block)
	// Serialize invalid transaction bit array into block metadata field
	block.Metadata.Metadata[common.BlockMetadataIndex_TRANSACTIONS_FILTER] = txsfltr.ToBytes()
}

func (v *vsccValidatorImpl) VSCCValidateTx(payload *common.Payload, envBytes []byte) error {
	// Chain ID
	chainID := payload.Header.ChainHeader.ChainID
	if chainID == "" {
		err := fmt.Errorf("transaction header does not contain an chain ID")
		logger.Errorf("%s", err)
		return err
	}

	// Get transaction id
	txid := payload.Header.ChainHeader.TxID
	logger.Info("[XXX remove me XXX] Transaction type,", common.HeaderType(payload.Header.ChainHeader.Type))
	if txid == "" {
		err := fmt.Errorf("transaction header does not contain transaction ID")
		logger.Errorf("%s", err)
		return err
	}

	// build arguments for VSCC invocation
	// args[0] - function name (not used now)
	// args[1] - serialized Envelope
	args := [][]byte{[]byte(""), envBytes}

	// get context for the chaincode execution
	lgr := v.ledger
	txsim, err := lgr.NewTxSimulator()
	if err != nil {
		logger.Errorf("Cannot obtain tx simulator txid=%s, err %s", txid, err)
		return err
	}
	defer txsim.Done()
	ctxt := context.WithValue(context.Background(), chaincode.TXSimulatorKey, txsim)

	// get header extensions so we have the visibility field
	hdrExt, err := utils.GetChaincodeHeaderExtension(payload.Header)
	if err != nil {
		return err
	}

	// TODO: Temporary solution until FAB-1422 get resolved
	// Explanation: we actually deploying chaincode transaction,
	// hence no lccc yet to query for the data, therefore currently
	// introducing a workaround to skip obtaining LCCC data.
	var data *chaincode.ChaincodeData
	if hdrExt.ChaincodeID.Name != "lccc" {
		// Extracting vscc from lccc
		logger.Info("Extracting chaincode data from LCCC txid = ", txid, "chainID", chainID, "chaincode name", hdrExt.ChaincodeID.Name)
		data, err = chaincode.GetChaincodeDataFromLCCC(ctxt, txid, nil, chainID, hdrExt.ChaincodeID.Name)
		if err != nil {
			logger.Errorf("Unable to get chaincode data from LCCC for txid %s, due to %s", txid, err)
			return err
		}
	}

	vscc := "vscc"
	// Check whenever VSCC defined for chaincode data
	if data != nil && data.Vscc != "" {
		vscc = data.Vscc
	}

	vscctxid := coreUtil.GenerateUUID()
	// Get chaincode version
	version := coreUtil.GetSysCCVersion()
	cccid := chaincode.NewCCContext(chainID, vscc, version, vscctxid, true, nil)

	// invoke VSCC
	logger.Info("Invoking VSCC txid", txid, "chaindID", chainID)
	_, _, err = chaincode.ExecuteChaincode(ctxt, cccid, args)
	if err != nil {
		logger.Errorf("VSCC check failed for transaction txid=%s, error %s", txid, err)
		return err
	}

	return nil
}
