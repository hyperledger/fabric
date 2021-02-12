/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package state

import (
	"time"

	"github.com/spf13/viper"
)

const (
	DefStateCheckInterval   = 10 * time.Second
	DefStateResponseTimeout = 3 * time.Second
	DefStateBatchSize       = 10
	DefStateMaxRetries      = 3
	DefStateBlockBufferSize = 20
	DefStateChannelSize     = 100
	DefStateEnabled         = false
)

type StateConfig struct {
	StateCheckInterval   time.Duration
	StateResponseTimeout time.Duration
	StateBatchSize       uint64
	StateMaxRetries      int
	StateBlockBufferSize int
	StateChannelSize     int
	StateEnabled         bool
	UseLeaderElection    bool
	OrgLeader            bool
}

func GlobalConfig() *StateConfig {
	c := &StateConfig{}
	c.loadStateConfig()
	return c
}

func (c *StateConfig) loadStateConfig() {
	c.StateCheckInterval = DefStateCheckInterval
	if viper.IsSet("peer.gossip.state.checkInterval") {
		c.StateCheckInterval = viper.GetDuration("peer.gossip.state.checkInterval")
	}
	c.StateResponseTimeout = DefStateResponseTimeout
	if viper.IsSet("peer.gossip.state.responseTimeout") {
		c.StateResponseTimeout = viper.GetDuration("peer.gossip.state.responseTimeout")
	}
	c.StateBatchSize = DefStateBatchSize
	if viper.IsSet("peer.gossip.state.batchSize") {
		c.StateBatchSize = uint64(viper.GetInt("peer.gossip.state.batchSize"))
	}
	c.StateMaxRetries = DefStateMaxRetries
	if viper.IsSet("peer.gossip.state.maxRetries") {
		c.StateMaxRetries = viper.GetInt("peer.gossip.state.maxRetries")
	}
	c.StateBlockBufferSize = DefStateBlockBufferSize
	if viper.IsSet("peer.gossip.state.blockBufferSize") {
		c.StateBlockBufferSize = viper.GetInt("peer.gossip.state.blockBufferSize")
	}
	c.StateChannelSize = DefStateChannelSize
	if viper.IsSet("peer.gossip.state.channelSize") {
		c.StateChannelSize = viper.GetInt("peer.gossip.state.channelSize")
	}
	c.StateEnabled = DefStateEnabled
	if viper.IsSet("peer.gossip.state.enabled") {
		c.StateEnabled = viper.GetBool("peer.gossip.state.enabled")
	}
	// The below two configuration parameters are used for straggler() which warns
	// if our peer is lagging behind the rest and has no way to catch up.
	c.UseLeaderElection = viper.GetBool("peer.gossip.useLeaderElection")
	c.OrgLeader = viper.GetBool("peer.gossip.orgLeader")
}
