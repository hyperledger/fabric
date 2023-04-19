/*
Copyright IBM Corp. 2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package server

import (
	"context"
	"testing"
	"time"

	cb "github.com/hyperledger/fabric-protos-go/common"
	ab "github.com/hyperledger/fabric-protos-go/orderer"
	"github.com/hyperledger/fabric/common/ledger/blockledger"
	"github.com/hyperledger/fabric/common/ledger/blockledger/fileledger"
	"github.com/hyperledger/fabric/common/metrics/disabled"
	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/orderer/common/blockcutter"
	"github.com/hyperledger/fabric/orderer/common/msgprocessor"
	"github.com/hyperledger/fabric/orderer/common/multichannel"
	"github.com/hyperledger/fabric/orderer/common/multichannel/mocks"
	"github.com/hyperledger/fabric/orderer/consensus"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

type AttestationServerStream struct {
	grpc.ServerStream
	ctx        context.Context
	ServerChan chan *ab.BlockAttestationResponse
}

func NewAttestationServerStream() *AttestationServerStream {
	return &AttestationServerStream{
		ctx:        context.Background(),
		ServerChan: make(chan *ab.BlockAttestationResponse, 10),
	}
}

func (ss *AttestationServerStream) Context() context.Context {
	return ss.ctx
}

func (ss *AttestationServerStream) Send(resp *ab.BlockAttestationResponse) error {
	ss.ServerChan <- resp
	return nil
}

func (ss *AttestationServerStream) RecvResponse() (*ab.BlockAttestationResponse, error) {
	response, more := <-ss.ServerChan
	if !more {
		return nil, errors.New("empty")
	}
	return response, nil
}

func newLedgerAndFactory(dir string, chainID string, genesisBlockSys *cb.Block) (blockledger.Factory, blockledger.ReadWriter) {
	rlf := newFactory(dir)
	rl := newLedger(rlf, chainID, genesisBlockSys)
	return rlf, rl
}

func newFactory(dir string) blockledger.Factory {
	rlf, err := fileledger.New(dir, &disabled.Provider{})
	if err != nil {
		panic(err)
	}

	return rlf
}

func newLedger(rlf blockledger.Factory, chainID string, genesisBlockSys *cb.Block) blockledger.ReadWriter {
	rl, err := rlf.GetOrCreate(chainID)
	if err != nil {
		panic(err)
	}

	if genesisBlockSys != nil {
		err = rl.Append(genesisBlockSys)
		if err != nil {
			panic(err)
		}
	}
	return rl
}

func mockCrypto() *mocks.SignerSerializer {
	return &mocks.SignerSerializer{}
}

type mockChainCluster struct {
	*mockChain
}

type mockChain struct {
	queue    chan *cb.Envelope
	cutter   blockcutter.Receiver
	support  consensus.ConsenterSupport
	metadata *cb.Metadata
	done     chan struct{}
}

func (mch *mockChain) Errored() <-chan struct{} {
	return nil
}

func (mch *mockChain) Order(env *cb.Envelope, configSeq uint64) error {
	mch.queue <- env
	return nil
}

func (mch *mockChain) Configure(config *cb.Envelope, configSeq uint64) error {
	mch.queue <- config
	return nil
}

func (mch *mockChain) WaitReady() error {
	return nil
}

func (mch *mockChain) Start() {
	go func() {
		defer close(mch.done)
		for {
			msg, ok := <-mch.queue
			if !ok {
				return
			}

			chdr, err := protoutil.ChannelHeader(msg)
			if err != nil {
				logger.Panicf("If a message has arrived to this point, it should already have had header inspected once: %s", err)
			}

			class := mch.support.ClassifyMsg(chdr)
			switch class {
			case msgprocessor.ConfigMsg:
				batch := mch.support.BlockCutter().Cut()
				if batch != nil {
					block := mch.support.CreateNextBlock(batch)
					mch.support.WriteBlock(block, nil)
				}

				_, err := mch.support.ProcessNormalMsg(msg)
				if err != nil {
					logger.Warningf("Discarding bad config message: %s", err)
					continue
				}
				block := mch.support.CreateNextBlock([]*cb.Envelope{msg})
				mch.support.WriteConfigBlock(block, nil)
			case msgprocessor.NormalMsg:
				batches, _ := mch.support.BlockCutter().Ordered(msg)
				for _, batch := range batches {
					block := mch.support.CreateNextBlock(batch)
					mch.support.WriteBlock(block, nil)
				}
			case msgprocessor.ConfigUpdateMsg:
				logger.Panicf("Not expecting msg class ConfigUpdateMsg here")
			default:
				logger.Panicf("Unsupported msg classification: %v", class)
			}
		}
	}()
}

func (mch *mockChain) Halt() {
	close(mch.queue)
}

func handleChainCluster(support consensus.ConsenterSupport, metadata *cb.Metadata) (consensus.Chain, error) {
	chain := &mockChain{
		queue:    make(chan *cb.Envelope),
		cutter:   support.BlockCutter(),
		support:  support,
		metadata: metadata,
		done:     make(chan struct{}),
	}

	return &mockChainCluster{mockChain: chain}, nil
}

func Test_attestationserver_BlockAttestations(t *testing.T) {
	ts := util.CreateUtcTimestamp()
	channelHeader := &cb.ChannelHeader{
		ChannelId: "mychannel",
		Timestamp: ts,
	}
	seekInfo := &ab.SeekInfo{
		Start: &ab.SeekPosition{
			Type: &ab.SeekPosition_Specified{
				Specified: &ab.SeekSpecified{Number: 0},
			},
		},
		Stop: &ab.SeekPosition{
			Type: &ab.SeekPosition_Specified{
				Specified: &ab.SeekSpecified{Number: 0},
			},
		},
	}
	envelope := &cb.Envelope{}
	payload := &cb.Payload{
		Header: &cb.Header{
			ChannelHeader:   protoutil.MarshalOrPanic(channelHeader),
			SignatureHeader: protoutil.MarshalOrPanic(&cb.SignatureHeader{}),
		},
		Data: protoutil.MarshalOrPanic(seekInfo),
	}
	envelope.Payload = protoutil.MarshalOrPanic(payload)

	// TODO This is preparation work for the BFT block puller. Right now it is used only in unit tests.
	// We need to update these tests.
	t.Run("Complete block attestation is successfully", func(t *testing.T) {
		t.Skip("Work in progress. BFT Block Puller preparatory work. This test needs to be rewritten.")
		// TODO
	})

	t.Run("Block attestation returns with BAD_REQUEST when envelope is malformed", func(t *testing.T) {
		mockstream := NewAttestationServerStream()
		asr := NewAttestationService(&multichannel.Registrar{}, &disabled.Provider{}, nil, time.Second, false, false)
		err := asr.BlockAttestations(&cb.Envelope{}, mockstream)
		require.NoError(t, err)
		StatusResponse, _ := mockstream.RecvResponse()
		require.Equal(t, cb.Status_BAD_REQUEST, StatusResponse.GetStatus())
	})

	t.Run("Block attestion returns with NOTFOUND when custom policy check fails", func(t *testing.T) {
		mockstream := NewAttestationServerStream()
		asr := NewAttestationService(&multichannel.Registrar{}, &disabled.Provider{}, nil, time.Second, false, false)
		err := asr.BlockAttestations(envelope, mockstream)
		require.NoError(t, err)
		StatusResponse, _ := mockstream.RecvResponse()
		require.Equal(t, cb.Status_NOT_FOUND, StatusResponse.GetStatus())
	})
}
