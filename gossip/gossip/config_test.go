/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package gossip_test

import (
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/hyperledger/fabric/gossip/discovery"
	"github.com/hyperledger/fabric/gossip/election"

	"github.com/hyperledger/fabric/gossip/comm"
	"github.com/hyperledger/fabric/gossip/gossip/algo"

	"github.com/hyperledger/fabric/gossip/gossip"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

func TestGlobalConfig(t *testing.T) {
	viper.Reset()
	endpoint := "0.0.0.0:7051"
	externalEndpoint := "0.0.0.0:7052"
	bootstrap := []string{"bootstrap1", "bootstrap2", "bootstrap3"}
	// Capture the configuration from viper
	viper.Set("peer.gossip.maxBlockCountToStore", 1)
	viper.Set("peer.gossip.maxPropagationBurstLatency", "2s")
	viper.Set("peer.gossip.maxPropagationBurstSize", 3)
	viper.Set("peer.gossip.propagateIterations", 4)
	viper.Set("peer.gossip.propagatePeerNum", 5)
	viper.Set("peer.gossip.pullInterval", "6s")
	viper.Set("peer.gossip.pullPeerNum", 7)
	viper.Set("peer.gossip.endpoint", endpoint)
	viper.Set("peer.gossip.externalEndpoint", externalEndpoint)
	viper.Set("peer.gossip.publishCertPeriod", "8s")
	viper.Set("peer.gossip.requestStateInfoInterval", "9s")
	viper.Set("peer.gossip.publishStateInfoInterval", "10s")
	viper.Set("peer.gossip.skipBlockVerification", true)
	viper.Set("peer.gossip.membershipTrackerInterval", "11s")
	viper.Set("peer.gossip.digestWaitTime", "12s")
	viper.Set("peer.gossip.requestWaitTime", "13s")
	viper.Set("peer.gossip.responseWaitTime", "14s")
	viper.Set("peer.gossip.dialTimeout", "15s")
	viper.Set("peer.gossip.connTimeout", "16s")
	viper.Set("peer.gossip.recvBuffSize", 17)
	viper.Set("peer.gossip.sendBuffSize", 18)
	viper.Set("peer.gossip.election.leaderAliveThreshold", "19s")
	viper.Set("peer.gossip.aliveTimeInterval", "20s")
	viper.Set("peer.gossip.aliveExpirationTimeout", "21s")
	viper.Set("peer.gossip.reconnectInterval", "22s")
	viper.Set("peer.gossip.maxConnectionAttempts", "100")
	viper.Set("peer.gossip.msgExpirationFactor", "10")

	coreConfig, err := gossip.GlobalConfig(endpoint, nil, bootstrap...)
	require.NoError(t, err)

	_, p, err := net.SplitHostPort(endpoint)
	require.NoError(t, err)
	port, err := strconv.ParseInt(p, 10, 64)
	require.NoError(t, err)

	expectedConfig := &gossip.Config{
		BindPort:                     int(port),
		BootstrapPeers:               []string{"bootstrap1", "bootstrap2", "bootstrap3"},
		ID:                           endpoint,
		MaxBlockCountToStore:         1,
		MaxPropagationBurstLatency:   2 * time.Second,
		MaxPropagationBurstSize:      3,
		PropagateIterations:          4,
		PropagatePeerNum:             5,
		PullInterval:                 6 * time.Second,
		PullPeerNum:                  7,
		InternalEndpoint:             endpoint,
		ExternalEndpoint:             externalEndpoint,
		PublishCertPeriod:            8 * time.Second,
		RequestStateInfoInterval:     9 * time.Second,
		PublishStateInfoInterval:     10 * time.Second,
		SkipBlockVerification:        true,
		TLSCerts:                     nil,
		TimeForMembershipTracker:     11 * time.Second,
		DigestWaitTime:               12 * time.Second,
		RequestWaitTime:              13 * time.Second,
		ResponseWaitTime:             14 * time.Second,
		DialTimeout:                  15 * time.Second,
		ConnTimeout:                  16 * time.Second,
		RecvBuffSize:                 17,
		SendBuffSize:                 18,
		MsgExpirationTimeout:         19 * time.Second * 10, // LeaderAliveThreshold * 10
		AliveTimeInterval:            20 * time.Second,
		AliveExpirationTimeout:       21 * time.Second,
		AliveExpirationCheckInterval: 21 * time.Second / 10, // AliveExpirationTimeout / 10
		ReconnectInterval:            22 * time.Second,
		MaxConnectionAttempts:        100,
		MsgExpirationFactor:          10,
	}

	require.Equal(t, expectedConfig, coreConfig)
}

func TestGlobalConfigDefaults(t *testing.T) {
	viper.Reset()
	endpoint := "0.0.0.0:7051"
	externalEndpoint := "0.0.0.0:7052"
	bootstrap := []string{"bootstrap1", "bootstrap2", "bootstrap3"}
	viper.Set("peer.gossip.endpoint", endpoint)
	viper.Set("peer.gossip.externalEndpoint", externalEndpoint)

	coreConfig, err := gossip.GlobalConfig(endpoint, nil, bootstrap...)
	require.NoError(t, err)

	_, p, err := net.SplitHostPort(endpoint)
	require.NoError(t, err)
	port, err := strconv.ParseInt(p, 10, 64)
	require.NoError(t, err)

	expectedConfig := &gossip.Config{
		BindPort:                     int(port),
		BootstrapPeers:               []string{"bootstrap1", "bootstrap2", "bootstrap3"},
		ID:                           endpoint,
		MaxBlockCountToStore:         10,
		MaxPropagationBurstLatency:   10 * time.Millisecond,
		MaxPropagationBurstSize:      10,
		PropagateIterations:          1,
		PropagatePeerNum:             3,
		PullInterval:                 4 * time.Second,
		PullPeerNum:                  3,
		InternalEndpoint:             endpoint,
		ExternalEndpoint:             externalEndpoint,
		PublishCertPeriod:            10 * time.Second,
		RequestStateInfoInterval:     4 * time.Second,
		PublishStateInfoInterval:     4 * time.Second,
		SkipBlockVerification:        false,
		TLSCerts:                     nil,
		TimeForMembershipTracker:     5 * time.Second,
		DigestWaitTime:               algo.DefDigestWaitTime,
		RequestWaitTime:              algo.DefRequestWaitTime,
		ResponseWaitTime:             algo.DefResponseWaitTime,
		DialTimeout:                  comm.DefDialTimeout,
		ConnTimeout:                  comm.DefConnTimeout,
		RecvBuffSize:                 comm.DefRecvBuffSize,
		SendBuffSize:                 comm.DefSendBuffSize,
		MsgExpirationTimeout:         election.DefLeaderAliveThreshold * 10,
		AliveTimeInterval:            discovery.DefAliveTimeInterval,
		AliveExpirationTimeout:       5 * discovery.DefAliveTimeInterval,
		AliveExpirationCheckInterval: 5 * discovery.DefAliveTimeInterval / 10,
		ReconnectInterval:            5 * discovery.DefAliveTimeInterval,
		MaxConnectionAttempts:        120,
		MsgExpirationFactor:          20,
	}

	require.Equal(t, expectedConfig, coreConfig)
}
