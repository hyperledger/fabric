/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package cluster_test

import (
	"io/ioutil"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/common/configtx"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/mocks/crypto"
	"github.com/hyperledger/fabric/core/comm"
	"github.com/hyperledger/fabric/orderer/common/cluster"
	"github.com/hyperledger/fabric/orderer/common/cluster/mocks"
	"github.com/hyperledger/fabric/orderer/common/localconfig"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/msp"
	"github.com/hyperledger/fabric/protos/orderer"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func TestIsReplicationNeeded(t *testing.T) {
	for _, testCase := range []struct {
		name                string
		bootBlock           *common.Block
		systemChannelHeight uint64
		systemChannelError  error
		expectedError       string
		replicationNeeded   bool
	}{
		{
			name:                "no replication needed",
			systemChannelHeight: 100,
			bootBlock:           &common.Block{Header: &common.BlockHeader{Number: 99}},
		},
		{
			name:                "replication is needed - bootstrap block's index equal to height",
			systemChannelHeight: 99,
			bootBlock:           &common.Block{Header: &common.BlockHeader{Number: 99}},
			replicationNeeded:   true,
		},
		{
			name:                "replication is needed - no ledger",
			systemChannelHeight: 0,
			bootBlock:           &common.Block{Header: &common.BlockHeader{Number: 99}},
			replicationNeeded:   true,
		},
		{
			name:               "IO error",
			systemChannelError: errors.New("IO error"),
			expectedError:      "IO error",
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			ledgerWriter := &mocks.LedgerWriter{}
			ledgerWriter.On("Height").Return(testCase.systemChannelHeight)

			ledgerFactory := &mocks.LedgerFactory{}
			ledgerFactory.On("GetOrCreate", "system").Return(ledgerWriter, testCase.systemChannelError)

			r := cluster.Replicator{
				Filter:        cluster.AnyChannel,
				Logger:        flogging.MustGetLogger("test"),
				BootBlock:     testCase.bootBlock,
				SystemChannel: "system",
				LedgerFactory: ledgerFactory,
			}

			ok, err := r.IsReplicationNeeded()
			if testCase.expectedError != "" {
				assert.EqualError(t, err, testCase.expectedError)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, testCase.replicationNeeded, ok)
			}
		})
	}
}

func TestReplicateChainsFailures(t *testing.T) {
	for _, testCase := range []struct {
		name                    string
		isProbeResponseDelayed  bool
		latestBlockSeqInOrderer uint64
		ledgerFactoryError      error
		appendBlockError        error
		expectedPanic           string
		mutateBlocks            func([]*common.Block)
		channelsReturns         []cluster.ChannelGenesisBlock
		badResponse             *orderer.DeliverResponse
	}{
		{
			name: "no block received",
			expectedPanic: "Failed pulling system channel: " +
				"failed obtaining the latest block for channel system",
			isProbeResponseDelayed: true,
		},
		{
			name: "received service unavailable",
			expectedPanic: "Failed pulling system channel: " +
				"failed obtaining the latest block for channel system",
			badResponse: &orderer.DeliverResponse{
				Type: &orderer.DeliverResponse_Status{
					Status: common.Status_SERVICE_UNAVAILABLE,
				},
			},
		},
		{
			name: "latest block seq is less than boot block seq",
			expectedPanic: "Failed pulling system channel: " +
				"latest height found among system channel(system) orderers is 19," +
				" but the boot block's sequence is 21",
			latestBlockSeqInOrderer: 18,
		},
		{
			name: "hash chain mismatch",
			expectedPanic: "Failed pulling system channel: " +
				"block header mismatch on sequence 11, " +
				"expected 9cd61b7e9a5ea2d128cc877e5304e7205888175a8032d40b97db7412dca41d9e, got 010203",
			latestBlockSeqInOrderer: 21,
			mutateBlocks: func(systemChannelBlocks []*common.Block) {
				systemChannelBlocks[len(systemChannelBlocks)/2].Header.PreviousHash = []byte{1, 2, 3}
			},
		},
		{
			name: "last pulled block doesn't match the boot block",
			expectedPanic: "Block header mismatch on last system channel block," +
				" expected 8ec93b2ef5ffdc302f0c0e24611be04ad2b17b099a1aeafd7cfb76a95923f146," +
				" got e428decfc78f8e4c97b26da9c16f9d0b73f886dafa80477a0dd9bac7eb14fe7a",
			latestBlockSeqInOrderer: 21,
			mutateBlocks: func(systemChannelBlocks []*common.Block) {
				systemChannelBlocks[21].Header.DataHash = nil
			},
		},
		{
			name:                    "failure in creating ledger",
			latestBlockSeqInOrderer: 21,
			ledgerFactoryError:      errors.New("IO error"),
			expectedPanic:           "Failed to create a ledger for channel system: IO error",
		},
		{
			name:                    "failure in appending a block to the ledger",
			latestBlockSeqInOrderer: 21,
			appendBlockError:        errors.New("IO error"),
			expectedPanic:           "Failed to write block [0]: IO error",
		},
		{
			name:                    "failure pulling the system chain",
			latestBlockSeqInOrderer: 21,
			expectedPanic: "Failed pulling system channel: " +
				"failed obtaining the latest block for channel system",
			isProbeResponseDelayed: true,
		},
		{
			name:                    "failure obtaining a ledger for a non participating channel",
			latestBlockSeqInOrderer: 21,
			channelsReturns: []cluster.ChannelGenesisBlock{
				{ChannelName: "channelWeAreNotPartOf"},
			},
			ledgerFactoryError: errors.New("IO error"),
			expectedPanic:      "Failed to create a ledger for channel channelWeAreNotPartOf: IO error",
		},
		{
			name:                    "pulled genesis block is malformed",
			latestBlockSeqInOrderer: 21,
			channelsReturns: []cluster.ChannelGenesisBlock{
				{ChannelName: "channelWeAreNotPartOf", GenesisBlock: &common.Block{Header: &common.BlockHeader{}}},
			},
			expectedPanic: "Failed converting channel creation block for channel channelWeAreNotPartOf to genesis" +
				" block: block data is nil",
		},
		{
			name:                    "pulled genesis block is malformed - bad payload",
			latestBlockSeqInOrderer: 21,
			channelsReturns: []cluster.ChannelGenesisBlock{
				{ChannelName: "channelWeAreNotPartOf", GenesisBlock: &common.Block{
					Header: &common.BlockHeader{},
					Data: &common.BlockData{
						Data: [][]byte{utils.MarshalOrPanic(&common.Envelope{
							Payload: []byte{1, 2, 3},
						})},
					},
				}},
			},
			expectedPanic: "Failed converting channel creation block for channel channelWeAreNotPartOf" +
				" to genesis block: no payload in envelope: proto: common.Payload: illegal tag 0 (wire type 1)",
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			systemChannelBlocks := createBlockChain(0, 21)
			if testCase.mutateBlocks != nil {
				testCase.mutateBlocks(systemChannelBlocks)
			}

			lw := &mocks.LedgerWriter{}
			lw.On("Append", mock.Anything).Return(testCase.appendBlockError)
			lw.On("Height").Return(uint64(0))

			lf := &mocks.LedgerFactory{}
			lf.On("GetOrCreate", "system").Return(lw, testCase.ledgerFactoryError)
			lf.On("GetOrCreate", "channelWeAreNotPartOf").Return(lw, testCase.ledgerFactoryError)

			osn := newClusterNode(t)
			defer osn.stop()

			dialer := newCountingDialer()
			bp := newBlockPuller(dialer, osn.srv.Address())
			// Put a big timeout, to reduce chance of flakes when the server gets stuck
			// and we get an un-called for timeout.
			bp.FetchTimeout = time.Hour

			cl := &mocks.ChannelLister{}
			cl.On("Channels").Return(testCase.channelsReturns)
			cl.On("Close")

			r := cluster.Replicator{
				Filter: cluster.AnyChannel,
				AmIPartOfChannel: func(configBlock *common.Block) error {
					return cluster.ErrNotInChannel
				},
				Logger:        flogging.MustGetLogger("test"),
				BootBlock:     systemChannelBlocks[21],
				SystemChannel: "system",
				LedgerFactory: lf,
				Puller:        bp,
				ChannelLister: cl,
			}

			if len(testCase.channelsReturns) > 0 {
				simulateNonParticipantChannelPull(osn)
			}

			if testCase.badResponse != nil {
				osn.blockResponses <- testCase.badResponse
			}

			if !testCase.isProbeResponseDelayed {
				osn.enqueueResponse(testCase.latestBlockSeqInOrderer)
				osn.enqueueResponse(testCase.latestBlockSeqInOrderer)
			} else {
				// Send a nil to force an EOF to the client
				osn.blockResponses <- nil
			}
			osn.addExpectProbeAssert()
			osn.addExpectProbeAssert()
			osn.addExpectPullAssert(0)

			if !testCase.isProbeResponseDelayed {
				for _, block := range systemChannelBlocks {
					osn.blockResponses <- &orderer.DeliverResponse{
						Type: &orderer.DeliverResponse_Block{Block: block},
					}
				}
			} else {
				// Send a nil to force an EOF to the client
				osn.blockResponses <- nil
			}

			assert.PanicsWithValue(t, testCase.expectedPanic, func() { r.ReplicateChains() })
			bp.Close()
			dialer.assertAllConnectionsClosed(t)
		})
	}
}

