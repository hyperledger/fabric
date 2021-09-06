/*
Copyright IBM Corp. All Rights Reserved.
SPDX-License-Identifier: Apache-2.0
*/

package event

import (
	"github.com/hyperledger/fabric-protos-go/gateway"
	"github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/common/ledger"
)

type ChaincodeEventsIterator struct {
	blockIter *BlockIterator
}

func NewChaincodeEventsIterator(iterator ledger.ResultsIterator) *ChaincodeEventsIterator {
	return &ChaincodeEventsIterator{
		blockIter: NewBlockIterator(iterator),
	}
}

func (iter *ChaincodeEventsIterator) Next() (*gateway.ChaincodeEventsResponse, error) {
	for {
		result, err := iter.nextBlock()
		if err != nil {
			return nil, err
		}

		if len(result.Events) > 0 {
			return result, nil
		}
	}
}

func (iter *ChaincodeEventsIterator) nextBlock() (*gateway.ChaincodeEventsResponse, error) {
	block, err := iter.blockIter.Next()
	if err != nil {
		return nil, err
	}

	events, err := chaincodeEventsFromBlock(block)
	if err != nil {
		return nil, err
	}

	result := &gateway.ChaincodeEventsResponse{
		BlockNumber: block.Number(),
		Events:      events,
	}
	return result, nil
}

func chaincodeEventsFromBlock(block *Block) ([]*peer.ChaincodeEvent, error) {
	transactions, err := block.Transactions()
	if err != nil {
		return nil, err
	}

	var results []*peer.ChaincodeEvent

	for _, transaction := range transactions {
		if !transaction.Valid() {
			continue
		}

		events, err := transaction.ChaincodeEvents()
		if err != nil {
			return nil, err
		}

		for _, event := range events {
			results = append(results, event.ProtoMessage())
		}
	}

	return results, nil
}

func (iter *ChaincodeEventsIterator) Close() {
	iter.blockIter.Close()
}
