/*
Copyright IBM Corp. 2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package follower_test

import (
	"sync"
	"testing"
	"time"

	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric/bccsp/sw"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/orderer/common/types"
	"github.com/hyperledger/fabric/orderer/consensus/follower"
	"github.com/hyperledger/fabric/orderer/consensus/follower/mocks"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var testLogger = flogging.MustGetLogger("follower.test")

var iAmNotInChannel = func(configBlock *common.Block) error {
	return errors.New("not in channel")
}

var iAmInChannel = func(configBlock *common.Block) error {
	return nil
}

func TestFollowerNewChain(t *testing.T) {
	joinNum := uint64(10)
	joinBlockAppRaft := protoutil.NewBlock(joinNum, []byte{})
	require.NotNil(t, joinBlockAppRaft)
	mockSupport := &mocks.Support{}
	mockSupport.ChannelIDReturns("my-channel")

	options := follower.Options{Logger: testLogger, Cert: []byte{1, 2, 3, 4}}

	t.Run("with join block, not in channel", func(t *testing.T) {
		mockSupport.HeightReturns(0)
		chain, err := follower.NewChain(mockSupport, joinBlockAppRaft, options, nil, nil, nil, iAmNotInChannel)
		assert.NoError(t, err)
		err = chain.Order(nil, 0)
		assert.EqualError(t, err, "orderer is a follower of channel my-channel")
		err = chain.Configure(nil, 0)
		assert.EqualError(t, err, "orderer is a follower of channel my-channel")
		err = chain.WaitReady()
		assert.EqualError(t, err, "orderer is a follower of channel my-channel")
		_, open := <-chain.Errored()
		assert.False(t, open)

		cRel, status := chain.StatusReport()
		assert.Equal(t, types.ClusterRelationFollower, cRel)
		assert.Equal(t, types.StatusOnBoarding, status)
	})

	t.Run("with join block, in channel", func(t *testing.T) {
		mockSupport.HeightReturns(0)
		createMockPuller := func() (follower.ChannelPuller, error) {
			return &mocks.ChannelPuller{}, nil
		}
		chain, err := follower.NewChain(mockSupport, joinBlockAppRaft, options, createMockPuller, nil, nil, iAmInChannel)
		assert.NoError(t, err)

		cRel, status := chain.StatusReport()
		assert.Equal(t, types.ClusterRelationMember, cRel)
		assert.Equal(t, types.StatusOnBoarding, status)

		// Start once, Halt once
		assert.False(t, chain.IsRunning())
		assert.NotPanics(t, chain.Start)
		assert.True(t, chain.IsRunning())
		assert.NotPanics(t, chain.Start)
		assert.True(t, chain.IsRunning())
		assert.NotPanics(t, chain.Halt)
		assert.False(t, chain.IsRunning())
		assert.NotPanics(t, chain.Halt)
		assert.NotPanics(t, chain.Start)
		assert.False(t, chain.IsRunning())
	})

	t.Run("bad join block", func(t *testing.T) {
		chain, err := follower.NewChain(mockSupport, &common.Block{}, options, nil, nil, nil, nil)
		assert.EqualError(t, err, "block header is nil")
		assert.Nil(t, chain)
		chain, err = follower.NewChain(mockSupport, &common.Block{Header: &common.BlockHeader{}}, options, nil, nil, nil, nil)
		assert.EqualError(t, err, "block data is nil")
		assert.Nil(t, chain)
	})

	t.Run("without join block", func(t *testing.T) {
		chain, err := follower.NewChain(mockSupport, nil, options, nil, nil, nil, nil)
		assert.NoError(t, err)

		cRel, status := chain.StatusReport()
		assert.Equal(t, types.ClusterRelationFollower, cRel)
		assert.True(t, status == types.StatusActive)
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

	blockchain := &memoryBlockChain{}
	blockchain.fill(joinNum + 1)

	// Before each test
	var resultChain *memoryBlockChain
	var mockSupport *mocks.Support
	var pullerCreator *mocks.BlockPullerCreator
	var puller *mocks.ChannelPuller
	var wg sync.WaitGroup
	var chainCreator *mocks.ChainCreator
	var timeAfterCount *timeAfterCounter

	setup := func() {
		resultChain = &memoryBlockChain{}

		mockSupport = &mocks.Support{}
		mockSupport.ChannelIDReturns("my-channel")
		mockSupport.HeightCalls(resultChain.Height)
		mockSupport.BlockCalls(resultChain.Block)
		mockSupport.AppendCalls(func(block *common.Block) error {
			resultChain.Append(block)
			return nil
		})

		pullerCreator = &mocks.BlockPullerCreator{}
		puller = &mocks.ChannelPuller{}
		puller.PullBlockCalls(func(i uint64) *common.Block { return blockchain.Block(i) })
		pullerCreator.CreateBlockPullerReturns(puller, nil)

		wg = sync.WaitGroup{}
		wg.Add(1)
		chainCreator = &mocks.ChainCreator{}
		chainCreator.CreateChainCalls(wg.Done)

		timeAfterCount = &timeAfterCounter{}
	}

	t.Run("zero until join block", func(t *testing.T) {
		setup()

		chain, err := follower.NewChain(
			mockSupport,
			joinBlockAppRaft,
			options,
			pullerCreator.CreateBlockPuller,
			chainCreator.CreateChain,
			cryptoProvider,
			iAmInChannel)
		assert.NoError(t, err)

		cRel, status := chain.StatusReport()
		assert.Equal(t, types.ClusterRelationMember, cRel)
		assert.Equal(t, types.StatusOnBoarding, status)

		assert.NotPanics(t, chain.Start)
		wg.Wait()
		assert.NotPanics(t, chain.Halt)
		assert.False(t, chain.IsRunning())

		cRel, status = chain.StatusReport()
		assert.Equal(t, types.ClusterRelationMember, cRel)
		assert.Equal(t, types.StatusActive, status)

		assert.Equal(t, 1, pullerCreator.CreateBlockPullerCallCount())
		assert.Equal(t, 11, mockSupport.AppendCallCount())
		for i := uint64(0); i <= joinNum; i++ {
			assert.Equal(t, blockchain.Block(i).Header, resultChain.Block(i).Header, "failed block i=%d", i)
		}
		assert.Equal(t, 1, chainCreator.CreateChainCallCount())
	})

	t.Run("existing half chain until join block", func(t *testing.T) {
		setup()
		resultChain.fill(joinNum / 2) //A gap between the ledger and the join block
		assert.True(t, joinBlockAppRaft.Header.Number > mockSupport.Height())
		assert.True(t, mockSupport.Height() > 0)

		chain, err := follower.NewChain(
			mockSupport,
			joinBlockAppRaft,
			options,
			pullerCreator.CreateBlockPuller,
			chainCreator.CreateChain,
			cryptoProvider,
			iAmNotInChannel)
		assert.NoError(t, err)

		cRel, status := chain.StatusReport()
		assert.Equal(t, types.ClusterRelationFollower, cRel)
		assert.Equal(t, types.StatusOnBoarding, status)

		assert.NotPanics(t, chain.Start)
		wg.Wait()
		assert.NotPanics(t, chain.Halt)
		assert.False(t, chain.IsRunning())

		cRel, status = chain.StatusReport()
		assert.Equal(t, types.ClusterRelationFollower, cRel)
		assert.Equal(t, types.StatusActive, status)

		assert.Equal(t, 1, pullerCreator.CreateBlockPullerCallCount())
		assert.Equal(t, 6, mockSupport.AppendCallCount())
		for i := uint64(0); i <= joinNum; i++ {
			assert.Equal(t, blockchain.Block(i).Header, resultChain.Block(i).Header, "failed block i=%d", i)
		}
		assert.Equal(t, 1, chainCreator.CreateChainCallCount())
	})

	t.Run("no need to pull", func(t *testing.T) {
		setup()
		resultChain.fill(joinNum + 1) //No gap between the ledger and the join block
		assert.True(t, joinBlockAppRaft.Header.Number < mockSupport.Height())

		chain, err := follower.NewChain(
			mockSupport,
			joinBlockAppRaft,
			options,
			pullerCreator.CreateBlockPuller,
			chainCreator.CreateChain,
			cryptoProvider,
			iAmNotInChannel)
		assert.NoError(t, err)

		cRel, status := chain.StatusReport()
		assert.Equal(t, types.ClusterRelationFollower, cRel)
		assert.Equal(t, types.StatusActive, status)

		assert.NotPanics(t, chain.Start)
		wg.Wait()
		assert.NotPanics(t, chain.Halt)
		assert.False(t, chain.IsRunning())

		cRel, status = chain.StatusReport()
		assert.Equal(t, types.ClusterRelationFollower, cRel)
		assert.Equal(t, types.StatusActive, status)

		assert.Equal(t, 0, pullerCreator.CreateBlockPullerCallCount())
		assert.Equal(t, 0, mockSupport.AppendCallCount())
		assert.Equal(t, 1, chainCreator.CreateChainCallCount())
	})

	t.Run("overcome pull failures", func(t *testing.T) {
		setup()

		failPull := 10
		pullerCreator = &mocks.BlockPullerCreator{}
		puller = &mocks.ChannelPuller{}
		puller.PullBlockCalls(func(i uint64) *common.Block {
			if i%2 == 1 && failPull > 0 {
				failPull = failPull - 1
				return nil
			}
			failPull = 10
			return blockchain.Block(i)
		})
		pullerCreator.CreateBlockPullerReturns(puller, nil)
		options.TimeAfter = timeAfterCount.After

		flogging.ActivateSpec("debug")
		chain, err := follower.NewChain(
			mockSupport,
			joinBlockAppRaft,
			options,
			pullerCreator.CreateBlockPuller,
			chainCreator.CreateChain,
			cryptoProvider,
			iAmInChannel)
		assert.NoError(t, err)

		cRel, status := chain.StatusReport()
		assert.Equal(t, types.ClusterRelationMember, cRel)
		assert.Equal(t, types.StatusOnBoarding, status)

		assert.NotPanics(t, chain.Start)
		wg.Wait()
		assert.NotPanics(t, chain.Halt)
		assert.False(t, chain.IsRunning())

		cRel, status = chain.StatusReport()
		assert.Equal(t, types.ClusterRelationMember, cRel)
		assert.Equal(t, types.StatusActive, status)

		assert.Equal(t, 51, pullerCreator.CreateBlockPullerCallCount())
		assert.Equal(t, 11, mockSupport.AppendCallCount())
		for i := uint64(0); i <= joinNum; i++ {
			assert.Equal(t, blockchain.Block(i).Header, resultChain.Block(i).Header, "failed block i=%d", i)
		}
		assert.Equal(t, 1, chainCreator.CreateChainCallCount())
		assert.Equal(t, 50, timeAfterCount.getInvocations())
		assert.Equal(t, int64(5000), timeAfterCount.getMaxDelay())
	})
}

type memoryBlockChain struct {
	lock  sync.Mutex
	chain []*common.Block
}

func (mbc *memoryBlockChain) Append(block *common.Block) {
	mbc.lock.Lock()
	mbc.lock.Unlock()
	mbc.chain = append(mbc.chain, block)
}

func (mbc *memoryBlockChain) Height() uint64 {
	mbc.lock.Lock()
	mbc.lock.Unlock()
	return uint64(len(mbc.chain))
}

func (mbc *memoryBlockChain) Block(i uint64) *common.Block {
	mbc.lock.Lock()
	mbc.lock.Unlock()
	if i < uint64(len(mbc.chain)) {
		return mbc.chain[i]
	}
	return nil
}

func (mbc *memoryBlockChain) fill(numBlocks uint64) {
	mbc.chain = []*common.Block{}
	prevHash := []byte{}
	for i := uint64(0); i < numBlocks; i++ {
		if i > 0 {
			prevHash = protoutil.BlockHeaderHash(mbc.Block(i - 1).Header)
		}
		mbc.Append(protoutil.NewBlock(i, prevHash))
	}
}

type timeAfterCounter struct {
	mutex       sync.Mutex
	invocations int
	maxDelay    int64
}

func (t *timeAfterCounter) After(d time.Duration) <-chan time.Time {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	t.invocations++
	if d.Nanoseconds() > t.maxDelay {
		t.maxDelay = d.Nanoseconds()
	}
	c := make(chan time.Time, 1)
	c <- time.Now()
	return c
}

func (t *timeAfterCounter) getMaxDelay() int64 {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	return t.maxDelay
}

func (t *timeAfterCounter) getInvocations() int {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	return t.invocations
}