func TestPullChannelFailure(t *testing.T) {
	blockchain := createBlockChain(0, 5)
	for _, testcase := range []struct {
		name                 string
		genesisBlockSequence int
		thirdBlockSequence   int
	}{
		{
			name:                 "Failed to pull genesis block",
			genesisBlockSequence: 1,
		},
		{
			name:                 "Failed to pull some non genesis block",
			genesisBlockSequence: 0,
			thirdBlockSequence:   0,
		},
	} {
		t.Run(testcase.name, func(t *testing.T) {
			lw := &mocks.LedgerWriter{}
			lw.On("Append", mock.Anything).Return(nil)
			lw.On("Height").Return(uint64(0))

			lf := &mocks.LedgerFactory{}
			lf.On("GetOrCreate", "mychannel").Return(lw, nil)

			osn := newClusterNode(t)
			defer osn.stop()

			enqueueBlock := func(seq int) {
				osn.blockResponses <- &orderer.DeliverResponse{
					Type: &orderer.DeliverResponse_Block{
						Block: blockchain[seq],
					},
				}
			}

			dialer := newCountingDialer()
			bp := newBlockPuller(dialer, osn.srv.Address())
			// Put a big timeout, to reduce chance of flakes when the server gets stuck
			// and we get an un-called for timeout.
			bp.FetchTimeout = time.Hour
			bp.MaxPullBlockRetries = 1
			// Do not buffer blocks in memory
			bp.MaxTotalBufferBytes = 1

			r := cluster.Replicator{
				Filter: cluster.AnyChannel,
				AmIPartOfChannel: func(configBlock *common.Block) error {
					return nil
				},
				Logger:        flogging.MustGetLogger("test"),
				SystemChannel: "system",
				LedgerFactory: lf,
				Puller:        bp,
			}

			osn.addExpectProbeAssert()
			enqueueBlock(5)
			osn.addExpectProbeAssert()
			enqueueBlock(5)
			osn.addExpectPullAssert(0)
			enqueueBlock(testcase.genesisBlockSequence)
			enqueueBlock(1)
			enqueueBlock(testcase.thirdBlockSequence)

			err := r.PullChannel("mychannel")
			assert.Equal(t, cluster.ErrRetryCountExhausted, err)
		})
	}

}

func TestPullerConfigFromTopLevelConfig(t *testing.T) {
	signer := &crypto.LocalSigner{}
	expected := cluster.PullerConfig{
		Channel:             "system",
		MaxTotalBufferBytes: 100,
		Signer:              signer,
		TLSCert:             []byte{3, 2, 1},
		TLSKey:              []byte{1, 2, 3},
		Timeout:             time.Hour,
	}

	topLevelConfig := &localconfig.TopLevel{
		General: localconfig.General{
			Cluster: localconfig.Cluster{
				ReplicationBufferSize: 100,
				RPCTimeout:            time.Hour,
			},
		},
	}

	config := cluster.PullerConfigFromTopLevelConfig("system", topLevelConfig, []byte{1, 2, 3}, []byte{3, 2, 1}, signer)
	assert.Equal(t, expected, config)
}

func TestReplicateChainsChannelClassificationFailure(t *testing.T) {
	// Scenario: We are unable to classify whether we are part of the channel,
	// so we crash, because this is a programming error.

	block30WithConfigBlockOf21 := common.NewBlock(30, nil)
	block30WithConfigBlockOf21.Metadata.Metadata[common.BlockMetadataIndex_LAST_CONFIG] = utils.MarshalOrPanic(&common.Metadata{
		Value: utils.MarshalOrPanic(&common.LastConfig{Index: 21}),
	})

	osn := newClusterNode(t)
	defer osn.stop()
	osn.blockResponses = make(chan *orderer.DeliverResponse, 1000)

	dialer := newCountingDialer()
	bp := newBlockPuller(dialer, osn.srv.Address())
	bp.FetchTimeout = time.Hour

	channelLister := &mocks.ChannelLister{}
	channelLister.On("Channels").Return([]cluster.ChannelGenesisBlock{{ChannelName: "A"}})
	channelLister.On("Close")

	// We probe for the latest block of the orderer
	osn.addExpectProbeAssert()
	osn.enqueueResponse(30)

	// And now pull it again (first poll and then pull it for real).
	osn.addExpectProbeAssert()
	osn.enqueueResponse(30)
	osn.addExpectPullAssert(30)
	osn.blockResponses <- &orderer.DeliverResponse{
		Type: &orderer.DeliverResponse_Block{Block: block30WithConfigBlockOf21},
	}
	// Now we pull the latest config block extracted from the previous block pulled.
	// Beforehand we reconnect to the orderer, so we put an artificial signal to close the stream on the server side,
	// in order to expect for a new stream to be established.
	osn.blockResponses <- nil
	// The orderer's last block's sequence is 30,
	osn.addExpectProbeAssert()
	osn.enqueueResponse(30)
	// And the Replicator now asks for block 21.
	osn.enqueueResponse(21)
	osn.addExpectPullAssert(21)

	r := cluster.Replicator{
		Filter: cluster.AnyChannel,
		AmIPartOfChannel: func(configBlock *common.Block) error {
			return errors.New("oops")
		},
		Logger:        flogging.MustGetLogger("test"),
		SystemChannel: "system",
		ChannelLister: channelLister,
		Puller:        bp,
	}

	assert.PanicsWithValue(t, "Failed classifying whether I belong to channel A: oops, skipping chain retrieval", func() {
		r.ReplicateChains()
	})

	bp.Close()
	dialer.assertAllConnectionsClosed(t)
}

