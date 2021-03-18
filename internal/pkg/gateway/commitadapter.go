/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package gateway

import (
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/peer"
	"github.com/pkg/errors"
)

type PeerNotifierAdapter struct {
	Peer *peer.Peer
}

func (adapter *PeerNotifierAdapter) CommitNotifications(done <-chan struct{}, channelName string) (<-chan *ledger.CommitNotification, error) {
	channel := adapter.Peer.Channel(channelName)
	if channel == nil {
		return nil, errors.Errorf("channel does not exist: %s", channelName)
	}

	return channel.Ledger().CommitNotificationsChannel(done)
}
