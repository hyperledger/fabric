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

// ErrChainStopped is returned when the chain is stopped during execution.
var ErrChainStopped = errors.New("chain stopped")

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
	mutex    sync.Mutex    // Protects the start/stop flags & channels, cRel & status. All the rest are immutable or accessed only by the go-routine.
	started  bool          // Start once.
	stopped  bool          // Stop once.
	stopChan chan struct{} // A 'closer' signals the go-routine to stop by closing this channel.
	doneChan chan struct{} // The go-routine signals the 'closer' that it is done by closing this channel.
	cRel     types.ClusterRelation
	status   types.Status

	ledgerResources  LedgerResources            // ledger & config resources
	clusterConsenter consensus.ClusterConsenter // detects whether a block indicates channel membership
	options          Options
	logger           *flogging.FabricLogger
	timeAfter        TimeAfter // time.After by default, or an alternative from Options.

	joinBlock   *common.Block // The join-block the follower was started with.
	lastConfig  *common.Block // The last config block from the ledger. Accessed only by the go-routine.
	firstHeight uint64        // The first ledger height

	createBlockPullerFunc CreateBlockPuller // A function that creates a block puller on demand // TODO change to interface
	chainCreationCallback CreateChain       // A method that creates a new consensus.Chain for this channel, to replace the current follower.Chain // TODO change to interface

	cryptoProvider bccsp.BCCSP   // Cryptographic services
	blockPuller    ChannelPuller // A block puller instance that is recreated every time a config is pulled.
}

