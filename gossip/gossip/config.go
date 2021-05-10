/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package gossip

import (
	"net"
	"strconv"
	"time"

	"github.com/hyperledger/fabric/gossip/comm"
	"github.com/hyperledger/fabric/gossip/common"
	"github.com/hyperledger/fabric/gossip/discovery"
	"github.com/hyperledger/fabric/gossip/election"
	"github.com/hyperledger/fabric/gossip/gossip/algo"
	"github.com/hyperledger/fabric/gossip/util"
	"github.com/spf13/viper"
)

// Config is the configuration of the gossip component
type Config struct {
	// BindPort is the port that gossip bind to, used only for tests.
	BindPort int
	// Id of the specific gossip instance.
	ID string
	// BootstrapPeers are peers we connect to at startup.
	BootstrapPeers []string
	// PropagateIterations is the number of times a message is pushed to remote peers.
	PropagateIterations int
	// PropagatePeerNum is the number of peers selected to push messages to.
	PropagatePeerNum int

	// MaxBlockCountToStore is the maximum count of blocks we store in memory.
	MaxBlockCountToStore int

	// MaxPropagationBurstSize is the max number of messages stored until it triggers a push to remote peers.
	MaxPropagationBurstSize int
	// MaxPropagationBurstLatency is the max time between consecutive message pushes.
	MaxPropagationBurstLatency time.Duration

	// PullInterval determines frequency of pull phases.
	PullInterval time.Duration
	// PullPeerNum is the number of peers to pull from.
	PullPeerNum int

	// SkipBlockVerification controls either we skip verifying block message or not.
	SkipBlockVerification bool

	// PublishCertPeriod is the time from startup certificates are included in Alive message.
	PublishCertPeriod time.Duration
	// PublishStateInfoInterval determines frequency of pushing state info messages to peers.
	PublishStateInfoInterval time.Duration
	// RequestStateInfoInterval determines frequency of pulling state info messages from peers.
	RequestStateInfoInterval time.Duration

	// TLSCerts is the TLS certificates of the peer.
	TLSCerts *common.TLSCertificates

	// InternalEndpoint is the endpoint we publish to peers in our organization.
	InternalEndpoint string
	// ExternalEndpoint is the peer publishes this endpoint instead of selfEndpoint to foreign organizations.
	ExternalEndpoint string
	// TimeForMembershipTracker determines time for polling with membershipTracker.
	TimeForMembershipTracker time.Duration

	// DigestWaitTime is the time to wait before pull engine processes incoming digests.
	DigestWaitTime time.Duration
	// RequestWaitTime is the time to wait before pull engine removes incoming nonce.
	RequestWaitTime time.Duration
	// ResponseWaitTime is the time to wait before pull engine ends pull.
	ResponseWaitTime time.Duration

	// DialTimeout indicate Dial timeout.
	DialTimeout time.Duration
	// ConnTimeout indicate Connection timeout.
	ConnTimeout time.Duration
	// RecvBuffSize is the buffer size of received message.
	RecvBuffSize int
	// SendBuffSize is the buffer size of sending message.
	SendBuffSize int

	// MsgExpirationTimeout indicate leadership message expiration timeout.
	MsgExpirationTimeout time.Duration

	// AliveTimeInterval is the alive check interval.
	AliveTimeInterval time.Duration
	// AliveExpirationTimeout is the alive expiration timeout.
	AliveExpirationTimeout time.Duration
	// AliveExpirationCheckInterval is the alive expiration check interval.
	AliveExpirationCheckInterval time.Duration
	// ReconnectInterval is the Reconnect interval.
	ReconnectInterval time.Duration
	// MsgExpirationFactor is the expiration factor for alive message TTL
	MsgExpirationFactor int
	// MaxConnectionAttempts is the max number of attempts to connect to a peer (wait for alive ack)
	MaxConnectionAttempts int
}

// GlobalConfig builds a Config from the given endpoint, certificate and bootstrap peers.
func GlobalConfig(endpoint string, certs *common.TLSCertificates, bootPeers ...string) (*Config, error) {
	c := &Config{}
	err := c.loadConfig(endpoint, certs, bootPeers...)
	return c, err
}

