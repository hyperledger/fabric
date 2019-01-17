/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package statebased

import (
	"fmt"
	"sync"

	commonerrors "github.com/hyperledger/fabric/common/errors"
	"github.com/hyperledger/fabric/core/handlers/validation/api/policies"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/rwsetutil"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/peer"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/pkg/errors"
)

/**********************************************************************************************************/
/**********************************************************************************************************/

type RWSetPolicyEvaluatorFactory interface {
	Evaluator(ccEP []byte) RWSetPolicyEvaluator
}

type RWSetPolicyEvaluator interface {
	Evaluate(blockNum, txNum uint64, NsRwSets []*rwsetutil.NsRwSet, ns string, sd []*common.SignedData) commonerrors.TxValidationError
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

func NewKeyLevelValidator(policySupport validation.PolicyEvaluator, vpmgr KeyLevelValidationParameterManager) *KeyLevelValidator {
	return &KeyLevelValidator{
		vpmgr:    vpmgr,
		blockDep: blockDependency{},
		pef: &policyCheckerFactory{
			policySupport: policySupport,
			vpmgr:         vpmgr,
		},
	}
}

func (klv *KeyLevelValidator) invokeOnce(block *common.Block, txnum uint64) *sync.Once {
	klv.blockDep.mutex.Lock()
	defer klv.blockDep.mutex.Unlock()

	if klv.blockDep.blockNum != block.Header.Number {
		klv.blockDep.blockNum = block.Header.Number
		klv.blockDep.txDepOnce = make([]sync.Once, len(block.Data.Data))
	}

	return &klv.blockDep.txDepOnce[txnum]
}

func (klv *KeyLevelValidator) extractDependenciesForTx(blockNum, txNum uint64, envelopeBytes []byte) {
	env, err := utils.GetEnvelopeFromBlock(envelopeBytes)
	if err != nil {
		logger.Warningf("while executing GetEnvelopeFromBlock got error '%s', skipping tx at height (%d,%d)", err, blockNum, txNum)
		return
	}

	payl, err := utils.GetPayload(env)
	if err != nil {
		logger.Warningf("while executing GetPayload got error '%s', skipping tx at height (%d,%d)", err, blockNum, txNum)
		return
	}

	tx, err := utils.GetTransaction(payl.Data)
	if err != nil {
		logger.Warningf("while executing GetTransaction got error '%s', skipping tx at height (%d,%d)", err, blockNum, txNum)
		return
	}

	cap, err := utils.GetChaincodeActionPayload(tx.Actions[0].Payload)
	if err != nil {
		logger.Warningf("while executing GetChaincodeActionPayload got error '%s', skipping tx at height (%d,%d)", err, blockNum, txNum)
		return
	}

	pRespPayload, err := utils.GetProposalResponsePayload(cap.Action.ProposalResponsePayload)
	if err != nil {
		logger.Warningf("while executing GetProposalResponsePayload got error '%s', skipping tx at height (%d,%d)", err, blockNum, txNum)
		return
	}

	respPayload, err := utils.GetChaincodeAction(pRespPayload.Extension)
	if err != nil {
		logger.Warningf("while executing GetChaincodeAction got error '%s', skipping tx at height (%d,%d)", err, blockNum, txNum)
		return
	}

	klv.vpmgr.ExtractValidationParameterDependency(blockNum, txNum, respPayload.Results)
}

// PreValidate implements the function of the StateBasedValidator interface
func (klv *KeyLevelValidator) PreValidate(txNum uint64, block *common.Block) {
	for i := int64(txNum); i >= 0; i-- {
		txPosition := uint64(i)

		klv.invokeOnce(block, txPosition).Do(
			func() {
				klv.extractDependenciesForTx(block.Header.Number, txPosition, block.Data.Data[txPosition])
			})
	}
}

// Validate implements the function of the StateBasedValidator interface
func (klv *KeyLevelValidator) Validate(cc string, blockNum, txNum uint64, rwsetBytes, prp, ccEP []byte, endorsements []*peer.Endorsement) commonerrors.TxValidationError {
	// construct signature set
	signatureSet := []*common.SignedData{}
	for _, endorsement := range endorsements {
		data := make([]byte, len(prp)+len(endorsement.Endorser))
		copy(data, prp)
		copy(data[len(prp):], endorsement.Endorser)

		signatureSet = append(signatureSet, &common.SignedData{
			// set the data that is signed; concatenation of proposal response bytes and endorser ID
			Data: data,
			// set the identity that signs the message: it's the endorser
			Identity: endorsement.Endorser,
			// set the signature
			Signature: endorsement.Signature})
	}

	// construct the policy checker object
	policyEvaluator := klv.pef.Evaluator(ccEP)

	// unpack the rwset
	rwset := &rwsetutil.TxRwSet{}
	if err := rwset.FromProtoBytes(rwsetBytes); err != nil {
		return policyErr(errors.WithMessage(err, fmt.Sprintf("txRWSet.FromProtoBytes failed on tx (%d,%d)", blockNum, txNum)))
	}

	// return the decision of the policy evaluator
	return policyEvaluator.Evaluate(blockNum, txNum, rwset.NsRwSets, cc, signatureSet)
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
