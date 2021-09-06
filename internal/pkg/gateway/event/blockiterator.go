/*
Copyright IBM Corp. All Rights Reserved.
SPDX-License-Identifier: Apache-2.0
*/

package event

import (
	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric/common/ledger"
	"github.com/pkg/errors"
)

type BlockIterator struct {
	ledgerIter ledger.ResultsIterator
}

func NewBlockIterator(iterator ledger.ResultsIterator) *BlockIterator {
	return &BlockIterator{
		ledgerIter: iterator,
	}
}

func (iter *BlockIterator) Next() (*Block, error) {
	result, err := iter.ledgerIter.Next()
	if err != nil {
		return nil, err
	}

	switch block := result.(type) {
	case *common.Block:
		return NewBlock(block), nil
	default:
		return nil, errors.Errorf("unexpected block type: %T", result)
	}
}

func (iter *BlockIterator) Close() {
	iter.ledgerIter.Close()
}
