/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package blocksprovider

import (
	"fmt"
	"sync"

	"github.com/hyperledger/fabric-lib-go/common/flogging"
	"github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/hyperledger/fabric-protos-go-apiv2/orderer"
	"github.com/hyperledger/fabric/common/deliverclient"
	"github.com/hyperledger/fabric/common/deliverclient/orderers"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
)

// BlockHandler abstracts the next stage of processing after the block is fetched from the orderer.
// In the peer the block is given to the gossip service.
// In the orderer the block is placed in a buffer from which the chain or the follower pull blocks.
//
//go:generate counterfeiter -o fake/block_handler.go --fake-name BlockHandler . BlockHandler
type BlockHandler interface {
	// HandleBlock gives the block to the next stage of processing after fetching it from a remote orderer.
	HandleBlock(channelID string, block *common.Block) error
}

type BlockReceiver struct {
	channelID              string
	blockHandler           BlockHandler
	updatableBlockVerifier UpdatableBlockVerifier
	deliverClient          orderer.AtomicBroadcast_DeliverClient
	cancelSendFunc         func()
	recvC                  chan *orderer.DeliverResponse
	stopC                  chan struct{}
	endpoint               *orderers.Endpoint

	mutex    sync.Mutex
	stopFlag bool

	logger *flogging.FabricLogger
}

// Start starts a goroutine that continuously receives blocks.
func (br *BlockReceiver) Start() {
	br.logger.Infof("BlockReceiver starting")
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
		br.logger.Infof("BlockReceiver already stopped")
		return
	}

	br.stopFlag = true
	close(br.stopC)
	br.logger.Infof("BlockReceiver stopped")
}

// ProcessIncoming processes incoming messages until stopped or encounters an error.
func (br *BlockReceiver) ProcessIncoming(onSuccess func(blockNum uint64, channelConfig *common.Config)) error {
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
			var channelConfig *common.Config
			blockNum, channelConfig, err = br.processMsg(response)
			if err != nil {
				br.logger.Warningf("Got error while attempting to receive blocks: %v", err)
				err = errors.WithMessagef(err, "got error while attempting to receive blocks from orderer `%s`", br.endpoint.Address)
				break RecvLoop
			}
			onSuccess(blockNum, channelConfig)
		case <-br.stopC:
			br.logger.Infof("BlockReceiver got a signal to stop")
			err = &ErrStopping{Message: "got a signal to stop"}
			break RecvLoop
		}
	}

	// cancel the sending side and wait for the `Start` goroutine to exit
	br.cancelSendFunc()
	<-br.recvC

	return err
}

func (br *BlockReceiver) processMsg(msg *orderer.DeliverResponse) (uint64, *common.Config, error) {
	switch t := msg.GetType().(type) {
	case *orderer.DeliverResponse_Status:
		if t.Status == common.Status_SUCCESS {
			return 0, nil, errors.Errorf("received success for a seek that should never complete")
		}

		return 0, nil, errors.Errorf("received bad status %v from orderer", t.Status)
	case *orderer.DeliverResponse_Block:
		blockNum := t.Block.Header.Number

		if err := br.updatableBlockVerifier.VerifyBlock(t.Block); err != nil {
			return 0, nil, errors.WithMessagef(err, "block [%d] from orderer [%s] could not be verified", blockNum, br.endpoint.String())
		}

		err := br.blockHandler.HandleBlock(br.channelID, t.Block)
		if err != nil {
			return 0, nil, errors.WithMessagef(err, "block [%d] from orderer [%s] could not be handled", blockNum, br.endpoint.String())
		}

		br.logger.Debugf("Handled block %d", blockNum)

		var channelConfig *common.Config
		if protoutil.IsConfigBlock(t.Block) {
			configEnv, err := deliverclient.ConfigFromBlock(t.Block)
			if err != nil {
				return 0, nil, errors.WithMessagef(err, "failed to extract channel-config from config block [%d] from orderer [%s]", blockNum, br.endpoint.String())
			}

			channelConfig = configEnv.GetConfig()
			br.logger.Debugf("channel config: %+v", channelConfig)

			if err := br.updatableBlockVerifier.UpdateConfig(t.Block); err != nil {
				return 0, nil, errors.WithMessagef(err, "config block [%d] from orderer [%s] failed to update block verifier", blockNum, br.endpoint.String())
			}
			br.logger.Infof("Updated config block %d", blockNum)
		}

		br.updatableBlockVerifier.UpdateBlockHeader(t.Block)

		return blockNum, channelConfig, nil
	default:
		return 0, nil, errors.Errorf("unknown message type: %T, message: %+v", t, msg)
	}
}
