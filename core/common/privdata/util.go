/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package privdata

import (
	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/cauthdsl"
	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/msp"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/pkg/errors"
)

func getPolicy(collectionPolicyConfig *common.CollectionPolicyConfig, deserializer msp.IdentityDeserializer) (policies.Policy, error) {
	if collectionPolicyConfig == nil {
		return nil, errors.New("Collection policy config is nil")
	}
	accessPolicyEnvelope := collectionPolicyConfig.GetSignaturePolicy()
	if accessPolicyEnvelope == nil {
		return nil, errors.New("Collection config access policy is nil")
	}
	// create access policy from the envelope
	npp := cauthdsl.NewPolicyProvider(deserializer)
	polBytes, err := proto.Marshal(accessPolicyEnvelope)
	if err != nil {
		return nil, err
	}
	accessPolicy, _, err := npp.NewPolicy(polBytes)
	if err != nil {
		return nil, err
	}
	return accessPolicy, nil
}
