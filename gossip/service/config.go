/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package service

import (
	"time"

	"github.com/hyperledger/fabric/gossip/election"
	"github.com/hyperledger/fabric/gossip/util"
	"github.com/spf13/viper"
)

const btlPullMarginDefault = 10

// ServiceConfig is the config struct for gossip services
type ServiceConfig struct {
	Endpoint                         string
	PvtDataPullRetryThreshold        time.Duration
	PvtDataPushAckTimeout            time.Duration
	NonBlockingCommitMode            bool
	UseLeaderElection                bool
	OrgLeader                        bool
	ElectionStartupGracePeriod       time.Duration
	ElectionMembershipSampleInterval time.Duration
	ElectionLeaderAliveThreshold     time.Duration
	ElectionLeaderElectionDuration   time.Duration
	BtlPullMargin                    uint64
}

func GlobalConfig() *ServiceConfig {
	c := &ServiceConfig{}
	c.loadGossipConfig()
	return c
}

func (c *ServiceConfig) loadGossipConfig() {

	c.PvtDataPullRetryThreshold = viper.GetDuration("peer.gossip.pvtData.pullRetryThreshold")
	c.Endpoint = viper.GetString("peer.gossip.endpoint")
	c.PvtDataPushAckTimeout = viper.GetDuration("peer.gossip.pvtData.pushAckTimeout")
	c.NonBlockingCommitMode = viper.GetBool("peer.gossip.nonBlockingCommitMode")
	c.UseLeaderElection = viper.GetBool("peer.gossip.useLeaderElection")
	c.OrgLeader = viper.GetBool("peer.gossip.orgLeader")

	c.ElectionStartupGracePeriod = util.GetDurationOrDefault("peer.gossip.election.startupGracePeriod", election.DefStartupGracePeriod)
	c.ElectionMembershipSampleInterval = util.GetDurationOrDefault("peer.gossip.election.membershipSampleInterval", election.DefMembershipSampleInterval)
	c.ElectionLeaderAliveThreshold = util.GetDurationOrDefault("peer.gossip.election.leaderAliveThreshold", election.DefLeaderAliveThreshold)
	c.ElectionLeaderElectionDuration = util.GetDurationOrDefault("peer.gossip.election.leaderElectionDuration", election.DefLeaderElectionDuration)

	c.BtlPullMargin = btlPullMarginDefault
	if viper.IsSet("peer.gossip.pvtData.btlPullMargin") {
		btlMarginVal := viper.GetInt("peer.gossip.pvtData.btlPullMargin")
		if btlMarginVal >= 0 {
			c.BtlPullMargin = uint64(btlMarginVal)
		}
	}
}
