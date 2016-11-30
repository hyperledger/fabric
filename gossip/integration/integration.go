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
	"bytes"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/hyperledger/fabric/gossip/api"
	"github.com/hyperledger/fabric/gossip/common"
	"github.com/hyperledger/fabric/gossip/gossip"
	"google.golang.org/grpc"
)

// This file is used to bootstrap a gossip instance for integration/demo purposes ONLY

func newConfig(selfEndpoint string, bootPeers ...string) *gossip.Config {
	port, err := strconv.ParseInt(strings.Split(selfEndpoint, ":")[1], 10, 64)
	if err != nil {
		panic(err)
	}
	return &gossip.Config{
		BindPort:       int(port),
		BootstrapPeers: bootPeers,
		ID:             selfEndpoint,
		MaxMessageCountToStore:     100,
		MaxPropagationBurstLatency: time.Millisecond * 50,
		MaxPropagationBurstSize:    3,
		PropagateIterations:        1,
		PropagatePeerNum:           3,
		PullInterval:               time.Second * 5,
		PullPeerNum:                3,
		SelfEndpoint:               selfEndpoint,
		PublishCertPeriod:          time.Duration(time.Second * 4),
	}
}

// NewGossipComponent creates a gossip component that attaches itself to the given gRPC server
func NewGossipComponent(endpoint string, s *grpc.Server, dialOpts []grpc.DialOption, bootPeers ...string) gossip.Gossip {
	conf := newConfig(endpoint, bootPeers...)
	return gossip.NewGossipService(conf, s, &naiveCryptoService{}, []byte(endpoint), dialOpts...)
}

type naiveCryptoService struct {
}

// ValidateIdentity validates the given identity.
// Returns error on failure, nil on success
func (*naiveCryptoService) ValidateIdentity(peerIdentity api.PeerIdentityType) error {
	return nil
}

// GetPKIidOfCert returns the PKI-ID of a peer's identity
func (*naiveCryptoService) GetPKIidOfCert(peerIdentity api.PeerIdentityType) common.PKIidType {
	return common.PKIidType(peerIdentity)
}

// VerifyBlock returns nil if the block is properly signed,
// else returns error
func (*naiveCryptoService) VerifyBlock(signedBlock api.SignedBlock) error {
	return nil
}

// Sign signs msg with this peer's signing key and outputs
// the signature if no error occurred.
func (*naiveCryptoService) Sign(msg []byte) ([]byte, error) {
	return msg, nil
}

// Verify verifies a signature on a message that came from a peer with a certain vkID
func (cs *naiveCryptoService) Verify(vkID api.PeerIdentityType, signature, message []byte) error {
	if !bytes.Equal(signature, message) {
		return fmt.Errorf("Invalid signature")
	}
	return nil
}