// NewChain constructs a follower.Chain object.
func NewChain(
	ledgerResources LedgerResources,
	clusterConsenter consensus.ClusterConsenter,
	joinBlock *common.Block,
	options Options,
	createBlockPullerFunc CreateBlockPuller,
	chainCreationCallback CreateChain,
	cryptoProvider bccsp.BCCSP,
) (*Chain, error) {
	// Check the block puller creation function once before we start the follower.
	puller, errBP := createBlockPullerFunc()
	if errBP != nil {
		return nil, errors.Wrap(errBP, "error creating a block puller from join-block")
	}
	defer puller.Close()

	options.applyDefaults()

	chain := &Chain{
		stopChan:              make(chan (struct{})),
		doneChan:              make(chan (struct{})),
		cRel:                  types.ClusterRelationFollower,
		status:                types.StatusOnBoarding,
		ledgerResources:       ledgerResources,
		clusterConsenter:      clusterConsenter,
		joinBlock:             joinBlock,
		firstHeight:           ledgerResources.Height(),
		options:               options,
		logger:                options.Logger.With("channel", ledgerResources.ChannelID()),
		timeAfter:             options.TimeAfter,
		createBlockPullerFunc: createBlockPullerFunc,
		chainCreationCallback: chainCreationCallback,
		cryptoProvider:        cryptoProvider,
	}

	if joinBlock == nil {
		chain.status = types.StatusActive
		if err := chain.loadLastConfig(); err != nil {
			return nil, err
		}
		if isMem, _ := chain.clusterConsenter.IsChannelMember(chain.lastConfig); isMem {
			chain.cRel = types.ClusterRelationMember
		}
		chain.logger.Infof("Created with a nil join-block, ledger height: %d", chain.firstHeight)
	} else {
		if joinBlock.Header == nil {
			return nil, errors.New("block header is nil")
		}
		if joinBlock.Data == nil {
			return nil, errors.New("block data is nil")
		}

		if chain.joinBlock.Header.Number < chain.ledgerResources.Height() {
			chain.status = types.StatusActive
		}
		if isMem, _ := chain.clusterConsenter.IsChannelMember(chain.joinBlock); isMem {
			chain.cRel = types.ClusterRelationMember
		}

		chain.logger.Infof("Created with join-block number: %d, ledger height: %d", joinBlock.Header.Number, chain.firstHeight)
	}
	chain.logger.Debugf("Options are: %v", chain.options)

	if chain.chainCreationCallback == nil {
		chain.chainCreationCallback = func() {
			chain.logger.Warnf("No-Op: chain creation callback is nil")
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

// Halt signals the Chain to stop and waits for the internal go-routine to exit.
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

	return c.cRel, c.status
}

func (c *Chain) setStatusReport(clusterRelation types.ClusterRelation, status types.Status) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.cRel = clusterRelation
	c.status = status
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

	if err := c.pull(); err != nil {
		c.logger.Warnf("Pull failed, error: %s", err)
		// TODO set the status to StatusError
	}
}

func (c *Chain) increaseRetryInterval(retryInterval *time.Duration, upperLimit time.Duration) {
	if *retryInterval == upperLimit {
		return
	}
	//assuming this will never overflow int64, as upperLimit cannot be over MaxInt64/2
	*retryInterval = time.Duration(1.5 * float64(*retryInterval))
	if *retryInterval > upperLimit {
		*retryInterval = upperLimit
	}
	c.logger.Debugf("retry interval increased to: %v", *retryInterval)
}

func (c *Chain) resetRetryInterval(retryInterval *time.Duration, lowerLimit time.Duration) {
	if *retryInterval == lowerLimit {
		return
	}
	*retryInterval = lowerLimit
	c.logger.Debugf("retry interval reset to: %v", *retryInterval)
}

func (c *Chain) decreaseRetryInterval(retryInterval *time.Duration, lowerLimit time.Duration) {
	if *retryInterval == lowerLimit {
		return
	}

	*retryInterval = *retryInterval - lowerLimit
	if *retryInterval < lowerLimit {
		*retryInterval = lowerLimit
	}
	c.logger.Debugf("retry interval decreased to: %v", *retryInterval)
}

// pull blocks from other orderers until a config block indicates the orderer has become a member of the cluster.
// When the follower.Chain's job is done, this method halts, triggers the creation of a new consensus.Chain,
// and returns nil. The method returns an error only when the chain is stopped or due to unrecoverable errors.
func (c *Chain) pull() error {
	var err error
	if c.joinBlock != nil {
		err = c.pullUpToJoin()
		if err != nil {
			return errors.Wrap(err, "failed to pull up to join block")
		}

		// TODO remove the join-block from the file repo

		// The join block never returns an error. This is checked before the follower is started.
		if isMem, _ := c.clusterConsenter.IsChannelMember(c.joinBlock); isMem {
			c.setStatusReport(types.ClusterRelationMember, types.StatusActive)
			// Trigger creation of a new consensus.Chain.
			c.logger.Info("On-boarding finished successfully, invoking chain creation callback")
			c.halt()
			c.chainCreationCallback()
			return nil
		}

		c.setStatusReport(types.ClusterRelationFollower, types.StatusActive)
		c.joinBlock = nil
		err = c.loadLastConfig()
		if err != nil {
			return errors.Wrap(err, "failed to load last config block")
		}

		c.logger.Info("On-boarding finished successfully, continuing to pull blocks after join-block")
	}

	err = c.pullAfterJoin()
	if err != nil {
		return errors.Wrap(err, "failed to pull after join block")
	}

	// Trigger creation of a new consensus.Chain.
	c.logger.Info("Block pulling finished successfully, invoking chain creation callback")
	c.halt()
	c.chainCreationCallback()

	return nil
}

// pullUpToJoin pulls blocks up to the join-block height without inspecting membership on fetched config blocks.
// It checks whether the chain was stopped between blocks.
func (c *Chain) pullUpToJoin() error {
	targetHeight := c.joinBlock.Header.Number + 1
	if c.ledgerResources.Height() >= targetHeight {
		c.logger.Infof("Target height according to join block (%d) is <= to our ledger height (%d), no need to pull up to join block", targetHeight, c.ledgerResources.Height())
		return nil
	}

	var err error
	c.blockPuller, err = c.createBlockPullerFunc() // TODO replace with a factory that has a join block as input
	if err != nil {                                //This should never happen since we check the join-block before we start.
		return errors.Wrapf(err, "error creating block puller")
	}
	defer c.blockPuller.Close()

	err = c.pullUntilLatestWithRetry(targetHeight, false)
	if err != nil {
		return err
	}

	c.logger.Infof("Pulled blocks from %d until %d", c.firstHeight, targetHeight-1)
	return nil
}

// pullAfterJoin pulls blocks continuously, inspecting the fetched config blocks for membership.
// On every config block, it renews the BlockPuller, to take in the new configuration.
// It will exit with 'nil' if it detects a config block that indicates the orderer is a member of the cluster.
// It checks whether the chain was stopped between blocks.
func (c *Chain) pullAfterJoin() error {
	var err error

	c.blockPuller, err = c.createBlockPullerFunc() // TODO replace with a factory that has last config block as input
	if err != nil {
		return errors.Wrap(err, "error creating block puller")
	}
	defer c.blockPuller.Close()

	heightPollInterval := c.options.HeightPollMinInterval
	for {
		// Check membership
		isMember, errMem := c.clusterConsenter.IsChannelMember(c.lastConfig)
		if errMem != nil {
			return errors.Wrap(err, "failed to determine channel membership from last config")
		}
		if isMember {
			c.setStatusReport(types.ClusterRelationMember, types.StatusActive)
			return nil
		}

		// Poll for latest network height to advance beyond ledger height.
		var latestNetworkHeight uint64
	heightPollLoop:
		for {
			endpoint, networkHeight, errHeight := cluster.LatestHeightAndEndpoint(c.blockPuller)
			if errHeight != nil {
				c.logger.Errorf("Failed to get latest height and endpoint, error: %s", errHeight)
			} else {
				c.logger.Debugf("Orderer endpoint %s has the biggest ledger height: %d", endpoint, networkHeight)
			}

			if networkHeight > c.ledgerResources.Height() {
				// On success, slowly decrease the polling interval
				c.decreaseRetryInterval(&heightPollInterval, c.options.HeightPollMinInterval)
				latestNetworkHeight = networkHeight
				break heightPollLoop
			}

			c.logger.Debugf("My height: %d, latest network height: %d; going to wait %v for latest height to grow",
				c.ledgerResources.Height(), networkHeight, heightPollInterval)
			select {
			case <-c.stopChan:
				c.logger.Debug("Received a stop signal")
				return ErrChainStopped
			case <-c.timeAfter(heightPollInterval):
				// Exponential back-off, to avoid calling LatestHeightAndEndpoint too often.
				c.increaseRetryInterval(&heightPollInterval, c.options.HeightPollMaxInterval)
			}
		}

		// Pull to latest height or chain stop signal
		err = c.pullUntilLatestWithRetry(latestNetworkHeight, true)
		if err != nil {
			return err
		}
	}
}

// pullUntilLatestWithRetry is given a target-height and exits without an error when it reaches that target.
// It return with an error only if the chain is stopped.
// On internal pull errors it employs exponential back-off and retries.
func (c *Chain) pullUntilLatestWithRetry(latestNetworkHeight uint64, inspectConfig bool) error {
	//Pull until latest
	retryInterval := c.options.PullRetryMinInterval
	for {
		numPulled, errPull := c.pullUntilTarget(latestNetworkHeight, inspectConfig)
		if numPulled > 0 {
			c.resetRetryInterval(&retryInterval, c.options.PullRetryMinInterval) // On any progress, reset retry interval.
		}
		if errPull == nil {
			c.logger.Debugf("Pulled %d blocks until latest network height: %d", numPulled, latestNetworkHeight)
			break
		}

		c.logger.Debugf("Error while trying to pull to latest height: %d; going to try again in %v",
			latestNetworkHeight, retryInterval)
		select {
		case <-c.stopChan:
			c.logger.Debug("Received a stop signal")
			return ErrChainStopped
		case <-c.timeAfter(retryInterval):
			// Exponential back-off on successive errors w/o progress.
			c.increaseRetryInterval(&retryInterval, c.options.PullRetryMaxInterval)
		}
	}

	return nil
}

// pullUntilTarget is given a target-height and exits without an error when it reaches that target.
// It may return with an error before the target, always returning the number of blocks pulled.
func (c *Chain) pullUntilTarget(targetHeight uint64, inspectConfig bool) (uint64, error) {
	firstBlockToPull := c.ledgerResources.Height()
	if firstBlockToPull >= targetHeight {
		c.logger.Debugf("Target height (%d) is <= to our ledger height (%d), skipping pulling", targetHeight, firstBlockToPull)
		return 0, nil
	}

	var actualPrevHash []byte
	// Initialize the actual previous hash
	if firstBlockToPull > 0 {
		prevBlock := c.ledgerResources.Block(firstBlockToPull - 1)
		if prevBlock == nil {
			return 0, errors.Errorf("cannot retrieve previous block %d", firstBlockToPull-1)
		}
		actualPrevHash = protoutil.BlockHeaderHash(prevBlock.Header)
	}

	// Pull until the latest height
	for seq := firstBlockToPull; seq < targetHeight; seq++ {
		n := seq - firstBlockToPull
		select {
		case <-c.stopChan:
			c.logger.Debug("Received a stop signal")
			return n, ErrChainStopped
		default:
			nextBlock := c.blockPuller.PullBlock(seq)
			if nextBlock == nil {
				return n, errors.Wrapf(cluster.ErrRetryCountExhausted, "failed to pull block %d", seq)
			}
			reportedPrevHash := nextBlock.Header.PreviousHash
			if (nextBlock.Header.Number > 0) && !bytes.Equal(reportedPrevHash, actualPrevHash) {
				return n, errors.Errorf("block header mismatch on sequence %d, expected %x, got %x",
					nextBlock.Header.Number, actualPrevHash, reportedPrevHash)
			}
			actualPrevHash = protoutil.BlockHeaderHash(nextBlock.Header)
			if err := c.ledgerResources.Append(nextBlock); err != nil {
				return n, errors.Wrapf(err, "failed to append block %d to the ledger", nextBlock.Header.Number)
			}

			if inspectConfig && protoutil.IsConfigBlock(nextBlock) {
				c.logger.Debugf("Pulled blocks from %d to %d, last block is config", firstBlockToPull, nextBlock.Header.Number)
				c.lastConfig = nextBlock
				// Re-create the block puller to apply the new config, which may update the endpoints.
				c.blockPuller.Close()
				var err error
				if c.blockPuller, err = c.createBlockPullerFunc(); err != nil { // TODO replace with a factory that has a config block as input
					return n, errors.Wrapf(err, "failed to re-create block puller from last config,  block number: %d", nextBlock.Header.Number)
				}
			}
		}
	}
	c.logger.Debugf("Pulled blocks from %d to %d", firstBlockToPull, targetHeight)
	return targetHeight - firstBlockToPull, nil
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
