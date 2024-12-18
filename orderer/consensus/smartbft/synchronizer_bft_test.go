/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package smartbft_test

import (
	"os"
	"sync"
	"testing"

	"github.com/hyperledger-labs/SmartBFT/pkg/types"
	"github.com/hyperledger-labs/SmartBFT/smartbftprotos"
	"github.com/hyperledger/fabric-lib-go/common/flogging"
	cb "github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/hyperledger/fabric-protos-go-apiv2/orderer"
	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/common/deliverclient"
	"github.com/hyperledger/fabric/internal/pkg/comm"
	"github.com/hyperledger/fabric/orderer/common/cluster"
	"github.com/hyperledger/fabric/orderer/common/localconfig"
	mocks2 "github.com/hyperledger/fabric/orderer/consensus/mocks"
	"github.com/hyperledger/fabric/orderer/consensus/smartbft"
	"github.com/hyperledger/fabric/orderer/consensus/smartbft/mocks"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

//go:generate counterfeiter -o mocks/updatable_block_verifier.go --fake-name UpdatableBlockVerifier . updatableBlockVerifier
type updatableBlockVerifier interface {
	deliverclient.CloneableUpdatableBlockVerifier
}

//go:generate counterfeiter -o mocks/orderer_config.go --fake-name OrdererConfig . ordererConfig
type ordererConfig interface {
	channelconfig.Orderer
}

func TestBFTSynchronizer(t *testing.T) {
	flogging.ActivateSpec("debug")
	blockBytes, err := os.ReadFile("testdata/mychannel.block")
	require.NoError(t, err)

	goodConfigBlock := &cb.Block{}
	require.NoError(t, proto.Unmarshal(blockBytes, goodConfigBlock))

	b42 := makeConfigBlockWithMetadata(goodConfigBlock, 42, &smartbftprotos.ViewMetadata{ViewId: 1, LatestSequence: 8})
	b99 := makeBlockWithMetadata(99, 42, &smartbftprotos.ViewMetadata{ViewId: 1, LatestSequence: 12})
	b100 := makeBlockWithMetadata(100, 42, &smartbftprotos.ViewMetadata{ViewId: 1, LatestSequence: 13})
	b101 := makeConfigBlockWithMetadata(goodConfigBlock, 101, &smartbftprotos.ViewMetadata{ViewId: 2, LatestSequence: 1})
	b102 := makeBlockWithMetadata(102, 101, &smartbftprotos.ViewMetadata{ViewId: 2, LatestSequence: 3})

	blockNum2configSqn := map[uint64]uint64{
		99:  7,
		100: 7,
		101: 8,
		102: 8,
	}

	t.Run("no remote endpoints but myself", func(t *testing.T) {
		bp := &mocks.FakeBlockPuller{}
		bpf := &mocks.FakeBlockPullerFactory{}
		bpf.CreateBlockPullerReturns(bp, nil)

		bp.HeightsByEndpointsReturns(
			map[string]uint64{
				"example.com:1": 100,
			},
			"example.com:1",
			nil,
		)

		fakeCS := &mocks2.FakeConsenterSupport{}
		fakeCS.HeightReturns(100)
		fakeCS.BlockReturns(b99)

		decision := &types.SyncResponse{
			Latest: types.Decision{
				Proposal:   types.Proposal{Header: []byte{1, 1, 1, 1}},
				Signatures: []types.Signature{{ID: 1}, {ID: 2}, {ID: 3}},
			},
			Reconfig: types.ReconfigSync{
				InReplicatedDecisions: false,
				CurrentNodes:          []uint64{1, 2, 3, 4},
				CurrentConfig:         types.Configuration{SelfID: 1},
			},
		}

		bftSynchronizer := &smartbft.BFTSynchronizer{
			LatestConfig: func() (types.Configuration, []uint64) {
				return types.Configuration{
					SelfID: 1,
				}, []uint64{1, 2, 3, 4}
			},
			BlockToDecision: func(block *cb.Block) *types.Decision {
				if block == b99 {
					return &decision.Latest
				}
				return nil
			},
			OnCommit:           noopUpdateLastHash,
			Support:            fakeCS,
			LocalConfigCluster: localconfig.Cluster{},
			BlockPullerFactory: bpf,
			Logger:             flogging.MustGetLogger("test.smartbft"),
		}

		require.NotNil(t, bftSynchronizer)

		resp := bftSynchronizer.Sync()
		require.NotNil(t, resp)
		require.Equal(t, *decision, resp)
	})

	t.Run("no remote endpoints", func(t *testing.T) {
		bp := &mocks.FakeBlockPuller{}
		bpf := &mocks.FakeBlockPullerFactory{}
		bpf.CreateBlockPullerReturns(bp, nil)

		bp.HeightsByEndpointsReturns(map[string]uint64{}, "", nil)

		fakeCS := &mocks2.FakeConsenterSupport{}
		fakeCS.HeightReturns(100)
		fakeCS.BlockReturns(b99)

		decision := &types.SyncResponse{
			Latest: types.Decision{
				Proposal:   types.Proposal{Header: []byte{1, 1, 1, 1}},
				Signatures: []types.Signature{{ID: 1}, {ID: 2}, {ID: 3}},
			},
			Reconfig: types.ReconfigSync{
				InReplicatedDecisions: false,
				CurrentNodes:          []uint64{1, 2, 3, 4},
				CurrentConfig:         types.Configuration{SelfID: 1},
			},
		}

		bftSynchronizer := &smartbft.BFTSynchronizer{
			LatestConfig: func() (types.Configuration, []uint64) {
				return types.Configuration{
					SelfID: 1,
				}, []uint64{1, 2, 3, 4}
			},
			BlockToDecision: func(block *cb.Block) *types.Decision {
				if block == b99 {
					return &decision.Latest
				}
				return nil
			},
			OnCommit:           noopUpdateLastHash,
			Support:            fakeCS,
			LocalConfigCluster: localconfig.Cluster{},
			BlockPullerFactory: bpf,
			Logger:             flogging.MustGetLogger("test.smartbft"),
		}

		require.NotNil(t, bftSynchronizer)

		resp := bftSynchronizer.Sync()
		require.NotNil(t, resp)
		require.Equal(t, *decision, resp)
	})

	t.Run("error creating block puller", func(t *testing.T) {
		bpf := &mocks.FakeBlockPullerFactory{}
		bpf.CreateBlockPullerReturns(nil, errors.New("oops"))

		fakeCS := &mocks2.FakeConsenterSupport{}
		fakeCS.HeightReturns(100)
		fakeCS.BlockReturns(b99)

		decision := &types.SyncResponse{
			Latest: types.Decision{
				Proposal:   types.Proposal{Header: []byte{1, 1, 1, 1}},
				Signatures: []types.Signature{{ID: 1}, {ID: 2}, {ID: 3}},
			},
			Reconfig: types.ReconfigSync{
				InReplicatedDecisions: false,
				CurrentNodes:          []uint64{1, 2, 3, 4},
				CurrentConfig:         types.Configuration{SelfID: 1},
			},
		}

		bftSynchronizer := &smartbft.BFTSynchronizer{
			LatestConfig: func() (types.Configuration, []uint64) {
				return types.Configuration{
					SelfID: 1,
				}, []uint64{1, 2, 3, 4}
			},
			BlockToDecision: func(block *cb.Block) *types.Decision {
				if block == b99 {
					return &decision.Latest
				}
				return nil
			},
			OnCommit:           noopUpdateLastHash,
			Support:            fakeCS,
			LocalConfigCluster: localconfig.Cluster{},
			BlockPullerFactory: bpf,
			Logger:             flogging.MustGetLogger("test.smartbft"),
		}

		require.NotNil(t, bftSynchronizer)
		resp := bftSynchronizer.Sync()
		require.NotNil(t, resp)
		require.Equal(t, *decision, resp)
	})

	t.Run("no remote endpoints above my height", func(t *testing.T) {
		bp := &mocks.FakeBlockPuller{}
		bpf := &mocks.FakeBlockPullerFactory{}
		bpf.CreateBlockPullerReturns(bp, nil)

		bp.HeightsByEndpointsReturns(
			map[string]uint64{
				"example.com:1": 100,
				"example.com:2": 100,
			},
			"example.com:1",
			nil,
		)

		fakeCS := &mocks2.FakeConsenterSupport{}
		fakeCS.HeightReturns(100)
		fakeCS.BlockReturns(b99)
		fakeOrdererConfig := &mocks.OrdererConfig{}
		fakeOrdererConfig.ConsentersReturns([]*cb.Consenter{
			{Id: 1}, {Id: 2}, {Id: 3}, {Id: 4},
		})
		fakeCS.SharedConfigReturns(fakeOrdererConfig)

		decision := &types.SyncResponse{
			Latest: types.Decision{
				Proposal:   types.Proposal{Header: []byte{1, 1, 1, 1}},
				Signatures: []types.Signature{{ID: 1}, {ID: 2}, {ID: 3}},
			},
			Reconfig: types.ReconfigSync{
				InReplicatedDecisions: false,
				CurrentNodes:          []uint64{1, 2, 3, 4},
				CurrentConfig:         types.Configuration{SelfID: 1},
			},
		}

		bftSynchronizer := &smartbft.BFTSynchronizer{
			LatestConfig: func() (types.Configuration, []uint64) {
				return types.Configuration{
					SelfID: 1,
				}, []uint64{1, 2, 3, 4}
			},
			BlockToDecision: func(block *cb.Block) *types.Decision {
				if block == b99 {
					return &decision.Latest
				}
				return nil
			},
			OnCommit:           noopUpdateLastHash,
			Support:            fakeCS,
			LocalConfigCluster: localconfig.Cluster{},
			BlockPullerFactory: bpf,
			Logger:             flogging.MustGetLogger("test.smartbft"),
		}

		require.NotNil(t, bftSynchronizer)

		resp := bftSynchronizer.Sync()
		require.NotNil(t, resp)
		require.Equal(t, *decision, resp)
	})

	t.Run("remote endpoints above my height: 2 blocks", func(t *testing.T) {
		bp := &mocks.FakeBlockPuller{}
		bpf := &mocks.FakeBlockPullerFactory{}
		bpf.CreateBlockPullerReturns(bp, nil)

		bp.HeightsByEndpointsReturns(
			map[string]uint64{
				"example.com:1": 100,
				"example.com:2": 101,
				"example.com:3": 102,
				"example.com:4": 103,
			},
			"example.com:1",
			nil,
		)

		var ledger []*cb.Block
		for i := uint64(0); i < 100; i++ {
			ledger = append(ledger, &cb.Block{Header: &cb.BlockHeader{Number: i}})
		}
		ledger[42] = b42
		ledger[99] = b99

		fakeCS := &mocks2.FakeConsenterSupport{}
		fakeCS.HeightCalls(func() uint64 {
			return uint64(len(ledger))
		})
		fakeCS.BlockCalls(func(u uint64) *cb.Block {
			b := ledger[u]
			t.Logf("Block Calls: %d, %v", u, b)
			return ledger[u]
		})
		fakeCS.SequenceCalls(func() uint64 { return blockNum2configSqn[uint64(len(ledger))] })
		fakeCS.WriteConfigBlockCalls(func(b *cb.Block, m []byte) {
			ledger = append(ledger, b)
		})
		fakeCS.WriteBlockSyncCalls(func(b *cb.Block, m []byte) {
			ledger = append(ledger, b)
		})

		fakeOrdererConfig := &mocks.OrdererConfig{}
		fakeOrdererConfig.ConsentersReturns([]*cb.Consenter{
			{Id: 1}, {Id: 2}, {Id: 3}, {Id: 4},
		})
		fakeOrdererConfig.BatchSizeReturns(&orderer.BatchSize{
			MaxMessageCount:   100,
			AbsoluteMaxBytes:  1000000,
			PreferredMaxBytes: 500000,
		})
		fakeCS.SharedConfigReturns(fakeOrdererConfig)

		fakeVerifierFactory := &mocks.VerifierFactory{}
		fakeVerifier := &mocks.UpdatableBlockVerifier{}
		fakeVerifierFactory.CreateBlockVerifierReturns(fakeVerifier, nil)

		fakeBFTDelivererFactory := &mocks.BFTDelivererFactory{}
		fakeBFTDeliverer := &mocks.BFTBlockDeliverer{}
		fakeBFTDelivererFactory.CreateBFTDelivererReturns(fakeBFTDeliverer)

		decision := &types.SyncResponse{
			Latest: types.Decision{
				Proposal:   types.Proposal{Header: []byte{1, 1, 1, 1}},
				Signatures: []types.Signature{{ID: 1}, {ID: 2}, {ID: 3}},
			},
			Reconfig: types.ReconfigSync{
				InReplicatedDecisions: true,
				CurrentNodes:          []uint64{1, 2, 3, 4},
				CurrentConfig:         types.Configuration{SelfID: 1},
			},
		}

		bftSynchronizer := &smartbft.BFTSynchronizer{
			LatestConfig: func() (types.Configuration, []uint64) {
				return types.Configuration{
					SelfID: 1,
				}, []uint64{1, 2, 3, 4}
			},
			BlockToDecision: func(block *cb.Block) *types.Decision {
				if block == b101 {
					return &decision.Latest
				}
				return nil
			},
			OnCommit: func(block *cb.Block) types.Reconfig {
				if block == b101 {
					return types.Reconfig{
						InLatestDecision: true,
						CurrentNodes:     []uint64{1, 2, 3, 4},
						CurrentConfig:    types.Configuration{SelfID: 1},
					}
				}
				return types.Reconfig{}
			},
			Support:             fakeCS,
			ClusterDialer:       &cluster.PredicateDialer{Config: comm.ClientConfig{}},
			LocalConfigCluster:  localconfig.Cluster{},
			BlockPullerFactory:  bpf,
			VerifierFactory:     fakeVerifierFactory,
			BFTDelivererFactory: fakeBFTDelivererFactory,
			Logger:              flogging.MustGetLogger("test.smartbft"),
		}

		require.NotNil(t, bftSynchronizer)

		wg := sync.WaitGroup{}
		wg.Add(1)
		stopDeliverCh := make(chan struct{})
		fakeBFTDeliverer.DeliverBlocksCalls(func() {
			b := bftSynchronizer.Buffer()
			require.NotNil(t, b)
			err := b.HandleBlock("mychannel", b100)
			require.NoError(t, err)
			err = b.HandleBlock("mychannel", b101)
			require.NoError(t, err)
			<-stopDeliverCh // the goroutine will block here
			wg.Done()
		})
		fakeBFTDeliverer.StopCalls(func() {
			close(stopDeliverCh)
		})

		resp := bftSynchronizer.Sync()
		require.NotNil(t, resp)
		require.Equal(t, *decision, resp)
		require.Equal(t, 102, len(ledger))
		require.Equal(t, 1, fakeCS.WriteBlockSyncCallCount())
		require.Equal(t, 1, fakeCS.WriteConfigBlockCallCount())
		wg.Wait()
	})

	t.Run("remote endpoints above my height: 3 blocks", func(t *testing.T) {
		bp := &mocks.FakeBlockPuller{}
		bpf := &mocks.FakeBlockPullerFactory{}
		bpf.CreateBlockPullerReturns(bp, nil)

		bp.HeightsByEndpointsReturns(
			map[string]uint64{
				"example.com:1": 100,
				"example.com:2": 103,
				"example.com:3": 103,
				"example.com:4": 200,
			},
			"example.com:1",
			nil,
		)

		var ledger []*cb.Block
		for i := uint64(0); i < 100; i++ {
			ledger = append(ledger, &cb.Block{Header: &cb.BlockHeader{Number: i}})
		}
		ledger[42] = b42
		ledger[99] = b99

		fakeCS := &mocks2.FakeConsenterSupport{}
		fakeCS.HeightCalls(func() uint64 {
			return uint64(len(ledger))
		})
		fakeCS.BlockCalls(func(u uint64) *cb.Block {
			b := ledger[u]
			t.Logf("Block Calls: %d, %v", u, b)
			return ledger[u]
		})
		fakeCS.SequenceCalls(func() uint64 { return blockNum2configSqn[uint64(len(ledger))] })
		fakeCS.WriteConfigBlockCalls(func(b *cb.Block, m []byte) {
			ledger = append(ledger, b)
		})
		fakeCS.WriteBlockSyncCalls(func(b *cb.Block, m []byte) {
			ledger = append(ledger, b)
		})

		fakeOrdererConfig := &mocks.OrdererConfig{}
		fakeOrdererConfig.ConsentersReturns([]*cb.Consenter{
			{Id: 1}, {Id: 2}, {Id: 3}, {Id: 4},
		})
		fakeOrdererConfig.BatchSizeReturns(&orderer.BatchSize{
			MaxMessageCount:   100,
			AbsoluteMaxBytes:  1000000,
			PreferredMaxBytes: 500000,
		})
		fakeCS.SharedConfigReturns(fakeOrdererConfig)

		fakeVerifierFactory := &mocks.VerifierFactory{}
		fakeVerifier := &mocks.UpdatableBlockVerifier{}
		fakeVerifierFactory.CreateBlockVerifierReturns(fakeVerifier, nil)

		fakeBFTDelivererFactory := &mocks.BFTDelivererFactory{}
		fakeBFTDeliverer := &mocks.BFTBlockDeliverer{}
		fakeBFTDelivererFactory.CreateBFTDelivererReturns(fakeBFTDeliverer)

		decision := &types.SyncResponse{
			Latest: types.Decision{
				Proposal:   types.Proposal{Header: []byte{1, 1, 1, 1}},
				Signatures: []types.Signature{{ID: 1}, {ID: 2}, {ID: 3}},
			},
			Reconfig: types.ReconfigSync{
				InReplicatedDecisions: true,
				CurrentNodes:          []uint64{1, 2, 3, 4},
				CurrentConfig:         types.Configuration{SelfID: 1},
			},
		}

		bftSynchronizer := &smartbft.BFTSynchronizer{
			LatestConfig: func() (types.Configuration, []uint64) {
				return types.Configuration{
					SelfID: 1,
				}, []uint64{1, 2, 3, 4}
			},
			BlockToDecision: func(block *cb.Block) *types.Decision {
				if block == b102 {
					return &decision.Latest
				}
				return nil
			},
			OnCommit: func(block *cb.Block) types.Reconfig {
				if block == b101 {
					return types.Reconfig{
						InLatestDecision: true,
						CurrentNodes:     []uint64{1, 2, 3, 4},
						CurrentConfig:    types.Configuration{SelfID: 1},
					}
				} else if block == b102 {
					return types.Reconfig{
						InLatestDecision: false,
						CurrentNodes:     []uint64{1, 2, 3, 4},
						CurrentConfig:    types.Configuration{SelfID: 1},
					}
				}
				return types.Reconfig{}
			},
			Support:             fakeCS,
			ClusterDialer:       &cluster.PredicateDialer{Config: comm.ClientConfig{}},
			LocalConfigCluster:  localconfig.Cluster{},
			BlockPullerFactory:  bpf,
			VerifierFactory:     fakeVerifierFactory,
			BFTDelivererFactory: fakeBFTDelivererFactory,
			Logger:              flogging.MustGetLogger("test.smartbft"),
		}
		require.NotNil(t, bftSynchronizer)

		wg := sync.WaitGroup{}
		wg.Add(1)
		stopDeliverCh := make(chan struct{})
		fakeBFTDeliverer.DeliverBlocksCalls(func() {
			b := bftSynchronizer.Buffer()
			require.NotNil(t, b)
			err := b.HandleBlock("mychannel", b100)
			require.NoError(t, err)
			err = b.HandleBlock("mychannel", b101)
			require.NoError(t, err)
			err = b.HandleBlock("mychannel", b102)
			require.NoError(t, err)

			<-stopDeliverCh // the goroutine will block here
			wg.Done()
		})
		fakeBFTDeliverer.StopCalls(func() {
			close(stopDeliverCh)
		})

		resp := bftSynchronizer.Sync()
		require.NotNil(t, resp)
		require.Equal(t, *decision, resp)
		require.Equal(t, 103, len(ledger))
		require.Equal(t, 2, fakeCS.WriteBlockSyncCallCount())
		require.Equal(t, 1, fakeCS.WriteConfigBlockCallCount())
		wg.Wait()
	})
}
