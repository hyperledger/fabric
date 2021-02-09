/*
Copyright 2021 IBM All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package commit

import (
	commonProto "github.com/hyperledger/fabric-protos-go/common"
	ordererProto "github.com/hyperledger/fabric-protos-go/orderer"
	peerProto "github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/common/ledger/blockledger"
)

// Notifier enables notification of commits to a peer ledger.
type Notifier struct {
	channelFactory ChannelFactory
}

// NewNotifier returns a new commit notifier that obtains channels from a given channel factory.
func NewNotifier(channelFactory ChannelFactory) *Notifier {
	return &Notifier{
		channelFactory: channelFactory,
	}
}

// Notify the supplied channel when the specified transaction is committed to a channel's ledger.
func (notifier *Notifier) Notify(commit chan<- peerProto.TxValidationCode, channelID string, transactionID string) error {
	channel, err := notifier.channelFactory.Channel(channelID)
	if err != nil {
		return err
	}

	blockReader := channel.Reader()
	go readCommit(commit, blockReader, transactionID)

	return nil
}

func readCommit(commit chan<- peerProto.TxValidationCode, blockReader blockledger.Reader, transactionID string) {
	blockIter, _ := blockReader.Iterator(seekNextCommit())
	defer blockIter.Close()
	defer close(commit)

	for {
		block, status := blockIter.Next()
		if status != commonProto.Status_SUCCESS {
			return
		}

		if status, exists := transactionStatus(block, transactionID); exists {
			commit <- status
			return
		}
	}
}

func seekNextCommit() *ordererProto.SeekPosition {
	return &ordererProto.SeekPosition{
		Type: &ordererProto.SeekPosition_NextCommit{
			NextCommit: &ordererProto.SeekNextCommit{},
		},
	}
}

func transactionStatus(block *commonProto.Block, transactionID string) (peerProto.TxValidationCode, bool) {
	parser := blockParser{
		Block: block,
	}
	validationCodes, err := parser.TransactionValidationCodes()
	if err != nil {
		// TODO: log error
		return peerProto.TxValidationCode(0), false
	}

	code, ok := validationCodes[transactionID]
	return code, ok
}

//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6 -o mocks/channelfactory.go --fake-name ChannelFactory . ChannelFactory

// ChannelFactory is a single-method interface view of peer.Peer to allow access to channels.
type ChannelFactory interface {
	Channel(channelID string) (Channel, error)
}

// Channel interface view of peer.Channel Provides only the methods needed for commit notification.
type Channel interface {
	Reader() blockledger.Reader
}
