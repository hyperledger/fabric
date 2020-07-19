/*
Copyright IBM Corp. 2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package follower_test

import (
	"github.com/hyperledger/fabric/bccsp/sw"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/stretchr/testify/assert"
	"sync"
	"testing"
	"time"

	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric/common/crypto/tlsgen"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/orderer/common/follower"
	"github.com/hyperledger/fabric/orderer/common/follower/mocks"
	"github.com/hyperledger/fabric/orderer/common/types"
	"github.com/hyperledger/fabric/orderer/consensus"
	"github.com/stretchr/testify/require"
)

//TODO skeleton

//go:generate counterfeiter -o mocks/cluster_consenter.go -fake-name ClusterConsenter . clusterConsenter
type clusterConsenter interface {
	consensus.ClusterConsenter
}

//go:generate counterfeiter -o mocks/puller_creator.go -fake-name BlockPullerCreator . blockPullerCreator
type blockPullerCreator interface {
	CreateBlockPuller() (follower.ChannelPuller, error)
}

//go:generate counterfeiter -o mocks/chain_creator.go -fake-name ChainCreator . chainCreator
type chainCreator interface {
	CreateChain()
}

//go:generate counterfeiter -o mocks/follower_creator.go -fake-name FollowerCreator . followerCreator
type followerCreator interface {
	CreateFollower()
}

//go:generate counterfeiter -o mocks/time_after.go -fake-name TimeAfter . timeAfter
type timeAfter interface {
	After(d time.Duration) <-chan time.Time
}

var testLogger = flogging.MustGetLogger("follower.test")

func TestFollowerNewChain(t *testing.T) {
	tlsCA, _ := tlsgen.NewCA()
	channelID := "my-raft-channel"
	joinBlockAppRaft := generateJoinBlock(t, tlsCA, channelID, 10)
	require.NotNil(t, joinBlockAppRaft)

	options := follower.Options{Logger: testLogger, Cert: []byte{1, 2, 3, 4}}
	channelPuller := &mocks.ChannelPuller{}
	createBlockPuller := func() (follower.ChannelPuller, error) { return channelPuller, nil }

	t.Run("with join block, not in channel", func(t *testing.T) {
		mockResources := &mocks.LedgerResources{}
		mockResources.ChannelIDReturns("my-channel")
		mockResources.HeightReturns(5)
		clusterConsenter := &mocks.ClusterConsenter{}
		clusterConsenter.IsChannelMemberReturns(false, nil)
		chain, err := follower.NewChain(mockResources, clusterConsenter, joinBlockAppRaft, options, createBlockPuller, nil, nil, nil)
		require.NoError(t, err)

		cRel, status := chain.StatusReport()
		require.Equal(t, types.ClusterRelationFollower, cRel)
		require.Equal(t, types.StatusOnBoarding, status)
	})

	t.Run("with join block, in channel", func(t *testing.T) {
		mockResources := &mocks.LedgerResources{}
		mockResources.ChannelIDReturns("my-channel")
		mockResources.HeightReturns(5)
		clusterConsenter := &mocks.ClusterConsenter{}
		clusterConsenter.IsChannelMemberReturns(true, nil)
		chain, err := follower.NewChain(mockResources, clusterConsenter, joinBlockAppRaft, options, createBlockPuller, nil, nil, nil)
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
		mockResources := &mocks.LedgerResources{}
		mockResources.ChannelIDReturns("my-channel")
		chain, err := follower.NewChain(mockResources, nil, &common.Block{}, options, createBlockPuller, nil, nil, nil)
		require.EqualError(t, err, "block header is nil")
		require.Nil(t, chain)
		chain, err = follower.NewChain(mockResources, nil, &common.Block{Header: &common.BlockHeader{}}, options, createBlockPuller, nil, nil, nil)
		require.EqualError(t, err, "block data is nil")
		require.Nil(t, chain)
	})

	t.Run("without join block", func(t *testing.T) {
		mockResources := &mocks.LedgerResources{}
		mockResources.ChannelIDReturns("my-channel")
		mockResources.HeightReturns(5)
		clusterConsenter := &mocks.ClusterConsenter{}
		clusterConsenter.IsChannelMemberReturns(false, nil)
		chain, err := follower.NewChain(mockResources, clusterConsenter, nil, options, createBlockPuller, nil, nil, nil)
		require.NoError(t, err)

		cRel, status := chain.StatusReport()
		require.Equal(t, types.ClusterRelationFollower, cRel)
		require.True(t, status == types.StatusActive)
	})
}

func TestFollowerPullUpToJoin(t *testing.T) {
	joinNum := uint64(10)
	joinBlockAppRaft := protoutil.NewBlock(joinNum, []byte{})
	require.NotNil(t, joinBlockAppRaft)

	options := follower.Options{
		Logger:               testLogger,
		PullRetryMinInterval: 1 * time.Microsecond,
		PullRetryMaxInterval: 5 * time.Microsecond, // ~9 retries with 1.2 base
		Cert:                 []byte{1, 2, 3, 4}}

	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	assert.NoError(t, err)

	remoteBlockchain := &memoryBlockChain{}
	remoteBlockchain.fill(joinNum + 1)

	// Before each test
	var localBlockchain *memoryBlockChain
	var ledgerResources *mocks.LedgerResources
	var clusterConsenter *mocks.ClusterConsenter
	var pullerCreator *mocks.BlockPullerCreator
	var puller *mocks.ChannelPuller
	var wgChain sync.WaitGroup
	var chainCreator *mocks.ChainCreator
	var wgFollower sync.WaitGroup
	var followerCreator *mocks.FollowerCreator
	var timeAfterCount *mocks.TimeAfter
	var maxDelay int64

	setup := func() {
		localBlockchain = &memoryBlockChain{}

		ledgerResources = &mocks.LedgerResources{}
		ledgerResources.ChannelIDReturns("my-channel")
		ledgerResources.HeightCalls(localBlockchain.Height)
		ledgerResources.BlockCalls(localBlockchain.Block)
		ledgerResources.AppendCalls(localBlockchain.Append)

		clusterConsenter = &mocks.ClusterConsenter{}

		pullerCreator = &mocks.BlockPullerCreator{}
		puller = &mocks.ChannelPuller{}
		puller.PullBlockCalls(func(i uint64) *common.Block { return remoteBlockchain.Block(i) })
		pullerCreator.CreateBlockPullerReturns(puller, nil)

		wgChain = sync.WaitGroup{}
		wgChain.Add(1)
		chainCreator = &mocks.ChainCreator{}
		chainCreator.CreateChainCalls(wgChain.Done)

		wgFollower = sync.WaitGroup{}
		wgFollower.Add(1)
		followerCreator = &mocks.FollowerCreator{}
		followerCreator.CreateFollowerCalls(wgFollower.Done)

		options.TimeAfter = nil
		maxDelay = 0
		timeAfterCount = &mocks.TimeAfter{}
		timeAfterCount.AfterCalls(
			func(d time.Duration) <-chan time.Time {
				if d.Nanoseconds() > maxDelay {
					maxDelay = d.Nanoseconds()
				}
				c := make(chan time.Time, 1)
				c <- time.Now()
				return c
			},
		)
	}

	t.Run("zero until join block", func(t *testing.T) {
		setup()
		clusterConsenter.IsChannelMemberReturns(true, nil)

		chain, err := follower.NewChain(
			ledgerResources,
			clusterConsenter,
			joinBlockAppRaft,
			options,
			pullerCreator.CreateBlockPuller,
			chainCreator.CreateChain,
			followerCreator.CreateFollower,
			cryptoProvider)
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

		assert.Equal(t, 2, pullerCreator.CreateBlockPullerCallCount())
		assert.Equal(t, 11, ledgerResources.AppendCallCount())
		for i := uint64(0); i <= joinNum; i++ {
			assert.Equal(t, remoteBlockchain.Block(i).Header, localBlockchain.Block(i).Header, "failed block i=%d", i)
		}
		assert.Equal(t, 1, chainCreator.CreateChainCallCount())
		assert.Equal(t, 0, followerCreator.CreateFollowerCallCount())
	})
	t.Run("existing half chain until join block", func(t *testing.T) {
		setup()
		clusterConsenter.IsChannelMemberReturns(false, nil)
		localBlockchain.fill(joinNum / 2) //A gap between the ledger and the join block
		assert.True(t, joinBlockAppRaft.Header.Number > ledgerResources.Height())
		assert.True(t, ledgerResources.Height() > 0)

		chain, err := follower.NewChain(
			ledgerResources,
			clusterConsenter,
			joinBlockAppRaft,
			options,
			pullerCreator.CreateBlockPuller,
			chainCreator.CreateChain,
			followerCreator.CreateFollower,
			cryptoProvider)
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

		assert.Equal(t, 2, pullerCreator.CreateBlockPullerCallCount())
		assert.Equal(t, 6, ledgerResources.AppendCallCount())
		for i := uint64(0); i <= joinNum; i++ {
			assert.Equal(t, remoteBlockchain.Block(i).Header, localBlockchain.Block(i).Header, "failed block i=%d", i)
		}
		assert.Equal(t, 0, chainCreator.CreateChainCallCount())
		assert.Equal(t, 1, followerCreator.CreateFollowerCallCount())
	})
	t.Run("no need to pull", func(t *testing.T) {
		setup()
		clusterConsenter.IsChannelMemberReturns(false, nil)
		localBlockchain.fill(joinNum + 1) //No gap between the ledger and the join block
		assert.True(t, joinBlockAppRaft.Header.Number < ledgerResources.Height())

		chain, err := follower.NewChain(
			ledgerResources,
			clusterConsenter,
			joinBlockAppRaft,
			options,
			pullerCreator.CreateBlockPuller,
			chainCreator.CreateChain,
			followerCreator.CreateFollower,
			cryptoProvider)
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

		assert.Equal(t, 1, pullerCreator.CreateBlockPullerCallCount())
		assert.Equal(t, 0, ledgerResources.AppendCallCount())

		assert.Equal(t, 0, chainCreator.CreateChainCallCount())
		assert.Equal(t, 1, followerCreator.CreateFollowerCallCount())
	})
	t.Run("overcome pull failures", func(t *testing.T) {
		setup()
		clusterConsenter.IsChannelMemberReturns(true, nil)

		failPull := 10
		pullerCreator = &mocks.BlockPullerCreator{}
		puller = &mocks.ChannelPuller{}
		puller.PullBlockCalls(func(i uint64) *common.Block {
			if i%2 == 1 && failPull > 0 {
				failPull = failPull - 1
				return nil
			}
			failPull = 10
			return remoteBlockchain.Block(i)
		})
		pullerCreator.CreateBlockPullerReturns(puller, nil)
		options.TimeAfter = timeAfterCount.After

		chain, err := follower.NewChain(
			ledgerResources,
			clusterConsenter,
			joinBlockAppRaft,
			options,
			pullerCreator.CreateBlockPuller,
			chainCreator.CreateChain,
			followerCreator.CreateFollower,
			cryptoProvider)
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

		assert.Equal(t, 52, pullerCreator.CreateBlockPullerCallCount())
		assert.Equal(t, 11, ledgerResources.AppendCallCount())
		for i := uint64(0); i <= joinNum; i++ {
			assert.Equal(t, remoteBlockchain.Block(i).Header, localBlockchain.Block(i).Header, "failed block i=%d", i)
		}
		assert.Equal(t, 1, chainCreator.CreateChainCallCount())
		assert.Equal(t, 0, followerCreator.CreateFollowerCallCount())

		assert.Equal(t, 50, timeAfterCount.AfterCallCount())
		assert.Equal(t, int64(5000), maxDelay)
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

	prevHash := []byte{}
	for i := uint64(0); i < numBlocks; i++ {
		if i > 0 {
			prevHash = protoutil.BlockHeaderHash(mbc.chain[i-1].Header)
		}
		mbc.chain = append(mbc.chain, protoutil.NewBlock(i, prevHash))
	}
}
