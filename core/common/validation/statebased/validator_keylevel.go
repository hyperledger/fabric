/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package statebased

import (
	"sync"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric-protos-go/peer"
	commonerrors "github.com/hyperledger/fabric/common/errors"
	validation "github.com/hyperledger/fabric/core/handlers/validation/api/policies"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/rwsetutil"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
)

type epEvaluator interface {
	CheckCCEPIfNotChecked(cc, coll string, blockNum, txNum uint64, sd []*protoutil.SignedData) commonerrors.TxValidationError
	CheckCCEPIfNoEPChecked(cc string, blockNum, txNum uint64, sd []*protoutil.SignedData) commonerrors.TxValidationError
	SBEPChecked()
}

/**********************************************************************************************************/
/**********************************************************************************************************/

type baseEvaluator struct {
	epEvaluator
	vpmgr         KeyLevelValidationParameterManager
	policySupport validation.PolicyEvaluator
}

func (p *baseEvaluator) checkSBAndCCEP(cc, coll, key string, blockNum, txNum uint64, signatureSet []*protoutil.SignedData) commonerrors.TxValidationError {
	// see if there is a key-level validation parameter for this key
	vp, err := p.vpmgr.GetValidationParameterForKey(cc, coll, key, blockNum, txNum)
	if err != nil {
		// error handling for GetValidationParameterForKey follows this rationale:
		switch err := errors.Cause(err).(type) {
		// 1) if there is a conflict because validation params have been updated
		//    by another transaction in this block, we will get ValidationParameterUpdatedError.
		//    This should lead to invalidating the transaction by calling policyErr
		case *ValidationParameterUpdatedError:
			return policyErr(err)
		// 2) if the ledger returns "determinstic" errors, that is, errors that
		//    every peer in the channel will also return (such as errors linked to
		//    an attempt to retrieve metadata from a non-defined collection) should be
		//    logged and ignored. The ledger will take the most appropriate action
		//    when performing its side of the validation.
		case *ledger.CollConfigNotDefinedError, *ledger.InvalidCollNameError:
			logger.Warningf(errors.WithMessage(err, "skipping key-level validation").Error())
		// 3) any other type of error should return an execution failure which will
		//    lead to halting the processing on this channel. Note that any non-categorized
		//    deterministic error would be caught by the default and would lead to
		//    a processing halt. This would certainly be a bug, but - in the absence of a
		//    single, well-defined deterministic error returned by the ledger, it is
		//    best to err on the side of caution and rather halt processing (because a
		//    deterministic error is treated like an I/O one) rather than risking a fork
		//    (in case an I/O error is treated as a deterministic one).
		default:
			return &commonerrors.VSCCExecutionFailureError{
				Err: err,
			}
		}
	}

	// if no key-level validation parameter has been specified, the regular cc endorsement policy needs to hold
	if len(vp) == 0 {
		return p.CheckCCEPIfNotChecked(cc, coll, blockNum, txNum, signatureSet)
	}

	// validate against key-level vp
	err = p.policySupport.Evaluate(vp, signatureSet)
	if err != nil {
		return policyErr(errors.Wrapf(err, "validation of key %s (coll'%s':ns'%s') in tx %d:%d failed", key, coll, cc, blockNum, txNum))
	}

	p.SBEPChecked()

	return nil
}