func TestReplicateChainsGreenPath(t *testing.T) {
	// Scenario: There are 5 channels in the system: A-E.
	// We are in channel A but not in channel B, therefore
	// we should pull channel A and then the system channel.
	// However, this is not the first attempt of replication for
	// our node, but the second.
	// In the past, the node pulled 10 blocks of channel A and crashed.
	// Therefore, it should pull blocks, but commit for channel A
	// only blocks starting from block number 10.
	// For channel C - we are forbidden from pulling any blocks.
	// Channel D is a deserted channel - all OSNs have left it,
	// therefore we should not pull it at all.
	// Channel E cannot be pulled at all, due to the OSN being unavailable
	// at that time.

	systemChannelBlocks := createBlockChain(0, 21)
	block30WithConfigBlockOf21 := common.NewBlock(30, nil)
	block30WithConfigBlockOf21.Metadata.Metadata[common.BlockMetadataIndex_LAST_CONFIG] = utils.MarshalOrPanic(&common.Metadata{
		Value: utils.MarshalOrPanic(&common.LastConfig{Index: 21}),
	})

	osn := newClusterNode(t)
	defer osn.stop()
	osn.blockResponses = make(chan *orderer.DeliverResponse, 1000)

	dialer := newCountingDialer()
	bp := newBlockPuller(dialer, osn.srv.Address())
	bp.FetchTimeout = time.Hour
	bp.MaxPullBlockRetries = 1

	channelLister := &mocks.ChannelLister{}
	channelLister.On("Channels").Return([]cluster.ChannelGenesisBlock{
		{ChannelName: "E", GenesisBlock: fakeGB},
		{ChannelName: "D", GenesisBlock: fakeGB}, {ChannelName: "C", GenesisBlock: fakeGB},
		{ChannelName: "A", GenesisBlock: fakeGB}, {ChannelName: "B", GenesisBlock: fakeGB},
	})
	channelLister.On("Close")

	amIPartOfChannelMock := &mock.Mock{}
	// For channel A
	amIPartOfChannelMock.On("func13").Return(nil).Once()
	// For channel B
	amIPartOfChannelMock.On("func13").Return(cluster.ErrNotInChannel).Once()

	// 22 is for the system channel, and 31 is for channel A, and for channel B we only need 1 block (the GB).
	blocksCommittedToLedgerA := make(chan *common.Block, 31)
	blocksCommittedToLedgerB := make(chan *common.Block, 1)
	blocksCommittedToLedgerC := make(chan *common.Block, 1)
	blocksCommittedToLedgerD := make(chan *common.Block, 1)
	blocksCommittedToLedgerE := make(chan *common.Block, 1)
	blocksCommittedToSystemLedger := make(chan *common.Block, 22)
	// Put 10 blocks in the ledger of channel A, to simulate
	// that the ledger had blocks when the node started.
	for seq := 0; seq < 10; seq++ {
		blocksCommittedToLedgerA <- &common.Block{
			Header: &common.BlockHeader{Number: uint64(seq)},
		}
	}

	lwA := &mocks.LedgerWriter{}
	lwA.On("Append", mock.Anything).Return(nil).Run(func(arg mock.Arguments) {
		blocksCommittedToLedgerA <- arg.Get(0).(*common.Block)
	})
	lwA.On("Height").Return(func() uint64 {
		return uint64(len(blocksCommittedToLedgerA))
	})

	lwB := &mocks.LedgerWriter{}
	lwB.On("Height").Return(func() uint64 {
		return uint64(len(blocksCommittedToLedgerB))
	})
	lwB.On("Append", mock.Anything).Return(nil).Run(func(arg mock.Arguments) {
		blocksCommittedToLedgerB <- arg.Get(0).(*common.Block)
	})

	lwC := &mocks.LedgerWriter{}
	lwC.On("Height").Return(func() uint64 {
		return uint64(len(blocksCommittedToLedgerC))
	})
	lwC.On("Append", mock.Anything).Return(nil).Run(func(arg mock.Arguments) {
		blocksCommittedToLedgerC <- arg.Get(0).(*common.Block)
	})

	lwD := &mocks.LedgerWriter{}
	lwD.On("Height").Return(func() uint64 {
		return uint64(len(blocksCommittedToLedgerD))
	})
	lwD.On("Append", mock.Anything).Return(nil).Run(func(arg mock.Arguments) {
		blocksCommittedToLedgerD <- arg.Get(0).(*common.Block)
	})

	lwE := &mocks.LedgerWriter{}
	lwE.On("Height").Return(func() uint64 {
		return uint64(len(blocksCommittedToLedgerE))
	})
	lwE.On("Append", mock.Anything).Return(nil).Run(func(arg mock.Arguments) {
		blocksCommittedToLedgerE <- arg.Get(0).(*common.Block)
	})

	lwSystem := &mocks.LedgerWriter{}
	lwSystem.On("Append", mock.Anything).Return(nil).Run(func(arg mock.Arguments) {
		blocksCommittedToSystemLedger <- arg.Get(0).(*common.Block)
	})
	lwSystem.On("Height").Return(func() uint64 {
		return uint64(len(blocksCommittedToSystemLedger))
	})

	lf := &mocks.LedgerFactory{}
	lf.On("Close")
	lf.On("GetOrCreate", "A").Return(lwA, nil)
	lf.On("GetOrCreate", "B").Return(lwB, nil)
	lf.On("GetOrCreate", "C").Return(lwC, nil)
	lf.On("GetOrCreate", "D").Return(lwD, nil)
	lf.On("GetOrCreate", "E").Return(lwE, nil)
	lf.On("GetOrCreate", "system").Return(lwSystem, nil)

	r := cluster.Replicator{
		Filter:        cluster.AnyChannel,
		LedgerFactory: lf,
		AmIPartOfChannel: func(configBlock *common.Block) error {
			return amIPartOfChannelMock.Called().Error(0)
		},
		Logger:        flogging.MustGetLogger("test"),
		SystemChannel: "system",
		ChannelLister: channelLister,
		Puller:        bp,
		BootBlock:     systemChannelBlocks[21],
	}

	// The first thing the orderer gets is a seek to channel E.
	// Unfortunately, it's not available!
	osn.seekAssertions <- func(info *orderer.SeekInfo, actualChannel string) {
		// Ensure the seek came to the right channel
		assert.NotNil(osn.t, info.GetStart().GetNewest())
		assert.Equal(t, "E", actualChannel)
	}
	// Send an EOF down the stream.
	osn.blockResponses <- nil

	// The second thing the orderer gets is a seek to channel D,
	// which is followed by a response of service unavailable
	osn.seekAssertions <- func(info *orderer.SeekInfo, actualChannel string) {
		// Ensure the seek came to the right channel
		assert.NotNil(osn.t, info.GetStart().GetNewest())
		assert.Equal(t, "D", actualChannel)
	}
	osn.blockResponses <- &orderer.DeliverResponse{
		Type: &orderer.DeliverResponse_Status{
			Status: common.Status_SERVICE_UNAVAILABLE,
		},
	}

	// The third thing the orderer gets is a seek to channel C,
	// which is followed by a response of forbidden
	osn.seekAssertions <- func(info *orderer.SeekInfo, actualChannel string) {
		// Ensure the seek came to the right channel
		assert.NotNil(osn.t, info.GetStart().GetNewest())
		assert.Equal(t, "C", actualChannel)
	}

	osn.blockResponses <- &orderer.DeliverResponse{
		Type: &orderer.DeliverResponse_Status{
			Status: common.Status_FORBIDDEN,
		},
	}

	for _, channel := range []string{"A", "B"} {
		channel := channel
		// First, the orderer needs to figure out whether it is in the channel,
		// so it reaches to find the latest block from all orderers to get
		// the latest config block and see whether it is among the consenters.

		// Orderer is expecting a poll for last block of the current channel
		osn.seekAssertions <- func(info *orderer.SeekInfo, actualChannel string) {
			// Ensure the seek came to the right channel
			assert.NotNil(osn.t, info.GetStart().GetNewest())
			assert.Equal(t, channel, actualChannel)
		}

		// Orderer returns its last block is 30.
		// This is needed to get the latest height by comparing among all orderers.
		osn.enqueueResponse(30)

		// First we poll for the block sequence we got previously again, from some orderer.
		osn.addExpectProbeAssert()
		osn.enqueueResponse(30)

		// And afterwards pull the block from the first orderer.
		osn.addExpectPullAssert(30)
		osn.blockResponses <- &orderer.DeliverResponse{
			Type: &orderer.DeliverResponse_Block{Block: block30WithConfigBlockOf21},
		}
		// And the last config block is pulled via reconnecting to the orderer.
		osn.blockResponses <- nil
		// The orderer's last block's sequence is 30,
		osn.addExpectProbeAssert()
		osn.enqueueResponse(30)
		// And the Replicator now asks for block 21.
		osn.enqueueResponse(21)
		osn.addExpectPullAssert(21)
		// We always close the connection before attempting to pull the next block
		osn.blockResponses <- nil
	}

	// Next, the Replicator figures out the latest block sequence for that chain
	// to know until when to pull

	// We expect a probe for channel A only, because channel B isn't in the channel
	osn.seekAssertions <- func(info *orderer.SeekInfo, actualChannel string) {
		// Ensure the seek came to the right channel
		assert.NotNil(osn.t, info.GetStart().GetNewest())
		assert.Equal(t, "A", actualChannel)
	}
	osn.enqueueResponse(30)
	// From this point onwards, we pull the blocks for the chain.
	osn.enqueueResponse(30)
	osn.addExpectProbeAssert()
	osn.addExpectPullAssert(10)
	// Enqueue 31 blocks in its belly
	for _, block := range createBlockChain(10, 30) {
		osn.blockResponses <- &orderer.DeliverResponse{
			Type: &orderer.DeliverResponse_Block{Block: block},
		}
	}
	// Signal the orderer to stop sending us blocks since we're going to reconnect
	// to it to ask for the next channel
	osn.blockResponses <- nil

	// Now we define assertions for the system channel
	// Pull assertions for the system channel
	osn.seekAssertions <- func(info *orderer.SeekInfo, actualChannel string) {
		// Ensure the seek came to the system channel.
		assert.NotNil(osn.t, info.GetStart().GetNewest())
		assert.Equal(t, "system", actualChannel)
	}
	osn.blockResponses <- &orderer.DeliverResponse{
		Type: &orderer.DeliverResponse_Block{Block: systemChannelBlocks[21]},
	}
	osn.addExpectProbeAssert()
	osn.enqueueResponse(21)
	osn.addExpectPullAssert(0)
	for _, block := range systemChannelBlocks {
		osn.blockResponses <- &orderer.DeliverResponse{
			Type: &orderer.DeliverResponse_Block{Block: block},
		}
	}

	// This is where all the work is done.
	// The above lines were all assertions and preparations
	// for the expected flow of the test.
	r.ReplicateChains()

	// We replicated the chains, so all that left is to ensure
	// the blocks were committed in order, and all blocks we expected
	// to be committed (for channel A and the system channel) were committed.
	close(blocksCommittedToLedgerA)
	close(blocksCommittedToSystemLedger)
	assert.Len(t, blocksCommittedToLedgerA, cap(blocksCommittedToLedgerA))
	assert.Len(t, blocksCommittedToSystemLedger, cap(blocksCommittedToSystemLedger))
	assert.Len(t, blocksCommittedToLedgerB, 1)
	assert.Len(t, blocksCommittedToLedgerC, 1)
	assert.Len(t, blocksCommittedToLedgerD, 1)
	assert.Len(t, blocksCommittedToLedgerE, 1)
	// Count the blocks for channel A
	var expectedSequence uint64
	for block := range blocksCommittedToLedgerA {
		assert.Equal(t, expectedSequence, block.Header.Number)
		expectedSequence++
	}

	// Count the blocks for the system channel
	expectedSequence = uint64(0)
	for block := range blocksCommittedToSystemLedger {
		assert.Equal(t, expectedSequence, block.Header.Number)
		expectedSequence++
	}

	bp.Close()
	dialer.assertAllConnectionsClosed(t)
}

