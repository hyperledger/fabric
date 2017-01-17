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
	"fmt"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/cauthdsl"
	coreUtil "github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/core/common/ccprovider"
	"github.com/hyperledger/fabric/core/common/validation"
	"github.com/hyperledger/fabric/core/ledger"
	ledgerUtil "github.com/hyperledger/fabric/core/ledger/util"
	"github.com/hyperledger/fabric/core/peer/msp"
	"github.com/hyperledger/fabric/msp"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/op/go-logging"
	"github.com/syndtr/goleveldb/leveldb/errors"
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
	ledger     ledger.PeerLedger
	ccprovider ccprovider.ChaincodeProvider
}

// implementation of Validator interface, keeps
// reference to the ledger to enable tx simulation
// and execution of vscc
type txValidator struct {
	ledger ledger.PeerLedger
	vscc   vsccValidator
}

var logger *logging.Logger // package-level logger

func init() {
	// Init logger with module name
	logger = logging.MustGetLogger("txvalidator")
}

// NewTxValidator creates new transactions validator
func NewTxValidator(ledger ledger.PeerLedger) Validator {
	// Encapsulates interface implementation
	return &txValidator{ledger, &vsccValidatorImpl{ledger: ledger, ccprovider: ccprovider.GetChaincodeProvider()}}
}

func (v *txValidator) chainExists(chain string) bool {
	// TODO: implement this function!
	return true
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
				var payload *common.Payload
				var err error
				if payload, err = validation.ValidateTransaction(env); err != nil {
					logger.Errorf("Invalid transaction with index %d, error %s", tIdx, err)
					continue
				}

				chain := payload.Header.ChainHeader.ChainID
				logger.Debug("Transaction is for chain %s", chain)

				if !v.chainExists(chain) {
					logger.Errorf("Dropping transaction for non-existent chain %s", chain)
					continue
				}

				if common.HeaderType(payload.Header.ChainHeader.Type) == common.HeaderType_ENDORSER_TRANSACTION {
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
				} else if common.HeaderType(payload.Header.ChainHeader.Type) == common.HeaderType_CONFIGURATION_TRANSACTION {
					// TODO: here we should call CSCC and pass it the config tx
					// note that there is quite a bit more validation necessary
					// on this tx, namely, validation that each config item has
					// signature matching the policy required for the item from
					// the existing configuration; this is taken care of nicely
					// by configtx.Manager (see fabric/common/configtx).
					logger.Debug("config transaction received for chain %s", chain)
				}

				if _, err := proto.Marshal(env); err != nil {
					logger.Warningf("Cannot marshal transaction due to %s", err)
					continue
				}
				// Succeeded to pass down here, transaction is valid,
				// just unset the filter bit array flag.
				txsfltr.Unset(uint(tIdx))
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

// getHardcodedPolicy returns a policy that requests
// one valid signature from the first MSP in this
// chain's MSP manager
// FIXME: this needs to be removed as soon as we extract the policy from LCCC
func getHardcodedPolicy(chainID string) ([]byte, error) {
	// 1) determine the MSP identifier for the first MSP in this chain
	var msp msp.MSP
	mspMgr := mspmgmt.GetManagerForChain(chainID)
	msps, err := mspMgr.GetMSPs()
	if err != nil {
		return nil, fmt.Errorf("Could not retrieve the MSPs for the chain manager, err %s", err)
	}
	if len(msps) == 0 {
		return nil, errors.New("At least one MSP was expected")
	}
	for _, m := range msps {
		msp = m
		break
	}
	mspid, err := msp.GetIdentifier()
	if err != nil {
		return nil, fmt.Errorf("Failure getting the msp identifier, err %s", err)
	}

	// 2) get the policy
	p := cauthdsl.SignedByMspMember(mspid)

	// 3) marshal it and return it
	b, err := proto.Marshal(p)
	if err != nil {
		return nil, fmt.Errorf("Could not marshal policy, err %s", err)
	}

	return b, err
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

	// TODO: temporary workaround until the policy is specified
	// by the deployer and can be retrieved via LCCC: we create
	// a policy that requests 1 valid signature from this chain's
	// MSP
	policy, err := getHardcodedPolicy(chainID)
	if err != nil {
		return err
	}

	// build arguments for VSCC invocation
	// args[0] - function name (not used now)
	// args[1] - serialized Envelope
	// args[2] - serialized policy
	args := [][]byte{[]byte(""), envBytes, policy}

	ctxt, err := v.ccprovider.GetContext(v.ledger)
	if err != nil {
		logger.Errorf("Cannot obtain context for txid=%s, err %s", txid, err)
		return err
	}
	defer v.ccprovider.ReleaseContext()

	// get header extensions so we have the visibility field
	hdrExt, err := utils.GetChaincodeHeaderExtension(payload.Header)
	if err != nil {
		return err
	}

	// TODO: Temporary solution until FAB-1422 get resolved
	// Explanation: we actually deploying chaincode transaction,
	// hence no lccc yet to query for the data, therefore currently
	// introducing a workaround to skip obtaining LCCC data.
	vscc := "vscc"
	if hdrExt.ChaincodeID.Name != "lccc" {
		// Extracting vscc from lccc
		// TODO: extract policy as well when available; it's the second argument returned by GetCCValidationInfoFromLCCC
		vscc, _, err = v.ccprovider.GetCCValidationInfoFromLCCC(ctxt, txid, nil, chainID, hdrExt.ChaincodeID.Name)
		if err != nil {
			logger.Errorf("Unable to get chaincode data from LCCC for txid %s, due to %s", txid, err)
			return err
		}
	}

	vscctxid := coreUtil.GenerateUUID()
	// Get chaincode version
	version := coreUtil.GetSysCCVersion()
	cccid := v.ccprovider.GetCCContext(chainID, vscc, version, vscctxid, true, nil)

	// invoke VSCC
	logger.Info("Invoking VSCC txid", txid, "chaindID", chainID)
	_, _, err = v.ccprovider.ExecuteChaincode(ctxt, cccid, args)
	if err != nil {
		logger.Errorf("VSCC check failed for transaction txid=%s, error %s", txid, err)
		return err
	}

	return nil
}
