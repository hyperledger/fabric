/*
Copyright IBM Corp. 2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package follower_test

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric/bccsp"
	"github.com/hyperledger/fabric/bccsp/sw"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/orderer/common/follower"
	"github.com/hyperledger/fabric/orderer/common/follower/mocks"
	"github.com/hyperledger/fabric/orderer/common/types"
	"github.com/hyperledger/fabric/orderer/consensus"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

//go:generate counterfeiter -o mocks/cluster_consenter.go -fake-name ClusterConsenter . clusterConsenter
type clusterConsenter interface {
	consensus.ClusterConsenter
}

//go:generate counterfeiter -o mocks/puller_creator.go -fake-name BlockPullerFactory . blockPullerFactory
type blockPullerFactory interface {
	BlockPuller(configBlock *common.Block) (follower.ChannelPuller, error)
}

//go:generate counterfeiter -o mocks/chain_creator.go -fake-name ChainCreator . chainCreator
type chainCreator interface {
	CreateChain(channelName string)
}

//go:generate counterfeiter -o mocks/follower_creator.go -fake-name FollowerCreator . followerCreator
type followerCreator interface {
	CreateFollower(channelName string)
}

//go:generate counterfeiter -o mocks/time_after.go -fake-name TimeAfter . timeAfter
type timeAfter interface {
	After(d time.Duration) <-chan time.Time
}

var testLogger = flogging.MustGetLogger("follower.test")

// Global test variables
var (
	cryptoProvider       bccsp.BCCSP
	localBlockchain      *memoryBlockChain
	remoteBlockchain     *memoryBlockChain
	ledgerResources      *mocks.LedgerResources
	mockClusterConsenter *mocks.ClusterConsenter
	pullerCreator        *mocks.BlockPullerFactory
	puller               *mocks.ChannelPuller
	mockChainCreator     *mocks.ChainCreator
	options              follower.Options
	timeAfterCount       *mocks.TimeAfter
	maxDelay             int64
)

// Before each test in all test cases
func globalSetup(t *testing.T) {
	var err error
	cryptoProvider, err = sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	require.NoError(t, err)

	localBlockchain = &memoryBlockChain{}
	remoteBlockchain = &memoryBlockChain{}

	ledgerResources = &mocks.LedgerResources{}
	ledgerResources.ChannelIDReturns("my-channel")
	ledgerResources.HeightCalls(localBlockchain.Height)
	ledgerResources.BlockCalls(localBlockchain.Block)

	mockClusterConsenter = &mocks.ClusterConsenter{}

	pullerCreator = &mocks.BlockPullerFactory{}
	puller = &mocks.ChannelPuller{}
	pullerCreator.BlockPullerReturns(puller, nil)

	mockChainCreator = &mocks.ChainCreator{}

	options = follower.Options{
		Logger:               testLogger,
		PullRetryMinInterval: 1 * time.Microsecond,
		PullRetryMaxInterval: 5 * time.Microsecond,
		Cert:                 []byte{1, 2, 3, 4},
	}

	atomic.StoreInt64(&maxDelay, 0)
	timeAfterCount = &mocks.TimeAfter{}
	timeAfterCount.AfterCalls(
		func(d time.Duration) <-chan time.Time {
			if d.Nanoseconds() > atomic.LoadInt64(&maxDelay) {
				atomic.StoreInt64(&maxDelay, d.Nanoseconds())
			}
			c := make(chan time.Time, 1)
			c <- time.Now()
			return c
		},
	)
}

func TestFollowerNewChain(t *testing.T) {
	joinBlockAppRaft := makeConfigBlock(10, []byte{}, 0)
	require.NotNil(t, joinBlockAppRaft)

	t.Run("with join block, not in channel", func(t *testing.T) {
		globalSetup(t)
		ledgerResources.HeightReturns(5)
		mockClusterConsenter.IsChannelMemberReturns(false, nil)
		chain, err := follower.NewChain(ledgerResources, mockClusterConsenter, joinBlockAppRaft, options, pullerCreator, nil, nil)
		require.NoError(t, err)

		cRel, status := chain.StatusReport()
		require.Equal(t, types.ClusterRelationFollower, cRel)
		require.Equal(t, types.StatusOnBoarding, status)
	})

	t.Run("with join block, in channel", func(t *testing.T) {
		globalSetup(t)
		ledgerResources.HeightReturns(5)
		mockClusterConsenter.IsChannelMemberReturns(true, nil)
		chain, err := follower.NewChain(ledgerResources, mockClusterConsenter, joinBlockAppRaft, options, pullerCreator, nil, nil)
		require.NoError(t, err)

		cRel, status := chain.StatusReport()
		require.Equal(t, types.ClusterRelationMember, cRel)
		require.Equal(t, types.StatusOnBoarding, status)

		require.NotPanics(t, chain.Start)
		require.NotPanics(t, chain.Start)
		require.NotPanics(t, chain.Halt)
		require.NotPanics(t, chain.Halt)
		require.NotPanics(t, chain.Start)
	})

	t.Run("bad join block", func(t *testing.T) {
		globalSetup(t)
		chain, err := follower.NewChain(ledgerResources, nil, &common.Block{}, options, pullerCreator, nil, nil)
		require.EqualError(t, err, "block header is nil")
		require.Nil(t, chain)
		chain, err = follower.NewChain(ledgerResources, nil, &common.Block{Header: &common.BlockHeader{}}, options, pullerCreator, nil, nil)
		require.EqualError(t, err, "block data is nil")
		require.Nil(t, chain)
	})

	t.Run("without join block", func(t *testing.T) {
		globalSetup(t)
		ledgerResources.HeightReturns(5)
		mockClusterConsenter.IsChannelMemberReturns(false, nil)
		chain, err := follower.NewChain(ledgerResources, mockClusterConsenter, nil, options, pullerCreator, nil, nil)
		require.NoError(t, err)

		cRel, status := chain.StatusReport()
		require.Equal(t, types.ClusterRelationFollower, cRel)
		require.True(t, status == types.StatusActive)
	})
}

func TestFollowerPullUpToJoin(t *testing.T) {
	joinNum := uint64(10)
	joinBlockAppRaft := makeConfigBlock(joinNum, []byte{}, 0)
	require.NotNil(t, joinBlockAppRaft)

	var wgChain sync.WaitGroup
	var wgFollower sync.WaitGroup

	setup := func() {
		globalSetup(t)
		remoteBlockchain.fill(joinNum + 1)

		ledgerResources.AppendCalls(localBlockchain.Append)

		puller.PullBlockCalls(func(i uint64) *common.Block { return remoteBlockchain.Block(i) })
		pullerCreator.BlockPullerReturns(puller, nil)

		wgChain = sync.WaitGroup{}
		wgChain.Add(1)
		mockChainCreator.SwitchFollowerToChainCalls(func(_ string) { wgChain.Done() })

		wgFollower = sync.WaitGroup{}
		wgFollower.Add(1)
		mockChainCreator.SwitchFollowerCalls(func(_ string) { wgFollower.Done() })
	}

	t.Run("zero until join block", func(t *testing.T) {
		setup()
		mockClusterConsenter.IsChannelMemberReturns(true, nil)

		chain, err := follower.NewChain(ledgerResources, mockClusterConsenter, joinBlockAppRaft, options, pullerCreator, mockChainCreator, cryptoProvider)
		assert.NoError(t, err)

		cRel, status := chain.StatusReport()
		assert.Equal(t, types.ClusterRelationMember, cRel)
		assert.Equal(t, types.StatusOnBoarding, status)

		assert.NotPanics(t, chain.Start)
		wgChain.Wait()
		assert.NotPanics(t, chain.Halt)
		assert.False(t, chain.IsRunning())

		cRel, status = chain.StatusReport()
		assert.Equal(t, types.ClusterRelationMember, cRel)
		assert.Equal(t, types.StatusActive, status)

		assert.Equal(t, 2, pullerCreator.BlockPullerCallCount())
		assert.Equal(t, 11, ledgerResources.AppendCallCount())
		for i := uint64(0); i <= joinNum; i++ {
			assert.Equal(t, remoteBlockchain.Block(i).Header, localBlockchain.Block(i).Header, "failed block i=%d", i)
		}
		assert.Equal(t, 1, mockChainCreator.SwitchFollowerToChainCallCount())
		assert.Equal(t, 0, mockChainCreator.SwitchFollowerCallCount())
	})
	t.Run("existing half chain until join block", func(t *testing.T) {
		setup()
		mockClusterConsenter.IsChannelMemberReturns(false, nil)
		localBlockchain.fill(joinNum / 2) //A gap between the ledger and the join block
		assert.True(t, joinBlockAppRaft.Header.Number > ledgerResources.Height())
		assert.True(t, ledgerResources.Height() > 0)

		chain, err := follower.NewChain(ledgerResources, mockClusterConsenter, joinBlockAppRaft, options, pullerCreator, mockChainCreator, cryptoProvider)
		assert.NoError(t, err)

		cRel, status := chain.StatusReport()
		assert.Equal(t, types.ClusterRelationFollower, cRel)
		assert.Equal(t, types.StatusOnBoarding, status)

		assert.NotPanics(t, chain.Start)
		wgFollower.Wait()
		assert.NotPanics(t, chain.Halt)
		assert.False(t, chain.IsRunning())

		cRel, status = chain.StatusReport()
		assert.Equal(t, types.ClusterRelationFollower, cRel)
		assert.Equal(t, types.StatusActive, status)

		assert.Equal(t, 2, pullerCreator.BlockPullerCallCount())
		assert.Equal(t, 6, ledgerResources.AppendCallCount())
		for i := uint64(0); i <= joinNum; i++ {
			assert.Equal(t, remoteBlockchain.Block(i).Header, localBlockchain.Block(i).Header, "failed block i=%d", i)
		}
		assert.Equal(t, 0, mockChainCreator.SwitchFollowerToChainCallCount())
		assert.Equal(t, 1, mockChainCreator.SwitchFollowerCallCount())
	})
	t.Run("no need to pull", func(t *testing.T) {
		setup()
		mockClusterConsenter.IsChannelMemberReturns(false, nil)
		localBlockchain.fill(joinNum + 1) //No gap between the ledger and the join block
		assert.True(t, joinBlockAppRaft.Header.Number < ledgerResources.Height())

		chain, err := follower.NewChain(ledgerResources, mockClusterConsenter, joinBlockAppRaft, options, pullerCreator, mockChainCreator, cryptoProvider)
		assert.NoError(t, err)

		cRel, status := chain.StatusReport()
		require.Equal(t, types.ClusterRelationFollower, cRel)
		require.Equal(t, types.StatusActive, status)

		assert.NotPanics(t, chain.Start)
		wgFollower.Wait()
		assert.NotPanics(t, chain.Halt)
		assert.False(t, chain.IsRunning())

		cRel, status = chain.StatusReport()
		assert.Equal(t, types.ClusterRelationFollower, cRel)
		assert.Equal(t, types.StatusActive, status)

		assert.Equal(t, 1, pullerCreator.BlockPullerCallCount())
		assert.Equal(t, 0, ledgerResources.AppendCallCount())

		assert.Equal(t, 0, mockChainCreator.SwitchFollowerToChainCallCount())
		assert.Equal(t, 1, mockChainCreator.SwitchFollowerCallCount())
	})
	t.Run("overcome pull failures", func(t *testing.T) {
		setup()
		mockClusterConsenter.IsChannelMemberReturns(true, nil)

		failPull := 10
		pullerCreator = &mocks.BlockPullerFactory{}
		puller = &mocks.ChannelPuller{}
		puller.PullBlockCalls(func(i uint64) *common.Block {
			if i%2 == 1 && failPull > 0 {
				failPull = failPull - 1
				return nil
			}
			failPull = 10
			return remoteBlockchain.Block(i)
		})
		pullerCreator.BlockPullerReturns(puller, nil)
		options.TimeAfter = timeAfterCount.After

		chain, err := follower.NewChain(ledgerResources, mockClusterConsenter, joinBlockAppRaft, options, pullerCreator, mockChainCreator, cryptoProvider)
		assert.NoError(t, err)

		cRel, status := chain.StatusReport()
		assert.Equal(t, types.ClusterRelationMember, cRel)
		assert.Equal(t, types.StatusOnBoarding, status)

		assert.NotPanics(t, chain.Start)
		wgChain.Wait()
		assert.NotPanics(t, chain.Halt)
		assert.False(t, chain.IsRunning())

		cRel, status = chain.StatusReport()
		assert.Equal(t, types.ClusterRelationMember, cRel)
		assert.Equal(t, types.StatusActive, status)

		assert.Equal(t, 52, pullerCreator.BlockPullerCallCount())
		assert.Equal(t, 11, ledgerResources.AppendCallCount())
		for i := uint64(0); i <= joinNum; i++ {
			assert.Equal(t, remoteBlockchain.Block(i).Header, localBlockchain.Block(i).Header, "failed block i=%d", i)
		}
		assert.Equal(t, 1, mockChainCreator.SwitchFollowerToChainCallCount())
		assert.Equal(t, 0, mockChainCreator.SwitchFollowerCallCount())

		assert.Equal(t, 50, timeAfterCount.AfterCallCount())
		assert.Equal(t, int64(5000), atomic.LoadInt64(&maxDelay))
	})
}

func TestFollowerPullAfterJoin(t *testing.T) {
	joinNum := uint64(10)
	var wgChain sync.WaitGroup

	setup := func() {
		globalSetup(t)
		remoteBlockchain.fill(joinNum + 11)
		localBlockchain.fill(joinNum + 1)

		mockClusterConsenter.IsChannelMemberCalls(amIReallyInChannel)

		puller.PullBlockCalls(func(i uint64) *common.Block { return remoteBlockchain.Block(i) })
		puller.HeightsByEndpointsCalls(
			func() (map[string]uint64, error) {
				m := make(map[string]uint64)
				m["good-node"] = remoteBlockchain.Height()
				m["lazy-node"] = remoteBlockchain.Height() - 2
				return m, nil
			},
		)
		pullerCreator.BlockPullerReturns(puller, nil)

		wgChain = sync.WaitGroup{}
		wgChain.Add(1)

		mockChainCreator.SwitchFollowerCalls(func(_ string) { assert.Fail(t, "CreatedFollower called") })
	}

	t.Run("No config in the middle", func(t *testing.T) {
		setup()
		ledgerResources.AppendCalls(func(block *common.Block) error {
			localBlockchain.Append(block)
			//Stop when we catch-up with latest
			if remoteBlockchain.Height() == localBlockchain.Height() {
				wgChain.Done()
			}
			return nil
		})
		assert.Equal(t, joinNum+1, localBlockchain.Height())

		chain, err := follower.NewChain(ledgerResources, mockClusterConsenter, nil, options, pullerCreator, mockChainCreator, cryptoProvider)
		assert.NoError(t, err)

		cRel, status := chain.StatusReport()
		assert.Equal(t, types.ClusterRelationFollower, cRel)
		assert.Equal(t, types.StatusActive, status)

		assert.NotPanics(t, chain.Start)
		wgChain.Wait()
		assert.NotPanics(t, chain.Halt)
		assert.False(t, chain.IsRunning())

		cRel, status = chain.StatusReport()
		assert.Equal(t, types.ClusterRelationFollower, cRel)
		assert.Equal(t, types.StatusActive, status)

		assert.Equal(t, 1, pullerCreator.BlockPullerCallCount())
		assert.Equal(t, 10, ledgerResources.AppendCallCount())
		assert.Equal(t, uint64(21), localBlockchain.Height())
		for i := uint64(0); i < localBlockchain.Height(); i++ {
			assert.Equal(t, remoteBlockchain.Block(i).Header, localBlockchain.Block(i).Header, "failed block i=%d", i)
		}
		assert.Equal(t, 0, mockChainCreator.SwitchFollowerToChainCallCount())
	})
	t.Run("No config in the middle, latest height increasing", func(t *testing.T) {
		setup()
		ledgerResources.AppendCalls(func(block *common.Block) error {
			localBlockchain.Append(block)
			if remoteBlockchain.Height() == localBlockchain.Height() {
				if remoteBlockchain.Height() < 50 {
					remoteBlockchain.fill(10)
				} else {
					//Stop when we catch-up with latest
					wgChain.Done()
				}
			}
			return nil
		})
		assert.Equal(t, joinNum+1, localBlockchain.Height())

		chain, err := follower.NewChain(ledgerResources, mockClusterConsenter, nil, options, pullerCreator, mockChainCreator, cryptoProvider)

		assert.NoError(t, err)

		cRel, status := chain.StatusReport()
		assert.Equal(t, types.ClusterRelationFollower, cRel)
		assert.Equal(t, types.StatusActive, status)

		assert.NotPanics(t, chain.Start)
		wgChain.Wait()
		assert.NotPanics(t, chain.Halt)
		assert.False(t, chain.IsRunning())

		cRel, status = chain.StatusReport()
		assert.Equal(t, types.ClusterRelationFollower, cRel)
		assert.Equal(t, types.StatusActive, status)

		assert.Equal(t, 1, pullerCreator.BlockPullerCallCount())
		assert.Equal(t, 40, ledgerResources.AppendCallCount())
		assert.Equal(t, uint64(51), localBlockchain.Height())
		for i := uint64(0); i < localBlockchain.Height(); i++ {
			assert.Equal(t, remoteBlockchain.Block(i).Header, localBlockchain.Block(i).Header, "failed block i=%d", i)
		}
		assert.Equal(t, 0, mockChainCreator.SwitchFollowerToChainCallCount())
	})
	t.Run("Configs in the middle, latest height increasing", func(t *testing.T) {
		setup()
		ledgerResources.AppendCalls(func(block *common.Block) error {
			localBlockchain.Append(block)

			if remoteBlockchain.Height() == localBlockchain.Height() {
				if remoteBlockchain.Height() < 50 {
					remoteBlockchain.fill(9)
					//Each config appended will trigger the creation of a new puller in the next round
					remoteBlockchain.appendConfig(0)
				} else {
					remoteBlockchain.fill(9)
					//This will trigger the creation of a new chain
					remoteBlockchain.appendConfig(1)
				}
			}
			return nil
		})
		mockChainCreator.SwitchFollowerToChainCalls(func(_ string) { wgChain.Done() }) //Stop when a new chain is created
		assert.Equal(t, joinNum+1, localBlockchain.Height())

		chain, err := follower.NewChain(ledgerResources, mockClusterConsenter, nil, options, pullerCreator, mockChainCreator, cryptoProvider)
		assert.NoError(t, err)

		cRel, status := chain.StatusReport()
		assert.Equal(t, types.ClusterRelationFollower, cRel)
		assert.Equal(t, types.StatusActive, status)

		assert.NotPanics(t, chain.Start)
		wgChain.Wait()
		assert.NotPanics(t, chain.Halt)
		assert.False(t, chain.IsRunning())

		cRel, status = chain.StatusReport()
		assert.Equal(t, types.ClusterRelationFollower, cRel)
		assert.Equal(t, types.StatusActive, status)

		assert.Equal(t, 4, pullerCreator.BlockPullerCallCount(), "after finding a config, block puller is created")
		assert.Equal(t, 50, ledgerResources.AppendCallCount())
		assert.Equal(t, uint64(61), localBlockchain.Height())
		for i := uint64(0); i < localBlockchain.Height(); i++ {
			assert.Equal(t, remoteBlockchain.Block(i).Header, localBlockchain.Block(i).Header, "failed block i=%d", i)
		}
		assert.Equal(t, 1, mockChainCreator.SwitchFollowerToChainCallCount())
	})
	t.Run("Overcome puller errors, configs in the middle, latest height increasing", func(t *testing.T) {
		setup()
		ledgerResources.AppendCalls(func(block *common.Block) error {
			localBlockchain.Append(block)

			if remoteBlockchain.Height() == localBlockchain.Height() {
				if remoteBlockchain.Height() < 50 {
					remoteBlockchain.fill(9)
					//Each config appended will trigger the creation of a new puller in the next round
					remoteBlockchain.appendConfig(0)
				} else {
					remoteBlockchain.fill(9)
					//This will trigger the creation of a new chain
					remoteBlockchain.appendConfig(1)
				}
			}
			return nil
		})
		mockChainCreator.SwitchFollowerToChainCalls(func(_ string) { wgChain.Done() }) //Stop when a new chain is created
		assert.Equal(t, joinNum+1, localBlockchain.Height())

		failPull := 10
		puller.PullBlockCalls(func(i uint64) *common.Block {
			if i%2 == 1 && failPull > 0 {
				failPull = failPull - 1
				return nil
			}
			failPull = 10
			return remoteBlockchain.Block(i)
		})

		failHeight := 1
		puller.HeightsByEndpointsCalls(
			func() (map[string]uint64, error) {
				if failHeight > 0 {
					failHeight = failHeight - 1
					return nil, errors.New("failed to get heights")
				}
				failHeight = 1
				m := make(map[string]uint64)
				m["good-node"] = remoteBlockchain.Height()
				m["lazy-node"] = remoteBlockchain.Height() - 2
				return m, nil
			},
		)
		pullerCreator.BlockPullerReturns(puller, nil)

		options.TimeAfter = timeAfterCount.After

		chain, err := follower.NewChain(ledgerResources, mockClusterConsenter, nil, options, pullerCreator, mockChainCreator, cryptoProvider)

		assert.NoError(t, err)

		cRel, status := chain.StatusReport()
		assert.Equal(t, types.ClusterRelationFollower, cRel)
		assert.Equal(t, types.StatusActive, status)

		assert.NotPanics(t, chain.Start)
		wgChain.Wait()
		assert.NotPanics(t, chain.Halt)
		assert.False(t, chain.IsRunning())

		cRel, status = chain.StatusReport()
		assert.Equal(t, types.ClusterRelationFollower, cRel)
		assert.Equal(t, types.StatusActive, status)

		assert.Equal(t, 509, pullerCreator.BlockPullerCallCount(), "after finding a config, or error, block puller is created")
		assert.Equal(t, 50, ledgerResources.AppendCallCount())
		assert.Equal(t, uint64(61), localBlockchain.Height())
		for i := uint64(0); i < localBlockchain.Height(); i++ {
			assert.Equal(t, remoteBlockchain.Block(i).Header, localBlockchain.Block(i).Header, "failed block i=%d", i)
		}

		assert.Equal(t, 1, mockChainCreator.SwitchFollowerToChainCallCount())
		assert.Equal(t, 0, mockChainCreator.SwitchFollowerCallCount())

		assert.Equal(t, 505, timeAfterCount.AfterCallCount())
		assert.Equal(t, int64(5000), atomic.LoadInt64(&maxDelay))
	})
}

type memoryBlockChain struct {
	lock  sync.Mutex
	chain []*common.Block
}

func (mbc *memoryBlockChain) Append(block *common.Block) error {
	mbc.lock.Lock()
	defer mbc.lock.Unlock()

	mbc.chain = append(mbc.chain, block)
	return nil
}

func (mbc *memoryBlockChain) Height() uint64 {
	mbc.lock.Lock()
	defer mbc.lock.Unlock()

	return uint64(len(mbc.chain))
}

func (mbc *memoryBlockChain) Block(i uint64) *common.Block {
	mbc.lock.Lock()
	defer mbc.lock.Unlock()

	if i < uint64(len(mbc.chain)) {
		return mbc.chain[i]
	}
	return nil
}

func (mbc *memoryBlockChain) fill(numBlocks uint64) {
	mbc.lock.Lock()
	defer mbc.lock.Unlock()

	height := uint64(len(mbc.chain))
	prevHash := []byte{}

	for i := height; i < height+numBlocks; i++ {
		if i > 0 {
			prevHash = protoutil.BlockHeaderHash(mbc.chain[i-1].Header)

		}

		var block *common.Block
		if i == 0 {
			block = makeConfigBlock(i, prevHash, 0)
		} else {
			block = protoutil.NewBlock(i, prevHash)
			protoutil.CopyBlockMetadata(mbc.chain[i-1], block)
		}

		mbc.chain = append(mbc.chain, block)
	}
}

func (mbc *memoryBlockChain) appendConfig(isMember uint8) {
	mbc.lock.Lock()
	defer mbc.lock.Unlock()

	h := uint64(len(mbc.chain))
	configBlock := makeConfigBlock(h, protoutil.BlockHeaderHash(mbc.chain[h-1].Header), isMember)
	mbc.chain = append(mbc.chain, configBlock)
}

func amIReallyInChannel(configBlock *common.Block) (bool, error) {
	if !protoutil.IsConfigBlock(configBlock) {
		return false, errors.New("not a config")
	}
	env, err := protoutil.ExtractEnvelope(configBlock, 0)
	if err != nil {
		return false, err
	}
	payload := protoutil.UnmarshalPayloadOrPanic(env.Payload)
	if len(payload.Data) == 0 {
		return false, errors.New("empty data")
	}
	if payload.Data[0] > 0 {
		return true, nil
	}
	return false, nil
}

func makeConfigBlock(num uint64, prevHash []byte, isMember uint8) *common.Block {
	block := protoutil.NewBlock(num, prevHash)
	env := &common.Envelope{
		Payload: protoutil.MarshalOrPanic(&common.Payload{
			Header: protoutil.MakePayloadHeader(
				protoutil.MakeChannelHeader(common.HeaderType_CONFIG, 0, "my-chennel", 0),
				protoutil.MakeSignatureHeader([]byte{}, []byte{}),
			),
			Data: []byte{isMember},
		},
		),
	}
	block.Data.Data = append(block.Data.Data, protoutil.MarshalOrPanic(env))
	protoutil.InitBlockMetadata(block)
	obm := &common.OrdererBlockMetadata{LastConfig: &common.LastConfig{Index: num}}
	block.Metadata.Metadata[common.BlockMetadataIndex_SIGNATURES] = protoutil.MarshalOrPanic(
		&common.Metadata{
			Value: protoutil.MarshalOrPanic(obm),
		},
	)
	protoutil.InitBlockMetadata(block)

	return block
}

func TestChain_makeConfigBlock(t *testing.T) {
	joinBlockAppRaft := makeConfigBlock(10, []byte{1, 2, 3, 4}, 0)
	assert.NotNil(t, joinBlockAppRaft)
	assert.True(t, protoutil.IsConfigBlock(joinBlockAppRaft))
	assert.NotPanics(t, func() { protoutil.GetLastConfigIndexFromBlockOrPanic(joinBlockAppRaft) })
	assert.Equal(t, uint64(10), protoutil.GetLastConfigIndexFromBlockOrPanic(joinBlockAppRaft))
	assert.NotPanics(t, func() { amIReallyInChannel(joinBlockAppRaft) })
	isMem, err := amIReallyInChannel(joinBlockAppRaft)
	assert.NoError(t, err)
	assert.False(t, isMem)
	joinBlockAppRaft = makeConfigBlock(11, []byte{1, 2, 3, 4}, 1)
	isMem, err = amIReallyInChannel(joinBlockAppRaft)
	assert.NoError(t, err)
	assert.True(t, isMem)
	isMem, err = amIReallyInChannel(protoutil.NewBlock(10, []byte{1, 2, 3, 4}))
	assert.EqualError(t, err, "not a config")
	assert.False(t, isMem)
}