func (c *Config) loadConfig(endpoint string, certs *common.TLSCertificates, bootPeers ...string) error {
	_, p, err := net.SplitHostPort(endpoint)
	if err != nil {
		return err
	}
	port, err := strconv.ParseInt(p, 10, 64)
	if err != nil {
		return err
	}

	c.BindPort = int(port)
	c.BootstrapPeers = bootPeers
	c.ID = endpoint
	c.MaxBlockCountToStore = util.GetIntOrDefault("peer.gossip.maxBlockCountToStore", 10)
	c.MaxPropagationBurstLatency = util.GetDurationOrDefault("peer.gossip.maxPropagationBurstLatency", 10*time.Millisecond)
	c.MaxPropagationBurstSize = util.GetIntOrDefault("peer.gossip.maxPropagationBurstSize", 10)
	c.PropagateIterations = util.GetIntOrDefault("peer.gossip.propagateIterations", 1)
	c.PropagatePeerNum = util.GetIntOrDefault("peer.gossip.propagatePeerNum", 3)
	c.PullInterval = util.GetDurationOrDefault("peer.gossip.pullInterval", 4*time.Second)
	c.PullPeerNum = util.GetIntOrDefault("peer.gossip.pullPeerNum", 3)
	c.InternalEndpoint = endpoint
	c.ExternalEndpoint = viper.GetString("peer.gossip.externalEndpoint")
	c.PublishCertPeriod = util.GetDurationOrDefault("peer.gossip.publishCertPeriod", 10*time.Second)
	c.RequestStateInfoInterval = util.GetDurationOrDefault("peer.gossip.requestStateInfoInterval", 4*time.Second)
	c.PublishStateInfoInterval = util.GetDurationOrDefault("peer.gossip.publishStateInfoInterval", 4*time.Second)
	c.SkipBlockVerification = viper.GetBool("peer.gossip.skipBlockVerification")
	c.TLSCerts = certs
	c.TimeForMembershipTracker = util.GetDurationOrDefault("peer.gossip.membershipTrackerInterval", 5*time.Second)
	c.DigestWaitTime = util.GetDurationOrDefault("peer.gossip.digestWaitTime", algo.DefDigestWaitTime)
	c.RequestWaitTime = util.GetDurationOrDefault("peer.gossip.requestWaitTime", algo.DefRequestWaitTime)
	c.ResponseWaitTime = util.GetDurationOrDefault("peer.gossip.responseWaitTime", algo.DefResponseWaitTime)
	c.DialTimeout = util.GetDurationOrDefault("peer.gossip.dialTimeout", comm.DefDialTimeout)
	c.ConnTimeout = util.GetDurationOrDefault("peer.gossip.connTimeout", comm.DefConnTimeout)
	c.RecvBuffSize = util.GetIntOrDefault("peer.gossip.recvBuffSize", comm.DefRecvBuffSize)
	c.SendBuffSize = util.GetIntOrDefault("peer.gossip.sendBuffSize", comm.DefSendBuffSize)
	c.MsgExpirationTimeout = util.GetDurationOrDefault("peer.gossip.election.leaderAliveThreshold", election.DefLeaderAliveThreshold) * 10
	c.AliveTimeInterval = util.GetDurationOrDefault("peer.gossip.aliveTimeInterval", discovery.DefAliveTimeInterval)
	c.AliveExpirationTimeout = util.GetDurationOrDefault("peer.gossip.aliveExpirationTimeout", 5*c.AliveTimeInterval)
	c.AliveExpirationCheckInterval = c.AliveExpirationTimeout / 10
	c.ReconnectInterval = util.GetDurationOrDefault("peer.gossip.reconnectInterval", c.AliveExpirationTimeout)
	c.MaxConnectionAttempts = util.GetIntOrDefault("peer.gossip.maxConnectionAttempts", discovery.DefMaxConnectionAttempts)
	c.MsgExpirationFactor = util.GetIntOrDefault("peer.gossip.msgExpirationFactor", discovery.DefMsgExpirationFactor)

	return nil
}
