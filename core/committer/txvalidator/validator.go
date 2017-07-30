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
	"github.com/hyperledger/fabric/core/common/sysccprovider"
	"github.com/hyperledger/fabric/core/common/validation"
	"github.com/hyperledger/fabric/core/ledger"
	ledgerUtil "github.com/hyperledger/fabric/core/ledger/util"
	"github.com/hyperledger/fabric/msp"

	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/peer"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/op/go-logging"

	"github.com/hyperledger/fabric/common/cauthdsl"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/rwsetutil"
)

// Support provides all of the needed to evaluate the VSCC
type Support interface {
	// Ledger returns the ledger associated with this validator
	Ledger() ledger.PeerLedger

	// MSPManager returns the MSP manager for this chain
	MSPManager() msp.MSPManager

	// Apply attempts to apply a configtx to become the new config
	Apply(configtx *common.ConfigEnvelope) error

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
	VSCCValidateTx(payload *common.Payload, envBytes []byte, env *common.Envelope) (error, peer.TxValidationCode)
}

// vsccValidator implementation which used to call
// vscc chaincode and validate block transactions
type vsccValidatorImpl struct {
	support     Support
	ccprovider  ccprovider.ChaincodeProvider
	sccprovider sysccprovider.SystemChaincodeProvider
}

// implementation of Validator interface, keeps
// reference to the ledger to enable tx simulation
// and execution of vscc
type txValidator struct {
	support Support
	vscc    vsccValidator
}

// VSCCInfoLookupFailureError error to indicate inability
// to obtain VSCC information from LCCC
type VSCCInfoLookupFailureError struct {
	reason string
}

// Error returns reasons which lead to the failure
func (e VSCCInfoLookupFailureError) Error() string {
	return e.reason
}

// VSCCEndorsementPolicyError error to mark transaction
// failed endrosement policy check
type VSCCEndorsementPolicyError struct {
	reason string
}

// Error returns reasons which lead to the failure
func (e VSCCEndorsementPolicyError) Error() string {
	return e.reason
}

// VSCCExecutionFailureError error to indicate
// failure during attempt of executing VSCC
// endorsement policy check
type VSCCExecutionFailureError struct {
	reason string
}

// Error returns reasons which lead to the failure
func (e VSCCExecutionFailureError) Error() string {
	return e.reason
}

var logger *logging.Logger // package-level logger

func init() {
	// Init logger with module name
	logger = flogging.MustGetLogger("txvalidator")
}

// NewTxValidator creates new transactions validator
func NewTxValidator(support Support) Validator {
	// Encapsulates interface implementation
	return &txValidator{support,
		&vsccValidatorImpl{
			support:     support,
			ccprovider:  ccprovider.GetChaincodeProvider(),
			sccprovider: sysccprovider.GetSystemChaincodeProvider()}}
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
	// txsChaincodeNames records all the invoked chaincodes by tx in a block
	txsChaincodeNames := make(map[int]*sysccprovider.ChaincodeInstance)
	// upgradedChaincodes records all the chaincodes that are upgrded in a block
	txsUpgradedChaincodes := make(map[int]*sysccprovider.ChaincodeInstance)
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
					logger.Errorf("Invalid transaction with index %d", tIdx)
					txsfltr.SetFlag(tIdx, txResult)
					continue
				}

				chdr, err := utils.UnmarshalChannelHeader(payload.Header.ChannelHeader)
				if err != nil {
					logger.Warningf("Could not unmarshal channel header, err %s, skipping", err)
					txsfltr.SetFlag(tIdx, peer.TxValidationCode_INVALID_OTHER_REASON)
					continue
				}

				channel := chdr.ChannelId
				logger.Debugf("Transaction is for chain %s", channel)

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

					// Validate tx with vscc and policy
					logger.Debug("Validating transaction vscc tx validate")
					err, cde := v.vscc.VSCCValidateTx(payload, d, env)
					if err != nil {
						txID := txID
						logger.Errorf("VSCCValidateTx for transaction txId = %s returned error %s", txID, err)
						switch err.(type) {
						case *VSCCExecutionFailureError:
							return err
						case *VSCCInfoLookupFailureError:
							return err
						default:
							txsfltr.SetFlag(tIdx, cde)
							continue
						}
					}

					invokeCC, upgradeCC, err := v.getTxCCInstance(payload)
					if err != nil {
						logger.Errorf("Get chaincode instance from transaction txId = %s returned error %s", txID, err)
						txsfltr.SetFlag(tIdx, peer.TxValidationCode_INVALID_OTHER_REASON)
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
				} else {
					logger.Warningf("Unknown transaction type [%s] in block number [%d] transaction index [%d]",
						common.HeaderType(chdr.Type), block.Header.Number, tIdx)
					txsfltr.SetFlag(tIdx, peer.TxValidationCode_UNKNOWN_TX_TYPE)
					continue
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
func (v *txValidator) invalidTXsForUpgradeCC(txsChaincodeNames map[int]*sysccprovider.ChaincodeInstance, txsUpgradedChaincodes map[int]*sysccprovider.ChaincodeInstance, txsfltr ledgerUtil.TxValidationFlags) ledgerUtil.TxValidationFlags {
	if len(txsUpgradedChaincodes) == 0 {
		return txsfltr
	}

	// Invalid former cc upgrade txs if there're two or more txs upgrade the same cc
	finalValidUpgradeTXs := make(map[string]int)
	upgradedChaincodes := make(map[string]*sysccprovider.ChaincodeInstance)
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
			txsfltr.SetFlag(finalIdx, peer.TxValidationCode_CHAINCODE_VERSION_CONFLICT)

			// record latter cc upgrade tx info
			finalValidUpgradeTXs[upgradedCCKey] = tIdx
			upgradedChaincodes[upgradedCCKey] = cc
		} else {
			logger.Infof("Invalid transaction with index %d: chaincode was upgraded by latter tx", tIdx)
			txsfltr.SetFlag(tIdx, peer.TxValidationCode_CHAINCODE_VERSION_CONFLICT)
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
				txsfltr.SetFlag(tIdx, peer.TxValidationCode_CHAINCODE_VERSION_CONFLICT)
			}
		}
	}

	return txsfltr
}

