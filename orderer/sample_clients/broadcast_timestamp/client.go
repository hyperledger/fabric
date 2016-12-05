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

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/orderer/common/bootstrap/static"
	"github.com/hyperledger/fabric/orderer/localconfig"
	cb "github.com/hyperledger/fabric/protos/common"
	ab "github.com/hyperledger/fabric/protos/orderer"
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
	payload, err := proto.Marshal(&cb.Payload{
		Header: &cb.Header{
			ChainHeader: &cb.ChainHeader{
				ChainID: static.TestChainID,
			},
		},
		Data: transaction,
	})
	if err != nil {
		panic(err)
	}
	return s.client.Send(&cb.Envelope{Payload: payload})
}

func (s *broadcastClient) getAck() error {
	msg, err := s.client.Recv()
	if err != nil {
		return err
	}
	if msg.Status != cb.Status_SUCCESS {
		return fmt.Errorf("Got unexpected status: %v", msg.Status)
	}
	return nil
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
	err = s.getAck()
	if err != nil {
		fmt.Printf("\nError: %v\n", err)
	}
}
