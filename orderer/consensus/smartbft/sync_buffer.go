/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package smartbft

import (
	"sync"

	"github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/pkg/errors"
)

type SyncBuffer struct {
	blockCh  chan *common.Block
	stopCh   chan struct{}
	stopOnce sync.Once
}

func NewSyncBuffer(capacity uint) *SyncBuffer {
	if capacity == 0 {
		capacity = 10
	}
	return &SyncBuffer{
		blockCh: make(chan *common.Block, capacity),
		stopCh:  make(chan struct{}),
	}
}

// HandleBlock gives the block to the next stage of processing after fetching it from a remote orderer.
func (sb *SyncBuffer) HandleBlock(channelID string, block *common.Block) error {
	if block == nil || block.Header == nil {
		return errors.Errorf("empty block or block header, channel: %s", channelID)
	}

	select {
	case sb.blockCh <- block:
		return nil
	case <-sb.stopCh:
		return errors.Errorf("SyncBuffer stopping, channel: %s", channelID)
	}
}

func (sb *SyncBuffer) PullBlock(seq uint64) *common.Block {
	var block *common.Block
	for {
		select {
		case block = <-sb.blockCh:
			if block == nil || block.Header == nil {
				return nil
			}
			if block.GetHeader().GetNumber() == seq {
				return block
			}
			if block.GetHeader().GetNumber() < seq {
				continue
			}
			if block.GetHeader().GetNumber() > seq {
				return nil
			}
		case <-sb.stopCh:
			return nil
		}
	}
}

func (sb *SyncBuffer) Stop() {
	sb.stopOnce.Do(func() {
		close(sb.stopCh)
	})
}
