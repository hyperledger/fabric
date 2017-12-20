/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package integration

import (
	"net"
	"strconv"
	"time"

	"github.com/hyperledger/fabric/gossip/api"
	"github.com/hyperledger/fabric/gossip/common"
	"github.com/hyperledger/fabric/gossip/gossip"
	"github.com/hyperledger/fabric/gossip/util"
	"github.com/pkg/errors"
	"github.com/spf13/viper"
	"google.golang.org/grpc"
)

// This file is used to bootstrap a gossip instance and/or leader election service instance

func newConfig(selfEndpoint string, externalEndpoint string, certs *common.TLSCertificates, bootPeers ...string) (*gossip.Config, error) {
	_, p, err := net.SplitHostPort(selfEndpoint)

	if err != nil {
		return nil, errors.Wrapf(err, "misconfigured endpoint %s", selfEndpoint)
	}

	port, err := strconv.ParseInt(p, 10, 64)
	if err != nil {
		return nil, errors.Wrapf(err, "misconfigured endpoint %s, failed to parse port number", selfEndpoint)
	}

	conf := &gossip.Config{
		BindPort:                   int(port),
		BootstrapPeers:             bootPeers,
		ID:                         selfEndpoint,
		MaxBlockCountToStore:       util.GetIntOrDefault("peer.gossip.maxBlockCountToStore", 100),
		MaxPropagationBurstLatency: util.GetDurationOrDefault("peer.gossip.maxPropagationBurstLatency", 10*time.Millisecond),
		MaxPropagationBurstSize:    util.GetIntOrDefault("peer.gossip.maxPropagationBurstSize", 10),
		PropagateIterations:        util.GetIntOrDefault("peer.gossip.propagateIterations", 1),
		PropagatePeerNum:           util.GetIntOrDefault("peer.gossip.propagatePeerNum", 3),
		PullInterval:               util.GetDurationOrDefault("peer.gossip.pullInterval", 4*time.Second),
		PullPeerNum:                util.GetIntOrDefault("peer.gossip.pullPeerNum", 3),
		InternalEndpoint:           selfEndpoint,
		ExternalEndpoint:           externalEndpoint,
		PublishCertPeriod:          util.GetDurationOrDefault("peer.gossip.publishCertPeriod", 10*time.Second),
		RequestStateInfoInterval:   util.GetDurationOrDefault("peer.gossip.requestStateInfoInterval", 4*time.Second),
		PublishStateInfoInterval:   util.GetDurationOrDefault("peer.gossip.publishStateInfoInterval", 4*time.Second),
		SkipBlockVerification:      viper.GetBool("peer.gossip.skipBlockVerification"),
		TLSCerts:                   certs,
	}

	return conf, nil
}

// NewGossipComponent creates a gossip component that attaches itself to the given gRPC server
func NewGossipComponent(peerIdentity []byte, endpoint string, s *grpc.Server,
	secAdv api.SecurityAdvisor, cryptSvc api.MessageCryptoService,
	secureDialOpts api.PeerSecureDialOpts, certs *common.TLSCertificates, bootPeers ...string) (gossip.Gossip, error) {

	externalEndpoint := viper.GetString("peer.gossip.externalEndpoint")

	conf, err := newConfig(endpoint, externalEndpoint, certs, bootPeers...)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	gossipInstance := gossip.NewGossipService(conf, s, secAdv, cryptSvc,
		peerIdentity, secureDialOpts)

	return gossipInstance, nil
}
