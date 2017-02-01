/*
Copyright IBM Corp. 2016 All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

		 http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package integration

import (
	"crypto/tls"
	"strconv"
	"strings"
	"time"

	"github.com/hyperledger/fabric/gossip/api"
	"github.com/hyperledger/fabric/gossip/gossip"
	"github.com/hyperledger/fabric/gossip/identity"
	"github.com/spf13/viper"
	"google.golang.org/grpc"
)

// This file is used to bootstrap a gossip instance and/or leader election service instance

func getIntOrDefault(key string, defVal int) int {
	if viper.GetInt(key) == 0 {
		return defVal
	} else {
		return viper.GetInt(key)
	}
}

func getDurationOrDefault(key string, defVal time.Duration) time.Duration {
	if viper.GetDuration(key) == 0 {
		return defVal
	} else {
		return viper.GetDuration(key)
	}
}

func newConfig(selfEndpoint string, externalEndpoint string, bootPeers ...string) *gossip.Config {
	port, err := strconv.ParseInt(strings.Split(selfEndpoint, ":")[1], 10, 64)
	if err != nil {
		panic(err)
	}

	var cert *tls.Certificate
	if viper.GetBool("peer.tls.enabled") {
		*cert, err = tls.LoadX509KeyPair(viper.GetString("peer.tls.cert.file"), viper.GetString("peer.tls.key.file"))
		if err != nil {
			panic(err)
		}
	}

	return &gossip.Config{
		BindPort:                   int(port),
		BootstrapPeers:             bootPeers,
		ID:                         selfEndpoint,
		MaxBlockCountToStore:       getIntOrDefault("peer.gossip.maxBlockCountToStore", 100),
		MaxPropagationBurstLatency: getDurationOrDefault("peer.gossip.maxPropagationBurstLatency", 10*time.Millisecond),
		MaxPropagationBurstSize:    getIntOrDefault("peer.gossip.maxPropagationBurstSize", 10),
		PropagateIterations:        getIntOrDefault("peer.gossip.propagateIterations", 1),
		PropagatePeerNum:           getIntOrDefault("peer.gossip.propagatePeerNum", 3),
		PullInterval:               getDurationOrDefault("peer.gossip.pullInterval", 4*time.Second),
		PullPeerNum:                getIntOrDefault("peer.gossip.pullPeerNum", 3),
		InternalEndpoint:           selfEndpoint,
		ExternalEndpoint:           externalEndpoint,
		PublishCertPeriod:          getDurationOrDefault("peer.gossip.publishCertPeriod", 10*time.Second),
		RequestStateInfoInterval:   getDurationOrDefault("peer.gossip.requestStateInfoInterval", 4*time.Second),
		PublishStateInfoInterval:   getDurationOrDefault("peer.gossip.publishStateInfoInterval", 4*time.Second),
		SkipBlockVerification:      viper.GetBool("peer.gossip.skipBlockVerification"),
		TLSServerCert:              cert,
	}
}

// NewGossipComponent creates a gossip component that attaches itself to the given gRPC server
func NewGossipComponent(peerIdentity []byte, endpoint string, s *grpc.Server, secAdv api.SecurityAdvisor, cryptSvc api.MessageCryptoService, idMapper identity.Mapper, dialOpts []grpc.DialOption, bootPeers ...string) gossip.Gossip {

	externalEndpoint := viper.GetString("peer.gossip.externalEndpoint")

	conf := newConfig(endpoint, externalEndpoint, bootPeers...)
	gossipInstance := gossip.NewGossipService(conf, s, secAdv, cryptSvc, idMapper, peerIdentity, dialOpts...)

	return gossipInstance
}
