/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package builtin1_3

import (
	"fmt"
	"regexp"

	commonerrors "github.com/hyperledger/fabric/common/errors"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/core/chaincode/platforms/ccmetadata"
	. "github.com/hyperledger/fabric/core/handlers/validation/api/capabilities"
	. "github.com/hyperledger/fabric/core/handlers/validation/api/identities"
	. "github.com/hyperledger/fabric/core/handlers/validation/api/policies"
	. "github.com/hyperledger/fabric/core/handlers/validation/api/state"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/pkg/errors"
)

var logger = flogging.MustGetLogger("vscc")

const (
	DUPLICATED_IDENTITY_ERROR = "Endorsement policy evaluation failure might be caused by duplicated identities"
)

var validCollectionNameRegex = regexp.MustCompile(ccmetadata.AllowedCharsCollectionName)

//go:generate mockery -dir ../../api/capabilities/ -name Capabilities -case underscore -output mocks/
//go:generate mockery -dir ../../api/state/ -name StateFetcher -case underscore -output mocks/
//go:generate mockery -dir ../../api/identities/ -name IdentityDeserializer -case underscore -output mocks/
//go:generate mockery -dir ../../api/policies/ -name PolicyEvaluator -case underscore -output mocks/

// New creates a new instance of the default VSCC
// Typically this will only be invoked once per peer
func New(c Capabilities, s StateFetcher, d IdentityDeserializer, pe PolicyEvaluator) *Validator {
	return &Validator{
		capabilities:    c,
		stateFetcher:    s,
		deserializer:    d,
		policyEvaluator: pe,
	}
}

// Validator implements the default transaction validation policy,
// which is to check the correctness of the read-write set and the endorsement
// signatures against an endorsement policy that is supplied as argument to
// every invoke
type Validator struct {
	deserializer    IdentityDeserializer
	capabilities    Capabilities
	stateFetcher    StateFetcher
	policyEvaluator PolicyEvaluator
}

// Validate validates the given envelope corresponding to a transaction with an endorsement
// policy as given in its serialized form
func (vscc *Validator) Validate(
	block *common.Block,
	namespace string,
	txPosition int,
	actionPosition int,
	policyBytes []byte,
) commonerrors.TxValidationError {
	// get the envelope...
	env, err := utils.GetEnvelopeFromBlock(block.Data.Data[txPosition])
	if err != nil {
		logger.Errorf("VSCC error: GetEnvelope failed, err %s", err)
		return policyErr(err)
	}

	// ...and the payload...
	payl, err := utils.GetPayload(env)
	if err != nil {
		logger.Errorf("VSCC error: GetPayload failed, err %s", err)
		return policyErr(err)
	}

	chdr, err := utils.UnmarshalChannelHeader(payl.Header.ChannelHeader)
	if err != nil {
		return policyErr(err)
	}

	// validate the payload type
	if common.HeaderType(chdr.Type) != common.HeaderType_ENDORSER_TRANSACTION {
		logger.Errorf("Only Endorser Transactions are supported, provided type %d", chdr.Type)
		return policyErr(fmt.Errorf("Only Endorser Transactions are supported, provided type %d", chdr.Type))
	}

	// ...and the transaction...
	tx, err := utils.GetTransaction(payl.Data)
	if err != nil {
		logger.Errorf("VSCC error: GetTransaction failed, err %s", err)
		return policyErr(err)
	}

	cap, err := utils.GetChaincodeActionPayload(tx.Actions[actionPosition].Payload)
	if err != nil {
		logger.Errorf("VSCC error: GetChaincodeActionPayload failed, err %s", err)
		return policyErr(err)
	}

	signatureSet, err := vscc.deduplicateIdentity(cap)
	if err != nil {
		return policyErr(err)
	}

	// evaluate the signature set against the policy
	err = vscc.policyEvaluator.Evaluate(policyBytes, signatureSet)
	if err != nil {
		logger.Warningf("Endorsement policy failure for transaction txid=%s, err: %s", chdr.GetTxId(), err.Error())
		if len(signatureSet) < len(cap.Action.Endorsements) {
			// Warning: duplicated identities exist, endorsement failure might be cause by this reason
			return policyErr(errors.New(DUPLICATED_IDENTITY_ERROR))
		}
		return policyErr(fmt.Errorf("VSCC error: endorsement policy failure, err: %s", err))
	}

	// do some extra validation that is specific to lscc
	if namespace == "lscc" {
		logger.Debugf("VSCC info: doing special validation for LSCC")
		err := vscc.ValidateLSCCInvocation(chdr.ChannelId, env, cap, payl, vscc.capabilities)
		if err != nil {
			logger.Errorf("VSCC error: ValidateLSCCInvocation failed, err %s", err)
			return err
		}
	}

	return nil
}

func policyErr(err error) *commonerrors.VSCCEndorsementPolicyError {
	return &commonerrors.VSCCEndorsementPolicyError{
		Err: err,
	}
}
