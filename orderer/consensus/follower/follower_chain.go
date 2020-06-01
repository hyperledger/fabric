/*
Copyright IBM Corp. 2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package follower

import (
	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric/orderer/common/types"
)

//TODO skeleton

// Chain implements a component that allows the orderer to follow a specific channel when is not a cluster member,
// that is, be a "follower" of the cluster. This means that the current orderer is not a member of the consenters set
// of the channel, and is only pulling blocks from other orderers.
//
// The follower is inspecting config blocks as they are pulled and if it discovers that it was introduced into the
// consenters set, it will trigger the creation of a regular etcdraft.Chain, that is, turn into a "member" of the
// cluster.
//
// The follower is started in one of two ways: 1) following an API Join request with a join-block that does not include
// the orderer in its conseters set, or 2) when the orderer was a cluster member and was removed from the consenters
// set.
//
// The follower is in status "onboarding" when it pulls blocks below the join-block number, or "active" when it
// pulls blocks equal or above the join-block number.
type Chain struct {
	Err error
	//TODO skeleton
}

func (c *Chain) Order(_ *common.Envelope, _ uint64) error {
	return c.Err
}

func (c *Chain) Configure(_ *common.Envelope, _ uint64) error {
	return c.Err
}

func (c *Chain) WaitReady() error {
	return c.Err
}

func (*Chain) Errored() <-chan struct{} {
	closedChannel := make(chan struct{})
	close(closedChannel)
	return closedChannel
}

func (c *Chain) Start() {
	//TODO skeleton
}

func (c *Chain) Halt() {
	//TODO skeleton
}

// StatusReport returns the ClusterRelation & Status
func (c *Chain) StatusReport() (types.ClusterRelation, types.Status) {
	status := types.StatusActive
	//TODO if (height is >= join-block.height) return "active"; else return "onboarding"
	return types.ClusterRelationFollower, status
}
