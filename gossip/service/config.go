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

const (
	btlPullMarginDefault           = 10
	transientBlockRetentionDefault = 1000
)

// ServiceConfig is the config struct for gossip services
type ServiceConfig struct {
	// PeerTLSEnabled enables/disables Peer TLS.
	PeerTLSEnabled bool
	// Endpoint which overrides the endpoint the peer publishes to peers in its organization.
	Endpoint              string
	NonBlockingCommitMode bool
	// UseLeaderElection defines whenever peer will initialize dynamic algorithm for "leader" selection.
	UseLeaderElection bool
	// OrgLeader statically defines peer to be an organization "leader".
	OrgLeader bool
	// ElectionStartupGracePeriod is the longest time peer waits for stable membership during leader
	// election startup (unit: second).
	ElectionStartupGracePeriod time.Duration
	// ElectionMembershipSampleInterval is the time interval for gossip membership samples to check its stability (unit: second).
	ElectionMembershipSampleInterval time.Duration
	// ElectionLeaderAliveThreshold is the time passes since last declaration message before peer decides to
	// perform leader election (unit: second).
	ElectionLeaderAliveThreshold time.Duration
	// ElectionLeaderElectionDuration is the time passes since last declaration message before peer decides to perform
	// leader election (unit: second).
	ElectionLeaderElectionDuration time.Duration
	// PvtDataPullRetryThreshold determines the maximum duration of time private data corresponding for
	// a given block.
	PvtDataPullRetryThreshold time.Duration
	// PvtDataPushAckTimeout is the maximum time to wait for the acknoledgement from each peer at private
	// data push at endorsement time.
	PvtDataPushAckTimeout time.Duration
	// BtlPullMargin is the block to live pulling margin, used as a buffer to prevent peer from trying to pull private data
	// from peers that is soon to be purged in next N blocks.
	BtlPullMargin uint64
	// TransientstoreMaxBlockRetention defines the maximum difference between the current ledger's height upon commit,
	// and the private data residing inside the transient store that is guaranteed not to be purged.
	TransientstoreMaxBlockRetention uint64
	// SkipPullingInvalidTransactionsDuringCommit is a flag that indicates whether pulling of invalid
	// transaction's private data from other peers need to be skipped during the commit time and pulled
	// only through reconciler.
	SkipPullingInvalidTransactionsDuringCommit bool
}

func GlobalConfig() *ServiceConfig {
	c := &ServiceConfig{}
	c.loadGossipConfig()
	return c
}

func (c *ServiceConfig) loadGossipConfig() {
	c.PeerTLSEnabled = viper.GetBool("peer.tls.enabled")
	c.Endpoint = viper.GetString("peer.gossip.endpoint")
	c.NonBlockingCommitMode = viper.GetBool("peer.gossip.nonBlockingCommitMode")
	c.UseLeaderElection = viper.GetBool("peer.gossip.useLeaderElection")
	c.OrgLeader = viper.GetBool("peer.gossip.orgLeader")

	c.ElectionStartupGracePeriod = util.GetDurationOrDefault("peer.gossip.election.startupGracePeriod", election.DefStartupGracePeriod)
	c.ElectionMembershipSampleInterval = util.GetDurationOrDefault("peer.gossip.election.membershipSampleInterval", election.DefMembershipSampleInterval)
	c.ElectionLeaderAliveThreshold = util.GetDurationOrDefault("peer.gossip.election.leaderAliveThreshold", election.DefLeaderAliveThreshold)
	c.ElectionLeaderElectionDuration = util.GetDurationOrDefault("peer.gossip.election.leaderElectionDuration", election.DefLeaderElectionDuration)

	c.PvtDataPushAckTimeout = viper.GetDuration("peer.gossip.pvtData.pushAckTimeout")
	c.PvtDataPullRetryThreshold = viper.GetDuration("peer.gossip.pvtData.pullRetryThreshold")
	c.SkipPullingInvalidTransactionsDuringCommit = viper.GetBool("peer.gossip.pvtData.skipPullingInvalidTransactionsDuringCommit")

	c.BtlPullMargin = btlPullMarginDefault
	if viper.IsSet("peer.gossip.pvtData.btlPullMargin") {
		btlMarginVal := viper.GetInt("peer.gossip.pvtData.btlPullMargin")
		if btlMarginVal >= 0 {
			c.BtlPullMargin = uint64(btlMarginVal)
		}
	}

	c.TransientstoreMaxBlockRetention = uint64(viper.GetInt("peer.gossip.pvtData.transientstoreMaxBlockRetention"))
	if c.TransientstoreMaxBlockRetention == 0 {
		logger.Warning("Configuration key peer.gossip.pvtData.transientstoreMaxBlockRetention isn't set, defaulting to", transientBlockRetentionDefault)
		c.TransientstoreMaxBlockRetention = transientBlockRetentionDefault
	}
}
