/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package gateway

import (
	peerproto "github.com/hyperledger/fabric-protos-go/peer"
	commonledger "github.com/hyperledger/fabric/common/ledger"
	coreledger "github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/peer"
	"github.com/pkg/errors"
)

// peerAdapter presents a small piece of the Peer in a form that can be easily used (and mocked) by the gateway's
// transaction status checking and eventing.
type peerAdapter struct {
	Peer *peer.Peer
}

func (adapter *peerAdapter) CommitNotifications(done <-chan struct{}, channelName string) (<-chan *coreledger.CommitNotification, error) {
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

	status, blockNumber, err := channel.Ledger().GetTxValidationCodeByTxID(transactionID)
	if err != nil {
		return 0, 0, err
	}

	return status, blockNumber, nil
}

func (adapter *peerAdapter) Ledger(channelName string) (commonledger.Ledger, error) {
	channel, err := adapter.channel(channelName)
	if err != nil {
		return nil, err
	}

	return channel.Ledger(), nil
}

func (adapter *peerAdapter) channel(name string) (*peer.Channel, error) {
	channel := adapter.Peer.Channel(name)
	if channel == nil {
		return nil, errors.Errorf("channel does not exist: %s", name)
	}

	return channel, nil
}
