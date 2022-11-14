/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package smartbft

import (
	"encoding/asn1"
	"sync/atomic"

	"github.com/SmartBFT-Go/consensus/pkg/types"
	cb "github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/orderer/common/cluster"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
)

//go:generate mockery -dir . -name Ledger -case underscore -output mocks

// Ledger returns the height and a block with the given number
type Ledger interface {
	// Height returns the number of blocks in the ledger this channel is associated with.
	Height() uint64

	// Block returns a block with the given number,
	// or nil if such a block doesn't exist.
	Block(number uint64) *cb.Block
}

// Assembler is the proposal assembler
type Assembler struct {
	RuntimeConfig   *atomic.Value
	Logger          *flogging.FabricLogger
	VerificationSeq func() uint64
}

// AssembleProposal assembles a proposal from the metadata and the request
func (a *Assembler) AssembleProposal(metadata []byte, requests [][]byte) (nextProp types.Proposal) {
	rtc := a.RuntimeConfig.Load().(RuntimeConfig)

	lastConfigBlockNum := rtc.LastConfigBlock.Header.Number
	lastBlock := rtc.LastBlock

	if len(requests) == 0 {
		a.Logger.Panicf("Programming error, no requests in proposal")
	}
	batchedRequests := singleConfigTxOrSeveralNonConfigTx(requests, a.Logger)

	block := protoutil.NewBlock(lastBlock.Header.Number+1, protoutil.BlockHeaderHash(lastBlock.Header))
	block.Data = &cb.BlockData{Data: batchedRequests}
	block.Header.DataHash = protoutil.BlockDataHash(block.Data)

	if protoutil.IsConfigBlock(block) {
		lastConfigBlockNum = block.Header.Number
	}

	block.Metadata.Metadata[cb.BlockMetadataIndex_LAST_CONFIG] = protoutil.MarshalOrPanic(&cb.Metadata{
		Value: protoutil.MarshalOrPanic(&cb.LastConfig{Index: lastConfigBlockNum}),
	})
	block.Metadata.Metadata[cb.BlockMetadataIndex_SIGNATURES] = protoutil.MarshalOrPanic(&cb.Metadata{
		Value: protoutil.MarshalOrPanic(&cb.OrdererBlockMetadata{
			ConsenterMetadata: metadata,
			LastConfig: &cb.LastConfig{
				Index: lastConfigBlockNum,
			},
		}),
	})

	tuple := &ByteBufferTuple{
		A: protoutil.MarshalOrPanic(block.Data),
		B: protoutil.MarshalOrPanic(block.Metadata),
	}

	prop := types.Proposal{
		Header:               protoutil.BlockHeaderBytes(block.Header),
		Payload:              tuple.ToBytes(),
		Metadata:             metadata,
		VerificationSequence: int64(a.VerificationSeq()),
	}

	return prop
}

func singleConfigTxOrSeveralNonConfigTx(requests [][]byte, logger Logger) [][]byte {
	// Scan until a config transaction is found
	var batchedRequests [][]byte
	var i int
	for i < len(requests) {
		currentRequest := requests[i]
		envelope, err := protoutil.UnmarshalEnvelope(currentRequest)
		if err != nil {
			logger.Panicf("Programming error, received bad envelope but should have validated it: %v", err)
			continue
		}

		// If we saw a config transaction, we cannot add any more transactions to the batch.
		if protoutil.IsConfigTransaction(envelope) {
			break
		}

		// Else, it's not a config transaction, so add it to the batch.
		batchedRequests = append(batchedRequests, currentRequest)
		i++
	}

	// If we don't have any transaction in the batch, it is safe to assume we only
	// saw a single transaction which is a config transaction.
	if len(batchedRequests) == 0 {
		batchedRequests = [][]byte{requests[0]}
	}

	// At this point, batchedRequests contains either a single config transaction, or a few non config transactions.
	return batchedRequests
}

// LastConfigBlockFromLedgerOrPanic returns the last config block from the ledger
func LastConfigBlockFromLedgerOrPanic(ledger Ledger, logger Logger) *cb.Block {
	block, err := lastConfigBlockFromLedger(ledger)
	if err != nil {
		logger.Panicf("Failed retrieving last config block: %v", err)
	}
	return block
}

func lastConfigBlockFromLedger(ledger Ledger) (*cb.Block, error) {
	lastBlockSeq := ledger.Height() - 1
	lastBlock := ledger.Block(lastBlockSeq)
	if lastBlock == nil {
		return nil, errors.Errorf("unable to retrieve block [%d]", lastBlockSeq)
	}
	lastConfigBlock, err := cluster.LastConfigBlock(lastBlock, ledger)
	if err != nil {
		return nil, err
	}
	return lastConfigBlock, nil
}

func PreviousConfigBlockFromLedgerOrPanic(ledger Ledger, logger Logger) *cb.Block {
	block, err := previousConfigBlockFromLedger(ledger)
	if err != nil {
		logger.Panicf("Failed retrieving previous config block: %v", err)
	}
	return block
}

func previousConfigBlockFromLedger(ledger Ledger) (*cb.Block, error) {
	previousBlockSeq := ledger.Height() - 2
	if ledger.Height() == 1 {
		previousBlockSeq = 0
	}
	previousBlock := ledger.Block(previousBlockSeq)
	if previousBlock == nil {
		return nil, errors.Errorf("unable to retrieve block [%d]", previousBlockSeq)
	}
	previousConfigBlock, err := cluster.LastConfigBlock(previousBlock, ledger)
	if err != nil {
		return nil, err
	}
	return previousConfigBlock, nil
}

// LastBlockFromLedgerOrPanic returns the last block from the ledger
func LastBlockFromLedgerOrPanic(ledger Ledger, logger Logger) *cb.Block {
	lastBlockSeq := ledger.Height() - 1
	lastBlock := ledger.Block(lastBlockSeq)
	if lastBlock == nil {
		logger.Panicf("Failed retrieving last block")
	}
	return lastBlock
}

// ByteBufferTuple is the byte slice tuple
type ByteBufferTuple struct {
	A []byte
	B []byte
}

// ToBytes marshals the buffer tuple to bytes
func (bbt *ByteBufferTuple) ToBytes() []byte {
	bytes, err := asn1.Marshal(*bbt)
	if err != nil {
		panic(err)
	}
	return bytes
}

// FromBytes unmarshals bytes to a buffer tuple
func (bbt *ByteBufferTuple) FromBytes(bytes []byte) error {
	_, err := asn1.Unmarshal(bytes, bbt)
	return err
}