func TestParticipant(t *testing.T) {
	for _, testCase := range []struct {
		name                  string
		heightsByEndpoints    map[string]uint64
		heightsByEndpointsErr error
		latestBlockSeq        uint64
		latestBlock           *common.Block
		latestConfigBlockSeq  uint64
		latestConfigBlock     *common.Block
		expectedError         string
		predicateReturns      error
	}{
		{
			name:          "No available orderer",
			expectedError: cluster.ErrRetryCountExhausted.Error(),
		},
		{
			name:                  "Unauthorized for the channel",
			expectedError:         cluster.ErrForbidden.Error(),
			heightsByEndpointsErr: cluster.ErrForbidden,
		},
		{
			name:                  "No OSN services the channel",
			expectedError:         cluster.ErrServiceUnavailable.Error(),
			heightsByEndpointsErr: cluster.ErrServiceUnavailable,
		},
		{
			name: "Pulled block has no metadata",
			heightsByEndpoints: map[string]uint64{
				"orderer.example.com:7050": 100,
			},
			latestBlockSeq: uint64(99),
			latestBlock:    &common.Block{},
			expectedError:  "no metadata in block",
		},
		{
			name: "Pulled block has no last config sequence in metadata",
			heightsByEndpoints: map[string]uint64{
				"orderer.example.com:7050": 100,
			},
			latestBlockSeq: uint64(99),
			latestBlock: &common.Block{
				Metadata: &common.BlockMetadata{
					Metadata: [][]byte{{1, 2, 3}},
				},
			},
			expectedError: "no metadata in block",
		},
		{
			name: "Pulled block's metadata is malformed",
			heightsByEndpoints: map[string]uint64{
				"orderer.example.com:7050": 100,
			},
			latestBlockSeq: uint64(99),
			latestBlock: &common.Block{
				Metadata: &common.BlockMetadata{
					Metadata: [][]byte{{1, 2, 3}, {1, 2, 3}},
				},
			},
			expectedError: "error unmarshaling metadata from" +
				" block at index [LAST_CONFIG]: proto: common.Metadata: illegal tag 0 (wire type 1)",
		},
		{
			name: "Pulled block's metadata is valid and has a last config",
			heightsByEndpoints: map[string]uint64{
				"orderer.example.com:7050": 100,
			},
			latestBlockSeq: uint64(99),
			latestBlock: &common.Block{
				Metadata: &common.BlockMetadata{
					Metadata: [][]byte{{1, 2, 3}, utils.MarshalOrPanic(&common.Metadata{
						Value: utils.MarshalOrPanic(&common.LastConfig{
							Index: 42,
						}),
					})},
				},
			},
			latestConfigBlockSeq: 42,
			latestConfigBlock:    &common.Block{Header: &common.BlockHeader{Number: 42}},
			predicateReturns:     cluster.ErrNotInChannel,
		},
		{
			name:          "Failed pulling last block",
			expectedError: cluster.ErrRetryCountExhausted.Error(),
			heightsByEndpoints: map[string]uint64{
				"orderer.example.com:7050": 100,
			},
			latestBlockSeq: uint64(99),
			latestBlock:    nil,
		},
		{
			name:          "Failed pulling last config block",
			expectedError: cluster.ErrRetryCountExhausted.Error(),
			heightsByEndpoints: map[string]uint64{
				"orderer.example.com:7050": 100,
			},
			latestBlockSeq: uint64(99),
			latestBlock: &common.Block{
				Metadata: &common.BlockMetadata{
					Metadata: [][]byte{{1, 2, 3}, utils.MarshalOrPanic(&common.Metadata{
						Value: utils.MarshalOrPanic(&common.LastConfig{
							Index: 42,
						}),
					})},
				},
			},
			latestConfigBlockSeq: 42,
			latestConfigBlock:    nil,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			configBlocks := make(chan *common.Block, 1)
			predicate := func(configBlock *common.Block) error {
				configBlocks <- configBlock
				return testCase.predicateReturns
			}
			puller := &mocks.ChainPuller{}
			puller.On("HeightsByEndpoints").Return(testCase.heightsByEndpoints, testCase.heightsByEndpointsErr)
			puller.On("PullBlock", testCase.latestBlockSeq).Return(testCase.latestBlock)
			puller.On("PullBlock", testCase.latestConfigBlockSeq).Return(testCase.latestConfigBlock)
			puller.On("Close")

			err := cluster.Participant(puller, predicate)
			if testCase.expectedError != "" {
				assert.EqualError(t, err, testCase.expectedError)
				assert.Len(t, configBlocks, 0)
			} else {
				assert.Len(t, configBlocks, 1)
				assert.Equal(t, err, testCase.predicateReturns)
			}
		})
	}
}