func (p *baseEvaluator) Evaluate(blockNum, txNum uint64, NsRwSets []*rwsetutil.NsRwSet, ns string, sd []*protoutil.SignedData) commonerrors.TxValidationError {
	// iterate over all writes in the rwset
	for _, nsRWSet := range NsRwSets {
		// skip other namespaces
		if nsRWSet.NameSpace != ns {
			continue
		}

		// public writes
		// we validate writes against key-level validation parameters
		// if any are present or the chaincode-wide endorsement policy
		for _, pubWrite := range nsRWSet.KvRwSet.Writes {
			err := p.checkSBAndCCEP(ns, "", pubWrite.Key, blockNum, txNum, sd)
			if err != nil {
				return err
			}
		}
		// public metadata writes
		// we validate writes against key-level validation parameters
		// if any are present or the chaincode-wide endorsement policy
		for _, pubMdWrite := range nsRWSet.KvRwSet.MetadataWrites {
			err := p.checkSBAndCCEP(ns, "", pubMdWrite.Key, blockNum, txNum, sd)
			if err != nil {
				return err
			}
		}
		// writes in collections
		// we validate writes against key-level validation parameters
		// if any are present or the chaincode-wide endorsement policy
		for _, collRWSet := range nsRWSet.CollHashedRwSets {
			coll := collRWSet.CollectionName
			for _, hashedWrite := range collRWSet.HashedRwSet.HashedWrites {
				key := string(hashedWrite.KeyHash)
				err := p.checkSBAndCCEP(ns, coll, key, blockNum, txNum, sd)
				if err != nil {
					return err
				}
			}
		}
		// metadata writes in collections
		// we validate writes against key-level validation parameters
		// if any are present or the chaincode-wide endorsement policy
		for _, collRWSet := range nsRWSet.CollHashedRwSets {
			coll := collRWSet.CollectionName
			for _, hashedMdWrite := range collRWSet.HashedRwSet.MetadataWrites {
				key := string(hashedMdWrite.KeyHash)
				err := p.checkSBAndCCEP(ns, coll, key, blockNum, txNum, sd)
				if err != nil {
					return err
				}
			}
		}
	}

	// we make sure that we check at least the CCEP to honour FAB-9473
	return p.CheckCCEPIfNoEPChecked(ns, blockNum, txNum, sd)
}

/**********************************************************************************************************/
/**********************************************************************************************************/

// RWSetPolicyEvaluatorFactory is a factory for policy evaluators
type RWSetPolicyEvaluatorFactory interface {
	// Evaluator returns a new policy evaluator
	// given the supplied chaincode endorsement policy
	Evaluator(ccEP []byte) RWSetPolicyEvaluator
}

// RWSetPolicyEvaluator provides means to evaluate transaction artefacts
type RWSetPolicyEvaluator interface {
	Evaluate(blockNum, txNum uint64, NsRwSets []*rwsetutil.NsRwSet, ns string, sd []*protoutil.SignedData) commonerrors.TxValidationError
}

/**********************************************************************************************************/
/**********************************************************************************************************/

type blockDependency struct {
	mutex     sync.Mutex
	blockNum  uint64
	txDepOnce []sync.Once
}

// KeyLevelValidator implements per-key level ep validation
type KeyLevelValidator struct {
	vpmgr    KeyLevelValidationParameterManager
	blockDep blockDependency
	pef      RWSetPolicyEvaluatorFactory
}

func NewKeyLevelValidator(evaluator RWSetPolicyEvaluatorFactory, vpmgr KeyLevelValidationParameterManager) *KeyLevelValidator {
	return &KeyLevelValidator{
		vpmgr:    vpmgr,
		blockDep: blockDependency{},
		pef:      evaluator,
	}
}

func (klv *KeyLevelValidator) extractDependenciesForTx(blockNum, txNum uint64, envelopeBytes []byte) {
	env, err := protoutil.GetEnvelopeFromBlock(envelopeBytes)
	if err != nil {
		logger.Warningf("while executing GetEnvelopeFromBlock got error '%s', skipping tx at height (%d,%d)", err, blockNum, txNum)
		return
	}

	payl, err := protoutil.UnmarshalPayload(env.Payload)
	if err != nil {
		logger.Warningf("while executing GetPayload got error '%s', skipping tx at height (%d,%d)", err, blockNum, txNum)
		return
	}

	tx, err := protoutil.UnmarshalTransaction(payl.Data)
	if err != nil {
		logger.Warningf("while executing GetTransaction got error '%s', skipping tx at height (%d,%d)", err, blockNum, txNum)
		return
	}

	cap, err := protoutil.UnmarshalChaincodeActionPayload(tx.Actions[0].Payload)
	if err != nil {
		logger.Warningf("while executing GetChaincodeActionPayload got error '%s', skipping tx at height (%d,%d)", err, blockNum, txNum)
		return
	}

	pRespPayload, err := protoutil.UnmarshalProposalResponsePayload(cap.Action.ProposalResponsePayload)
	if err != nil {
		logger.Warningf("while executing GetProposalResponsePayload got error '%s', skipping tx at height (%d,%d)", err, blockNum, txNum)
		return
	}

	respPayload, err := protoutil.UnmarshalChaincodeAction(pRespPayload.Extension)
	if err != nil {
		logger.Warningf("while executing GetChaincodeAction got error '%s', skipping tx at height (%d,%d)", err, blockNum, txNum)
		return
	}

	klv.vpmgr.ExtractValidationParameterDependency(blockNum, txNum, respPayload.Results)
}

