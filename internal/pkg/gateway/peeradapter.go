/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package gateway

import (
	peerproto "github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/peer"
	"github.com/pkg/errors"
)

// peerAdapter presents a small piece of the Peer in a form that can be easily used (and mocked) by the gateway's
// transaction status checking.
type peerAdapter struct {
	Peer *peer.Peer
}

func (adapter *peerAdapter) CommitNotifications(done <-chan struct{}, channelName string) (<-chan *ledger.CommitNotification, error) {
	channel, err := adapter.channel(channelName)
	if err != nil {
		return nil, err
	}

	return channel.Ledger().CommitNotificationsChannel(done)
}

func (adapter *peerAdapter) TransactionStatus(channelName string, transactionID string) (peerproto.TxValidationCode, uint64, error) {
	channel, err := adapter.channel(channelName)
	if err != nil {
		return 0, 0, err
	}

	ledger := channel.Ledger()

	status, blockNumber, err := ledger.GetTxValidationCodeByTxID(transactionID)
	if err != nil {
		return 0, 0, err
	}

	return status, blockNumber, nil
}

func (adapter *peerAdapter) channel(name string) (*peer.Channel, error) {
	channel := adapter.Peer.Channel(name)
	if channel == nil {
		return nil, errors.Errorf("channel does not exist: %s", name)
	}

	return channel, nil
}