func TestBlockPullerFromConfigBlockFailures(t *testing.T) {
	blockBytes, err := ioutil.ReadFile("testdata/mychannel.block")
	assert.NoError(t, err)

	validBlock := &common.Block{}
	assert.NoError(t, proto.Unmarshal(blockBytes, validBlock))

	for _, testCase := range []struct {
		name         string
		expectedErr  string
		pullerConfig cluster.PullerConfig
		block        *common.Block
	}{
		{
			name:        "nil block",
			expectedErr: "nil block",
		},
		{
			name:        "invalid block",
			expectedErr: "block data is nil",
			block:       &common.Block{},
		},
		{
			name: "bad envelope inside block",
			expectedErr: "failed extracting bundle from envelope: " +
				"failed to unmarshal payload from envelope: " +
				"error unmarshaling Payload: " +
				"proto: common.Payload: illegal tag 0 (wire type 1)",
			block: &common.Block{
				Data: &common.BlockData{
					Data: [][]byte{utils.MarshalOrPanic(&common.Envelope{
						Payload: []byte{1, 2, 3},
					})},
				},
			},
		},
		{
			name:        "invalid TLS certificate",
			expectedErr: "unable to decode TLS certificate PEM: ////",
			block:       validBlock,
			pullerConfig: cluster.PullerConfig{
				TLSCert: []byte{255, 255, 255},
			},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			verifierRetriever := &mocks.VerifierRetriever{}
			verifierRetriever.On("RetrieveVerifier", mock.Anything).Return(&cluster.NoopBlockVerifier{})
			bp, err := cluster.BlockPullerFromConfigBlock(testCase.pullerConfig, testCase.block, verifierRetriever)
			assert.EqualError(t, err, testCase.expectedErr)
			assert.Nil(t, bp)
		})
	}
}

func testBlockPullerFromConfig(t *testing.T, blockVerifiers []cluster.BlockVerifier, expectedLogMsg string, iterations int) {
	verifierRetriever := &mocks.VerifierRetriever{}
	for _, blockVerifier := range blockVerifiers {
		verifierRetriever.On("RetrieveVerifier", mock.Anything).Return(blockVerifier).Once()
	}

	caCert, err := ioutil.ReadFile(filepath.Join("testdata", "ca.crt"))
	assert.NoError(t, err)

	tlsCert, err := ioutil.ReadFile(filepath.Join("testdata", "server.crt"))
	assert.NoError(t, err)

	tlsKey, err := ioutil.ReadFile(filepath.Join("testdata", "server.key"))
	assert.NoError(t, err)

	osn := newClusterNode(t)
	osn.srv.Stop()
	// Replace the gRPC server with a TLS one
	osn.srv, err = comm.NewGRPCServer("127.0.0.1:0", comm.ServerConfig{
		SecOpts: &comm.SecureOptions{
			Key:               tlsKey,
			RequireClientCert: true,
			Certificate:       tlsCert,
			ClientRootCAs:     [][]byte{caCert},
			UseTLS:            true,
		},
	})
	assert.NoError(t, err)
	orderer.RegisterAtomicBroadcastServer(osn.srv.Server(), osn)
	// And start it
	go osn.srv.Start()
	defer osn.stop()

	// Start from a valid configuration block
	blockBytes, err := ioutil.ReadFile(filepath.Join("testdata", "mychannel.block"))
	assert.NoError(t, err)

	validBlock := &common.Block{}
	assert.NoError(t, proto.Unmarshal(blockBytes, validBlock))

	// And inject into it a 127.0.0.1 orderer endpoint endpoint and a new TLS CA certificate.
	injectTLSCACert(t, validBlock, caCert)
	injectGlobalOrdererEndpoint(t, validBlock, osn.srv.Address())
	validBlock.Header.DataHash = validBlock.Data.Hash()

	for attempt := 0; attempt < iterations; attempt++ {
		blockMsg := &orderer.DeliverResponse_Block{
			Block: validBlock,
		}

		osn.blockResponses <- &orderer.DeliverResponse{
			Type: blockMsg,
		}

		osn.blockResponses <- &orderer.DeliverResponse{
			Type: blockMsg,
		}

		osn.blockResponses <- nil

		osn.addExpectProbeAssert()
		osn.addExpectPullAssert(0)
	}

	bp, err := cluster.BlockPullerFromConfigBlock(cluster.PullerConfig{
		TLSCert:             tlsCert,
		TLSKey:              tlsKey,
		MaxTotalBufferBytes: 1,
		Channel:             "mychannel",
		Signer:              &crypto.LocalSigner{},
		Timeout:             time.Hour,
	}, validBlock, verifierRetriever)
	bp.RetryTimeout = time.Millisecond * 10
	assert.NoError(t, err)
	defer bp.Close()

	var seenExpectedLogMsg bool

	bp.Logger = bp.Logger.WithOptions(zap.Hooks(func(entry zapcore.Entry) error {
		if strings.Contains(entry.Message, expectedLogMsg) {
			seenExpectedLogMsg = true
		}
		return nil
	}))

	block := bp.PullBlock(0)
	assert.Equal(t, uint64(0), block.Header.Number)
	assert.True(t, seenExpectedLogMsg)
}

func TestSkipPullingPulledChannels(t *testing.T) {
	blockchain := createBlockChain(0, 5)
	lw := &mocks.LedgerWriter{}
	lw.On("Height").Return(uint64(6))

	lf := &mocks.LedgerFactory{}
	lf.On("GetOrCreate", "mychannel").Return(lw, nil)

	osn := newClusterNode(t)
	defer osn.stop()

	enqueueBlock := func(seq int) {
		osn.blockResponses <- &orderer.DeliverResponse{
			Type: &orderer.DeliverResponse_Block{
				Block: blockchain[seq],
			},
		}
	}

	dialer := newCountingDialer()
	bp := newBlockPuller(dialer, osn.srv.Address())
	bp.FetchTimeout = time.Hour

	r := cluster.Replicator{
		Filter: cluster.AnyChannel,
		AmIPartOfChannel: func(configBlock *common.Block) error {
			return nil
		},
		Logger:        flogging.MustGetLogger("test"),
		SystemChannel: "system",
		LedgerFactory: lf,
		Puller:        bp,
	}

	var detectedChannelPulled bool
	r.Logger = r.Logger.WithOptions(zap.Hooks(func(entry zapcore.Entry) error {
		if strings.Contains(entry.Message, "Latest height found (6) is equal to our height, skipping pulling channel mychannel") {
			detectedChannelPulled = true
		}
		return nil
	}))

	osn.addExpectProbeAssert()
	enqueueBlock(5)
	osn.addExpectProbeAssert()
	enqueueBlock(5)

	err := r.PullChannel("mychannel")
	assert.NoError(t, err)
	assert.True(t, detectedChannelPulled)
}

