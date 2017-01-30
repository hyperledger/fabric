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

package service

import (
	"fmt"
	"net"
	"sync"
	"testing"

	"github.com/hyperledger/fabric/gossip/api"
	"github.com/hyperledger/fabric/msp/mgmt"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
)

func TestInitGossipService(t *testing.T) {
	// Test whenever gossip service is indeed singleton
	grpcServer := grpc.NewServer()
	socket, error := net.Listen("tcp", fmt.Sprintf("%s:%d", "", 5611))
	assert.NoError(t, error)

	go grpcServer.Serve(socket)
	defer grpcServer.Stop()

	mgmt.LoadFakeSetupWithLocalMspAndTestChainMsp("../../msp/sampleconfig")
	identity, _ := mgmt.GetLocalSigningIdentityOrPanic().Serialize()

	wg := sync.WaitGroup{}
	wg.Add(10)
	for i := 0; i < 10; i++ {
		go func() {
			InitGossipService(identity, "localhost:5611", grpcServer)
			wg.Done()
		}()
	}
	wg.Wait()

	defer GetGossipService().Stop()
	gossip := GetGossipService()

	for i := 0; i < 10; i++ {
		go func(gossipInstance GossipService) {
			assert.Equal(t, gossip, GetGossipService())
		}(gossip)
	}
}

// Make sure *joinChannelMessage implements the api.JoinChannelMessage
func TestJCMInterface(t *testing.T) {
	_ = api.JoinChannelMessage(&joinChannelMessage{})
}
