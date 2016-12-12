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

	cb "github.com/hyperledger/fabric/protos/common"
	ab "github.com/hyperledger/fabric/protos/orderer"
	context "golang.org/x/net/context"
)

func (c *clientImpl) broadcast(envelope *cb.Envelope) {
	stream, err := c.rpc.Broadcast(context.Background())
	if err != nil {
		panic(fmt.Errorf("Failed to invoke broadcast RPC: %s", err))
	}
	go c.recvBroadcastReplies(stream)

	if err := stream.Send(envelope); err != nil {
		panic(fmt.Errorf("Failed to send broadcast message to ordering service: %s", err))
	}
	logger.Debugf("Sent broadcast message \"%v\" to ordering service\n", envelope)

	if err := stream.CloseSend(); err != nil {
		panic(fmt.Errorf("Failed to close the send direction of the broadcast stream: %v", err))
	}

	<-c.doneChan // Wait till we've had a chance to get back a reply (or an error)
	logger.Info("Client shutting down")
}

func (c *clientImpl) recvBroadcastReplies(stream ab.AtomicBroadcast_BroadcastClient) {
	defer close(c.doneChan)
	for {
		reply, err := stream.Recv()
		if err == io.EOF {
			return
		}
		if err != nil {
			panic(fmt.Errorf("Failed to receive a broadcast reply from orderer: %v", err))
		}
		logger.Info("Broadcast reply from orderer:", reply.Status.String())
		break
	}
}