func TestBlockPullerFromConfigBlockGreenPath(t *testing.T) {
	for _, testCase := range []struct {
		description        string
		blockVerifiers     []cluster.BlockVerifier
		expectedLogMessage string
		iterations         int
	}{
		{
			description:        "Success",
			blockVerifiers:     []cluster.BlockVerifier{&cluster.NoopBlockVerifier{}},
			expectedLogMessage: "Got block [0] of size",
			iterations:         1,
		},
		{
			description: "Failure",
			iterations:  2,
			// First time it returns nil, second time returns like the success case
			blockVerifiers: []cluster.BlockVerifier{nil, &cluster.NoopBlockVerifier{}},
			expectedLogMessage: "Failed verifying received blocks: " +
				"couldn't acquire verifier for channel mychannel",
		},
	} {
		t.Run(testCase.description, func(t *testing.T) {
			testBlockPullerFromConfig(t, testCase.blockVerifiers,
				testCase.expectedLogMessage, testCase.iterations)
		})
	}
}

func TestNoopBlockVerifier(t *testing.T) {
	v := &cluster.NoopBlockVerifier{}
	assert.Nil(t, v.VerifyBlockSignature(nil, nil))
}

func injectGlobalOrdererEndpoint(t *testing.T, block *common.Block, endpoint string) {
	ordererAddresses := channelconfig.OrdererAddressesValue([]string{endpoint})
	// Unwrap the layers until we reach the orderer addresses
	env, err := utils.ExtractEnvelope(block, 0)
	assert.NoError(t, err)
	payload, err := utils.ExtractPayload(env)
	assert.NoError(t, err)
	confEnv, err := configtx.UnmarshalConfigEnvelope(payload.Data)
	assert.NoError(t, err)
	// Replace the orderer addresses
	confEnv.Config.ChannelGroup.Values[ordererAddresses.Key()].Value = utils.MarshalOrPanic(ordererAddresses.Value())
	// Remove the per org addresses, if applicable
	ordererGrps := confEnv.Config.ChannelGroup.Groups[channelconfig.OrdererGroupKey].Groups
	for _, grp := range ordererGrps {
		if grp.Values[channelconfig.EndpointsKey] == nil {
			continue
		}
		grp.Values[channelconfig.EndpointsKey].Value = nil
	}
	// And put it back into the block
	payload.Data = utils.MarshalOrPanic(confEnv)
	env.Payload = utils.MarshalOrPanic(payload)
	block.Data.Data[0] = utils.MarshalOrPanic(env)
}

func injectTLSCACert(t *testing.T, block *common.Block, tlsCA []byte) {
	// Unwrap the layers until we reach the TLS CA certificates
	env, err := utils.ExtractEnvelope(block, 0)
	assert.NoError(t, err)
	payload, err := utils.ExtractPayload(env)
	assert.NoError(t, err)
	confEnv, err := configtx.UnmarshalConfigEnvelope(payload.Data)
	assert.NoError(t, err)
	mspKey := confEnv.Config.ChannelGroup.Groups[channelconfig.OrdererGroupKey].Groups["OrdererOrg"].Values[channelconfig.MSPKey]
	rawMSPConfig := mspKey.Value
	mspConf := &msp.MSPConfig{}
	proto.Unmarshal(rawMSPConfig, mspConf)
	fabricMSPConf := &msp.FabricMSPConfig{}
	proto.Unmarshal(mspConf.Config, fabricMSPConf)
	// Replace the TLS root certs with the given ones
	fabricMSPConf.TlsRootCerts = [][]byte{tlsCA}
	// And put it back into the block
	mspConf.Config = utils.MarshalOrPanic(fabricMSPConf)
	mspKey.Value = utils.MarshalOrPanic(mspConf)
	payload.Data = utils.MarshalOrPanic(confEnv)
	env.Payload = utils.MarshalOrPanic(payload)
	block.Data.Data[0] = utils.MarshalOrPanic(env)
}

