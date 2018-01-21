/*
Copyright Digital Asset Holdings, LLC 2016 All Rights Reserved.

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
	"math"
	"time"

	"github.com/hyperledger/fabric/common/configtx/tool/provisional"
	cb "github.com/hyperledger/fabric/protos/common"
	ab "github.com/hyperledger/fabric/protos/orderer"
	"github.com/hyperledger/fabric/protos/utils"

	"github.com/golang/protobuf/proto"
	"github.com/op/go-logging"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
)

var logger = logging.MustGetLogger("single_tx_client")

var UPDATE byte = 0
var SEND byte = 1

var NEEDED_UPDATES = 2
var NEEDED_SENT = 1

func main() {
	logger.Info("Creating an Atomic Broadcast GRPC connection.")
	timeout := 4 * time.Second
	clientconn, err := grpc.Dial(":7050", grpc.WithBlock(), grpc.WithTimeout(timeout), grpc.WithInsecure())
	if err != nil {
		logger.Errorf("Failed to connect to GRPC: %s", err)
		return
	}
	client := ab.NewAtomicBroadcastClient(clientconn)

	resultch := make(chan byte)
	errorch := make(chan error)

	logger.Info("Starting a goroutine waiting for ledger updates.")
	go updateReceiver(resultch, errorch, client)

	logger.Info("Starting a single broadcast sender goroutine.")
	go broadcastSender(resultch, errorch, client)

	checkResults(resultch, errorch)
}

func checkResults(resultch chan byte, errorch chan error) {
	l := len(errorch)
	for i := 0; i < l; i++ {
		errres := <-errorch
		logger.Error(errres)
	}

	updates := 0
	sentBroadcast := 0
	for i := 0; i < 3; i++ {
		select {
		case result := <-resultch:
			switch result {
			case UPDATE:
				updates++
			case SEND:
				sentBroadcast++
			}
		case <-time.After(30 * time.Second):
			continue
		}
	}
	if updates != NEEDED_UPDATES {
		logger.Errorf("We did not get all the ledger updates.")
	} else if sentBroadcast != NEEDED_SENT {
		logger.Errorf("We were unable to send all the broadcasts.")
	} else {
		logger.Info("Successfully sent and received everything.")
	}
}

func updateReceiver(resultch chan byte, errorch chan error, client ab.AtomicBroadcastClient) {
	logger.Info("{Update Receiver} Creating a ledger update delivery stream.")
	dstream, err := client.Deliver(context.Background())
	if err != nil {
		errorch <- fmt.Errorf("Failed to get Deliver stream: %s", err)
		return
	}
	dstream.Send(&cb.Envelope{
		Payload: utils.MarshalOrPanic(&cb.Payload{
			Header: &cb.Header{
				ChannelHeader: utils.MarshalOrPanic(&cb.ChannelHeader{
					ChannelId: provisional.TestChainID,
				}),
				SignatureHeader: utils.MarshalOrPanic(&cb.SignatureHeader{}),
			},
			Data: utils.MarshalOrPanic(&ab.SeekInfo{
				Start: &ab.SeekPosition{Type: &ab.SeekPosition_Newest{
					Newest: &ab.SeekNewest{}}},
				Stop:     &ab.SeekPosition{Type: &ab.SeekPosition_Specified{Specified: &ab.SeekSpecified{Number: math.MaxUint64}}},
				Behavior: ab.SeekInfo_BLOCK_UNTIL_READY,
			}),
		}),
	})

	logger.Info("{Update Receiver} Listening to ledger updates.")
	for i := 0; i < 3; i++ {
		m, inerr := dstream.Recv()
		if inerr != nil {
			errorch <- fmt.Errorf("Failed to receive consensus: %s", inerr)
			return
		}
		switch b := m.Type.(type) {
		case *ab.DeliverResponse_Status:
			fmt.Println("Got status ", b)
		case *ab.DeliverResponse_Block:
			logger.Info("{Update Receiver} Received a ledger update.")
			for i, tx := range b.Block.Data.Data {
				pl := &cb.Payload{}
				e := &cb.Envelope{}
				merr1 := proto.Unmarshal(tx, e)
				merr2 := proto.Unmarshal(e.Payload, pl)
				if merr1 == nil && merr2 == nil {
					logger.Infof("{Update Receiver} %d - %v", i+1, pl.Data)
				}
			}
			resultch <- UPDATE
		}
	}
	logger.Info("{Update Receiver} Exiting...")
}

func broadcastSender(resultch chan byte, errorch chan error, client ab.AtomicBroadcastClient) {
	logger.Info("{Broadcast Sender} Waiting before sending.")
	<-time.After(5 * time.Second)
	bstream, err := client.Broadcast(context.Background())
	if err != nil {
		errorch <- fmt.Errorf("Failed to get broadcast stream: %s", err)
		return
	}
	bs := []byte{0, 1, 2, 3}
	pl := &cb.Payload{
		Header: &cb.Header{
			ChannelHeader: utils.MarshalOrPanic(&cb.ChannelHeader{
				ChannelId: provisional.TestChainID,
			}),
		},
		Data: bs,
	}
	mpl, err := proto.Marshal(pl)
	if err != nil {
		panic("Failed to marshal payload.")
	}
	bstream.Send(&cb.Envelope{Payload: mpl})
	logger.Infof("{Broadcast Sender} Broadcast sent: %v", bs)
	logger.Info("{Broadcast Sender} Exiting...")
	resultch <- SEND
}
