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
	reader BlockReader
}

// NewNotifier returns a new commit notifier that obtains channels from a given channel factory.
func NewNotifier(reader BlockReader) *Notifier {
	return &Notifier{
		reader: reader,
	}
}

// Notify the supplied channel when the specified transaction is committed to a channel's ledger.
func (notifier *Notifier) Notify(channelID string, transactionID string) (<-chan peerProto.TxValidationCode, error) {
	// TODO: add a context parameter to enable the notifier to be cancelled
	// TODO: pooling of ledger iterators over multiple invocations
	blockIterator, err := notifier.reader.Iterator(channelID, seekNextCommit())
	if err != nil {
		return nil, err
	}

	commitChannel := make(chan peerProto.TxValidationCode, 1)
	go readCommit(commitChannel, blockIterator, transactionID)

	return commitChannel, nil
}

func readCommit(commit chan<- peerProto.TxValidationCode, blockIterator blockledger.Iterator, transactionID string) {
	defer blockIterator.Close()
	defer close(commit)

	for {
		block, status := blockIterator.Next()
		if status != commonProto.Status_SUCCESS {
			return
		}

		parser := &blockParser{block}
		validationCodes, err := parser.TransactionValidationCodes()
		if err != nil {
			// TODO: log error -- ledger is broken at this point
			return
		}

		if status, exists := validationCodes[transactionID]; exists {
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

//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6 -o mocks/blockreader.go --fake-name BlockReader . BlockReader

// BlockReader allows blocks to be read from the local peer's ledgers.
type BlockReader interface {
	Iterator(channelID string, startType *ordererProto.SeekPosition) (blockledger.Iterator, error)
}
