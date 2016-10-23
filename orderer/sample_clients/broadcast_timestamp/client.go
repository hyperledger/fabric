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

package main

import (
	"fmt"
	"time"

	ab "github.com/hyperledger/fabric/orderer/atomicbroadcast"
	"github.com/hyperledger/fabric/orderer/config"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
)

type broadcastClient struct {
	client ab.AtomicBroadcast_BroadcastClient
}

// newBroadcastClient creates a simple instance of the broadcastClient interface
func newBroadcastClient(client ab.AtomicBroadcast_BroadcastClient) *broadcastClient {
	return &broadcastClient{client: client}
}

func (s *broadcastClient) broadcast(transaction []byte) error {
	return s.client.Send(&ab.BroadcastMessage{Data: transaction})
}

func main() {
	config := config.Load()
	serverAddr := fmt.Sprintf("%s:%d", config.General.ListenAddress, config.General.ListenPort)
	conn, err := grpc.Dial(serverAddr, grpc.WithInsecure())
	defer conn.Close()
	if err != nil {
		fmt.Println("Error connecting:", err)
		return
	}
	client, err := ab.NewAtomicBroadcastClient(conn).Broadcast(context.TODO())
	if err != nil {
		fmt.Println("Error connecting:", err)
		return
	}

	s := newBroadcastClient(client)
	s.broadcast([]byte(fmt.Sprintf("Testing %v", time.Now())))
}
