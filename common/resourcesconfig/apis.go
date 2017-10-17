/*
Copyright IBM Corp. 2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package resourcesconfig

import (
	cb "github.com/hyperledger/fabric/protos/common"
	pb "github.com/hyperledger/fabric/protos/peer"

	"github.com/golang/protobuf/proto"
	"github.com/pkg/errors"
)

// apisGroup represents the ConfigGroup names APIs off the resources group
type apisGroup struct {
	apiPolicyRefs map[string]string
}

func (ag *apisGroup) PolicyRefForAPI(apiName string) string {
	return ag.apiPolicyRefs[apiName]
}

func newAPIsGroup(group *cb.ConfigGroup) (*apisGroup, error) {
	if len(group.Groups) > 0 {
		return nil, errors.New("apis group does not support sub-groups")
	}

	apiPolicyRefs := make(map[string]string)

	for key, value := range group.Values {
		api := &pb.Resource{}
		if err := proto.Unmarshal(value.Value, api); err != nil {
			return nil, err
		}

		// If the policy is fully qualified, ie to /Channel/Application/Readers leave it alone
		// otherwise, make it fully qualified referring to /Resources/APIs/policyName
		if '/' != api.PolicyRef[0] {
			apiPolicyRefs[key] = "/" + RootGroupKey + "/" + APIsGroupKey + "/" + api.PolicyRef
		} else {
			apiPolicyRefs[key] = api.PolicyRef
		}
	}

	return &apisGroup{
		apiPolicyRefs: apiPolicyRefs,
	}, nil
}
