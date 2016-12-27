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
	txsfltr := ledgerUtil.NewFilterBitArray(uint(len(block.Data.Data)))
	for tIdx, d := range block.Data.Data {
		if d != nil {
			if env, err := utils.GetEnvelopeFromBlock(d); err != nil {
				logger.Warningf("Error getting tx from block(%s)", err)
			} else if env != nil {
				// validate the transaction: here we check that the transaction
				// is properly formed, properly signed and that the security
				// chain binding proposal to endorsements to tx holds. We do
				// NOT check the validity of endorsements, though. That's a
				// job for VSCC below
				if payload, _, err := peer.ValidateTransaction(env); err != nil {
					// TODO: this code needs to receive a bit more attention and discussion:
					// it's not clear what it means if a transaction which causes a failure
					// in validation is just dropped on the floor
					logger.Errorf("Invalid transaction with index %s, error %s", tIdx, err)
					txsfltr.Set(uint(tIdx))
				} else {
					//the payload is used to get headers
					if err = v.vscc.VSCCValidateTx(payload, d); err != nil {
						// TODO: this code needs to receive a bit more attention and discussion:
						// it's not clear what it means if a transaction which causes a failure
						// in validation is just dropped on the floor
						txID := payload.Header.ChainHeader.TxID
						logger.Errorf("isTxValidForVscc for transaction txId = %s returned error %s", txID, err)
						txsfltr.Set(uint(tIdx))
						continue
					}

					if t, err := proto.Marshal(env); err == nil {
						block.Data.Data = append(block.Data.Data, t)
					} else {
						logger.Warningf("Cannot marshal transactoins %s", err)
						txsfltr.Set(uint(tIdx))
					}
				}
			} else {
				logger.Warning("Nil tx from block")
				txsfltr.Set(uint(tIdx))
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

	// Extracting vscc from lccc
	/*
		data, err := chaincode.GetChaincodeDataFromLCCC(ctxt, txid, nil, chainID, "vscc")
		if err != nil {
			logger.Errorf("Unable to get chaincode data from LCCC for txid %s, due to %s", txid, err)
			return err
		}
	*/

	// Get chaincode version
	version := coreUtil.GetSysCCVersion()
	cccid := chaincode.NewCCContext(chainID, "vscc", version, txid, true, nil)

	// invoke VSCC
	_, _, err = chaincode.ExecuteChaincode(ctxt, cccid, args)
	if err != nil {
		logger.Errorf("VSCC check failed for transaction txid=%s, error %s", txid, err)
		return err
	}

	return nil
}
