/*
Copyright IBM Corp. 2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package resources

import (
	"fmt"

	cb "github.com/hyperledger/fabric/protos/common"
	pb "github.com/hyperledger/fabric/protos/peer"

	"github.com/golang/protobuf/proto"
)

// ResourceGroup represents the ConfigGroup at the base of the resource configuration
type resourceGroup struct {
	resourcePolicyRefs map[string]string
}

func (rg *resourceGroup) PolicyRefForResource(resourceName string) string {
	return rg.resourcePolicyRefs[resourceName]
}

func newResourceGroup(root *cb.ConfigGroup) (*resourceGroup, error) {
	resourcePolicyRefs := make(map[string]string)

	for key, value := range root.Values {
		resource := &pb.Resource{}
		if err := proto.Unmarshal(value.Value, resource); err != nil {
			return nil, err
		}

		// If the policy is fully qualified, ie to /Channel/Application/Readers leave it alone
		// otherwise, make it fully qualified referring to /Resources/policyName
		if '/' != resource.PolicyRef[0] {
			resourcePolicyRefs[key] = "/" + RootGroupKey + "/" + resource.PolicyRef
		} else {
			resourcePolicyRefs[key] = resource.PolicyRef
		}
	}

	for _, subGroup := range root.Groups {
		if err := verifyNoMoreValues(subGroup); err != nil {
			return nil, err
		}
	}

	return &resourceGroup{
		resourcePolicyRefs: resourcePolicyRefs,
	}, nil
}

func verifyNoMoreValues(subGroup *cb.ConfigGroup) error {
	if len(subGroup.Values) > 0 {
		return fmt.Errorf("sub-groups not allowed to have values")
	}
	for _, subGroup := range subGroup.Groups {
		if err := verifyNoMoreValues(subGroup); err != nil {
			return err
		}
	}
	return nil
}
