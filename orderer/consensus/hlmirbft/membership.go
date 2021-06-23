/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package hlmirbft

import (
	"fmt"

	"github.com/fly2plan/fabric-protos-go/orderer/hlmirbft"

	"github.com/golang/protobuf/proto"
	"github.com/pkg/errors"
)

// MembershipByCert convert consenters map into set encapsulated by map
// where key is client TLS certificate
func MembershipByCert(consenters map[uint64]*hlmirbft.Consenter) map[string]uint64 {
	set := map[string]uint64{}
	for nodeID, c := range consenters {
		set[string(c.ClientTlsCert)] = nodeID
	}
	return set
}

type Type int

const (
	ConfChangeRemoveNode Type = iota
	ConfChangeAddNode
)

type ConfChange struct {
	NodeID uint64
	Type   Type
}

// MembershipChanges keeps information about membership
// changes introduced during configuration update
type MembershipChanges struct {
	NewBlockMetadata *hlmirbft.BlockMetadata
	NewConsenters    map[uint64]*hlmirbft.Consenter
	AddedNodes       []*hlmirbft.Consenter
	RemovedNodes     []*hlmirbft.Consenter
	ConfChange       *ConfChange
	RotatedNode      uint64
}

// ComputeMembershipChanges computes membership update based on information about new consenters, returns
// two slices: a slice of added consenters and a slice of consenters to be removed
func ComputeMembershipChanges(oldMetadata *hlmirbft.BlockMetadata, oldConsenters map[uint64]*hlmirbft.Consenter, newConsenters []*hlmirbft.Consenter) (mc *MembershipChanges, err error) {
	result := &MembershipChanges{
		NewConsenters:    map[uint64]*hlmirbft.Consenter{},
		NewBlockMetadata: proto.Clone(oldMetadata).(*hlmirbft.BlockMetadata),
		AddedNodes:       []*hlmirbft.Consenter{},
		RemovedNodes:     []*hlmirbft.Consenter{},
	}

	result.NewBlockMetadata.ConsenterIds = make([]uint64, len(newConsenters))

	var addedNodeIndex int
	currentConsentersSet := MembershipByCert(oldConsenters)
	for i, c := range newConsenters {
		if nodeID, exists := currentConsentersSet[string(c.ClientTlsCert)]; exists {
			result.NewBlockMetadata.ConsenterIds[i] = nodeID
			result.NewConsenters[nodeID] = c
			continue
		}
		addedNodeIndex = i
		result.AddedNodes = append(result.AddedNodes, c)
	}

	var deletedNodeID uint64
	newConsentersSet := ConsentersToMap(newConsenters)
	for nodeID, c := range oldConsenters {
		if !newConsentersSet.Exists(c) {
			result.RemovedNodes = append(result.RemovedNodes, c)
			deletedNodeID = nodeID
		}
	}

	switch {
	case len(result.AddedNodes) == 1 && len(result.RemovedNodes) == 1:
		// A cert is considered being rotated, iff exact one new node is being added
		// AND exact one existing node is being removed
		result.RotatedNode = deletedNodeID
		result.NewBlockMetadata.ConsenterIds[addedNodeIndex] = deletedNodeID
		result.NewConsenters[deletedNodeID] = result.AddedNodes[0]
	case len(result.AddedNodes) == 1 && len(result.RemovedNodes) == 0:
		// new node
		nodeID := result.NewBlockMetadata.NextConsenterId
		result.NewConsenters[nodeID] = result.AddedNodes[0]
		result.NewBlockMetadata.ConsenterIds[addedNodeIndex] = nodeID
		result.NewBlockMetadata.NextConsenterId++
		result.ConfChange = &ConfChange{
			NodeID: nodeID,
			Type:   ConfChangeAddNode,
		}
	case len(result.AddedNodes) == 0 && len(result.RemovedNodes) == 1:
		// removed node
		nodeID := deletedNodeID
		result.ConfChange = &ConfChange{
			NodeID: nodeID,
			Type:   ConfChangeRemoveNode,
		}
		delete(result.NewConsenters, nodeID)
	case len(result.AddedNodes) == 0 && len(result.RemovedNodes) == 0:
		// no change
	default:
		// len(result.AddedNodes) > 1 || len(result.RemovedNodes) > 1 {
		return nil, errors.Errorf("update of more than one consenter at a time is not supported, requested changes: %s", result)
	}

	return result, nil
}

// Stringer implements fmt.Stringer interface
func (mc *MembershipChanges) String() string {
	return fmt.Sprintf("add %d node(s), remove %d node(s)", len(mc.AddedNodes), len(mc.RemovedNodes))
}

// Changed indicates whether these changes actually do anything
func (mc *MembershipChanges) Changed() bool {
	return len(mc.AddedNodes) > 0 || len(mc.RemovedNodes) > 0
}

// Rotated indicates whether the change was a rotation
func (mc *MembershipChanges) Rotated() bool {
	return len(mc.AddedNodes) == 1 && len(mc.RemovedNodes) == 1
}

// UnacceptableQuorumLoss returns true if membership change will result in avoidable quorum loss.
// There is no need to check for the number of active nodes since we assert that there are at most f byzantine nodes.
// By this assertion the remaining nodes are active and configuration consensus will be reached.
func (mc *MembershipChanges) UnacceptableQuorumLoss() bool {
	isBFT := len(mc.NewConsenters) > 3 // if resulting cluster is not byzantine fault tolerant, quorum correctness in not guaranteed

	switch {
	case mc.ConfChange != nil && mc.ConfChange.Type == ConfChangeAddNode: // Add
		return !isBFT

	case mc.RotatedNode != 0: // Rotate
		return !isBFT

	case mc.ConfChange != nil && mc.ConfChange.Type == ConfChangeRemoveNode: // Remove
		return !isBFT

	default: // No change
		return false
	}
}
