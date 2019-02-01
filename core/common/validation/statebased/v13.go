/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package statebased

import (
	commonerrors "github.com/hyperledger/fabric/common/errors"
	validation "github.com/hyperledger/fabric/core/handlers/validation/api/policies"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
)

// NewV13Evaluator returns a policy evaluator that checks
// 2 kinds of policies:
// 1) chaincode endorsement policies;
// 2) state-based endorsement policies.
func NewV13Evaluator(policySupport validation.PolicyEvaluator, vpmgr KeyLevelValidationParameterManager) *policyCheckerFactoryV13 {
	return &policyCheckerFactoryV13{
		policySupport: policySupport,
		vpmgr:         vpmgr,
	}
}

type policyCheckerFactoryV13 struct {
	vpmgr         KeyLevelValidationParameterManager
	policySupport validation.PolicyEvaluator
}

func (p *policyCheckerFactoryV13) Evaluator(ccEP []byte) RWSetPolicyEvaluator {
	return &baseEvaluator{
		epEvaluator: &policyCheckerV13{
			policySupport: p.policySupport,
			ccEP:          ccEP,
		},
		vpmgr:         p.vpmgr,
		policySupport: p.policySupport,
	}
}

/**********************************************************************************************************/
/**********************************************************************************************************/

type policyCheckerV13 struct {
	someEPChecked bool
	ccEPChecked   bool
	policySupport validation.PolicyEvaluator
	ccEP          []byte
}

func (p *policyCheckerV13) checkCCEPIfCondition(cc string, blockNum, txNum uint64, condition bool, sd []*protoutil.SignedData) commonerrors.TxValidationError {
	if condition {
		return nil
	}

	// validate against cc ep
	err := p.policySupport.Evaluate(p.ccEP, sd)
	if err != nil {
		return policyErr(errors.Wrapf(err, "validation of endorsement policy for chaincode %s in tx %d:%d failed", cc, blockNum, txNum))
	}

	p.ccEPChecked = true
	p.someEPChecked = true
	return nil
}

func (p *policyCheckerV13) CheckCCEPIfNotChecked(cc, coll string, blockNum, txNum uint64, sd []*protoutil.SignedData) commonerrors.TxValidationError {
	return p.checkCCEPIfCondition(cc, blockNum, txNum, p.ccEPChecked, sd)
}

func (p *policyCheckerV13) CheckCCEPIfNoEPChecked(cc string, blockNum, txNum uint64, sd []*protoutil.SignedData) commonerrors.TxValidationError {
	return p.checkCCEPIfCondition(cc, blockNum, txNum, p.someEPChecked, sd)
}

func (p *policyCheckerV13) SBEPChecked() {
	p.someEPChecked = true
}
