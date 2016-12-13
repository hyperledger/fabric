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
	"math"

	"github.com/hyperledger/fabric/orderer/common/bootstrap/provisional"
	"github.com/hyperledger/fabric/orderer/localconfig"
	ab "github.com/hyperledger/fabric/protos/orderer"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
)

type deliverClient struct {
	client  ab.AtomicBroadcast_DeliverClient
	chainID string
}

func newDeliverClient(client ab.AtomicBroadcast_DeliverClient, chainID string) *deliverClient {
	return &deliverClient{client: client, chainID: chainID}
}

func (r *deliverClient) seekOldest() error {
	return r.client.Send(&ab.SeekInfo{
		ChainID:  r.chainID,
		Start:    &ab.SeekPosition{Type: &ab.SeekPosition_Oldest{Oldest: &ab.SeekOldest{}}},
		Stop:     &ab.SeekPosition{Type: &ab.SeekPosition_Specified{Specified: &ab.SeekSpecified{Number: math.MaxUint64}}},
		Behavior: ab.SeekInfo_BLOCK_UNTIL_READY,
	})
}

func (r *deliverClient) seekNewest() error {
	return r.client.Send(&ab.SeekInfo{
		ChainID:  r.chainID,
		Start:    &ab.SeekPosition{Type: &ab.SeekPosition_Newest{Newest: &ab.SeekNewest{}}},
		Stop:     &ab.SeekPosition{Type: &ab.SeekPosition_Specified{Specified: &ab.SeekSpecified{Number: math.MaxUint64}}},
		Behavior: ab.SeekInfo_BLOCK_UNTIL_READY,
	})
}

func (r *deliverClient) seek(blockNumber uint64) error {
	return r.client.Send(&ab.SeekInfo{
		ChainID:  r.chainID,
		Start:    &ab.SeekPosition{Type: &ab.SeekPosition_Specified{Specified: &ab.SeekSpecified{Number: blockNumber}}},
		Stop:     &ab.SeekPosition{Type: &ab.SeekPosition_Specified{Specified: &ab.SeekSpecified{Number: math.MaxUint64}}},
		Behavior: ab.SeekInfo_BLOCK_UNTIL_READY,
	})
}

func (r *deliverClient) readUntilClose() {
	for {
		msg, err := r.client.Recv()
		if err != nil {
			fmt.Println("Error receiving:", err)
			return
		}

		switch t := msg.Type.(type) {
		case *ab.DeliverResponse_Status:
			fmt.Println("Got status ", t)
			return
		case *ab.DeliverResponse_Block:
			fmt.Println("Received block: ", t.Block)
		}
	}
}

func main() {
	config := config.Load()

	var chainID string
	var serverAddr string

	flag.StringVar(&serverAddr, "server", fmt.Sprintf("%s:%d", config.General.ListenAddress, config.General.ListenPort), "The RPC server to connect to.")
	flag.StringVar(&chainID, "chainID", provisional.TestChainID, "The chain ID to deliver from.")
	flag.Parse()

	conn, err := grpc.Dial(serverAddr, grpc.WithInsecure())
	if err != nil {
		fmt.Println("Error connecting:", err)
		return
	}
	client, err := ab.NewAtomicBroadcastClient(conn).Deliver(context.TODO())
	if err != nil {
		fmt.Println("Error connecting:", err)
		return
	}

	s := newDeliverClient(client, chainID)
	err = s.seekOldest()
	if err != nil {
		fmt.Println("Received error:", err)
	}

	s.readUntilClose()
}
