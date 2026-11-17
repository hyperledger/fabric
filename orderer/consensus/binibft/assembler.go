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

// assembler builds a complete Fabric block per proposal, so every consenter
// commits and signs byte-identical blocks. A config transaction must be alone
// in its block; requests that cannot share the block wait in `carry`.
type assembler struct {
	support consensus.ConsenterSupport
	tip     func() tipState

	mu    sync.Mutex
	carry [][]byte
}

// AssembleProposal returns a zero Proposal when there is nothing to assemble.
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

func (a *assembler) hasCarry() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.carry) > 0
}

// splitConfigTx returns the requests for the next block and the rest: a block
// holds either a single config transaction or only non-config ones.
func splitConfigTx(requests [][]byte) (batch, rest [][]byte) {
	for i, raw := range requests {
		env, err := protoutil.UnmarshalEnvelope(raw)
		if err != nil || !protoutil.IsConfigTransaction(env) {
			batch = append(batch, raw)
			continue
		}
		if len(batch) == 0 {
			return [][]byte{raw}, requests[i+1:]
		}
		return batch, requests[i:]
	}
	return batch, nil
}
