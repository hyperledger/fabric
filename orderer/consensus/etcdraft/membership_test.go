/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package etcdraft_test

import (
	"testing"

	etcdraftproto "github.com/hyperledger/fabric-protos-go/orderer/etcdraft"
	"github.com/hyperledger/fabric/orderer/consensus/etcdraft"
	"github.com/stretchr/testify/require"
	"go.etcd.io/etcd/raft/raftpb"
)

func TestQuorumCheck(t *testing.T) {
	tests := []struct {
		Name          string
		NewConsenters map[uint64]*etcdraftproto.Consenter
		ConfChange    *raftpb.ConfChange
		RotateNode    uint64
		ActiveNodes   []uint64
		QuorumLoss    bool
	}{
		// Notations:
		//  1     - node 1 is alive
		// (1)    - node 1 is dead
		//  1'    - node 1's cert is being rotated. Node is considered to be dead in new set

		// Add
		{
			Name:          "[1]->[1,(2)]",
			NewConsenters: map[uint64]*etcdraftproto.Consenter{1: nil, 2: nil},
			ConfChange:    &raftpb.ConfChange{NodeID: 2, Type: raftpb.ConfChangeAddNode},
			ActiveNodes:   []uint64{1},
			QuorumLoss:    false,
		},
		{
			Name:          "[1,2]->[1,2,(3)]",
			NewConsenters: map[uint64]*etcdraftproto.Consenter{1: nil, 2: nil, 3: nil},
			ConfChange:    &raftpb.ConfChange{NodeID: 3, Type: raftpb.ConfChangeAddNode},
			ActiveNodes:   []uint64{1, 2},
			QuorumLoss:    false,
		},
		{
			Name:          "[1,2,(3)]->[1,2,(3),(4)]",
			NewConsenters: map[uint64]*etcdraftproto.Consenter{1: nil, 2: nil, 3: nil, 4: nil},
			ConfChange:    &raftpb.ConfChange{NodeID: 4, Type: raftpb.ConfChangeAddNode},
			ActiveNodes:   []uint64{1, 2},
			QuorumLoss:    true,
		},
		{
			Name:          "[1,2,3,(4)]->[1,2,3,(4),(5)]",
			NewConsenters: map[uint64]*etcdraftproto.Consenter{1: nil, 2: nil, 3: nil, 4: nil, 5: nil},
			ConfChange:    &raftpb.ConfChange{NodeID: 5, Type: raftpb.ConfChangeAddNode},
			ActiveNodes:   []uint64{1, 2, 3},
			QuorumLoss:    false,
		},
		// Rotate
		{
			Name:          "[1]->[1']",
			NewConsenters: map[uint64]*etcdraftproto.Consenter{1: nil},
			RotateNode:    1,
			ActiveNodes:   []uint64{1},
			QuorumLoss:    false,
		},
		{
			Name:          "[1,2]->[1,2']",
			NewConsenters: map[uint64]*etcdraftproto.Consenter{1: nil, 2: nil},
			RotateNode:    2,
			ActiveNodes:   []uint64{1, 2},
			QuorumLoss:    false,
		},
		{
			Name:          "[1,2,(3)]->[1,2',(3)]",
			NewConsenters: map[uint64]*etcdraftproto.Consenter{1: nil, 2: nil, 3: nil},
			RotateNode:    2,
			ActiveNodes:   []uint64{1, 2},
			QuorumLoss:    true,
		},
		{
			Name:          "[1,2,(3)]->[1,2,(3')]",
			NewConsenters: map[uint64]*etcdraftproto.Consenter{1: nil, 2: nil, 3: nil},
			RotateNode:    3,
			ActiveNodes:   []uint64{1, 2},
			QuorumLoss:    false,
		},
		// Remove
		{
			Name:          "[1,2,(3)]->[1,2]",
			NewConsenters: map[uint64]*etcdraftproto.Consenter{1: nil, 2: nil},
			ConfChange:    &raftpb.ConfChange{NodeID: 3, Type: raftpb.ConfChangeRemoveNode},
			ActiveNodes:   []uint64{1, 2},
			QuorumLoss:    false,
		},
		{
			Name:          "[1,2,(3)]->[1,(3)]",
			NewConsenters: map[uint64]*etcdraftproto.Consenter{1: nil, 3: nil},
			ConfChange:    &raftpb.ConfChange{NodeID: 2, Type: raftpb.ConfChangeRemoveNode},
			ActiveNodes:   []uint64{1, 2},
			QuorumLoss:    true,
		},
		{
			Name:          "[1,2]->[1]",
			NewConsenters: map[uint64]*etcdraftproto.Consenter{1: nil},
			ConfChange:    &raftpb.ConfChange{NodeID: 2, Type: raftpb.ConfChangeRemoveNode},
			ActiveNodes:   []uint64{1, 2},
			QuorumLoss:    false,
		},
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			changes := &etcdraft.MembershipChanges{
				NewConsenters: test.NewConsenters,
				ConfChange:    test.ConfChange,
				RotatedNode:   test.RotateNode,
			}

			require.Equal(t, test.QuorumLoss, changes.UnacceptableQuorumLoss(test.ActiveNodes))
		})
	}
}

