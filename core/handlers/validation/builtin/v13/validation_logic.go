/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package v13

import (
	"fmt"
	"regexp"

	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric-protos-go/peer"
	commonerrors "github.com/hyperledger/fabric/common/errors"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/core/common/validation/statebased"
	vc "github.com/hyperledger/fabric/core/handlers/validation/api/capabilities"
	vi "github.com/hyperledger/fabric/core/handlers/validation/api/identities"
	vp "github.com/hyperledger/fabric/core/handlers/validation/api/policies"
	vs "github.com/hyperledger/fabric/core/handlers/validation/api/state"
	"github.com/hyperledger/fabric/protoutil"
)

var logger = flogging.MustGetLogger("vscc")

// previously imported from ccmetadata.AllowedCharsCollectionName but could not change to avoid non-determinism
const AllowedCharsCollectionName = "[A-Za-z0-9_-]+"

var validCollectionNameRegex = regexp.MustCompile(AllowedCharsCollectionName)

//go:generate mockery -dir . -name Capabilities -case underscore -output mocks/

// Capabilities is the local interface that used to generate mocks for foreign interface.
type Capabilities interface {
	vc.Capabilities
}

//go:generate mockery -dir . -name StateFetcher -case underscore -output mocks/

// StateFetcher is the local interface that used to generate mocks for foreign interface.
type StateFetcher interface {
	vs.StateFetcher
}

//go:generate mockery -dir . -name IdentityDeserializer -case underscore -output mocks/

// IdentityDeserializer is the local interface that used to generate mocks for foreign interface.
type IdentityDeserializer interface {
	vi.IdentityDeserializer
}

//go:generate mockery -dir . -name PolicyEvaluator -case underscore -output mocks/

// PolicyEvaluator is the local interface that used to generate mocks for foreign interface.
type PolicyEvaluator interface {
	vp.PolicyEvaluator
}

//go:generate mockery -dir . -name StateBasedValidator -case underscore -output mocks/

// noopTranslator implements statebased.PolicyTranslator
// by performing no policy translation; this is okay because
// the implementation of the 1.3 validator has a policy
// evaluator that sirectly consumes SignaturePolicyEnvelope
// policies directly
type noopTranslator struct{}

func (n *noopTranslator) Translate(b []byte) ([]byte, error) {
	return b, nil
}

// New creates a new instance of the default VSCC
// Typically this will only be invoked once per peer
func New(c vc.Capabilities, s vs.StateFetcher, d vi.IdentityDeserializer, pe vp.PolicyEvaluator) *Validator {
	vpmgr := &statebased.KeyLevelValidationParameterManagerImpl{
		StateFetcher:     s,
		PolicyTranslator: &noopTranslator{},
	}
	eval := statebased.NewV13Evaluator(pe, vpmgr)
	sbv := statebased.NewKeyLevelValidator(eval, vpmgr)

	return &Validator{
		capabilities:        c,
		stateFetcher:        s,
		deserializer:        d,
		policyEvaluator:     pe,
		stateBasedValidator: sbv,
	}
}

// Validator implements the default transaction validation policy,
// which is to check the correctness of the read-write set and the endorsement
// signatures against an endorsement policy that is supplied as argument to
// every invoke
type Validator struct {
	deserializer        vi.IdentityDeserializer
	capabilities        vc.Capabilities
	stateFetcher        vs.StateFetcher
	policyEvaluator     vp.PolicyEvaluator
	stateBasedValidator StateBasedValidator
}

type validationArtifacts struct {
	rwset        []byte
	prp          []byte
	endorsements []*peer.Endorsement
	chdr         *common.ChannelHeader
	env          *common.Envelope
	payl         *common.Payload
	cap          *peer.ChaincodeActionPayload
}

