/*
Copyright IBM Corp. 2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package resources

import (
	"github.com/hyperledger/fabric/common/config"
	pb "github.com/hyperledger/fabric/protos/peer"

	"github.com/golang/protobuf/proto"
)

// ResourceGroup represents the ConfigGroup at the base of the resource configuration
type resourceGroup struct {
	pendingResourceToPolicyRef map[string]string
	vpr                        *valueProposerRoot
}

func newResourceGroup(vpr *valueProposerRoot) *resourceGroup {
	return &resourceGroup{
		vpr: vpr,
		pendingResourceToPolicyRef: make(map[string]string),
	}
}

// BeginValueProposals is invoked for each sub-group.  These sub-groups are only for defining policies, not other config
// so, an emptyGroup is returned to handle them
func (rg *resourceGroup) BeginValueProposals(tx interface{}, groups []string) (config.ValueDeserializer, []config.ValueProposer, error) {
	subGroups := make([]config.ValueProposer, len(groups))
	for i := range subGroups {
		subGroups[i] = emptyGroup{}
	}
	return &resourceGroupDeserializer{rg: rg}, subGroups, nil
}

// RollbackConfig a no-op
func (rg *resourceGroup) RollbackProposals(tx interface{}) {}

// PreCommit is a no-op
func (rg *resourceGroup) PreCommit(tx interface{}) error { return nil }

// CommitProposals writes the pendingResourceToPolicyRef map to the resource config root
func (rg *resourceGroup) CommitProposals(tx interface{}) {
	rg.vpr.updatePolicyRefForResources(rg.pendingResourceToPolicyRef)
}

type resourceGroupDeserializer struct {
	rg *resourceGroup
}

// Deserialize unmarshals bytes to a pb.Resource
func (rgd *resourceGroupDeserializer) Deserialize(key string, value []byte) (proto.Message, error) {
	resource := &pb.Resource{}
	if err := proto.Unmarshal(value, resource); err != nil {
		return nil, err
	}

	// If the policy is fully qualified, ie to /Channel/Application/Readers leave it alone
	// otherwise, make it fully qualified referring to /Resources/policyName
	if '/' != resource.PolicyRef[0] {
		rgd.rg.pendingResourceToPolicyRef[key] = "/" + RootGroupKey + "/" + resource.PolicyRef
	} else {
		rgd.rg.pendingResourceToPolicyRef[key] = resource.PolicyRef
	}

	return resource, nil
}

type emptyGroup struct{}

func (eg emptyGroup) BeginValueProposals(tx interface{}, groups []string) (config.ValueDeserializer, []config.ValueProposer, error) {
	subGroups := make([]config.ValueProposer, len(groups))
	for i := range subGroups {
		subGroups[i] = emptyGroup{}
	}
	return failDeserializer("sub-groups not allowed to have values"), subGroups, nil
}

// RollbackConfig a no-op
func (eg emptyGroup) RollbackProposals(tx interface{}) {}

// PreCommit is a no-op
func (eg emptyGroup) PreCommit(tx interface{}) error { return nil }

// CommitConfig is a no-op
func (eg emptyGroup) CommitProposals(tx interface{}) {}
