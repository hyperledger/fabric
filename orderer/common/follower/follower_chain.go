/*
Copyright IBM Corp. 2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package follower

import (
	"bytes"
	"sync"
	"time"

	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric/bccsp"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/orderer/common/cluster"
	"github.com/hyperledger/fabric/orderer/common/types"
	"github.com/hyperledger/fabric/orderer/consensus"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
)

//go:generate counterfeiter -o mocks/ledger_resources.go -fake-name LedgerResources . LedgerResources

// LedgerResources defines dome the interfaces of ledger & config resources needed by the follower.Chain.
type LedgerResources interface {
	// ChannelID The channel ID.
	ChannelID() string

	// Block returns a block with the given number,
	// or nil if such a block doesn't exist.
	Block(number uint64) *common.Block

	// Height returns the number of blocks in the chain this channel is associated with.
	Height() uint64

	// Append appends a new block to the ledger in its raw form.
	Append(block *common.Block) error
}

// TimeAfter has the signature of time.After and allows tests to provide an alternative implementation to it.
type TimeAfter func(d time.Duration) <-chan time.Time

// CreateBlockPuller is a function to create BlockPuller on demand.
// It is passed into chain initializer so that tests could mock this.
type CreateBlockPuller func() (ChannelPuller, error)

// CreateChain is a function that creates a new consensus.Chain for this channel, to replace the current follower.Chain
type CreateChain func()

// CreateFollower is a function that creates a new follower.Chain for this channel, to replace the current
// follower.Chain. This happens when on-boarding ends (ledger reached join-block), and a new follower needs to be
// created to pull blocks after the join block.
type CreateFollower func()

// Chain implements a component that allows the orderer to follow a specific channel when is not a cluster member,
// that is, be a "follower" of the cluster. It also allows the orderer to perform "on-boarding" for
// channels it is joining as a member, with a join-block.
//
// When an orderer is following a channel, it means that the current orderer is not a member of the consenters set
// of the channel, and is only pulling blocks from other orderers. In this mode, the follower is inspecting config
// blocks as they are pulled and if it discovers that it was introduced into the consenters set, it will trigger the
// creation of a regular etcdraft.Chain, that is, turn into a "member" of the cluster.
//
// A follower is also used to on-board a channel when joining as a member with a join-block that has number >0. In this
// mode the follower will pull blocks up until join-block.number, and then will trigger the creation of a regular
// etcdraft.Chain.
//
// The follower is started in one of two ways: 1) following an API Join request with a join-block that has
// block number >0, or 2) when the orderer was a cluster member (i.e. was running a etcdraft.Chain) and was removed
// from the consenters set.
//
// The follower is in status "onboarding" when it pulls blocks below the join-block number, or "active" when it
// pulls blocks equal or above the join-block number.
//
// The follower return clusterRelation "member" when the join-block indicates the orderer is in the consenters set,
// i.e. the follower is performing on-boarding for an etcdraft.Chain. Otherwise, the follower return clusterRelation
// "follower".
type Chain struct {
	mutex    sync.Mutex    // Protects the start/stop flags & channels, all the rest are immutable or accessed only by the go-routine.
	started  bool          // Start once.
	stopped  bool          // Stop once.
	stopChan chan struct{} // A 'closer' signals the go-routine to stop by closing this channel.
	doneChan chan struct{} // The go-routine signals the 'closer' that it is done by closing this channel.

	ledgerResources  LedgerResources            // ledger & config resources
	clusterConsenter consensus.ClusterConsenter // detects whether a block indicates channel membership
	options          Options
	logger           *flogging.FabricLogger
	timeAfter        TimeAfter // time.After by default, or an alternative form Options.

	joinBlock   *common.Block // The join-block the follower was started with.
	lastConfig  *common.Block // The last config block from the ledger. Accessed only by the go-routine.
	firstHeight uint64        // The first ledger height

	createBlockPullerFunc    CreateBlockPuller // A function that creates a block puller on demand
	chainCreationCallback    CreateChain       // A method that creates a new consensus.Chain for this channel, to replace the current follower.Chain
	followerCreationCallback CreateFollower    // A method that creates a new follower.Chain for this channel, to replace the current follower.Chain

	cryptoProvider bccsp.BCCSP

	retryInterval time.Duration // Retry interval default min/max are different for until and after join-block pulling.
}

// NewChain constructs a follower.Chain object.
func NewChain(
	ledgerResources LedgerResources,
	clusterConsenter consensus.ClusterConsenter,
	joinBlock *common.Block,
	options Options,
	createBlockPullerFunc CreateBlockPuller,
	chainCreationCallback CreateChain,
	followerCreationCallback CreateFollower,
	cryptoProvider bccsp.BCCSP,
) (*Chain, error) {
	// Check the block puller creation function once before we start the follower.
	puller, errBP := createBlockPullerFunc()
	if errBP != nil {
		return nil, errors.Wrap(errBP, "error creating a block puller from join-block")
	}
	defer puller.Close()

	options.applyDefaults(joinBlock != nil)

	chain := &Chain{
		stopChan:                 make(chan (struct{})),
		doneChan:                 make(chan (struct{})),
		ledgerResources:          ledgerResources,
		clusterConsenter:         clusterConsenter,
		joinBlock:                joinBlock,
		firstHeight:              ledgerResources.Height(),
		options:                  options,
		logger:                   options.Logger.With("channel", ledgerResources.ChannelID()),
		timeAfter:                options.TimeAfter,
		createBlockPullerFunc:    createBlockPullerFunc,
		chainCreationCallback:    chainCreationCallback,
		followerCreationCallback: followerCreationCallback,
		cryptoProvider:           cryptoProvider,
		retryInterval:            options.PullRetryMinInterval,
	}

	if joinBlock == nil {
		chain.logger.Infof("Created with a nil join-block, ledger height: %d", chain.firstHeight)
	} else {
		if joinBlock.Header == nil {
			return nil, errors.New("block header is nil")
		}
		if joinBlock.Data == nil {
			return nil, errors.New("block data is nil")
		}

		chain.logger.Infof("Created with join-block number: %d, ledger height: %d", joinBlock.Header.Number, chain.firstHeight)
	}
	chain.logger.Debugf("Options are: %v", chain.options)

	if chain.chainCreationCallback == nil {
		chain.chainCreationCallback = func() {
			chain.logger.Warnf("No-Op: chain creation callback is nil")
		}
	}

	if chain.followerCreationCallback == nil {
		chain.followerCreationCallback = func() {
			chain.logger.Warnf("No-Op: follower creation callback is nil")
		}
	}

	return chain, nil
}

func (c *Chain) Start() {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if c.started || c.stopped {
		c.logger.Debugf("Not starting because: started=%v, stopped=%v", c.started, c.stopped)
		return
	}

	c.started = true

	go c.run()

	c.logger.Info("Started")
}

func (c *Chain) Halt() {
	c.halt()

	select {
	case <-c.doneChan:
	}
}

func (c *Chain) halt() {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if c.stopped {
		c.logger.Debug("Already stopped")
		return
	}
	c.stopped = true
	close(c.stopChan)
	c.logger.Info("Stopped")
}

// StatusReport returns the ClusterRelation & Status.
func (c *Chain) StatusReport() (types.ClusterRelation, types.Status) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	clusterRelation := types.ClusterRelationFollower
	if c.joinBlock != nil {
		isMem, _ := c.clusterConsenter.IsChannelMember(c.joinBlock)
		if isMem {
			clusterRelation = types.ClusterRelationMember
		}
	}

	status := types.StatusActive
	if (c.joinBlock != nil) && (c.joinBlock.Header.Number >= c.ledgerResources.Height()) {
		status = types.StatusOnBoarding
	}

	return clusterRelation, status
}

func (c *Chain) Height() uint64 {
	return c.ledgerResources.Height()
}

func (c *Chain) IsRunning() bool {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if c.started {
		select {
		case <-c.doneChan:
			return false
		default:
			return true
		}
	}

	return false
}

func (c *Chain) run() {
	c.logger.Debug("The follower.Chain puller goroutine is starting")

	defer func() {
		close(c.doneChan)
		c.logger.Debug("The follower.Chain puller goroutine is exiting")
	}()

	err := c.pull()

	for err != nil {
		c.logger.Debugf("Pull failed, going to try again in %v; error: %s", c.retryInterval, err)

		select {
		case <-c.stopChan:
			c.logger.Debug("Received a stop signal")
			return

		case <-c.timeAfter(c.retryInterval):
			err = c.pull()
		}

		if err != nil {
			c.increaseRetryInterval()
		}
	}
}

func (c *Chain) increaseRetryInterval() {
	if c.retryInterval == c.options.PullRetryMaxInterval {
		return
	}
	//assuming this will never overflow int64, as PullRetryMaxInterval cannot be over MaxInt64/2
	c.retryInterval = time.Duration(1.5 * float64(c.retryInterval))
	if c.retryInterval > c.options.PullRetryMaxInterval {
		c.retryInterval = c.options.PullRetryMaxInterval
	}
	c.logger.Debugf("retry interval increased to: %v", c.retryInterval)
}

func (c *Chain) resetRetryInterval() {
	if c.retryInterval == c.options.PullRetryMinInterval {
		return
	}
	c.retryInterval = c.options.PullRetryMinInterval
	c.logger.Debugf("retry interval reset to: %v", c.retryInterval)
}

// pull blocks from other orderers, until the join block, or until a config block the indicates the orderer has become
// a member pf the cluster. When the follower.Chain's job is done, this method halts, triggers the creation of a new
// consensus.Chain, and returns nil.
func (c *Chain) pull() error {
	if c.joinBlock != nil {
		err := c.pullUpToJoin()
		if err != nil {
			return errors.Wrap(err, "failed to pull up to join block")
		}

		// TODO remove the join-block from the file repo
	} else {
		err := c.pullAfterJoin()
		if err != nil {
			return errors.Wrap(err, "failed to pull after join block")
		}
	}

	// Trigger creation of a new chain. This will regenerate 'ledgerResources' from the tip of the ledger, and will
	// start either a follower.Chain or an etcdraft.Chain, depending on whether the orderer is a follower or a
	// member of the cluster.
	c.halt()

	if c.joinBlock != nil {
		if isMem, _ := c.clusterConsenter.IsChannelMember(c.joinBlock); isMem {
			c.logger.Info("On-boarding finished successfully, invoking chain creation callback")
			c.chainCreationCallback()
		} else {
			c.logger.Info("On-boarding finished successfully, invoking follower creation callback")
			c.followerCreationCallback()
		}
	} else {
		c.logger.Info("Block pulling finished successfully, invoking chain creation callback")
		c.chainCreationCallback()
	}

	return nil
}

// pullUpToJoin pulls blocks up to the join-block height without inspecting membership on fetched config blocks.
// It checks whether the chain was stopped between blocks.
func (c *Chain) pullUpToJoin() error {
	targetHeight := c.joinBlock.Header.Number + 1
	firstBlockToPull := c.ledgerResources.Height()
	c.logger.Debugf("first block to pull: %d, target height: %d", firstBlockToPull, targetHeight)
	if firstBlockToPull >= targetHeight {
		c.logger.Infof("Target height (%d) is <= to our ledger height (%d), skipping pulling", targetHeight, firstBlockToPull)
		return nil
	}

	puller, err := c.createBlockPullerFunc()
	if err != nil {
		c.logger.Errorf("Error creating block puller: %s", err)
		return errors.Wrap(err, "error creating block puller")
	}
	defer puller.Close()

	var actualPrevHash []byte
	// Initialize the actual previous hash
	if firstBlockToPull > 0 {
		prevBlock := c.ledgerResources.Block(firstBlockToPull - 1)
		if prevBlock == nil {
			return errors.Errorf("cannot retrieve previous block %d", firstBlockToPull-1)
		}
		actualPrevHash = protoutil.BlockHeaderHash(prevBlock.Header)
	}

	// Pull the rest of the blocks
	for seq := firstBlockToPull; seq < targetHeight; seq++ {
		select {
		case <-c.stopChan:
			c.logger.Warnf("Stopped before pulling all the blocks: pulled %d blocks from the range %d until %d",
				seq-firstBlockToPull, firstBlockToPull, targetHeight-1)
			return errors.New("stopped before pulling all the blocks")
		default:
			nextBlock := puller.PullBlock(seq)
			if nextBlock == nil {
				return errors.Wrapf(cluster.ErrRetryCountExhausted, "failed to pull block %d", seq)
			}
			reportedPrevHash := nextBlock.Header.PreviousHash
			if (nextBlock.Header.Number > 0) && !bytes.Equal(reportedPrevHash, actualPrevHash) {
				return errors.Errorf("block header mismatch on sequence %d, expected %x, got %x",
					nextBlock.Header.Number, actualPrevHash, reportedPrevHash)
			}
			actualPrevHash = protoutil.BlockHeaderHash(nextBlock.Header)
			if err = c.ledgerResources.Append(nextBlock); err != nil {
				return errors.Wrapf(err, "failed to append block %d to the ledger", nextBlock.Header.Number)
			}
			c.resetRetryInterval()
		}
	}

	c.logger.Infof("Pulled blocks from %d until %d", c.firstHeight, targetHeight-1)
	return nil
}

// pullAfterJoin pulls blocks continuously, inspecting the fetched config blocks for membership.
// It will exit with 'nil' if it detects a config block that indicates the orderer is a member of the cluster.
// It checks whether the chain was stopped between blocks.
func (c *Chain) pullAfterJoin() error {
	if c.lastConfig == nil {
		err := c.loadLastConfig()
		if err != nil {
			return errors.Wrap(err, "failed to load last config block")
		}
	}

	c.logger.Debugf("last config block: %d", c.lastConfig.Header.Number)
	channelMember, err := c.clusterConsenter.IsChannelMember(c.lastConfig)
	if err != nil {
		return errors.Wrap(err, "failed to determine channel membership from last config")
	}

	for !channelMember {
		configBlock, err := c.pullUntilNextConfig()
		if err != nil {
			return errors.Wrap(err, "failed to pull until next config")
		}

		channelMember, err = c.clusterConsenter.IsChannelMember(configBlock)
		if err != nil {
			return errors.Wrap(err, "failed to determine channel membership from pulled config")
		}

		c.logger.Debugf("next config block: %d, channel member: %v", c.lastConfig.Header.Number, channelMember)
	}

	return err
}

// pullUntilNextConfig will return the next config block or an error.
func (c *Chain) pullUntilNextConfig() (*common.Block, error) {
	// This puller is built from the tip of the ledger. When we get a new config block we exit the method. That will
	// destroy the puller. Next time we enter, a new puller is built, taking in the new config.
	puller, err := c.createBlockPullerFunc()
	if err != nil {
		c.logger.Errorf("Error creating block puller: %s", err)
		return nil, errors.Wrap(err, "error creating block puller")
	}
	defer puller.Close()

	var configBlock *common.Block
	for configBlock == nil {
		endpoint, latestHeight, err := cluster.LatestHeightAndEndpoint(puller)
		if err != nil {
			return nil, errors.Wrap(err, "failed to get latest height and endpoint")
		}
		c.logger.Debugf("Orderer endpoint %s has the biggest ledger height: %d", endpoint, latestHeight)

		if latestHeight <= c.ledgerResources.Height() {
			c.logger.Debugf("My height: %d, latest height: %d; going to wait %v for latest height to grow",
				c.ledgerResources.Height(), latestHeight, c.retryInterval)
			select {
			case <-c.stopChan:
				return nil, errors.New("stopped while waiting for latest height to grow")
			case <-c.timeAfter(c.retryInterval):
				c.increaseRetryInterval()
				continue
			}
		}

		configBlock, err = c.pullUntilLatestOrConfig(puller, latestHeight)
		if err != nil {
			return nil, errors.Wrap(err, "failed to pull until latest height or config")
		}
		c.resetRetryInterval()
	}

	return configBlock, nil
}

func (c *Chain) pullUntilLatestOrConfig(puller ChannelPuller, latestHeight uint64) (*common.Block, error) {
	firstBlockToPull := c.ledgerResources.Height()
	var nextBlock = c.ledgerResources.Block(firstBlockToPull - 1)
	if nextBlock == nil {
		return nil, errors.Errorf("cannot retrieve previous block %d", firstBlockToPull-1)
	}
	var actualPrevHash = protoutil.BlockHeaderHash(nextBlock.Header)

	// Pull until the latest height or a config block
	for seq := firstBlockToPull; seq < latestHeight; seq++ {
		select {
		case <-c.stopChan:
			c.logger.Warnf("Stopped while pulling blocks: from %d until %d, last pulled %d",
				firstBlockToPull, latestHeight, nextBlock.Header.Number)
			return nil, errors.New("stopped while pulling blocks")
		default:
			nextBlock = puller.PullBlock(seq)
			if nextBlock == nil {
				return nil, errors.Wrapf(cluster.ErrRetryCountExhausted, "failed to pull block %d", seq)
			}
			reportedPrevHash := nextBlock.Header.PreviousHash
			if !bytes.Equal(reportedPrevHash, actualPrevHash) {
				return nil, errors.Errorf("block header mismatch on sequence %d, expected %x, got %x",
					nextBlock.Header.Number, actualPrevHash, reportedPrevHash)
			}
			actualPrevHash = protoutil.BlockHeaderHash(nextBlock.Header)
			if err := c.ledgerResources.Append(nextBlock); err != nil {
				return nil, errors.Wrapf(err, "failed to append block %d to the ledger", nextBlock.Header.Number)
			}
			c.resetRetryInterval()
			if protoutil.IsConfigBlock(nextBlock) {
				c.lastConfig = nextBlock
				c.logger.Debugf("Pulled blocks from %d to %d, last block is config", firstBlockToPull, nextBlock.Header.Number)
				return nextBlock, nil
			}
		}
	}
	c.logger.Debugf("Pulled blocks from %d to %d, no config blocks in that range", firstBlockToPull, nextBlock.Header.Number)
	return nil, nil
}

func (c *Chain) loadLastConfig() error {
	height := c.ledgerResources.Height()
	lastBlock := c.ledgerResources.Block(height - 1)
	index, err := protoutil.GetLastConfigIndexFromBlock(lastBlock)
	if err != nil {
		return errors.Wrap(err, "chain does have appropriately encoded last config in its latest block")
	}
	lastConfig := c.ledgerResources.Block(index)
	if lastConfig == nil {
		return errors.Wrapf(err, "could not retrieve config block from index %d", index)
	}
	c.lastConfig = lastConfig
	return nil
}
