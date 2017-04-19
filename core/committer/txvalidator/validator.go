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
	"github.com/hyperledger/fabric/common/flogging"
	coreUtil "github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/core/chaincode/shim"
	"github.com/hyperledger/fabric/core/common/ccprovider"
	"github.com/hyperledger/fabric/core/common/validation"
	"github.com/hyperledger/fabric/core/ledger"
	ledgerUtil "github.com/hyperledger/fabric/core/ledger/util"
	"github.com/hyperledger/fabric/msp"

	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/peer"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/op/go-logging"

	"github.com/hyperledger/fabric/common/cauthdsl"
)

// Support provides all of the needed to evaluate the VSCC
type Support interface {
	// Ledger returns the ledger associated with this validator
	Ledger() ledger.PeerLedger

	// MSPManager returns the MSP manager for this chain
	MSPManager() msp.MSPManager

	// Apply attempts to apply a configtx to become the new config
	Apply(configtx *common.ConfigEnvelope) error

	// PolicyManager returns the policies.Manager for the channel
	PolicyManager() policies.Manager

	// GetMSPIDs returns the IDs for the application MSPs
	// that have been defined in the channel
	GetMSPIDs(cid string) []string
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
	VSCCValidateTx(payload *common.Payload, envBytes []byte, env *common.Envelope) error
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
	logger = flogging.MustGetLogger("txvalidator")
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

// ChaincodeInstance is unique identifier of chaincode instance
type ChaincodeInstance struct {
	ChainID          string
	ChaincodeName    string
	ChaincodeVersion string
}

func (v *txValidator) Validate(block *common.Block) error {
	logger.Debug("START Block Validation")
	defer logger.Debug("END Block Validation")
	// Initialize trans as valid here, then set invalidation reason code upon invalidation below
	txsfltr := ledgerUtil.NewTxValidationFlags(len(block.Data.Data))
	// txsChaincodeNames records all the invoked chaincodes by tx in a block
	txsChaincodeNames := make(map[int]*ChaincodeInstance)
	// upgradedChaincodes records all the chaincodes that are upgrded in a block
	txsUpgradedChaincodes := make(map[int]*ChaincodeInstance)
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
					if err = v.vscc.VSCCValidateTx(payload, d, env); err != nil {
						txID := txID
						logger.Errorf("VSCCValidateTx for transaction txId = %s returned error %s", txID, err)
						txsfltr.SetFlag(tIdx, peer.TxValidationCode_ENDORSEMENT_POLICY_FAILURE)
						continue
					}

					invokeCC, upgradeCC, err := v.getTxCCInstance(payload)
					if err != nil {
						logger.Errorf("VSCCValidateTx for transaction txId = %s returned error %s", txID, err)
						txsfltr.SetFlag(tIdx, peer.TxValidationCode_ENDORSEMENT_POLICY_FAILURE)
						continue
					}
					txsChaincodeNames[tIdx] = invokeCC
					if upgradeCC != nil {
						logger.Infof("Find chaincode upgrade transaction for chaincode %s on chain %s with new version %s", upgradeCC.ChaincodeName, upgradeCC.ChainID, upgradeCC.ChaincodeVersion)
						txsUpgradedChaincodes[tIdx] = upgradeCC
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

	txsfltr = v.invalidTXsForUpgradeCC(txsChaincodeNames, txsUpgradedChaincodes, txsfltr)

	// Initialize metadata structure
	utils.InitBlockMetadata(block)

	block.Metadata.Metadata[common.BlockMetadataIndex_TRANSACTIONS_FILTER] = txsfltr

	return nil
}

// generateCCKey generates a unique identifier for chaincode in specific chain
func (v *txValidator) generateCCKey(ccName, chainID string) string {
	return fmt.Sprintf("%s/%s", ccName, chainID)
}

// invalidTXsForUpgradeCC invalid all txs that should be invalided because of chaincode upgrade txs
func (v *txValidator) invalidTXsForUpgradeCC(txsChaincodeNames map[int]*ChaincodeInstance, txsUpgradedChaincodes map[int]*ChaincodeInstance, txsfltr ledgerUtil.TxValidationFlags) ledgerUtil.TxValidationFlags {
	if len(txsUpgradedChaincodes) == 0 {
		return txsfltr
	}

	// Invalid former cc upgrade txs if there're two or more txs upgrade the same cc
	finalValidUpgradeTXs := make(map[string]int)
	upgradedChaincodes := make(map[string]*ChaincodeInstance)
	for tIdx, cc := range txsUpgradedChaincodes {
		if cc == nil {
			continue
		}
		upgradedCCKey := v.generateCCKey(cc.ChaincodeName, cc.ChainID)

		if finalIdx, exist := finalValidUpgradeTXs[upgradedCCKey]; !exist {
			finalValidUpgradeTXs[upgradedCCKey] = tIdx
			upgradedChaincodes[upgradedCCKey] = cc
		} else if finalIdx < tIdx {
			logger.Infof("Invalid transaction with index %d: chaincode was upgraded by latter tx", finalIdx)
			txsfltr.SetFlag(finalIdx, peer.TxValidationCode_EXPIRED_CHAINCODE)

			// record latter cc upgrade tx info
			finalValidUpgradeTXs[upgradedCCKey] = tIdx
			upgradedChaincodes[upgradedCCKey] = cc
		} else {
			logger.Infof("Invalid transaction with index %d: chaincode was upgraded by latter tx", tIdx)
			txsfltr.SetFlag(tIdx, peer.TxValidationCode_EXPIRED_CHAINCODE)
		}
	}

	// invalid txs which invoke the upgraded chaincodes
	for tIdx, cc := range txsChaincodeNames {
		if cc == nil {
			continue
		}
		ccKey := v.generateCCKey(cc.ChaincodeName, cc.ChainID)
		if _, exist := upgradedChaincodes[ccKey]; exist {
			if txsfltr.IsValid(tIdx) {
				logger.Infof("Invalid transaction with index %d: chaincode was upgraded in the same block", tIdx)
				txsfltr.SetFlag(tIdx, peer.TxValidationCode_EXPIRED_CHAINCODE)
			}
		}
	}

	return txsfltr
}

func (v *txValidator) getTxCCInstance(payload *common.Payload) (invokeCCIns, upgradeCCIns *ChaincodeInstance, err error) {
	// This is duplicated unpacking work, but make test easier.
	chdr, err := utils.UnmarshalChannelHeader(payload.Header.ChannelHeader)
	if err != nil {
		return nil, nil, err
	}

	// Chain ID
	chainID := chdr.ChannelId
	if chainID == "" {
		err := fmt.Errorf("transaction header does not contain an chain ID")
		logger.Errorf("%s", err)
		return nil, nil, err
	}

	// ChaincodeID
	hdrExt, err := utils.GetChaincodeHeaderExtension(payload.Header)
	if err != nil {
		return nil, nil, err
	}
	invokeCC := hdrExt.ChaincodeId
	invokeIns := &ChaincodeInstance{ChainID: chainID, ChaincodeName: invokeCC.Name, ChaincodeVersion: invokeCC.Version}

	// Transaction
	tx, err := utils.GetTransaction(payload.Data)
	if err != nil {
		logger.Errorf("GetTransaction failed: %s", err)
		return invokeIns, nil, nil
	}

	// ChaincodeActionPayload
	cap, err := utils.GetChaincodeActionPayload(tx.Actions[0].Payload)
	if err != nil {
		logger.Errorf("GetChaincodeActionPayload failed: %s", err)
		return invokeIns, nil, nil
	}

	// ChaincodeProposalPayload
	cpp, err := utils.GetChaincodeProposalPayload(cap.ChaincodeProposalPayload)
	if err != nil {
		logger.Errorf("GetChaincodeProposalPayload failed: %s", err)
		return invokeIns, nil, nil
	}

	// ChaincodeInvocationSpec
	cis := &peer.ChaincodeInvocationSpec{}
	err = proto.Unmarshal(cpp.Input, cis)
	if err != nil {
		logger.Errorf("GetChaincodeInvokeSpec failed: %s", err)
		return invokeIns, nil, nil
	}

	if invokeCC.Name == "lscc" {
		if string(cis.ChaincodeSpec.Input.Args[0]) == "upgrade" {
			upgradeIns, err := v.getUpgradeTxInstance(chainID, cis.ChaincodeSpec.Input.Args[2])
			if err != nil {
				return invokeIns, nil, nil
			}
			return invokeIns, upgradeIns, nil
		}
	}

	return invokeIns, nil, nil
}

func (v *txValidator) getUpgradeTxInstance(chainID string, cdsBytes []byte) (*ChaincodeInstance, error) {
	cds, err := utils.GetChaincodeDeploymentSpec(cdsBytes)
	if err != nil {
		return nil, err
	}

	return &ChaincodeInstance{
		ChainID:          chainID,
		ChaincodeName:    cds.ChaincodeSpec.ChaincodeId.Name,
		ChaincodeVersion: cds.ChaincodeSpec.ChaincodeId.Version,
	}, nil
}

func (v *vsccValidatorImpl) VSCCValidateTx(payload *common.Payload, envBytes []byte, env *common.Envelope) error {
	// get channel header
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

	var vscc string
	var policy []byte
	if hdrExt.ChaincodeId.Name != "lscc" {
		// when we are validating any chaincode other than
		// LSCC, we need to ask LSCC to give us the name
		// of VSCC and of the policy that should be used

		// obtain name of the VSCC and the policy from LSCC
		cd, err := v.getCDataForCC(hdrExt.ChaincodeId.Name)
		if err != nil {
			logger.Errorf("Unable to get chaincode data from ledger for txid %s, due to %s", txid, err)
			return err
		}
		vscc = cd.Vscc
		policy = cd.Policy
	} else {
		// when we are validating LSCC, we use the default
		// VSCC and a default policy that requires one signature
		// from any of the members of the channel
		vscc = "vscc"
		policy = cauthdsl.SignedByAnyMember(v.support.GetMSPIDs(chainID))
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

func (v *vsccValidatorImpl) getCDataForCC(ccid string) (*ccprovider.ChaincodeData, error) {
	l := v.support.Ledger()
	if l == nil {
		return nil, fmt.Errorf("nil ledger instance")
	}

	qe, err := l.NewQueryExecutor()
	if err != nil {
		return nil, fmt.Errorf("Could not retrieve QueryExecutor, error %s", err)
	}
	defer qe.Done()

	bytes, err := qe.GetState("lscc", ccid)
	if err != nil {
		return nil, fmt.Errorf("Could not retrieve state for chaincode %s, error %s", ccid, err)
	}

	if bytes == nil {
		return nil, fmt.Errorf("lscc's state for [%s] not found.", ccid)
	}

	cd := &ccprovider.ChaincodeData{}
	err = proto.Unmarshal(bytes, cd)
	if err != nil {
		return nil, fmt.Errorf("Unmarshalling ChaincodeQueryResponse failed, error %s", err)
	}

	if cd.Vscc == "" {
		return nil, fmt.Errorf("lscc's state for [%s] is invalid, vscc field must be set.", ccid)
	}

	if len(cd.Policy) == 0 {
		return nil, fmt.Errorf("lscc's state for [%s] is invalid, policy field must be set.", ccid)
	}

	return cd, err
}
