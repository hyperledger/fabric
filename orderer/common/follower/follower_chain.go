/*
Copyright IBM Corp. 2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package follower

import (
	"bytes"
	"github.com/hyperledger/fabric/orderer/common/cluster"
	"github.com/hyperledger/fabric/orderer/consensus"
	"github.com/hyperledger/fabric/protoutil"
	"math"
	"sync"
	"time"

	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric/bccsp"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/orderer/common/types"
	"github.com/pkg/errors"
)

//TODO skeleton

const (
	defaultPullRetryMinInterval time.Duration = 50 * time.Millisecond
	defaultPullRetryMaxInterval time.Duration = 60 * time.Second
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

// Options contains all the configurations relevant to the chain.
type Options struct {
	Logger               *flogging.FabricLogger
	PullRetryMinInterval time.Duration
	PullRetryMaxInterval time.Duration
	Cert                 []byte
	TimeAfter            TimeAfter // If nil, time.After is selected
}

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

	retryInterval time.Duration // Accessed only by the go-routine.
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

	chain := &Chain{
		stopChan:                 make(chan (struct{})),
		doneChan:                 make(chan (struct{})),
		ledgerResources:          ledgerResources,
		clusterConsenter:         clusterConsenter,
		joinBlock:                joinBlock,
		firstHeight:              ledgerResources.Height(),
		options:                  options,
		logger:                   options.Logger.With("channel", ledgerResources.ChannelID()),
		timeAfter:                time.After,
		createBlockPullerFunc:    createBlockPullerFunc,
		chainCreationCallback:    chainCreationCallback,
		followerCreationCallback: followerCreationCallback,
		cryptoProvider:           cryptoProvider,
	}

	if chain.options.PullRetryMinInterval <= 0 {
		chain.options.PullRetryMinInterval = defaultPullRetryMinInterval
	}
	if chain.options.PullRetryMaxInterval <= 0 {
		chain.options.PullRetryMaxInterval = defaultPullRetryMaxInterval
	}
	if chain.options.PullRetryMaxInterval > math.MaxInt64/2 {
		chain.options.PullRetryMaxInterval = math.MaxInt64 / 2
	}
	if chain.options.PullRetryMinInterval > chain.options.PullRetryMaxInterval {
		chain.options.PullRetryMaxInterval = chain.options.PullRetryMinInterval
	}
	chain.retryInterval = chain.options.PullRetryMinInterval

	if chain.options.TimeAfter != nil {
		chain.timeAfter = chain.options.TimeAfter
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
	//TODO
	return errors.New("not implemented yet: pull after join block")
}