func (vscc *Validator) extractValidationArtifacts(
	block *common.Block,
	txPosition int,
	actionPosition int,
) (*validationArtifacts, error) {
	// get the envelope...
	env, err := protoutil.GetEnvelopeFromBlock(block.Data.Data[txPosition])
	if err != nil {
		logger.Errorf("VSCC error: GetEnvelope failed, err %s", err)
		return nil, err
	}

	// ...and the payload...
	payl, err := protoutil.UnmarshalPayload(env.Payload)
	if err != nil {
		logger.Errorf("VSCC error: GetPayload failed, err %s", err)
		return nil, err
	}

	chdr, err := protoutil.UnmarshalChannelHeader(payl.Header.ChannelHeader)
	if err != nil {
		return nil, err
	}

	// validate the payload type
	if common.HeaderType(chdr.Type) != common.HeaderType_ENDORSER_TRANSACTION {
		logger.Errorf("Only Endorser Transactions are supported, provided type %d", chdr.Type)
		err = fmt.Errorf("Only Endorser Transactions are supported, provided type %d", chdr.Type)
		return nil, err
	}

	// ...and the transaction...
	tx, err := protoutil.UnmarshalTransaction(payl.Data)
	if err != nil {
		logger.Errorf("VSCC error: GetTransaction failed, err %s", err)
		return nil, err
	}

	cap, err := protoutil.UnmarshalChaincodeActionPayload(tx.Actions[actionPosition].Payload)
	if err != nil {
		logger.Errorf("VSCC error: GetChaincodeActionPayload failed, err %s", err)
		return nil, err
	}

	pRespPayload, err := protoutil.UnmarshalProposalResponsePayload(cap.Action.ProposalResponsePayload)
	if err != nil {
		err = fmt.Errorf("GetProposalResponsePayload error %s", err)
		return nil, err
	}
	if pRespPayload.Extension == nil {
		err = fmt.Errorf("nil pRespPayload.Extension")
		return nil, err
	}
	respPayload, err := protoutil.UnmarshalChaincodeAction(pRespPayload.Extension)
	if err != nil {
		err = fmt.Errorf("GetChaincodeAction error %s", err)
		return nil, err
	}

	return &validationArtifacts{
		rwset:        respPayload.Results,
		prp:          cap.Action.ProposalResponsePayload,
		endorsements: cap.Action.Endorsements,
		chdr:         chdr,
		env:          env,
		payl:         payl,
		cap:          cap,
	}, nil
}

// Validate validates the given envelope corresponding to a transaction with an endorsement
// policy as given in its serialized form.
// Note that in the case of dependencies in a block, such as tx_n modifying the endorsement policy
// for key a and tx_n+1 modifying the value of key a, Validate(tx_n+1) will block until Validate(tx_n)
// has been resolved. If working with a limited number of goroutines for parallel validation, ensure
// that they are allocated to transactions in ascending order.
func (vscc *Validator) Validate(
	block *common.Block,
	namespace string,
	txPosition int,
	actionPosition int,
	policyBytes []byte,
) commonerrors.TxValidationError {
	vscc.stateBasedValidator.PreValidate(uint64(txPosition), block)

	va, err := vscc.extractValidationArtifacts(block, txPosition, actionPosition)
	if err != nil {
		vscc.stateBasedValidator.PostValidate(namespace, block.Header.Number, uint64(txPosition), err)
		return policyErr(err)
	}

	txverr := vscc.stateBasedValidator.Validate(
		namespace,
		block.Header.Number,
		uint64(txPosition),
		va.rwset,
		va.prp,
		policyBytes,
		va.endorsements,
	)
	if txverr != nil {
		logger.Errorf("VSCC error: stateBasedValidator.Validate failed, err %s", txverr)
		vscc.stateBasedValidator.PostValidate(namespace, block.Header.Number, uint64(txPosition), txverr)
		return txverr
	}

	// do some extra validation that is specific to lscc
	if namespace == "lscc" {
		logger.Debugf("VSCC info: doing special validation for LSCC")
		err := vscc.ValidateLSCCInvocation(va.chdr.ChannelId, va.env, va.cap, va.payl, vscc.capabilities)
		if err != nil {
			logger.Errorf("VSCC error: ValidateLSCCInvocation failed, err %s", err)
			vscc.stateBasedValidator.PostValidate(namespace, block.Header.Number, uint64(txPosition), err)
			return err
		}
	}

	vscc.stateBasedValidator.PostValidate(namespace, block.Header.Number, uint64(txPosition), nil)
	return nil
}

func policyErr(err error) *commonerrors.VSCCEndorsementPolicyError {
	return &commonerrors.VSCCEndorsementPolicyError{
		Err: err,
	}
}