func (v *txValidator) getTxCCInstance(payload *common.Payload) (invokeCCIns, upgradeCCIns *sysccprovider.ChaincodeInstance, err error) {
	// This is duplicated unpacking work, but make test easier.
	chdr, err := utils.UnmarshalChannelHeader(payload.Header.ChannelHeader)
	if err != nil {
		return nil, nil, err
	}

	// Chain ID
	chainID := chdr.ChannelId // it is guaranteed to be an existing channel by now

	// ChaincodeID
	hdrExt, err := utils.GetChaincodeHeaderExtension(payload.Header)
	if err != nil {
		return nil, nil, err
	}
	invokeCC := hdrExt.ChaincodeId
	invokeIns := &sysccprovider.ChaincodeInstance{ChainID: chainID, ChaincodeName: invokeCC.Name, ChaincodeVersion: invokeCC.Version}

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

func (v *txValidator) getUpgradeTxInstance(chainID string, cdsBytes []byte) (*sysccprovider.ChaincodeInstance, error) {
	cds, err := utils.GetChaincodeDeploymentSpec(cdsBytes)
	if err != nil {
		return nil, err
	}

	return &sysccprovider.ChaincodeInstance{
		ChainID:          chainID,
		ChaincodeName:    cds.ChaincodeSpec.ChaincodeId.Name,
		ChaincodeVersion: cds.ChaincodeSpec.ChaincodeId.Version,
	}, nil
}

// GetInfoForValidate gets the ChaincodeInstance(with latest version) of tx, vscc and policy from lscc
func (v *vsccValidatorImpl) GetInfoForValidate(txid, chID, ccID string) (*sysccprovider.ChaincodeInstance, *sysccprovider.ChaincodeInstance, []byte, error) {
	cc := &sysccprovider.ChaincodeInstance{ChainID: chID}
	vscc := &sysccprovider.ChaincodeInstance{ChainID: chID}
	var policy []byte
	var err error
	if ccID != "lscc" {
		// when we are validating any chaincode other than
		// LSCC, we need to ask LSCC to give us the name
		// of VSCC and of the policy that should be used

		// obtain name of the VSCC and the policy from LSCC
		cd, err := v.getCDataForCC(ccID)
		if err != nil {
			msg := fmt.Sprintf("Unable to get chaincode data from ledger for txid %s, due to %s", txid, err)
			logger.Errorf(msg)
			return nil, nil, nil, err
		}
		cc.ChaincodeName = cd.Name
		cc.ChaincodeVersion = cd.Version
		vscc.ChaincodeName = cd.Vscc
		policy = cd.Policy
	} else {
		// when we are validating LSCC, we use the default
		// VSCC and a default policy that requires one signature
		// from any of the members of the channel
		cc.ChaincodeName = "lscc"
		cc.ChaincodeVersion = coreUtil.GetSysCCVersion()
		vscc.ChaincodeName = "vscc"
		p := cauthdsl.SignedByAnyMember(v.support.GetMSPIDs(chID))
		policy, err = utils.Marshal(p)
		if err != nil {
			return nil, nil, nil, err
		}
	}

	// Get vscc version
	vscc.ChaincodeVersion = coreUtil.GetSysCCVersion()

	return cc, vscc, policy, nil
}

func (v *vsccValidatorImpl) VSCCValidateTx(payload *common.Payload, envBytes []byte, env *common.Envelope) (error, peer.TxValidationCode) {
	// get header extensions so we have the chaincode ID
	hdrExt, err := utils.GetChaincodeHeaderExtension(payload.Header)
	if err != nil {
		return err, peer.TxValidationCode_BAD_HEADER_EXTENSION
	}

	// get channel header
	chdr, err := utils.UnmarshalChannelHeader(payload.Header.ChannelHeader)
	if err != nil {
		return err, peer.TxValidationCode_BAD_CHANNEL_HEADER
	}

	/* obtain the list of namespaces we're writing stuff to;
	   at first, we establish a few facts about this invocation:
	   1) which namespaces does it write to?
	   2) does it write to LSCC's namespace?
	   3) does it write to any cc that cannot be invoked? */
	wrNamespace := []string{}
	writesToLSCC := false
	writesToNonInvokableSCC := false
	respPayload, err := utils.GetActionFromEnvelope(envBytes)
	if err != nil {
		return fmt.Errorf("GetActionFromEnvelope failed, error %s", err), peer.TxValidationCode_BAD_RESPONSE_PAYLOAD
	}
	txRWSet := &rwsetutil.TxRwSet{}
	if err = txRWSet.FromProtoBytes(respPayload.Results); err != nil {
		return fmt.Errorf("txRWSet.FromProtoBytes failed, error %s", err), peer.TxValidationCode_BAD_RWSET
	}
	for _, ns := range txRWSet.NsRwSets {
		if len(ns.KvRwSet.Writes) > 0 {
			wrNamespace = append(wrNamespace, ns.NameSpace)

			if !writesToLSCC && ns.NameSpace == "lscc" {
				writesToLSCC = true
			}

			if !writesToNonInvokableSCC && v.sccprovider.IsSysCCAndNotInvokableCC2CC(ns.NameSpace) {
				writesToNonInvokableSCC = true
			}

			if !writesToNonInvokableSCC && v.sccprovider.IsSysCCAndNotInvokableExternal(ns.NameSpace) {
				writesToNonInvokableSCC = true
			}
		}
	}

	// get name and version of the cc we invoked
	ccID := hdrExt.ChaincodeId.Name
	ccVer := respPayload.ChaincodeId.Version

	// sanity check on ccID
	if ccID == "" {
		err := fmt.Errorf("invalid chaincode ID")
		logger.Errorf("%s", err)
		return err, peer.TxValidationCode_INVALID_OTHER_REASON
	}
	if ccID != respPayload.ChaincodeId.Name {
		err := fmt.Errorf("inconsistent ccid info (%s/%s)", ccID, respPayload.ChaincodeId.Name)
		logger.Errorf("%s", err)
		return err, peer.TxValidationCode_INVALID_OTHER_REASON
	}
	// sanity check on ccver
	if ccVer == "" {
		err := fmt.Errorf("invalid chaincode version")
		logger.Errorf("%s", err)
		return err, peer.TxValidationCode_INVALID_OTHER_REASON
	}

	// we've gathered all the info required to proceed to validation;
	// validation will behave differently depending on the type of
	// chaincode (system vs. application)

	if !v.sccprovider.IsSysCC(ccID) {
		// if we're here, we know this is an invocation of an application chaincode;
		// first of all, we make sure that:
		// 1) we don't write to LSCC - an application chaincode is free to invoke LSCC
		//    for instance to get information about itself or another chaincode; however
		//    these legitimate invocations only ready from LSCC's namespace; currently
		//    only two functions of LSCC write to its namespace: deploy and upgrade and
		//    neither should be used by an application chaincode
		if writesToLSCC {
			return fmt.Errorf("Chaincode %s attempted to write to the namespace of LSCC", ccID),
				peer.TxValidationCode_ILLEGAL_WRITESET
		}
		// 2) we don't write to the namespace of a chaincode that we cannot invoke - if
		//    the chaincode cannot be invoked in the first place, there's no legitimate
		//    way in which a transaction has a write set that writes to it; additionally
		//    we don't have any means of verifying whether the transaction had the rights
		//    to perform that write operation because in v1, system chaincodes do not have
		//    any endorsement policies to speak of. So if the chaincode can't be invoked
		//    it can't be written to by an invocation of an application chaincode
		if writesToNonInvokableSCC {
			return fmt.Errorf("Chaincode %s attempted to write to the namespace of a system chaincode that cannot be invoked", ccID),
				peer.TxValidationCode_ILLEGAL_WRITESET
		}

		// validate *EACH* read write set according to its chaincode's endorsement policy
		for _, ns := range wrNamespace {
			// Get latest chaincode version, vscc and validate policy
			txcc, vscc, policy, err := v.GetInfoForValidate(chdr.TxId, chdr.ChannelId, ns)
			if err != nil {
				logger.Errorf("GetInfoForValidate for txId = %s returned error %s", chdr.TxId, err)
				return err, peer.TxValidationCode_INVALID_OTHER_REASON
			}

			// if the namespace corresponds to the cc that was originally
			// invoked, we check that the version of the cc that was
			// invoked corresponds to the version that lscc has returned
			if ns == ccID && txcc.ChaincodeVersion != ccVer {
				err := fmt.Errorf("Chaincode %s:%s/%s didn't match %s:%s/%s in lscc", ccID, ccVer, chdr.ChannelId, txcc.ChaincodeName, txcc.ChaincodeVersion, chdr.ChannelId)
				logger.Errorf(err.Error())
				return err, peer.TxValidationCode_EXPIRED_CHAINCODE
			}

			// do VSCC validation
			if err = v.VSCCValidateTxForCC(envBytes, chdr.TxId, chdr.ChannelId, vscc.ChaincodeName, vscc.ChaincodeVersion, policy); err != nil {
				switch err.(type) {
				case *VSCCEndorsementPolicyError:
					return err, peer.TxValidationCode_ENDORSEMENT_POLICY_FAILURE
				default:
					return err, peer.TxValidationCode_INVALID_OTHER_REASON
				}
			}
		}
	} else {
		// make sure that we can invoke this system chaincode - if the chaincode
		// cannot be invoked through a proposal to this peer, we have to drop the
		// transaction; if we didn't, we wouldn't know how to decide whether it's
		// valid or not because in v1, system chaincodes have no endorsement policy
		if v.sccprovider.IsSysCCAndNotInvokableExternal(ccID) {
			return fmt.Errorf("Committing an invocation of cc %s is illegal", ccID),
				peer.TxValidationCode_ILLEGAL_WRITESET
		}

		// Get latest chaincode version, vscc and validate policy
		_, vscc, policy, err := v.GetInfoForValidate(chdr.TxId, chdr.ChannelId, ccID)
		if err != nil {
			logger.Errorf("GetInfoForValidate for txId = %s returned error %s", chdr.TxId, err)
			return err, peer.TxValidationCode_INVALID_OTHER_REASON
		}

		// validate the transaction as an invocation of this system chaincode;
		// vscc will have to do custom validation for this system chaincode
		// currently, VSCC does custom validation for LSCC only; if an hlf
		// user creates a new system chaincode which is invokable from the outside
		// they have to modify VSCC to provide appropriate validation
		if err = v.VSCCValidateTxForCC(envBytes, chdr.TxId, vscc.ChainID, vscc.ChaincodeName, vscc.ChaincodeVersion, policy); err != nil {
			switch err.(type) {
			case *VSCCEndorsementPolicyError:
				return err, peer.TxValidationCode_ENDORSEMENT_POLICY_FAILURE
			default:
				return err, peer.TxValidationCode_INVALID_OTHER_REASON
			}
		}
	}

	return nil, peer.TxValidationCode_VALID
}

func (v *vsccValidatorImpl) VSCCValidateTxForCC(envBytes []byte, txid, chid, vsccName, vsccVer string, policy []byte) error {
	ctxt, err := v.ccprovider.GetContext(v.support.Ledger())
	if err != nil {
		msg := fmt.Sprintf("Cannot obtain context for txid=%s, err %s", txid, err)
		logger.Errorf(msg)
		return &VSCCExecutionFailureError{msg}
	}
	defer v.ccprovider.ReleaseContext()

	// build arguments for VSCC invocation
	// args[0] - function name (not used now)
	// args[1] - serialized Envelope
	// args[2] - serialized policy
	args := [][]byte{[]byte(""), envBytes, policy}

	// get context to invoke VSCC
	vscctxid := coreUtil.GenerateUUID()
	cccid := v.ccprovider.GetCCContext(chid, vsccName, vsccVer, vscctxid, true, nil, nil)

	// invoke VSCC
	logger.Debug("Invoking VSCC txid", txid, "chaindID", chid)
	res, _, err := v.ccprovider.ExecuteChaincode(ctxt, cccid, args)
	if err != nil {
		msg := fmt.Sprintf("Invoke VSCC failed for transaction txid=%s, error %s", txid, err)
		logger.Errorf(msg)
		return &VSCCExecutionFailureError{msg}
	}
	if res.Status != shim.OK {
		logger.Errorf("VSCC check failed for transaction txid=%s, error %s", txid, res.Message)
		return &VSCCEndorsementPolicyError{fmt.Sprintf("%s", res.Message)}
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
		return nil, &VSCCInfoLookupFailureError{fmt.Sprintf("Could not retrieve state for chaincode %s, error %s", ccid, err)}
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
