/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

// Package commit provides an implementation for finding transaction commit status that is specific to the Gateway
// embedded within a peer.
package commit

import (
	"context"

	"github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/internal/pkg/gateway/ledger"

	"github.com/pkg/errors"
)

type Status struct {
	BlockNumber   uint64
	TransactionID string
	Code          peer.TxValidationCode
}

// Finder is used to obtain transaction status.
type Finder struct {
	provider ledger.Provider
	notifier *Notifier
}

func NewFinder(provider ledger.Provider, notifier *Notifier) *Finder {
	return &Finder{
		provider: provider,
		notifier: notifier,
	}
}

// TransactionStatus provides status of a specified transaction on a given channel. If the transaction has already
// committed, the status is returned immediately; otherwise this call blocks waiting for the transaction to be
// committed or the context to be cancelled.
func (finder *Finder) TransactionStatus(ctx context.Context, channelName string, transactionID string) (*Status, error) {
	// Set up notifier first to ensure no commit missed after completing query
	notifyDone := make(chan struct{})
	defer close(notifyDone)
	statusReceive, err := finder.notifier.notifyStatus(notifyDone, channelName, transactionID)
	if err != nil {
		return nil, err
	}

	ledger, err := finder.provider.Ledger(channelName)
	if err != nil {
		return nil, err
	}

	if code, blockNumber, err := ledger.GetTxValidationCodeByTxID(transactionID); err == nil {
		status := &Status{
			BlockNumber:   blockNumber,
			TransactionID: transactionID,
			Code:          code,
		}
		return status, nil
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case status, ok := <-statusReceive:
		if !ok {
			return nil, errors.New("unexpected close of commit notification channel")
		}
		return status, nil
	}
}
