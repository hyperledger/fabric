/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package ledger

import (
	"github.com/hyperledger/fabric-protos-go/common"
	peerproto "github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/common/ledger"
	peerledger "github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/peer"
	"github.com/pkg/errors"
)

// Ledger presents a subset of the PeerLedger used by the gateway packages to aid mocking.
type Ledger interface {
	CommitNotificationsChannel(done <-chan struct{}) (<-chan *peerledger.CommitNotification, error)
	GetBlockByTxID(txID string) (*common.Block, error)
	GetBlockchainInfo() (*common.BlockchainInfo, error)
	GetBlocksIterator(startBlockNumber uint64) (ledger.ResultsIterator, error)
	GetTxValidationCodeByTxID(txID string) (peerproto.TxValidationCode, uint64, error)
}

// Provider presents a small piece of the Peer in a form that can be easily used (and mocked) by gateway implementation.
type Provider interface {
	Ledger(channelName string) (Ledger, error)
}

// PeerAdapter presents a peer as a LedgerProvider to aid mocking.
type PeerAdapter struct {
	Peer *peer.Peer
}

func (adapter *PeerAdapter) Ledger(channelName string) (Ledger, error) {
	channel := adapter.Peer.Channel(channelName)
	if channel == nil {
		return nil, errors.Errorf("channel does not exist: %s", channelName)
	}

	return channel.Ledger(), nil
}
