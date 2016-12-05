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

	"github.com/hyperledger/fabric/orderer/common/bootstrap/static"
	"github.com/hyperledger/fabric/orderer/localconfig"
	cb "github.com/hyperledger/fabric/protos/common"
	ab "github.com/hyperledger/fabric/protos/orderer"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
)

type deliverClient struct {
	client         ab.AtomicBroadcast_DeliverClient
	windowSize     uint64
	unAcknowledged uint64
}

func newDeliverClient(client ab.AtomicBroadcast_DeliverClient, windowSize uint64) *deliverClient {
	return &deliverClient{client: client, windowSize: windowSize}
}

func (r *deliverClient) seekOldest() error {
	return r.client.Send(&ab.DeliverUpdate{
		Type: &ab.DeliverUpdate_Seek{
			Seek: &ab.SeekInfo{
				Start:      ab.SeekInfo_OLDEST,
				WindowSize: r.windowSize,
				ChainID:    static.TestChainID,
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
				ChainID:    static.TestChainID,
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
				ChainID:         static.TestChainID,
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
	serverAddr := fmt.Sprintf("%s:%d", config.General.ListenAddress, config.General.ListenPort)
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

	s := newDeliverClient(client, 10)
	s.seekOldest()
	s.readUntilClose()

}