func TestMembershipChanges(t *testing.T) {
	blockMetadata := &etcdraftproto.BlockMetadata{
		ConsenterIds:    []uint64{1, 2},
		NextConsenterId: 3,
	}
	c := []*etcdraftproto.Consenter{
		{ClientTlsCert: []byte("first")},
		{ClientTlsCert: []byte("second")},
		{ClientTlsCert: []byte("third")},
		{ClientTlsCert: []byte("4th")},
	}
	tests := []struct {
		Name                        string
		OldConsenters               map[uint64]*etcdraftproto.Consenter
		NewConsenters               []*etcdraftproto.Consenter
		Changes                     *etcdraft.MembershipChanges
		HaveError, Changed, Rotated bool
	}{
		{
			Name:          "Add a node",
			OldConsenters: map[uint64]*etcdraftproto.Consenter{1: c[0], 2: c[1]},
			NewConsenters: []*etcdraftproto.Consenter{c[0], c[1], c[2]},
			Changes: &etcdraft.MembershipChanges{
				NewBlockMetadata: &etcdraftproto.BlockMetadata{
					ConsenterIds:    []uint64{1, 2, 3},
					NextConsenterId: 4,
				},
				NewConsenters: map[uint64]*etcdraftproto.Consenter{1: c[0], 2: c[1], 3: c[2]},
				AddedNodes:    []*etcdraftproto.Consenter{c[2]},
				RemovedNodes:  []*etcdraftproto.Consenter{},
				ConfChange: &raftpb.ConfChange{
					NodeID: 3,
					Type:   raftpb.ConfChangeAddNode,
				},
			},
			HaveError: false,
			Changed:   true,
			Rotated:   false,
		},
		{
			Name:          "Remove a node",
			OldConsenters: map[uint64]*etcdraftproto.Consenter{1: c[0], 2: c[1]},
			NewConsenters: []*etcdraftproto.Consenter{c[1]},
			Changes: &etcdraft.MembershipChanges{
				NewBlockMetadata: &etcdraftproto.BlockMetadata{
					ConsenterIds:    []uint64{2},
					NextConsenterId: 3,
				},
				NewConsenters: map[uint64]*etcdraftproto.Consenter{2: c[1]},
				AddedNodes:    []*etcdraftproto.Consenter{},
				RemovedNodes:  []*etcdraftproto.Consenter{c[0]},
				ConfChange: &raftpb.ConfChange{
					NodeID: 1,
					Type:   raftpb.ConfChangeRemoveNode,
				},
			},
			HaveError: false,
			Changed:   true,
			Rotated:   false,
		},
		{
			Name:          "Rotate a certificate",
			OldConsenters: map[uint64]*etcdraftproto.Consenter{1: c[0], 2: c[1]},
			NewConsenters: []*etcdraftproto.Consenter{c[0], c[2]},
			Changes: &etcdraft.MembershipChanges{
				NewBlockMetadata: &etcdraftproto.BlockMetadata{
					ConsenterIds:    []uint64{1, 2},
					NextConsenterId: 3,
				},
				NewConsenters: map[uint64]*etcdraftproto.Consenter{1: c[0], 2: c[2]},
				AddedNodes:    []*etcdraftproto.Consenter{c[2]},
				RemovedNodes:  []*etcdraftproto.Consenter{c[1]},
				RotatedNode:   2,
			},
			HaveError: false,
			Changed:   true,
			Rotated:   true,
		},
		{
			Name:          "No change",
			OldConsenters: map[uint64]*etcdraftproto.Consenter{1: c[0], 2: c[1]},
			NewConsenters: []*etcdraftproto.Consenter{c[0], c[1]},
			Changes: &etcdraft.MembershipChanges{
				NewBlockMetadata: &etcdraftproto.BlockMetadata{
					ConsenterIds:    []uint64{1, 2},
					NextConsenterId: 3,
				},
				NewConsenters: map[uint64]*etcdraftproto.Consenter{1: c[0], 2: c[1]},
				AddedNodes:    []*etcdraftproto.Consenter{},
				RemovedNodes:  []*etcdraftproto.Consenter{},
			},
			HaveError: false,
			Changed:   false,
			Rotated:   false,
		},
		{
			Name:          "Too many adds",
			OldConsenters: map[uint64]*etcdraftproto.Consenter{1: c[0], 2: c[1]},
			NewConsenters: []*etcdraftproto.Consenter{c[0], c[1], c[2], c[3]},
			Changes:       nil,
			HaveError:     true,
		},
		{
			Name:          "Too many removes",
			OldConsenters: map[uint64]*etcdraftproto.Consenter{1: c[0], 2: c[1]},
			NewConsenters: []*etcdraftproto.Consenter{c[2]},
			Changes:       nil,
			HaveError:     true,
		},
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			changes, err := etcdraft.ComputeMembershipChanges(blockMetadata, test.OldConsenters, test.NewConsenters)

			if test.HaveError {
				require.NotNil(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, changes, test.Changes)
				require.Equal(t, changes.Changed(), test.Changed)
				require.Equal(t, changes.Rotated(), test.Rotated)
			}
		})
	}
}