func TestIsNewChannelBlock(t *testing.T) {
	for _, testCase := range []struct {
		name         string
		expectedErr  string
		returnedName string
		block        *common.Block
	}{
		{
			name:        "nil block",
			expectedErr: "nil block",
		},
		{
			name:        "no data section in block",
			expectedErr: "block data is nil",
			block:       &common.Block{},
		},
		{
			name: "corrupt envelope in block",
			expectedErr: "block data does not carry an" +
				" envelope at index 0: error unmarshaling Envelope: proto: common.Envelope: illegal tag 0 (wire type 1)",
			block: &common.Block{
				Data: &common.BlockData{
					Data: [][]byte{{1, 2, 3}},
				},
			},
		},
		{
			name:        "corrupt payload in envelope",
			expectedErr: "no payload in envelope: proto: common.Payload: illegal tag 0 (wire type 1)",
			block: &common.Block{
				Data: &common.BlockData{
					Data: [][]byte{utils.MarshalOrPanic(&common.Envelope{
						Payload: []byte{1, 2, 3},
					})},
				},
			},
		},
		{
			name:        "no header in block",
			expectedErr: "nil header in payload",
			block: &common.Block{
				Data: &common.BlockData{
					Data: [][]byte{utils.MarshalOrPanic(&common.Envelope{
						Payload: utils.MarshalOrPanic(&common.Payload{}),
					})},
				},
			},
		},
		{
			name: "corrupt channel header",
			expectedErr: "error unmarshaling ChannelHeader:" +
				" proto: common.ChannelHeader: illegal tag 0 (wire type 1)",
			block: &common.Block{
				Data: &common.BlockData{
					Data: [][]byte{utils.MarshalOrPanic(&common.Envelope{
						Payload: utils.MarshalOrPanic(&common.Payload{
							Header: &common.Header{
								ChannelHeader: []byte{1, 2, 3},
							},
						}),
					})},
				},
			},
		},
		{
			name:        "not an orderer transaction",
			expectedErr: "",
			block: &common.Block{
				Data: &common.BlockData{
					Data: [][]byte{utils.MarshalOrPanic(&common.Envelope{
						Payload: utils.MarshalOrPanic(&common.Payload{
							Header: &common.Header{
								ChannelHeader: utils.MarshalOrPanic(&common.ChannelHeader{
									Type: int32(common.HeaderType_CONFIG_UPDATE),
								}),
							},
						}),
					})},
				},
			},
		},
		{
			name:        "orderer transaction with corrupt inner envelope",
			expectedErr: "error unmarshaling Envelope: proto: common.Envelope: illegal tag 0 (wire type 1)",
			block: &common.Block{
				Data: &common.BlockData{
					Data: [][]byte{utils.MarshalOrPanic(&common.Envelope{
						Payload: utils.MarshalOrPanic(&common.Payload{
							Header: &common.Header{
								ChannelHeader: utils.MarshalOrPanic(&common.ChannelHeader{
									Type: int32(common.HeaderType_ORDERER_TRANSACTION),
								}),
							},
							Data: []byte{1, 2, 3},
						}),
					})},
				},
			},
		},
		{
			name:        "orderer transaction with corrupt inner payload",
			expectedErr: "error unmarshaling Payload: proto: common.Payload: illegal tag 0 (wire type 1)",
			block: &common.Block{
				Data: &common.BlockData{
					Data: [][]byte{utils.MarshalOrPanic(&common.Envelope{
						Payload: utils.MarshalOrPanic(&common.Payload{
							Header: &common.Header{
								ChannelHeader: utils.MarshalOrPanic(&common.ChannelHeader{
									Type: int32(common.HeaderType_ORDERER_TRANSACTION),
								}),
							},
							Data: utils.MarshalOrPanic(&common.Envelope{
								Payload: []byte{1, 2, 3},
							}),
						}),
					})},
				},
			},
		},
		{
			name:        "orderer transaction with nil inner header",
			expectedErr: "inner payload's header is nil",
			block: &common.Block{
				Data: &common.BlockData{
					Data: [][]byte{utils.MarshalOrPanic(&common.Envelope{
						Payload: utils.MarshalOrPanic(&common.Payload{
							Header: &common.Header{
								ChannelHeader: utils.MarshalOrPanic(&common.ChannelHeader{
									Type: int32(common.HeaderType_ORDERER_TRANSACTION),
								}),
							},
							Data: utils.MarshalOrPanic(&common.Envelope{
								Payload: utils.MarshalOrPanic(&common.Payload{}),
							}),
						}),
					})},
				},
			},
		},
		{
			name:        "orderer transaction with corrupt inner channel header",
			expectedErr: "error unmarshaling ChannelHeader: proto: common.ChannelHeader: illegal tag 0 (wire type 1)",
			block: &common.Block{
				Data: &common.BlockData{
					Data: [][]byte{utils.MarshalOrPanic(&common.Envelope{
						Payload: utils.MarshalOrPanic(&common.Payload{
							Header: &common.Header{
								ChannelHeader: utils.MarshalOrPanic(&common.ChannelHeader{
									Type: int32(common.HeaderType_ORDERER_TRANSACTION),
								}),
							},
							Data: utils.MarshalOrPanic(&common.Envelope{
								Payload: utils.MarshalOrPanic(&common.Payload{
									Header: &common.Header{
										ChannelHeader: []byte{1, 2, 3},
									},
								}),
							}),
						}),
					})},
				},
			},
		},
		{
			name:        "orderer transaction that is not a config, but a config update",
			expectedErr: "",
			block: &common.Block{
				Data: &common.BlockData{
					Data: [][]byte{utils.MarshalOrPanic(&common.Envelope{
						Payload: utils.MarshalOrPanic(&common.Payload{
							Header: &common.Header{
								ChannelHeader: utils.MarshalOrPanic(&common.ChannelHeader{
									Type: int32(common.HeaderType_ORDERER_TRANSACTION),
								}),
							},
							Data: utils.MarshalOrPanic(&common.Envelope{
								Payload: utils.MarshalOrPanic(&common.Payload{
									Header: &common.Header{
										ChannelHeader: utils.MarshalOrPanic(&common.ChannelHeader{
											Type: int32(common.HeaderType_CONFIG_UPDATE),
										}),
									},
								}),
							}),
						}),
					})},
				},
			},
		},
		{
			expectedErr: "",
			name:        "orderer transaction that is a system channel config block",
			block: &common.Block{
				Data: &common.BlockData{
					Data: [][]byte{utils.MarshalOrPanic(&common.Envelope{
						Payload: utils.MarshalOrPanic(&common.Payload{
							Header: &common.Header{
								ChannelHeader: utils.MarshalOrPanic(&common.ChannelHeader{
									ChannelId: "systemChannel",
									Type:      int32(common.HeaderType_ORDERER_TRANSACTION),
								}),
							},
							Data: utils.MarshalOrPanic(&common.Envelope{
								Payload: utils.MarshalOrPanic(&common.Payload{
									Header: &common.Header{
										ChannelHeader: utils.MarshalOrPanic(&common.ChannelHeader{
											Type:      int32(common.HeaderType_CONFIG),
											ChannelId: "systemChannel",
										}),
									},
								}),
							}),
						}),
					})},
				},
			},
		},
		{
			name:         "orderer transaction that creates a new application channel",
			expectedErr:  "",
			returnedName: "notSystemChannel",
			block: &common.Block{
				Data: &common.BlockData{
					Data: [][]byte{utils.MarshalOrPanic(&common.Envelope{
						Payload: utils.MarshalOrPanic(&common.Payload{
							Header: &common.Header{
								ChannelHeader: utils.MarshalOrPanic(&common.ChannelHeader{
									ChannelId: "systemChannel",
									Type:      int32(common.HeaderType_ORDERER_TRANSACTION),
								}),
							},
							Data: utils.MarshalOrPanic(&common.Envelope{
								Payload: utils.MarshalOrPanic(&common.Payload{
									Header: &common.Header{
										ChannelHeader: utils.MarshalOrPanic(&common.ChannelHeader{
											Type:      int32(common.HeaderType_CONFIG),
											ChannelId: "notSystemChannel",
										}),
									},
								}),
							}),
						}),
					})},
				},
			},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			channelName, err := cluster.IsNewChannelBlock(testCase.block)
			if testCase.expectedErr != "" {
				assert.EqualError(t, err, testCase.expectedErr)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, testCase.returnedName, channelName)
		})
	}
}

