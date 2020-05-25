/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package statebased

import (
	commonerrors "github.com/hyperledger/fabric/common/errors"
	validation "github.com/hyperledger/fabric/core/handlers/validation/api/policies"
	s "github.com/hyperledger/fabric/core/handlers/validation/api/state"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
)

// NewV20Evaluator returns a policy evaluator that checks
// 3 kinds of policies:
// 1) chaincode endorsement policies;
// 2) state-based endorsement policies;
// 3) collection-level endorsement policies.
func NewV20Evaluator(
	vpmgr KeyLevelValidationParameterManager,
	policySupport validation.PolicyEvaluator,
	collRes CollectionResources,
	StateFetcher s.StateFetcher,
) *policyCheckerFactoryV20 {
	return &policyCheckerFactoryV20{
		vpmgr:         vpmgr,
		policySupport: policySupport,
		StateFetcher:  StateFetcher,
		collRes:       collRes,
	}
}

type policyCheckerFactoryV20 struct {
	vpmgr         KeyLevelValidationParameterManager
	policySupport validation.PolicyEvaluator
	collRes       CollectionResources
	StateFetcher  s.StateFetcher
}

func (p *policyCheckerFactoryV20) Evaluator(ccEP []byte) RWSetPolicyEvaluator {
	return &baseEvaluator{
		epEvaluator: &policyCheckerV20{
			ccEP:          ccEP,
			policySupport: p.policySupport,
			nsEPChecked:   map[string]bool{},
			collRes:       p.collRes,
			StateFetcher:  p.StateFetcher,
		},
		vpmgr:         p.vpmgr,
		policySupport: p.policySupport,
	}
}

/**********************************************************************************************************/
/**********************************************************************************************************/

// CollectionResources provides access to collection artefacts
type CollectionResources interface {
	// CollectionValidationInfo returns collection-level endorsement policy for the supplied chaincode.
	// The function returns two types of errors, unexpected errors and validation errors. The
	// reason for this is that this function is to be called from the validation code, which
	// needs to tell apart the two types of error to halt processing on the channel if the
	// unexpected error is not nil and mark the transaction as invalid if the validation error
	// is not nil.
	CollectionValidationInfo(chaincodeName, collectionName string, state s.State) (args []byte, unexpectedErr error, validationErr error)
}

//go:generate mockery -dir . -name CollectionResources -case underscore -output mocks/
//go:generate mockery -dir . -name KeyLevelValidationParameterManager -case underscore -output mocks/

type policyCheckerV20 struct {
	someEPChecked bool
	policySupport validation.PolicyEvaluator
	ccEP          []byte
	nsEPChecked   map[string]bool
	collRes       CollectionResources
	StateFetcher  s.StateFetcher
}

func (p *policyCheckerV20) fetchCollEP(cc, coll string) ([]byte, commonerrors.TxValidationError) {
	state, err := p.StateFetcher.FetchState()
	if err != nil {
		return nil, &commonerrors.VSCCExecutionFailureError{
			Err: errors.WithMessage(err, "could not retrieve ledger"),
		}
	}
	defer state.Done()

	collEP, unexpectedErr, validationErr := p.collRes.CollectionValidationInfo(cc, coll, state)
	if unexpectedErr != nil {
		return nil, &commonerrors.VSCCExecutionFailureError{
			Err: unexpectedErr,
		}
	}
	if validationErr != nil {
		return nil, policyErr(validationErr)
	}

	return collEP, nil
}

func (p *policyCheckerV20) CheckCCEPIfNotChecked(cc, coll string, blockNum, txNum uint64, sd []*protoutil.SignedData) commonerrors.TxValidationError {
	if coll != "" {
		// at first we check whether we have already evaluated an endorsement
		// policy for this collection
		if p.nsEPChecked[coll] {
			return nil
		}

		// if not, we fetch the collection endorsement policy
		collEP, err := p.fetchCollEP(cc, coll)
		if err != nil {
			return err
		}

		// if there is an endorsement policy for the collection, we evaluate it
		if len(collEP) != 0 {
			err := p.policySupport.Evaluate(collEP, sd)
			if err != nil {
				return policyErr(errors.Wrapf(err, "validation of endorsement policy for collection %s chaincode %s in tx %d:%d failed", coll, cc, blockNum, txNum))
			}

			p.nsEPChecked[coll] = true
			p.someEPChecked = true
			return nil
		}
	}

	// we're here either because we're not in a collection or because there was
	// no endorsement policy for that collection - we turn to the chaincode EP
	if p.nsEPChecked[""] {
		return nil
	}

	// evaluate the cc EP
	err := p.policySupport.Evaluate(p.ccEP, sd)
	if err != nil {
		return policyErr(errors.Wrapf(err, "validation of endorsement policy for chaincode %s in tx %d:%d failed", cc, blockNum, txNum))
	}

	p.nsEPChecked[""] = true
	p.someEPChecked = true
	return nil
}

func (p *policyCheckerV20) CheckCCEPIfNoEPChecked(cc string, blockNum, txNum uint64, sd []*protoutil.SignedData) commonerrors.TxValidationError {
	if p.someEPChecked {
		return nil
	}

	// validate against cc ep
	err := p.policySupport.Evaluate(p.ccEP, sd)
	if err != nil {
		return policyErr(errors.Wrapf(err, "validation of endorsement policy for chaincode %s in tx %d:%d failed", cc, blockNum, txNum))
	}

	p.nsEPChecked[""] = true
	p.someEPChecked = true
	return nil
}

func (p *policyCheckerV20) SBEPChecked() {
	p.someEPChecked = true
}
