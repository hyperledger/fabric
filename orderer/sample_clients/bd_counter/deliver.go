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
	"io"
	"log"

	"github.com/hyperledger/fabric/orderer/common/bootstrap/static"
	ab "github.com/hyperledger/fabric/protos/orderer"
	context "golang.org/x/net/context"
)

func (c *clientImpl) deliver() {
	updateSeek := &ab.DeliverUpdate{
		Type: &ab.DeliverUpdate_Seek{
			Seek: &ab.SeekInfo{
				WindowSize: uint64(c.config.window),
				ChainID:    static.TestChainID,
			},
		},
	}

	switch c.config.seek {
	case -2:
		updateSeek.GetSeek().Start = ab.SeekInfo_OLDEST
	case -1:
		updateSeek.GetSeek().Start = ab.SeekInfo_NEWEST
	default:
		updateSeek.GetSeek().Start = ab.SeekInfo_SPECIFIED
		updateSeek.GetSeek().SpecifiedNumber = uint64(c.config.seek)
	}

	stream, err := c.rpc.Deliver(context.Background())
	if err != nil {
		panic(fmt.Errorf("Failed to invoke deliver RPC: %v", err))
	}

	go c.recvDeliverReplies(stream)

	err = stream.Send(updateSeek)
	if err != nil {
		log.Println("Failed to send seek update to orderer: ", err)
	}
	logger.Debugf("Sent seek message (start: %v, number: %v, window: %v) to orderer\n",
		updateSeek.GetSeek().Start, updateSeek.GetSeek().SpecifiedNumber, updateSeek.GetSeek().WindowSize)

	for range c.signalChan {
		err = stream.CloseSend()
		if err != nil {
			panic(fmt.Errorf("Failed to close the deliver stream: %v", err))
		}
		logger.Info("Client shutting down")
		return
	}
}

func (c *clientImpl) recvDeliverReplies(stream ab.AtomicBroadcast_DeliverClient) {
	var count int
	updateAck := &ab.DeliverUpdate{
		Type: &ab.DeliverUpdate_Acknowledgement{
			Acknowledgement: &ab.Acknowledgement{}, // Has a Number field
		},
	}

	for {
		reply, err := stream.Recv()
		if err == io.EOF {
			return
		}
		if err != nil {
			panic(err)
		}

		switch t := reply.GetType().(type) {
		case *ab.DeliverResponse_Block:
			logger.Infof("Deliver reply from orderer: block %v, payload %v, prevHash %v",
				t.Block.Header.Number, t.Block.Data.Data, t.Block.Header.PreviousHash)
			count++
			if (count > 0) && (count%c.config.ack == 0) {
				updateAck.GetAcknowledgement().Number = t.Block.Header.Number
				err = stream.Send(updateAck)
				if err != nil {
					logger.Info("Failed to send ACK update to orderer: ", err)
				}
				logger.Debugf("Sent ACK for block %d", t.Block.Header.Number)
			}
		case *ab.DeliverResponse_Error:
			logger.Info("Deliver reply from orderer:", t.Error.String())
		}
	}
}
