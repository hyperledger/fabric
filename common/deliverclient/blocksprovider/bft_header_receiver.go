/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package blocksprovider

import (
	"sync"
	"time"

	"github.com/hyperledger/fabric-lib-go/common/flogging"
	"github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/hyperledger/fabric-protos-go-apiv2/orderer"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
)

// BFTHeaderReceiver receives a stream of blocks from an orderer, where each block contains a header and metadata.
// It keeps track of the last header it received, and the time it was received.
// The header receivers verify each block as it arrives.
//
// The header receiver receives full config blocks.
// The header receiver maintains its own private block verifier that gets updated on every config block.
type BFTHeaderReceiver struct {
	mutex                  sync.Mutex
	chainID                string
	stop                   bool
	stopChan               chan struct{}
	started                bool
	errorStopTime          time.Time
	endpoint               string
	client                 orderer.AtomicBroadcast_DeliverClient
	updatableBlockVerifier UpdatableBlockVerifier

	// A block with Header & Metadata, without Data (i.e. lastHeader.Data==nil); except from config blocks, which are full.
	lastHeader *common.Block
	// The time lastHeader was received, or time.Time{}
	lastHeaderTime time.Time

	logger *flogging.FabricLogger
}

// NewBFTHeaderReceiver create a new BFTHeaderReceiver.
//
// If the previousReceiver is not nil, the lastHeader and lastHeaderTime are copied to the new instance.
// This allows a new receiver to start from the last know good header that has been received.
func NewBFTHeaderReceiver(
	chainID string,
	endpoint string,
	client orderer.AtomicBroadcast_DeliverClient,
	updatableBlockVerifier UpdatableBlockVerifier,
	previousReceiver *BFTHeaderReceiver,
	logger *flogging.FabricLogger,
) *BFTHeaderReceiver {
	hRcv := &BFTHeaderReceiver{
		chainID:                chainID,
		stopChan:               make(chan struct{}, 1),
		endpoint:               endpoint,
		client:                 client,
		updatableBlockVerifier: updatableBlockVerifier.Clone(),
		logger:                 logger,
	}

	if previousReceiver != nil {
		block, bTime, err := previousReceiver.LastBlock()
		if err == nil {
			hRcv.lastHeader = block
			hRcv.lastHeaderTime = bTime
		}
	}

	return hRcv
}

// DeliverHeaders starts to deliver headers from the stream client
func (hr *BFTHeaderReceiver) DeliverHeaders() {
	var normalExit bool

	defer func() {
		if !normalExit {
			hr.mutex.Lock()
			hr.errorStopTime = time.Now()
			hr.mutex.Unlock()
		}
		_ = hr.Stop()
		hr.logger.Debugf("[%s][%s] Stopped to deliver headers", hr.chainID, hr.endpoint)
	}()

	hr.logger.Debugf("[%s][%s] Starting to deliver headers", hr.chainID, hr.endpoint)
	hr.setStarted()

	for !hr.IsStopped() {
		msg, err := hr.client.Recv()
		if err != nil {
			hr.logger.Debugf("[%s][%s] Receive error: %s", hr.chainID, hr.endpoint, err.Error())
			return
		}

		switch t := msg.GetType().(type) {
		case *orderer.DeliverResponse_Status:
			if t.Status == common.Status_SUCCESS {
				hr.logger.Warningf("[%s][%s] Warning! Received %s for a seek that should never complete", hr.chainID, hr.endpoint, t.Status)
				return
			}

			hr.logger.Errorf("[%s][%s] Got bad status %s", hr.chainID, hr.endpoint, t.Status)
			return

		case *orderer.DeliverResponse_Block:
			blockNum := t.Block.Header.Number

			if !protoutil.IsConfigBlock(t.Block) { // normal blocks with block.Data==nil
				err := hr.updatableBlockVerifier.VerifyBlockAttestation(t.Block)
				if err != nil {
					hr.logger.Warningf("[%s][%s] Last block attestation verification failed, blockNum [%d], err: %s", hr.chainID, hr.endpoint, blockNum, err)
					return
				}
			} else { // a config block is a full block, so verify it as such, and update the verifier
				err := hr.updatableBlockVerifier.VerifyBlock(t.Block)
				if err != nil {
					hr.logger.Warningf("[%s][%s] Last config block verification failed,  blockNum [%d], err: %s", hr.chainID, hr.endpoint, blockNum, err)
					return
				}
				if err := hr.updatableBlockVerifier.UpdateConfig(t.Block); err != nil {
					hr.logger.Warningf("config block [%d] from orderer [%s] failed to update block verifier, error: %s", blockNum, hr.endpoint, err)
					return
				}
				hr.logger.Infof("[%s][%s] Applied config block to header verifier, blockNum = [%d]", hr.chainID, hr.endpoint, blockNum)
			}

			hr.logger.Debugf("[%s][%s] Saving block header & metadata, blockNum = [%d]", hr.chainID, hr.endpoint, blockNum)
			hr.updatableBlockVerifier.UpdateBlockHeader(t.Block)
			hr.mutex.Lock()
			hr.lastHeader = t.Block
			hr.lastHeaderTime = time.Now()
			hr.mutex.Unlock()

		default:
			hr.logger.Warningf("[%s][%s] Received unknown response type: %v", hr.chainID, hr.endpoint, t)
			return
		}
	}

	normalExit = true
}

func (hr *BFTHeaderReceiver) IsStopped() bool {
	hr.mutex.Lock()
	defer hr.mutex.Unlock()

	return hr.stop
}

func (hr *BFTHeaderReceiver) IsStarted() bool {
	hr.mutex.Lock()
	defer hr.mutex.Unlock()

	return hr.started
}

func (hr *BFTHeaderReceiver) setStarted() {
	hr.mutex.Lock()
	defer hr.mutex.Unlock()

	hr.started = true
}

func (hr *BFTHeaderReceiver) GetErrorStopTime() time.Time {
	hr.mutex.Lock()
	defer hr.mutex.Unlock()

	return hr.errorStopTime
}

// Stop the reception of headers and close the client connection
func (hr *BFTHeaderReceiver) Stop() error {
	hr.mutex.Lock()
	defer hr.mutex.Unlock()

	if hr.stop {
		hr.logger.Infof("[%s][%s] Already stopped", hr.chainID, hr.endpoint)
		return nil
	}

	hr.logger.Infof("[%s][%s] Stopping", hr.chainID, hr.endpoint)
	hr.stop = true
	_ = hr.client.CloseSend()
	// TODO close the underlying connection as well
	close(hr.stopChan)

	return nil
}

// LastBlockNum returns the last block number which was verified
func (hr *BFTHeaderReceiver) LastBlockNum() (uint64, time.Time, error) {
	hr.mutex.Lock()
	defer hr.mutex.Unlock()

	if hr.lastHeader == nil {
		return 0, time.Time{}, errors.New("not found")
	}

	return hr.lastHeader.Header.Number, hr.lastHeaderTime, nil
}

// LastBlock returns the last block which was verified
func (hr *BFTHeaderReceiver) LastBlock() (*common.Block, time.Time, error) {
	hr.mutex.Lock()
	defer hr.mutex.Unlock()

	if hr.lastHeader == nil {
		return nil, time.Time{}, errors.New("not found")
	}

	return hr.lastHeader, hr.lastHeaderTime, nil
}
