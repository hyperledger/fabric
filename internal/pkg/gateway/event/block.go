/*
Copyright IBM Corp. All Rights Reserved.
SPDX-License-Identifier: Apache-2.0
*/

package event

import (
	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric-protos-go/peer"
)

type Block struct {
	block        *common.Block
	transactions []*Transaction
}

func NewBlock(block *common.Block) *Block {
	return &Block{
		block: block,
	}
}

func (b *Block) Number() uint64 {
	return b.block.GetHeader().GetNumber()
}

func (b *Block) Transactions() ([]*Transaction, error) {
	var err error

	if b.transactions == nil {
		b.transactions, err = b.readTransactions()
	}

	return b.transactions, err
}

func (b *Block) readTransactions() ([]*Transaction, error) {
	transactions := make([]*Transaction, 0)

	txPayloads, err := b.payloads()
	if err != nil {
		return nil, err
	}

	for i, payload := range txPayloads {
		header := &common.ChannelHeader{}
		if err := proto.Unmarshal(payload.GetHeader().GetChannelHeader(), header); err != nil {
			return nil, err
		}

		if header.GetType() == int32(common.HeaderType_ENDORSER_TRANSACTION) {
			transaction := &Transaction{
				parent:    b,
				payload:   payload,
				id:        header.GetTxId(),
				timestamp: header.GetTimestamp(),
				status:    b.statusCode(i),
			}
			transactions = append(transactions, transaction)
		}
	}

	return transactions, nil
}

func (b *Block) payloads() ([]*common.Payload, error) {
	var payloads []*common.Payload

	for _, envelopeBytes := range b.block.GetData().GetData() {
		envelope := &common.Envelope{}
		if err := proto.Unmarshal(envelopeBytes, envelope); err != nil {
			return nil, err
		}

		payload := &common.Payload{}
		if err := proto.Unmarshal(envelope.Payload, payload); err != nil {
			return nil, err
		}

		payloads = append(payloads, payload)
	}

	return payloads, nil
}

func (b *Block) statusCode(txIndex int) peer.TxValidationCode {
	metadata := b.block.GetMetadata().GetMetadata()
	if int(common.BlockMetadataIndex_TRANSACTIONS_FILTER) >= len(metadata) {
		return peer.TxValidationCode_INVALID_OTHER_REASON
	}

	statusCodes := metadata[common.BlockMetadataIndex_TRANSACTIONS_FILTER]
	if txIndex >= len(statusCodes) {
		return peer.TxValidationCode_INVALID_OTHER_REASON
	}

	return peer.TxValidationCode(statusCodes[txIndex])
}