func TestChannels(t *testing.T) {
	makeBlock := func(outerChannelName, innerChannelName string) *common.Block {
		return &common.Block{
			Header: &common.BlockHeader{},
			Data: &common.BlockData{
				Data: [][]byte{utils.MarshalOrPanic(&common.Envelope{
					Payload: utils.MarshalOrPanic(&common.Payload{
						Header: &common.Header{
							ChannelHeader: utils.MarshalOrPanic(&common.ChannelHeader{
								ChannelId: outerChannelName,
								Type:      int32(common.HeaderType_ORDERER_TRANSACTION),
							}),
						},
						Data: utils.MarshalOrPanic(&common.Envelope{
							Payload: utils.MarshalOrPanic(&common.Payload{
								Header: &common.Header{
									ChannelHeader: utils.MarshalOrPanic(&common.ChannelHeader{
										Type:      int32(common.HeaderType_CONFIG),
										ChannelId: innerChannelName,
									}),
								},
							}),
						}),
					}),
				})},
			},
		}
	}

	for _, testCase := range []struct {
		name               string
		prepareSystemChain func(systemChain []*common.Block)
		assertion          func(t *testing.T, ci *cluster.ChainInspector)
	}{
		{
			name: "happy path - artificial blocks",
			prepareSystemChain: func(systemChain []*common.Block) {
				assignHashes(systemChain)
			},
			assertion: func(t *testing.T, ci *cluster.ChainInspector) {
				actual := cluster.GenesisBlocks(ci.Channels())
				// Assert that the returned channels are returned in any order
				assert.Contains(t, [][]string{{"mychannel", "mychannel2"}, {"mychannel2", "mychannel"}}, actual.Names())
			},
		},
		{
			name: "happy path - one block is not artificial but real",
			prepareSystemChain: func(systemChain []*common.Block) {
				blockbytes, err := ioutil.ReadFile(filepath.Join("testdata", "block3.pb"))
				assert.NoError(t, err)
				block := &common.Block{}
				err = proto.Unmarshal(blockbytes, block)
				assert.NoError(t, err)

				systemChain[len(systemChain)/2-1] = block
				assignHashes(systemChain)
			},
			assertion: func(t *testing.T, ci *cluster.ChainInspector) {
				actual := cluster.GenesisBlocks(ci.Channels())
				// Assert that the returned channels are returned in any order
				assert.Contains(t, [][]string{{"mychannel2", "bar"}, {"bar", "mychannel2"}}, actual.Names())
			},
		},
		{
			name: "bad path - pulled chain's last block hash doesn't match the last config block",
			prepareSystemChain: func(systemChain []*common.Block) {
				assignHashes(systemChain)
				systemChain[len(systemChain)-1].Header.PreviousHash = nil
			},
			assertion: func(t *testing.T, ci *cluster.ChainInspector) {
				panicValue := "System channel pulled doesn't match the boot last config block:" +
					" block [2]'s hash (bc4ef5cc8a61ac0747cc82df58bac9ad3278622c1cfc7a119b9b1068e422c9f1)" +
					" mismatches 3's prev block hash ()"
				assert.PanicsWithValue(t, panicValue, func() {
					ci.Channels()
				})
			},
		},
		{
			name: "bad path - hash chain mismatch",
			prepareSystemChain: func(systemChain []*common.Block) {
				assignHashes(systemChain)
				systemChain[len(systemChain)-2].Header.PreviousHash = nil
			},
			assertion: func(t *testing.T, ci *cluster.ChainInspector) {
				panicValue := "Claimed previous hash of block [2] is  but actual previous " +
					"hash is 920faeb0bd8a02b3f2553247359fb3b684819c75c6e5487bc7eed632841ddc5f"
				assert.PanicsWithValue(t, panicValue, func() {
					ci.Channels()
				})
			},
		},
		{
			name: "bad path - a block cannot be classified",
			prepareSystemChain: func(systemChain []*common.Block) {
				assignHashes(systemChain)
				systemChain[len(systemChain)-2].Data.Data = [][]byte{{1, 2, 3}}
			},
			assertion: func(t *testing.T, ci *cluster.ChainInspector) {
				panicValue := "Failed classifying block [2]: block data does not carry" +
					" an envelope at index 0: error unmarshaling Envelope: " +
					"proto: common.Envelope: illegal tag 0 (wire type 1)"
				assert.PanicsWithValue(t, panicValue, func() {
					ci.Channels()
				})
			},
		},
		{
			name: "bad path - failed pulling blocks",
			prepareSystemChain: func(systemChain []*common.Block) {
				assignHashes(systemChain)
				// Setting a block to nil makes the block puller return nil,
				// which signals failure of pulling a block.
				systemChain[len(systemChain)/2] = nil
			},
			assertion: func(t *testing.T, ci *cluster.ChainInspector) {
				panicValue := "Failed pulling block [2] from the system channel"
				assert.PanicsWithValue(t, panicValue, func() {
					ci.Channels()
				})
			},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			systemChain := []*common.Block{
				makeBlock("systemChannel", "systemChannel"),
				makeBlock("systemChannel", "mychannel"),
				makeBlock("systemChannel", "mychannel2"),
				makeBlock("systemChannel", "systemChannel"),
			}

			for i := 0; i < len(systemChain); i++ {
				systemChain[i].Header.DataHash = systemChain[i].Data.Hash()
				systemChain[i].Header.Number = uint64(i)
			}
			testCase.prepareSystemChain(systemChain)
			puller := &mocks.ChainPuller{}
			puller.On("Close")
			for seq := uint64(0); int(seq) < len(systemChain)-1; seq++ {
				puller.On("PullBlock", seq).Return(systemChain[int(seq)])
			}

			ci := &cluster.ChainInspector{
				Logger:          flogging.MustGetLogger("test"),
				Puller:          puller,
				LastConfigBlock: systemChain[len(systemChain)-1],
			}
			defer puller.AssertNumberOfCalls(t, "Close", 1)
			defer ci.Close()
			testCase.assertion(t, ci)
		})
	}
}

var fakeGB = &common.Block{
	Header: &common.BlockHeader{},
	Metadata: &common.BlockMetadata{
		Metadata: [][]byte{{}, {}, {}, {}},
	},
	Data: &common.BlockData{
		Data: [][]byte{
			utils.MarshalOrPanic(&common.Envelope{
				Payload: utils.MarshalOrPanic(&common.Envelope{
					Payload: utils.MarshalOrPanic(&common.Config{
						Sequence: 1,
					}),
				}),
			}),
		},
	},
}

func simulateNonParticipantChannelPull(osn *deliverServer) {
	lastBlock := common.NewBlock(1, nil)
	lastBlock.Metadata.Metadata[common.BlockMetadataIndex_LAST_CONFIG] = utils.MarshalOrPanic(&common.Metadata{
		Value: utils.MarshalOrPanic(&common.LastConfig{Index: 0}),
	})
	// We first present a channel with a last block of 'lastBlock', that points to
	// the genesis block
	osn.addExpectProbeAssert()
	osn.blockResponses <- &orderer.DeliverResponse{
		Type: &orderer.DeliverResponse_Block{Block: lastBlock},
	}
	osn.addExpectProbeAssert()
	osn.blockResponses <- &orderer.DeliverResponse{
		Type: &orderer.DeliverResponse_Block{Block: lastBlock},
	}
	osn.addExpectPullAssert(1)
	osn.blockResponses <- &orderer.DeliverResponse{
		Type: &orderer.DeliverResponse_Block{Block: lastBlock},
	}
	osn.blockResponses <- nil

	// and make it send back the genesis block.
	// First send is for probing,
	osn.addExpectProbeAssert()
	osn.blockResponses <- &orderer.DeliverResponse{
		Type: &orderer.DeliverResponse_Block{Block: fakeGB},
	}
	osn.addExpectPullAssert(0)
	// and the second one sends the actual block itself downstream
	osn.blockResponses <- &orderer.DeliverResponse{
		Type: &orderer.DeliverResponse_Block{Block: fakeGB},
	}

	osn.blockResponses <- nil
}

func TestChannelCreationBlockToGenesisBlock(t *testing.T) {
	for _, testCase := range []struct {
		name        string
		expectedErr string
		block       *common.Block
	}{
		{
			name:        "nil block",
			expectedErr: "nil block",
		},
		{
			name:        "no data",
			expectedErr: "block data is nil",
			block:       &common.Block{},
		},
		{
			name:        "no block data",
			expectedErr: "envelope index out of bounds",
			block: &common.Block{
				Data: &common.BlockData{},
			},
		},
		{
			name: "bad block data",
			expectedErr: "block data does not carry an envelope at index 0:" +
				" error unmarshaling Envelope: proto: common.Envelope:" +
				" illegal tag 0 (wire type 1)",
			block: &common.Block{
				Data: &common.BlockData{
					Data: [][]byte{{1, 2, 3}},
				},
			},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := cluster.ChannelCreationBlockToGenesisBlock(testCase.block)
			assert.EqualError(t, err, testCase.expectedErr)
		})
	}
}

func TestFilter(t *testing.T) {
	logger := flogging.MustGetLogger("test")
	logger = logger.WithOptions(zap.Hooks(func(entry zapcore.Entry) error {
		assert.Equal(t, "Channel foo shouldn't be pulled. Skipping it", entry.Message)
		return nil
	}))

	r := &cluster.Replicator{
		Filter: func(_ string) bool {
			return false
		},
		Logger: logger,
	}
	assert.Equal(t, cluster.ErrSkipped, r.PullChannel("foo"))
}
