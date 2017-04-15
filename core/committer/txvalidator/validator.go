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
	"github.com/hyperledger/fabric/common/configtx"
	coreUtil "github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/core/chaincode/shim"
	"github.com/hyperledger/fabric/core/common/ccprovider"
	"github.com/hyperledger/fabric/core/common/validation"
	"github.com/hyperledger/fabric/core/ledger"
	ledgerUtil "github.com/hyperledger/fabric/core/ledger/util"
	"github.com/hyperledger/fabric/msp"

	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/peer"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/op/go-logging"
)

// Support provides all of the needed to evaluate the VSCC
type Support interface {
	// Ledger returns the ledger associated with this validator
	Ledger() ledger.PeerLedger

	// MSPManager returns the MSP manager for this chain
	MSPManager() msp.MSPManager

	// Apply attempts to apply a configtx to become the new config
	Apply(configtx *common.ConfigEnvelope) error
}

//Validator interface which defines API to validate block transactions
// and return the bit array mask indicating invalid transactions which
// didn't pass validation.
type Validator interface {
	Validate(block *common.Block) error
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
	support    Support
	ccprovider ccprovider.ChaincodeProvider
}

// implementation of Validator interface, keeps
// reference to the ledger to enable tx simulation
// and execution of vscc
type txValidator struct {
	support Support
	vscc    vsccValidator
}

var logger *logging.Logger // package-level logger

func init() {
	// Init logger with module name
	logger = logging.MustGetLogger("txvalidator")
}

// NewTxValidator creates new transactions validator
func NewTxValidator(support Support) Validator {
	// Encapsulates interface implementation
	return &txValidator{support, &vsccValidatorImpl{support: support, ccprovider: ccprovider.GetChaincodeProvider()}}
}

func (v *txValidator) chainExists(chain string) bool {
	// TODO: implement this function!
	return true
}

func (v *txValidator) Validate(block *common.Block) error {
	logger.Debug("START Block Validation")
	defer logger.Debug("END Block Validation")
	// Initialize trans as valid here, then set invalidation reason code upon invalidation below
	txsfltr := ledgerUtil.NewTxValidationFlags(len(block.Data.Data))
	for tIdx, d := range block.Data.Data {
		if d != nil {
			if env, err := utils.GetEnvelopeFromBlock(d); err != nil {
				logger.Warningf("Error getting tx from block(%s)", err)
				txsfltr.SetFlag(tIdx, peer.TxValidationCode_INVALID_OTHER_REASON)
			} else if env != nil {
				// validate the transaction: here we check that the transaction
				// is properly formed, properly signed and that the security
				// chain binding proposal to endorsements to tx holds. We do
				// NOT check the validity of endorsements, though. That's a
				// job for VSCC below
				logger.Debug("Validating transaction peer.ValidateTransaction()")
				var payload *common.Payload
				var err error
				var txResult peer.TxValidationCode

				if payload, txResult = validation.ValidateTransaction(env); txResult != peer.TxValidationCode_VALID {
					logger.Errorf("Invalid transaction with index %d, error %s", tIdx, err)
					txsfltr.SetFlag(tIdx, txResult)
					continue
				}

				chdr, err := utils.UnmarshalChannelHeader(payload.Header.ChannelHeader)
				if err != nil {
					logger.Warning("Could not unmarshal channel header, err %s, skipping", err)
					txsfltr.SetFlag(tIdx, peer.TxValidationCode_INVALID_OTHER_REASON)
					continue
				}

				channel := chdr.ChannelId
				logger.Debug("Transaction is for chain %s", channel)

				if !v.chainExists(channel) {
					logger.Errorf("Dropping transaction for non-existent chain %s", channel)
					txsfltr.SetFlag(tIdx, peer.TxValidationCode_TARGET_CHAIN_NOT_FOUND)
					continue
				}

				if common.HeaderType(chdr.Type) == common.HeaderType_ENDORSER_TRANSACTION {
					// Check duplicate transactions
					txID := chdr.TxId
					if _, err := v.support.Ledger().GetTransactionByID(txID); err == nil {
						logger.Error("Duplicate transaction found, ", txID, ", skipping")
						txsfltr.SetFlag(tIdx, peer.TxValidationCode_DUPLICATE_TXID)
						continue
					}

					//the payload is used to get headers
					logger.Debug("Validating transaction vscc tx validate")
					if err = v.vscc.VSCCValidateTx(payload, d); err != nil {
						txID := txID
						logger.Errorf("VSCCValidateTx for transaction txId = %s returned error %s", txID, err)
						txsfltr.SetFlag(tIdx, peer.TxValidationCode_ENDORSEMENT_POLICY_FAILURE)
						continue
					}
				} else if common.HeaderType(chdr.Type) == common.HeaderType_CONFIG {
					configEnvelope, err := configtx.UnmarshalConfigEnvelope(payload.Data)
					if err != nil {
						err := fmt.Errorf("Error unmarshaling config which passed initial validity checks: %s", err)
						logger.Critical(err)
						return err
					}

					if err := v.support.Apply(configEnvelope); err != nil {
						err := fmt.Errorf("Error validating config which passed initial validity checks: %s", err)
						logger.Critical(err)
						return err
					}
					logger.Debugf("config transaction received for chain %s", channel)
				}

				if _, err := proto.Marshal(env); err != nil {
					logger.Warningf("Cannot marshal transaction due to %s", err)
					txsfltr.SetFlag(tIdx, peer.TxValidationCode_MARSHAL_TX_ERROR)
					continue
				}
				// Succeeded to pass down here, transaction is valid
				txsfltr.SetFlag(tIdx, peer.TxValidationCode_VALID)
			} else {
				logger.Warning("Nil tx from block")
				txsfltr.SetFlag(tIdx, peer.TxValidationCode_NIL_ENVELOPE)
			}
		}
	}
	// Initialize metadata structure
	utils.InitBlockMetadata(block)

	block.Metadata.Metadata[common.BlockMetadataIndex_TRANSACTIONS_FILTER] = txsfltr

	return nil
}