// PreValidate implements the function of the StateBasedValidator interface
func (klv *KeyLevelValidator) PreValidate(txNum uint64, block *common.Block) {
	klv.blockDep.mutex.Lock()
	if klv.blockDep.blockNum != block.Header.Number {
		klv.blockDep.blockNum = block.Header.Number
		klv.blockDep.txDepOnce = make([]sync.Once, len(block.Data.Data))
	}
	klv.blockDep.mutex.Unlock()

	for i := int64(txNum); i >= 0; i-- {
		txPosition := uint64(i)

		klv.blockDep.txDepOnce[i].Do(
			func() {
				klv.extractDependenciesForTx(block.Header.Number, txPosition, block.Data.Data[txPosition])
			})
	}
}

// Validate implements the function of the StateBasedValidator interface
func (klv *KeyLevelValidator) Validate(cc string, blockNum, txNum uint64, rwsetBytes, prp, ccEP []byte, endorsements []*peer.Endorsement) commonerrors.TxValidationError {
	// construct signature set
	signatureSet := []*protoutil.SignedData{}
	for _, endorsement := range endorsements {
		data := make([]byte, len(prp)+len(endorsement.Endorser))
		copy(data, prp)
		copy(data[len(prp):], endorsement.Endorser)

		signatureSet = append(signatureSet, &protoutil.SignedData{
			// set the data that is signed; concatenation of proposal response bytes and endorser ID
			Data: data,
			// set the identity that signs the message: it's the endorser
			Identity: endorsement.Endorser,
			// set the signature
			Signature: endorsement.Signature,
		})
	}

	// construct the policy checker object
	policyEvaluator := klv.pef.Evaluator(ccEP)

	// unpack the rwset
	rwset := &rwsetutil.TxRwSet{}
	if err := rwset.FromProtoBytes(rwsetBytes); err != nil {
		return policyErr(errors.WithMessagef(err, "txRWSet.FromProtoBytes failed on tx (%d,%d)", blockNum, txNum))
	}

	// return the decision of the policy evaluator
	err := policyEvaluator.Evaluate(blockNum, txNum, rwset.NsRwSets, cc, signatureSet)
	if err != nil {
		// If endorsement policy check fails, log the endorsement policy and endorser identities.
		// No need to handle Unmarshal() errors since it will simply result in endorsementPolicy being empty in the log message.
		endorsementPolicy := &peer.ApplicationPolicy{}
		proto.Unmarshal(ccEP, endorsementPolicy)
		logger.Warnw("Endorsment policy failure", "error", err, "chaincode", cc, "endorsementPolicy", endorsementPolicy, "endorsingIdentities", protoutil.LogMessageForSerializedIdentities(signatureSet))

	}
	return err
}

// PostValidate implements the function of the StateBasedValidator interface
func (klv *KeyLevelValidator) PostValidate(cc string, blockNum, txNum uint64, err error) {
	klv.vpmgr.SetTxValidationResult(cc, blockNum, txNum, err)
}

func policyErr(err error) *commonerrors.VSCCEndorsementPolicyError {
	return &commonerrors.VSCCEndorsementPolicyError{
		Err: err,
	}
}
