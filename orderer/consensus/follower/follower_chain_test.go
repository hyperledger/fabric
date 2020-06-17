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
	"github.com/hyperledger/fabric/orderer/common/cluster"
	"github.com/hyperledger/fabric/orderer/common/types"
	"github.com/hyperledger/fabric/orderer/consensus/follower"
	"github.com/hyperledger/fabric/orderer/consensus/follower/mocks"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
)

var testLogger = flogging.MustGetLogger("follower.test")

var iAmNotInChannel = func(configBlock *common.Block) error {
	return errors.New("not in channel")
}

var iAmInChannel = func(configBlock *common.Block) error {
	return nil
}

var amIReallyInChannel = func(configBlock *common.Block) error {
	if !protoutil.IsConfigBlock(configBlock) {
		return errors.New("not a config")
	}

	env, err := protoutil.ExtractEnvelope(configBlock, 0)
	if err != nil {
		return err
	}

	payload := protoutil.UnmarshalPayloadOrPanic(env.Payload)
	if len(payload.Data) > 0 && payload.Data[0] > 0 {
		return nil
	}

	return cluster.ErrNotInChannel
}

func TestFollowerNewChain(t *testing.T) {
	joinNum := uint64(10)
	joinBlockAppRaft := makeConfigBlock(joinNum, []byte{}, 0)

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
	joinBlockAppRaft := makeConfigBlock(joinNum, []byte{}, 1)

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

func TestFollowerPullAfterJoin(t *testing.T) {
	joinNum := uint64(10)

	options := follower.Options{
		Logger:               testLogger,
		PullRetryMinInterval: 1 * time.Microsecond,
		PullRetryMaxInterval: 5 * time.Microsecond,
		Cert:                 []byte{1, 2, 3, 4}}

	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	assert.NoError(t, err)

	// Before each test
	var blockchain *memoryBlockChain
	var resultChain *memoryBlockChain
	var mockSupport *mocks.Support
	var pullerCreator *mocks.BlockPullerCreator
	var puller *mocks.ChannelPuller
	var wg sync.WaitGroup
	var chainCreator *mocks.ChainCreator
	var timeAfterCount *timeAfterCounter

	setup := func() {
		blockchain = &memoryBlockChain{}
		blockchain.fill(joinNum + 11)
		resultChain = &memoryBlockChain{}
		resultChain.chain = append(resultChain.chain, blockchain.chain[0:joinNum+1]...)
		mockSupport = &mocks.Support{}
		mockSupport.ChannelIDReturns("my-channel")
		mockSupport.HeightCalls(resultChain.Height)
		mockSupport.BlockCalls(resultChain.Block)

		pullerCreator = &mocks.BlockPullerCreator{}
		puller = &mocks.ChannelPuller{}
		puller.PullBlockCalls(func(i uint64) *common.Block { return blockchain.Block(i) })
		puller.HeightsByEndpointsCalls(
			func() (map[string]uint64, error) {
				m := make(map[string]uint64)
				m["good-node"] = blockchain.Height()
				m["lazy-node"] = blockchain.Height() - 2
				return m, nil
			},
		)
		pullerCreator.CreateBlockPullerReturns(puller, nil)
		chainCreator = &mocks.ChainCreator{}

		wg = sync.WaitGroup{}
		wg.Add(1)

		timeAfterCount = &timeAfterCounter{}
	}

	t.Run("No config in the middle", func(t *testing.T) {
		setup()
		mockSupport.AppendCalls(func(block *common.Block) error {
			resultChain.Append(block)
			//Stop when we catch-up with latest
			if blockchain.Height() == resultChain.Height() {
				wg.Done()
			}
			return nil
		})
		assert.Equal(t, joinNum+1, resultChain.Height())

		chain, err := follower.NewChain(
			mockSupport,
			nil, // Past on-boarding
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

		assert.Equal(t, 1, pullerCreator.CreateBlockPullerCallCount())
		assert.Equal(t, 10, mockSupport.AppendCallCount())
		assert.Equal(t, uint64(21), resultChain.Height())
		for i := uint64(0); i < resultChain.Height(); i++ {
			assert.Equal(t, blockchain.Block(i).Header, resultChain.Block(i).Header, "failed block i=%d", i)
		}
		assert.Equal(t, 0, chainCreator.CreateChainCallCount())
	})

	t.Run("No config in the middle, latest height increasing", func(t *testing.T) {
		setup()
		mockSupport.AppendCalls(func(block *common.Block) error {
			resultChain.Append(block)
			if blockchain.Height() == resultChain.Height() {
				if blockchain.Height() < 50 {
					blockchain.fill(10)
				} else {
					//Stop when we catch-up with latest
					wg.Done()
				}
			}
			return nil
		})
		assert.Equal(t, joinNum+1, resultChain.Height())

		chain, err := follower.NewChain(
			mockSupport,
			nil, // Past on-boarding
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

		assert.Equal(t, 1, pullerCreator.CreateBlockPullerCallCount())
		assert.Equal(t, 40, mockSupport.AppendCallCount())
		assert.Equal(t, uint64(51), resultChain.Height())
		for i := uint64(0); i < resultChain.Height(); i++ {
			assert.Equal(t, blockchain.Block(i).Header, resultChain.Block(i).Header, "failed block i=%d", i)
		}
		assert.Equal(t, 0, chainCreator.CreateChainCallCount())
	})

	t.Run("Configs in the middle, latest height increasing", func(t *testing.T) {
		setup()
		mockSupport.AppendCalls(func(block *common.Block) error {
			resultChain.Append(block)

			if blockchain.Height() == resultChain.Height() {
				if blockchain.Height() < 50 {
					blockchain.fill(9)
					h := blockchain.Height()
					//Each config appended will trigger the creation of a new puller in the next round
					configBlock := makeConfigBlock(h, protoutil.BlockHeaderHash(blockchain.Block(h-1).Header), 0)
					blockchain.Append(configBlock)
				} else {
					blockchain.fill(9)
					h := blockchain.Height()
					//This will trigger the creation of a new chain
					configBlock := makeConfigBlock(h, protoutil.BlockHeaderHash(blockchain.Block(h-1).Header), 1)
					blockchain.Append(configBlock)
				}
			}
			return nil
		})
		chainCreator.CreateChainCalls(wg.Done) //Stop when a new chain is created
		assert.Equal(t, joinNum+1, resultChain.Height())

		chain, err := follower.NewChain(
			mockSupport,
			nil, // Past on-boarding
			options,
			pullerCreator.CreateBlockPuller,
			chainCreator.CreateChain,
			cryptoProvider,
			amIReallyInChannel)
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

		assert.Equal(t, 4, pullerCreator.CreateBlockPullerCallCount(), "after finding a config, block puller is created")
		assert.Equal(t, 50, mockSupport.AppendCallCount())
		assert.Equal(t, uint64(61), resultChain.Height())
		for i := uint64(0); i < resultChain.Height(); i++ {
			assert.Equal(t, blockchain.Block(i).Header, resultChain.Block(i).Header, "failed block i=%d", i)
		}
		assert.Equal(t, 1, chainCreator.CreateChainCallCount())
	})

	t.Run("Overcome puller errors, configs in the middle, latest height increasing", func(t *testing.T) {
		setup()
		mockSupport.AppendCalls(func(block *common.Block) error {
			resultChain.Append(block)

			if blockchain.Height() == resultChain.Height() {
				if blockchain.Height() < 50 {
					blockchain.fill(9)
					h := blockchain.Height()
					//Each config appended will trigger the creation of a new puller in the next round
					configBlock := makeConfigBlock(h, protoutil.BlockHeaderHash(blockchain.Block(h-1).Header), 0)
					blockchain.Append(configBlock)
				} else {
					blockchain.fill(9)
					h := blockchain.Height()
					//This will trigger the creation of a new chain
					configBlock := makeConfigBlock(h, protoutil.BlockHeaderHash(blockchain.Block(h-1).Header), 1)
					blockchain.Append(configBlock)
				}
			}
			return nil
		})
		chainCreator.CreateChainCalls(wg.Done) //Stop when a new chain is created
		assert.Equal(t, joinNum+1, resultChain.Height())

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

		failHeight := 1
		puller.HeightsByEndpointsCalls(
			func() (map[string]uint64, error) {
				if failHeight > 0 {
					failHeight = failHeight - 1
					return nil, errors.New("failed to get heights")
				}
				failHeight = 1
				m := make(map[string]uint64)
				m["good-node"] = blockchain.Height()
				m["lazy-node"] = blockchain.Height() - 2
				return m, nil
			},
		)
		pullerCreator.CreateBlockPullerReturns(puller, nil)

		options.TimeAfter = timeAfterCount.After

		chain, err := follower.NewChain(
			mockSupport,
			nil, // Past on-boarding
			options,
			pullerCreator.CreateBlockPuller,
			chainCreator.CreateChain,
			cryptoProvider,
			amIReallyInChannel)
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

		assert.Equal(t, 509, pullerCreator.CreateBlockPullerCallCount(), "after finding a config, or error, block puller is created")
		assert.Equal(t, 50, mockSupport.AppendCallCount())
		assert.Equal(t, uint64(61), resultChain.Height())
		for i := uint64(0); i < resultChain.Height(); i++ {
			assert.Equal(t, blockchain.Block(i).Header, resultChain.Block(i).Header, "failed block i=%d", i)
		}
		assert.Equal(t, 1, chainCreator.CreateChainCallCount())
		assert.Equal(t, 505, timeAfterCount.getInvocations())
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
	mbc.lock.Lock()
	mbc.lock.Unlock()

	height := len(mbc.chain)
	prevHash := []byte{}
	for i := uint64(height); i < uint64(height)+numBlocks; i++ {
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

		mbc.Append(block)
	}
}

func TestChain_makeConfigBlock(t *testing.T) {
	joinBlockAppRaft := makeConfigBlock(10, []byte{1, 2, 3, 4}, 0)
	assert.NotNil(t, joinBlockAppRaft)
	assert.True(t, protoutil.IsConfigBlock(joinBlockAppRaft))
	assert.NotPanics(t, func() { protoutil.GetLastConfigIndexFromBlockOrPanic(joinBlockAppRaft) })
	assert.Equal(t, uint64(10), protoutil.GetLastConfigIndexFromBlockOrPanic(joinBlockAppRaft))
	assert.NotPanics(t, func() { amIReallyInChannel(joinBlockAppRaft) })
	assert.EqualError(t, amIReallyInChannel(joinBlockAppRaft), cluster.ErrNotInChannel.Error())
	joinBlockAppRaft = makeConfigBlock(11, []byte{1, 2, 3, 4}, 1)
	assert.NoError(t, amIReallyInChannel(joinBlockAppRaft))
	assert.EqualError(t, amIReallyInChannel(protoutil.NewBlock(10, []byte{1, 2, 3, 4})), "not a config")
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
