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
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/hyperledger/fabric/msp/mgmt"
	"google.golang.org/grpc"
)

// This is just a test that shows how to instantiate a gossip component
func TestNewGossipCryptoService(t *testing.T) {
	s1 := grpc.NewServer()
	s2 := grpc.NewServer()
	s3 := grpc.NewServer()

	ll1, _ := net.Listen("tcp", fmt.Sprintf("%s:%d", "", 5611))
	ll2, _ := net.Listen("tcp", fmt.Sprintf("%s:%d", "", 5612))
	ll3, _ := net.Listen("tcp", fmt.Sprintf("%s:%d", "", 5613))

	endpoint1 := "localhost:5611"
	endpoint2 := "localhost:5612"
	endpoint3 := "localhost:5613"

	mgmt.LoadFakeSetupWithLocalMspAndTestChainMsp("../../msp/sampleconfig")
	identity, _ := mgmt.GetLocalSigningIdentityOrPanic().Serialize()

	g1 := NewGossipComponent(identity, endpoint1, s1, []grpc.DialOption{grpc.WithInsecure()})
	g2 := NewGossipComponent(identity, endpoint2, s2, []grpc.DialOption{grpc.WithInsecure()}, endpoint1)
	g3 := NewGossipComponent(identity, endpoint3, s3, []grpc.DialOption{grpc.WithInsecure()}, endpoint1)
	go s1.Serve(ll1)
	go s2.Serve(ll2)
	go s3.Serve(ll3)

	time.Sleep(time.Second * 5)
	fmt.Println(g1.Peers())
	fmt.Println(g2.Peers())
	fmt.Println(g3.Peers())
	time.Sleep(time.Second)
}