func (v *vsccValidatorImpl) VSCCValidateTx(payload *common.Payload, envBytes []byte) error {
	chdr, err := utils.UnmarshalChannelHeader(payload.Header.ChannelHeader)
	if err != nil {
		return err
	}

	// Chain ID
	chainID := chdr.ChannelId
	if chainID == "" {
		err := fmt.Errorf("transaction header does not contain an chain ID")
		logger.Errorf("%s", err)
		return err
	}

	// Get transaction id
	txid := chdr.TxId
	if txid == "" {
		err := fmt.Errorf("transaction header does not contain transaction ID")
		logger.Errorf("%s", err)
		return err
	}

	ctxt, err := v.ccprovider.GetContext(v.support.Ledger())
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

	// LCCC should not undergo standard VSCC type of
	// validation. It should instead go through system
	// policy validation to determine whether the issuer
	// is entitled to deploy a chaincode on our chain
	// VSCCValidateTx should
	if hdrExt.ChaincodeId.Name == "lccc" {
		// TODO: until FAB-1934 is in, we need to stop here
		logger.Debugf("Invocation of LCCC detected, no further VSCC validation necessary")
		return nil
	}

	// obtain name of the VSCC and the policy from LCCC
	vscc, policy, err := v.ccprovider.GetCCValidationInfoFromLCCC(ctxt, txid, nil, nil, chainID, hdrExt.ChaincodeId.Name)
	if err != nil {
		logger.Errorf("Unable to get chaincode data from LCCC for txid %s, due to %s", txid, err)
		return err
	}

	// build arguments for VSCC invocation
	// args[0] - function name (not used now)
	// args[1] - serialized Envelope
	// args[2] - serialized policy
	args := [][]byte{[]byte(""), envBytes, policy}

	vscctxid := coreUtil.GenerateUUID()

	// Get chaincode version
	version := coreUtil.GetSysCCVersion()
	cccid := v.ccprovider.GetCCContext(chainID, vscc, version, vscctxid, true, nil, nil)

	// invoke VSCC
	logger.Debug("Invoking VSCC txid", txid, "chaindID", chainID)
	res, _, err := v.ccprovider.ExecuteChaincode(ctxt, cccid, args)
	if err != nil {
		logger.Errorf("Invoke VSCC failed for transaction txid=%s, error %s", txid, err)
		return err
	}
	if res.Status != shim.OK {
		logger.Errorf("VSCC check failed for transaction txid=%s, error %s", txid, res.Message)
		return fmt.Errorf("%s", res.Message)
	}

	return nil
}
