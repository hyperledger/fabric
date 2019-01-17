/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package statebased

import (
	commonerrors "github.com/hyperledger/fabric/common/errors"
	"github.com/hyperledger/fabric/core/handlers/validation/api/policies"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/rwsetutil"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/pkg/errors"
)

type policyCheckerFactory struct {
	vpmgr         KeyLevelValidationParameterManager
	policySupport validation.PolicyEvaluator
}

func (p *policyCheckerFactory) Evaluator(ccEP []byte) RWSetPolicyEvaluator {
	return &policyChecker{
		ccEP:          ccEP,
		policySupport: p.policySupport,
		vpmgr:         p.vpmgr,
	}
}

/**********************************************************************************************************/
/**********************************************************************************************************/

type policyChecker struct {
	someEPChecked bool
	ccEPChecked   bool
	vpmgr         KeyLevelValidationParameterManager
	policySupport validation.PolicyEvaluator
	ccEP          []byte
	signatureSet  []*common.SignedData
}

func (p *policyChecker) Evaluate(blockNum, txNum uint64, NsRwSets []*rwsetutil.NsRwSet, ns string, sd []*common.SignedData) commonerrors.TxValidationError {
	p.signatureSet = sd

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
			err := p.checkSBAndCCEP(ns, "", pubWrite.Key, blockNum, txNum)
			if err != nil {
				return err
			}
		}
		// public metadata writes
		// we validate writes against key-level validation parameters
		// if any are present or the chaincode-wide endorsement policy
		for _, pubMdWrite := range nsRWSet.KvRwSet.MetadataWrites {
			err := p.checkSBAndCCEP(ns, "", pubMdWrite.Key, blockNum, txNum)
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
				err := p.checkSBAndCCEP(ns, coll, key, blockNum, txNum)
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
				err := p.checkSBAndCCEP(ns, coll, key, blockNum, txNum)
				if err != nil {
					return err
				}
			}
		}
	}

	// we make sure that we check at least the CCEP to honour FAB-9473
	return p.checkCCEPIfNoEPChecked(ns, blockNum, txNum)
}

func (p *policyChecker) checkCCEPIfCondition(cc string, blockNum, txNum uint64, condition bool) commonerrors.TxValidationError {
	if condition {
		return nil
	}

	// validate against cc ep
	err := p.policySupport.Evaluate(p.ccEP, p.signatureSet)
	if err != nil {
		return policyErr(errors.Wrapf(err, "validation of endorsement policy for chaincode %s in tx %d:%d failed", cc, blockNum, txNum))
	}

	p.ccEPChecked = true
	p.someEPChecked = true
	return nil
}

func (p *policyChecker) checkCCEPIfNotChecked(cc string, blockNum, txNum uint64) commonerrors.TxValidationError {
	return p.checkCCEPIfCondition(cc, blockNum, txNum, p.ccEPChecked)
}

func (p *policyChecker) checkCCEPIfNoEPChecked(cc string, blockNum, txNum uint64) commonerrors.TxValidationError {
	return p.checkCCEPIfCondition(cc, blockNum, txNum, p.someEPChecked)
}

func (p *policyChecker) checkSBAndCCEP(cc, coll, key string, blockNum, txNum uint64) commonerrors.TxValidationError {
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
			err = nil
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
		return p.checkCCEPIfNotChecked(cc, blockNum, txNum)
	}

	// validate against key-level vp
	err = p.policySupport.Evaluate(vp, p.signatureSet)
	if err != nil {
		return policyErr(errors.Wrapf(err, "validation of key %s (coll'%s':ns'%s') in tx %d:%d failed", key, coll, cc, blockNum, txNum))
	}

	p.someEPChecked = true

	return nil
}
