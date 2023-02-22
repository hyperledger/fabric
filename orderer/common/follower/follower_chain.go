/*
Copyright IBM Corp. 2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package follower

import (
	"bytes"
	"sync"
	"time"

	"github.com/golang/protobuf/proto"

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

// LedgerResources defines some of the interfaces of ledger & config resources needed by the follower.Chain.
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

//go:generate counterfeiter -o mocks/block_puller_factory.go -fake-name BlockPullerFactory . BlockPullerFactory

// BlockPullerFactory creates a ChannelPuller on demand, and exposes a method to update the a block signature verifier
// linked to that ChannelPuller.
type BlockPullerFactory interface {
	BlockPuller(configBlock *common.Block, stopChannel chan struct{}) (ChannelPuller, error)
	UpdateVerifierFromConfigBlock(configBlock *common.Block) error
}

//go:generate counterfeiter -o mocks/chain_creator.go -fake-name ChainCreator . ChainCreator

// ChainCreator defines a function that creates a new consensus.Chain for this channel, to replace the current
// follower.Chain. This interface is meant to be implemented by the multichannel.Registrar.
type ChainCreator interface {
	SwitchFollowerToChain(chainName string)
}

//go:generate counterfeiter -o mocks/channel_participation_metrics_reporter.go -fake-name ChannelParticipationMetricsReporter . ChannelParticipationMetricsReporter

type ChannelParticipationMetricsReporter interface {
	ReportConsensusRelationAndStatusMetrics(channelID string, relation types.ConsensusRelation, status types.Status)
}

// Chain implements a component that allows the orderer to follow a specific channel when is not a cluster member,
// that is, be a "follower" of the cluster. It also allows the orderer to perform "onboarding" for
// channels it is joining as a member, with a join-block.
//
// When an orderer is following a channel, it means that the current orderer is not a member of the consenters set
// of the channel, and is only pulling blocks from other orderers. In this mode, the follower is inspecting config
// blocks as they are pulled and if it discovers that it was introduced into the consenters set, it will trigger the
// creation of a regular etcdraft.Chain, that is, turn into a "member" of the cluster.
//
// A follower is also used to onboard a channel when joining as a member with a join-block that has number >0. In this
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
// i.e. the follower is performing onboarding for an etcdraft.Chain. Otherwise, the follower return clusterRelation
// "follower".
type Chain struct {
	mutex             sync.Mutex    // Protects the start/stop flags & channels, consensusRelation & status. All the rest are immutable or accessed only by the go-routine.
	started           bool          // Start once.
	stopped           bool          // Stop once.
	stopChan          chan struct{} // A 'closer' signals the go-routine to stop by closing this channel.
	doneChan          chan struct{} // The go-routine signals the 'closer' that it is done by closing this channel.
	consensusRelation types.ConsensusRelation
	status            types.Status

	ledgerResources  LedgerResources            // ledger & config resources
	clusterConsenter consensus.ClusterConsenter // detects whether a block indicates channel membership
	options          Options
	logger           *flogging.FabricLogger
	timeAfter        TimeAfter // time.After by default, or an alternative from Options.

	joinBlock   *common.Block // The join-block the follower was started with.
	lastConfig  *common.Block // The last config block from the ledger. Accessed only by the go-routine.
	firstHeight uint64        // The first ledger height

	// Creates a block puller on demand, and allows the update of the block signature verifier with each incoming
	// config block.
	blockPullerFactory BlockPullerFactory
	// A block puller instance, created either from the join-block or last-config-block. When pulling blocks using
	// the last-config-block, the endpoints are updated with each incoming config block.
	blockPuller ChannelPuller

	// Creates a new consensus.Chain for this channel, to replace the current follower.Chain.
	chainCreator ChainCreator

	cryptoProvider bccsp.BCCSP // Cryptographic services

	channelParticipationMetricsReporter ChannelParticipationMetricsReporter
}

// NewChain constructs a follower.Chain object.
func NewChain(
	ledgerResources LedgerResources,
	clusterConsenter consensus.ClusterConsenter,
	joinBlock *common.Block,
	options Options,
	blockPullerFactory BlockPullerFactory,
	chainCreator ChainCreator,
	cryptoProvider bccsp.BCCSP,
	channelParticipationMetricsReporter ChannelParticipationMetricsReporter,
) (*Chain, error) {
	options.applyDefaults()

	chain := &Chain{
		stopChan:                            make(chan struct{}),
		doneChan:                            make(chan struct{}),
		consensusRelation:                   types.ConsensusRelationFollower,
		status:                              types.StatusOnBoarding,
		ledgerResources:                     ledgerResources,
		clusterConsenter:                    clusterConsenter,
		joinBlock:                           joinBlock,
		firstHeight:                         ledgerResources.Height(),
		options:                             options,
		logger:                              options.Logger.With("channel", ledgerResources.ChannelID()),
		timeAfter:                           options.TimeAfter,
		blockPullerFactory:                  blockPullerFactory,
		chainCreator:                        chainCreator,
		cryptoProvider:                      cryptoProvider,
		channelParticipationMetricsReporter: channelParticipationMetricsReporter,
	}

	if ledgerResources.Height() > 0 {
		if err := chain.loadLastConfig(); err != nil {
			return nil, err
		}
		if err := blockPullerFactory.UpdateVerifierFromConfigBlock(chain.lastConfig); err != nil {
			return nil, err
		}
	}

	if joinBlock == nil {
		chain.status = types.StatusActive
		if isMem, _ := chain.clusterConsenter.IsChannelMember(chain.lastConfig); isMem {
			chain.consensusRelation = types.ConsensusRelationConsenter
		}

		chain.logger.Infof("Created with a nil join-block, ledger height: %d", chain.firstHeight)
	} else {
		if joinBlock.Header == nil {
			return nil, errors.New("block header is nil")
		}
		if joinBlock.Data == nil {
			return nil, errors.New("block data is nil")
		}

		// Check the block puller creation function once before we start the follower. This ensures we can extract
		// the endpoints from the join-block.
		puller, err := blockPullerFactory.BlockPuller(joinBlock, nil)
		if err != nil {
			return nil, errors.WithMessage(err, "error creating a block puller from join-block")
		}
		puller.Close()

		if chain.joinBlock.Header.Number < chain.ledgerResources.Height() {
			chain.status = types.StatusActive
		}
		if isMem, _ := chain.clusterConsenter.IsChannelMember(chain.joinBlock); isMem {
			chain.consensusRelation = types.ConsensusRelationConsenter
		}

		chain.logger.Infof("Created with join-block number: %d, ledger height: %d", joinBlock.Header.Number, chain.firstHeight)
	}

	chain.logger.Debugf("Options are: %v", chain.options)

	chain.channelParticipationMetricsReporter.ReportConsensusRelationAndStatusMetrics(ledgerResources.ChannelID(), chain.consensusRelation, chain.status)

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
	<-c.doneChan
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

// StatusReport returns the ConsensusRelation & Status.
func (c *Chain) StatusReport() (types.ConsensusRelation, types.Status) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	return c.consensusRelation, c.status
}

func (c *Chain) setStatus(status types.Status) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.status = status

	c.channelParticipationMetricsReporter.ReportConsensusRelationAndStatusMetrics(c.ledgerResources.ChannelID(), c.consensusRelation, c.status)
}

func (c *Chain) setConsensusRelation(clusterRelation types.ConsensusRelation) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.consensusRelation = clusterRelation

	c.channelParticipationMetricsReporter.ReportConsensusRelationAndStatusMetrics(c.ledgerResources.ChannelID(), c.consensusRelation, c.status)
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
		// TODO set the status to StatusError (see FAB-18106)
	}
}

func (c *Chain) increaseRetryInterval(retryInterval *time.Duration, upperLimit time.Duration) {
	if *retryInterval == upperLimit {
		return
	}
	// assuming this will never overflow int64, as upperLimit cannot be over MaxInt64/2
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
			return errors.WithMessage(err, "failed to pull up to join block")
		}
		c.logger.Info("Onboarding finished successfully, pulled blocks up to join-block")
	}

	if c.joinBlock != nil && !proto.Equal(c.ledgerResources.Block(c.joinBlock.Header.Number).Data, c.joinBlock.Data) {
		c.logger.Panicf("Join block (%d) we pulled mismatches block we joined with", c.joinBlock.Header.Number)
	}

	err = c.pullAfterJoin()
	if err != nil {
		return errors.WithMessage(err, "failed to pull after join block")
	}

	// Trigger creation of a new consensus.Chain.
	c.logger.Info("Block pulling finished successfully, going to switch from follower to a consensus.Chain")
	c.halt()
	c.chainCreator.SwitchFollowerToChain(c.ledgerResources.ChannelID())

	return nil
}

// pullUpToJoin pulls blocks up to the join-block height without inspecting membership on fetched config blocks.
// It checks whether the chain was stopped between blocks.
func (c *Chain) pullUpToJoin() error {
	targetHeight := c.joinBlock.Header.Number + 1
	if c.ledgerResources.Height() >= targetHeight {
		c.logger.Infof("Target height according to join block (%d) is <= to our ledger height (%d), no need to pull up to join block",
			targetHeight, c.ledgerResources.Height())
		return nil
	}

	var err error
	// Block puller created with endpoints from the join-block.
	c.blockPuller, err = c.blockPullerFactory.BlockPuller(c.joinBlock, c.stopChan)
	if err != nil { // This should never happen since we check the join-block before we start.
		return errors.WithMessagef(err, "error creating block puller")
	}
	defer c.blockPuller.Close()
	// Since we created the block-puller with the join-block, do not update the endpoints from the
	// config blocks that precede it.
	err = c.pullUntilLatestWithRetry(targetHeight, false)
	if err != nil {
		return err
	}

	c.logger.Infof("Pulled blocks from %d until %d", c.firstHeight, targetHeight-1)
	return nil
}

// pullAfterJoin pulls blocks continuously, inspecting the fetched config
// blocks for membership. On every config block, it renews the BlockPuller,
// to take in the new configuration. It will exit with 'nil' if it detects
// a config block that indicates the orderer is a member of the cluster. It
// checks whether the chain was stopped between blocks.
func (c *Chain) pullAfterJoin() error {
	c.logger.Infof("Pulling after join")
	defer c.logger.Infof("Pulled after join")
	c.setStatus(types.StatusActive)

	err := c.loadLastConfig()
	if err != nil {
		return errors.WithMessage(err, "failed to load last config block")
	}

	c.blockPuller, err = c.blockPullerFactory.BlockPuller(c.lastConfig, c.stopChan)
	if err != nil {
		return errors.WithMessage(err, "error creating block puller")
	}
	defer c.blockPuller.Close()

	heightPollInterval := c.options.HeightPollMinInterval
	for {
		// Check membership
		isMember, errMem := c.clusterConsenter.IsChannelMember(c.lastConfig)
		if errMem != nil {
			return errors.WithMessage(err, "failed to determine channel membership from last config")
		}
		if isMember {
			c.setConsensusRelation(types.ConsensusRelationConsenter)
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
				c.logger.Infof("Orderer endpoint %s has the biggest ledger height: %d", endpoint, networkHeight)
			}

			if networkHeight > c.ledgerResources.Height() {
				// On success, slowly decrease the polling interval
				c.decreaseRetryInterval(&heightPollInterval, c.options.HeightPollMinInterval)
				latestNetworkHeight = networkHeight
				break heightPollLoop
			}

			c.logger.Infof("My height: %d, latest network height: %d; going to wait %v for latest height to grow",
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
// When parameter updateEndpoints is true, the block-puller's endpoints are updated with every incoming config.
func (c *Chain) pullUntilLatestWithRetry(latestNetworkHeight uint64, updateEndpoints bool) error {
	retryInterval := c.options.PullRetryMinInterval
	for {
		numPulled, errPull := c.pullUntilTarget(latestNetworkHeight, updateEndpoints)
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
// When parameter updateEndpoints is true, the block-puller's endpoints are updated with every incoming config.
// The block-puller-factory which holds the block signature verifier is updated on every incoming config.
func (c *Chain) pullUntilTarget(targetHeight uint64, updateEndpoints bool) (uint64, error) {
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
				return n, errors.WithMessagef(cluster.ErrRetryCountExhausted, "failed to pull block %d", seq)
			}
			reportedPrevHash := nextBlock.Header.PreviousHash
			if (nextBlock.Header.Number > 0) && !bytes.Equal(reportedPrevHash, actualPrevHash) {
				return n, errors.Errorf("block header mismatch on sequence %d, expected %x, got %x",
					nextBlock.Header.Number, actualPrevHash, reportedPrevHash)
			}
			actualPrevHash = protoutil.BlockHeaderHash(nextBlock.Header)
			if err := c.ledgerResources.Append(nextBlock); err != nil {
				return n, errors.WithMessagef(err, "failed to append block %d to the ledger", nextBlock.Header.Number)
			}

			if protoutil.IsConfigBlock(nextBlock) {
				c.logger.Debugf("Pulled blocks from %d to %d, last block is config", firstBlockToPull, nextBlock.Header.Number)
				c.lastConfig = nextBlock
				if err := c.blockPullerFactory.UpdateVerifierFromConfigBlock(nextBlock); err != nil {
					return n, errors.WithMessagef(err, "failed to update verifier from last config,  block number: %d", nextBlock.Header.Number)
				}
				if updateEndpoints {
					endpoints, err := cluster.EndpointconfigFromConfigBlock(nextBlock, c.cryptoProvider)
					if err != nil {
						return n, errors.WithMessagef(err, "failed to extract endpoints from last config,  block number: %d", nextBlock.Header.Number)
					}
					c.blockPuller.UpdateEndpoints(endpoints)
				}
			}
		}
	}
	c.logger.Debugf("Pulled blocks from %d to %d", firstBlockToPull, targetHeight)
	return targetHeight - firstBlockToPull, nil
}

func (c *Chain) loadLastConfig() error {
	height := c.ledgerResources.Height()
	if height == 0 {
		return errors.New("ledger is empty")
	}
	lastBlock := c.ledgerResources.Block(height - 1)
	index, err := protoutil.GetLastConfigIndexFromBlock(lastBlock)
	if err != nil {
		return errors.WithMessage(err, "chain does have appropriately encoded last config in its latest block")
	}
	lastConfig := c.ledgerResources.Block(index)
	if lastConfig == nil {
		return errors.Errorf("could not retrieve config block from index %d", index)
	}
	c.lastConfig = lastConfig
	return nil
}
