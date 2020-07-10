/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package service_test

import (
	"testing"
	"time"

	"github.com/hyperledger/fabric/gossip/election"

	"github.com/hyperledger/fabric/gossip/service"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

func TestGlobalConfig(t *testing.T) {
	viper.Reset()
	// Capture the configuration from viper
	viper.Set("peer.tls.enabled", true)
	viper.Set("peer.gossip.pvtData.pullRetryThreshold", "10s")
	viper.Set("peer.gossip.endpoint", "gossip_endpoint")
	viper.Set("peer.gossip.pvtData.pushAckTimeout", "20s")
	viper.Set("peer.gossip.nonBlockingCommitMode", true)
	viper.Set("peer.gossip.useLeaderElection", true)
	viper.Set("peer.gossip.orgLeader", true)
	viper.Set("peer.gossip.election.leaderAliveThreshold", "10m")
	viper.Set("peer.gossip.election.leaderElectionDuration", "5s")
	viper.Set("peer.gossip.pvtData.btlPullMargin", 15)
	viper.Set("peer.gossip.pvtData.transientstoreMaxBlockRetention", 1000)
	viper.Set("peer.gossip.pvtData.skipPullingInvalidTransactionsDuringCommit", false)

	coreConfig := service.GlobalConfig()

	expectedConfig := &service.ServiceConfig{
		PeerTLSEnabled:                             true,
		PvtDataPullRetryThreshold:                  10 * time.Second,
		Endpoint:                                   "gossip_endpoint",
		PvtDataPushAckTimeout:                      20 * time.Second,
		NonBlockingCommitMode:                      true,
		UseLeaderElection:                          true,
		OrgLeader:                                  true,
		ElectionLeaderAliveThreshold:               10 * time.Minute,
		ElectionLeaderElectionDuration:             5 * time.Second,
		ElectionStartupGracePeriod:                 election.DefStartupGracePeriod,
		ElectionMembershipSampleInterval:           election.DefMembershipSampleInterval,
		BtlPullMargin:                              15,
		TransientstoreMaxBlockRetention:            uint64(1000),
		SkipPullingInvalidTransactionsDuringCommit: false,
	}

	require.Equal(t, coreConfig, expectedConfig)
}
