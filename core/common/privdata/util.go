/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package privdata

import (
	"github.com/hyperledger/fabric/common/cauthdsl"
	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/msp"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/pkg/errors"
)

// getPolicy creates a new policy from the policy envelope. It will return an error if the envelope has invalid policy config.
// Some caller (e.g., MembershipProvider.AsMemberOf) may drop the error and treat it as a RejectAll policy.
// In the future, we must revisit the callers if this method will return different types of errors.
func getPolicy(collectionPolicyConfig *common.CollectionPolicyConfig, deserializer msp.IdentityDeserializer) (policies.Policy, error) {
	if collectionPolicyConfig == nil {
		return nil, errors.New("collection policy config is nil")
	}
	accessPolicyEnvelope := collectionPolicyConfig.GetSignaturePolicy()
	if accessPolicyEnvelope == nil {
		return nil, errors.New("collection config access policy is nil")
	}
	// create access policy from the envelope

	pp := cauthdsl.EnvelopeBasedPolicyProvider{Deserializer: deserializer}
	accessPolicy, err := pp.NewPolicy(accessPolicyEnvelope)
	if err != nil {
		return nil, errors.WithMessage(err, "failed constructing policy object out of collection policy config")
	}

	return accessPolicy, nil
}
