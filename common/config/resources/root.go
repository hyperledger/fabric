/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package resources

import (
	"fmt"
	"sync/atomic"

	"github.com/hyperledger/fabric/common/config"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/flogging"
)

var logger = flogging.MustGetLogger("common/config/resource")

// PolicyMapper is an interface for
type PolicyMapper interface {
	// PolicyRefForResource takes the name of a resource, and returns the policy name for a resource
	// or the empty string is the resource is not found
	PolicyRefForResource(resourceName string) string
}

type valueProposerRoot struct {
	resourceToPolicyRefMap atomic.Value
}

// newValueProposerRoot creates a new instance of the resources config root
func newValueProposerRoot() *valueProposerRoot {
	vpr := &valueProposerRoot{}
	vpr.resourceToPolicyRefMap.Store(map[string]string{})
	return vpr
}

type failDeserializer string

func (fd failDeserializer) Deserialize(key string, value []byte) (proto.Message, error) {
	return nil, fmt.Errorf(string(fd))
}

// BeginValueProposals is used to start a new config proposal
func (vpr *valueProposerRoot) BeginValueProposals(tx interface{}, groups []string) (config.ValueDeserializer, []config.ValueProposer, error) {
	if len(groups) != 1 {
		return nil, nil, fmt.Errorf("Root config only supports having one base group")
	}
	if groups[0] != RootGroupKey {
		return nil, nil, fmt.Errorf("Root group must be %s", RootGroupKey)
	}
	return failDeserializer("Programming error, this should never be invoked"), []config.ValueProposer{newResourceGroup(vpr)}, nil
}

// RollbackConfig a no-op
func (vpr *valueProposerRoot) RollbackProposals(tx interface{}) {}

// PreCommit is a no-op
func (vpr *valueProposerRoot) PreCommit(tx interface{}) error { return nil }

// CommitConfig a no-op
func (vpr *valueProposerRoot) CommitProposals(tx interface{}) {}

// PolicyRefForResources implements the PolicyMapper interface
func (vpr *valueProposerRoot) PolicyRefForResource(resourceName string) string {
	return vpr.resourceToPolicyRefMap.Load().(map[string]string)[resourceName]
}

// updatePolicyRefForResources should be called to update the policyRefForResources map
// it wraps the operation in the atomic package for thread safety.
func (vpr *valueProposerRoot) updatePolicyRefForResources(newMap map[string]string) {
	logger.Debugf("Updating policy ref map for %p", vpr)
	vpr.resourceToPolicyRefMap.Store(newMap)
}
