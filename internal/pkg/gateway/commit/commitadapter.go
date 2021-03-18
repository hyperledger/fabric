/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package commit

import (
	peerproto "github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/peer"
	"github.com/pkg/errors"
)

// PeerAdapter presents a small piece of the Peer in a form that can be easily used (and mocked) by the gateway's
// transaction status checking.
type PeerAdapter struct {
	Peer *peer.Peer
}

func (adapter *PeerAdapter) CommitNotifications(done <-chan struct{}, channelName string) (<-chan *ledger.CommitNotification, error) {
	channel, err := adapter.channel(channelName)
	if err != nil {
		return nil, err
	}

	return channel.Ledger().CommitNotificationsChannel(done)
}

func (adapter *PeerAdapter) TransactionStatus(channelName string, transactionID string) (peerproto.TxValidationCode, error) {
	channel, err := adapter.channel(channelName)
	if err != nil {
		return 0, err
	}

	return channel.Ledger().GetTxValidationCodeByTxID(transactionID)
}

func (adapter *PeerAdapter) channel(name string) (*peer.Channel, error) {
	channel := adapter.Peer.Channel(name)
	if channel == nil {
		return nil, errors.Errorf("channel does not exist: %s", name)
	}

	return channel, nil
}
