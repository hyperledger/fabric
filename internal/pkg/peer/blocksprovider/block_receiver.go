/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package blocksprovider

import (
	"fmt"
	"sync"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric-protos-go/gossip"
	"github.com/hyperledger/fabric-protos-go/orderer"
	"github.com/hyperledger/fabric/common/flogging"
	gossipcommon "github.com/hyperledger/fabric/gossip/common"
	"github.com/hyperledger/fabric/internal/pkg/peer/orderers"
	"github.com/pkg/errors"
)

type BlockReceiver struct {
	channelID           string
	gossip              GossipServiceAdapter
	blockGossipDisabled bool
	blockVerifier       BlockVerifier
	deliverClient       orderer.AtomicBroadcast_DeliverClient
	cancelSendFunc      func()
	recvC               chan *orderer.DeliverResponse
	stopC               chan struct{}
	endpoint            *orderers.Endpoint

	mutex    sync.Mutex
	stopFlag bool

	logger *flogging.FabricLogger
}

// Start starts a goroutine that continuously receives blocks.
func (br *BlockReceiver) Start() {
	br.logger.Infof("Starting to receive")
	go func() {
		for {
			resp, err := br.deliverClient.Recv()
			if err != nil {
				br.logger.Warningf("Encountered an error reading from deliver stream: %s", err)
				close(br.recvC)
				return
			}

			select {
			case br.recvC <- resp:
			case <-br.stopC: // local stop signal
				close(br.recvC)
				return
			}

		}
	}()
}

func (br *BlockReceiver) Stop() {
	if br == nil {
		return
	}

	br.mutex.Lock()
	defer br.mutex.Unlock()

	if br.stopFlag {
		return
	}

	br.stopFlag = true
	close(br.stopC)
}

// ProcessIncoming processes incoming messages until stopped or encounters an error.
func (br *BlockReceiver) ProcessIncoming(onSuccess func(blockNum uint64)) error {
	var err error

RecvLoop: // Loop until the endpoint is refreshed, or there is an error on the connection
	for {
		select {
		case <-br.endpoint.Refreshed:
			br.logger.Infof("Ordering endpoints have been refreshed, disconnecting from deliver to reconnect using updated endpoints")
			err = &errRefreshEndpoint{message: fmt.Sprintf("orderer endpoint `%s` has been refreshed, ", br.endpoint.Address)}
			break RecvLoop
		case response, ok := <-br.recvC:
			if !ok {
				br.logger.Warningf("Orderer hung up without sending status")
				err = errors.Errorf("orderer `%s` hung up without sending status", br.endpoint.Address)
				break RecvLoop
			}
			var blockNum uint64
			blockNum, err = br.processMsg(response)
			if err != nil {
				br.logger.Warningf("Got error while attempting to receive blocks: %v", err)
				err = errors.WithMessagef(err, "got error while attempting to receive blocks from orderer `%s`", br.endpoint.Address)
				break RecvLoop
			}
			onSuccess(blockNum)
		case <-br.stopC:
			br.logger.Infof("BlockReceiver got a signal to stop")
			err = &errStopping{message: "got a signal to stop"}
			break RecvLoop
		}
	}

	// cancel the sending side and wait for the start goroutine to exit
	br.cancelSendFunc()
	<-br.recvC

	return err
}

func (br *BlockReceiver) processMsg(msg *orderer.DeliverResponse) (uint64, error) {
	switch t := msg.GetType().(type) {
	case *orderer.DeliverResponse_Status:
		if t.Status == common.Status_SUCCESS {
			return 0, errors.Errorf("received success for a seek that should never complete")
		}

		return 0, errors.Errorf("received bad status %v from orderer", t.Status)
	case *orderer.DeliverResponse_Block:
		blockNum := t.Block.Header.Number
		if err := br.blockVerifier.VerifyBlock(gossipcommon.ChannelID(br.channelID), blockNum, t.Block); err != nil {
			return 0, errors.WithMessage(err, "block from orderer could not be verified")
		}

		marshaledBlock, err := proto.Marshal(t.Block)
		if err != nil {
			return 0, errors.WithMessage(err, "block from orderer could not be re-marshaled")
		}

		// Create payload with a block received
		payload := &gossip.Payload{
			Data:   marshaledBlock,
			SeqNum: blockNum,
		}

		// Use payload to create gossip message
		gossipMsg := &gossip.GossipMessage{
			Nonce:   0,
			Tag:     gossip.GossipMessage_CHAN_AND_ORG,
			Channel: []byte(br.channelID),
			Content: &gossip.GossipMessage_DataMsg{
				DataMsg: &gossip.DataMessage{
					Payload: payload,
				},
			},
		}

		br.logger.Debugf("Adding payload to local buffer, blockNum = [%d]", blockNum)
		// Add payload to local state payloads buffer
		if err := br.gossip.AddPayload(br.channelID, payload); err != nil {
			br.logger.Warningf("Block [%d] received from ordering service wasn't added to payload buffer: %v", blockNum, err)
			return 0, errors.WithMessage(err, "could not add block as payload")
		}
		if br.blockGossipDisabled {
			return blockNum, nil
		}
		// Gossip messages with other nodes
		br.logger.Debugf("Gossiping block [%d]", blockNum)
		br.gossip.Gossip(gossipMsg)
		return blockNum, nil
	default:
		return 0, errors.Errorf("unknown message type: %T, message: %+v", t, msg)
	}
}
