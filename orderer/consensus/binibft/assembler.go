/*
Copyright BiniWorld. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package binibft

import (
	"sync"

	cb "github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/hyperledger/fabric/orderer/consensus"
	"github.com/hyperledger/fabric/orderer/consensus/binibft/engine"
	"github.com/hyperledger/fabric/protoutil"
	"google.golang.org/protobuf/proto"
)

// assembler turns a batch of requests into an engine proposal that carries a
// complete Fabric block. Carrying the whole block (not just the transactions)
// means every consenter commits byte-identical blocks and can therefore sign
// the same header.
//
// Fabric requires a config transaction to be alone in its block. The engine's
// request pool hands over everything it has batched and forgets it, so requests
// that cannot share the block are kept in `carry` and go first into the next one.
type assembler struct {
	support consensus.ConsenterSupport
	tip     func() tipState

	mu    sync.Mutex
	carry [][]byte
}

// AssembleProposal implements engine.Assembler. It returns a zero Proposal when
// there is nothing to assemble.
func (a *assembler) AssembleProposal(metadata []byte, requests [][]byte) engine.Proposal {
	a.mu.Lock()
	defer a.mu.Unlock()

	all := append(append([][]byte{}, a.carry...), requests...)
	a.carry = nil
	if len(all) == 0 {
		return engine.Proposal{}
	}

	batch, rest := splitConfigTx(all)
	a.carry = rest

	tip := a.tip()
	block := protoutil.NewBlock(tip.number+1, tip.hash)
	block.Data = &cb.BlockData{Data: batch}
	block.Header.DataHash = protoutil.ComputeBlockDataHash(block.Data)

	lastConfig := tip.lastConfig
	if protoutil.IsConfigBlock(block) {
		lastConfig = block.Header.Number
	}

	block.Metadata.Metadata[cb.BlockMetadataIndex_LAST_CONFIG] = protoutil.MarshalOrPanic(&cb.Metadata{
		Value: protoutil.MarshalOrPanic(&cb.LastConfig{Index: lastConfig}),
	})
	block.Metadata.Metadata[cb.BlockMetadataIndex_SIGNATURES] = protoutil.MarshalOrPanic(&cb.Metadata{
		Value: protoutil.MarshalOrPanic(&cb.OrdererBlockMetadata{
			ConsenterMetadata: metadata,
			LastConfig:        &cb.LastConfig{Index: lastConfig},
		}),
	})

	payload, err := proto.Marshal(block)
	if err != nil {
		// A block we just built cannot fail to marshal.
		panic(err)
	}

	return engine.Proposal{
		Header:               protoutil.BlockHeaderBytes(block.Header),
		Payload:              payload,
		Metadata:             metadata,
		VerificationSequence: int64(a.support.Sequence()),
	}
}

// hasCarry reports whether requests are waiting for the next block.
func (a *assembler) hasCarry() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.carry) > 0
}

// splitConfigTx returns the requests that go into the next block, and the rest.
// The block is either a single config transaction, or several non-config ones.
func splitConfigTx(requests [][]byte) (batch, rest [][]byte) {
	for i, raw := range requests {
		env, err := protoutil.UnmarshalEnvelope(raw)
		if err != nil || !protoutil.IsConfigTransaction(env) {
			batch = append(batch, raw)
			continue
		}
		if len(batch) == 0 {
			// The config transaction is first: it gets a block of its own.
			return [][]byte{raw}, requests[i+1:]
		}
		// Non-config transactions precede it: they go first, it waits.
		return batch, requests[i:]
	}
	return batch, nil
}
