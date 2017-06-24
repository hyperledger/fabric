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
	"flag"
	"fmt"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/configtx/tool/provisional"
	"github.com/hyperledger/fabric/orderer/localconfig"
	cb "github.com/hyperledger/fabric/protos/common"
	ab "github.com/hyperledger/fabric/protos/orderer"
	"github.com/hyperledger/fabric/protos/utils"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
)

type broadcastClient struct {
	client  ab.AtomicBroadcast_BroadcastClient
	chainID string
}

// newBroadcastClient creates a simple instance of the broadcastClient interface
func newBroadcastClient(client ab.AtomicBroadcast_BroadcastClient, chainID string) *broadcastClient {
	return &broadcastClient{client: client, chainID: chainID}
}

func (s *broadcastClient) broadcast(transaction []byte) error {
	payload, err := proto.Marshal(&cb.Payload{
		Header: &cb.Header{
			ChannelHeader: utils.MarshalOrPanic(&cb.ChannelHeader{
				ChannelId: s.chainID,
			}),
			SignatureHeader: utils.MarshalOrPanic(&cb.SignatureHeader{}),
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

	var chainID string
	var serverAddr string
	var messages uint64

	flag.StringVar(&serverAddr, "server", fmt.Sprintf("%s:%d", config.General.ListenAddress, config.General.ListenPort), "The RPC server to connect to.")
	flag.StringVar(&chainID, "chainID", provisional.TestChainID, "The chain ID to broadcast to.")
	flag.Uint64Var(&messages, "messages", 1, "The number of messages to broadcast.")
	flag.Parse()

	conn, err := grpc.Dial(serverAddr, grpc.WithInsecure())
	defer func() {
		_ = conn.Close()
	}()
	if err != nil {
		fmt.Println("Error connecting:", err)
		return
	}
	client, err := ab.NewAtomicBroadcastClient(conn).Broadcast(context.TODO())
	if err != nil {
		fmt.Println("Error connecting:", err)
		return
	}

	s := newBroadcastClient(client, chainID)
	for i := uint64(0); i < messages; i++ {
		s.broadcast([]byte(fmt.Sprintf("Testing %v", time.Now())))
		err = s.getAck()
	}
	if err != nil {
		fmt.Printf("\nError: %v\n", err)
	}
}
