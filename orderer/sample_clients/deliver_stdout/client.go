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

	"github.com/hyperledger/fabric/orderer/common/bootstrap/static"
	"github.com/hyperledger/fabric/orderer/localconfig"
	cb "github.com/hyperledger/fabric/protos/common"
	ab "github.com/hyperledger/fabric/protos/orderer"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
)

type deliverClient struct {
	client         ab.AtomicBroadcast_DeliverClient
	chainID        string
	windowSize     uint64
	unAcknowledged uint64
}

func newDeliverClient(client ab.AtomicBroadcast_DeliverClient, chainID string, windowSize uint64) *deliverClient {
	return &deliverClient{client: client, chainID: chainID, windowSize: windowSize}
}

func (r *deliverClient) seekOldest() error {
	return r.client.Send(&ab.DeliverUpdate{
		Type: &ab.DeliverUpdate_Seek{
			Seek: &ab.SeekInfo{
				Start:      ab.SeekInfo_OLDEST,
				WindowSize: r.windowSize,
				ChainID:    r.chainID,
			},
		},
	})
}

func (r *deliverClient) seekNewest() error {
	return r.client.Send(&ab.DeliverUpdate{
		Type: &ab.DeliverUpdate_Seek{
			Seek: &ab.SeekInfo{
				Start:      ab.SeekInfo_NEWEST,
				WindowSize: r.windowSize,
				ChainID:    r.chainID,
			},
		},
	})
}

func (r *deliverClient) seek(blockNumber uint64) error {
	return r.client.Send(&ab.DeliverUpdate{
		Type: &ab.DeliverUpdate_Seek{
			Seek: &ab.SeekInfo{
				Start:           ab.SeekInfo_SPECIFIED,
				SpecifiedNumber: blockNumber,
				WindowSize:      r.windowSize,
				ChainID:         r.chainID,
			},
		},
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
		case *ab.DeliverResponse_Error:
			if t.Error == cb.Status_SUCCESS {
				fmt.Println("ERROR! Received success in error field")
				return
			}
			fmt.Println("Got error ", t)
		case *ab.DeliverResponse_Block:
			fmt.Println("Received block: ", t.Block)
			r.unAcknowledged++
			if r.unAcknowledged >= r.windowSize/2 {
				fmt.Println("Sending acknowledgement")
				err = r.client.Send(&ab.DeliverUpdate{Type: &ab.DeliverUpdate_Acknowledgement{Acknowledgement: &ab.Acknowledgement{Number: t.Block.Header.Number}}})
				if err != nil {
					return
				}
				r.unAcknowledged = 0
			}
		default:
			fmt.Println("Received unknock: ", t)
			return
		}
	}
}

func main() {
	config := config.Load()

	var chainID string
	var serverAddr string
	var windowSize uint64

	flag.StringVar(&serverAddr, "server", fmt.Sprintf("%s:%d", config.General.ListenAddress, config.General.ListenPort), "The RPC server to connect to.")
	flag.StringVar(&chainID, "chainID", static.TestChainID, "The chain ID to deliver from.")
	flag.Uint64Var(&windowSize, "windowSize", 10, "The window size for the deliver.")
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

	s := newDeliverClient(client, chainID, windowSize)
	s.seekOldest()
	s.readUntilClose()

}
