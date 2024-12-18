/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package smartbft

import (
	"sort"
	"sync"
	"time"

	"github.com/hyperledger-labs/SmartBFT/pkg/types"
	"github.com/hyperledger/fabric-lib-go/bccsp"
	"github.com/hyperledger/fabric-lib-go/common/flogging"
	"github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/hyperledger/fabric/common/deliverclient"
	"github.com/hyperledger/fabric/common/deliverclient/blocksprovider"
	"github.com/hyperledger/fabric/common/deliverclient/orderers"
	"github.com/hyperledger/fabric/orderer/common/cluster"
	"github.com/hyperledger/fabric/orderer/common/localconfig"
	"github.com/hyperledger/fabric/orderer/consensus"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
)

type BFTSynchronizer struct {
	lastReconfig        types.Reconfig
	selfID              uint64
	LatestConfig        func() (types.Configuration, []uint64)
	BlockToDecision     func(*common.Block) *types.Decision
	OnCommit            func(*common.Block) types.Reconfig
	Support             consensus.ConsenterSupport
	CryptoProvider      bccsp.BCCSP
	ClusterDialer       *cluster.PredicateDialer
	LocalConfigCluster  localconfig.Cluster
	BlockPullerFactory  BlockPullerFactory
	VerifierFactory     VerifierFactory
	BFTDelivererFactory BFTDelivererFactory
	Logger              *flogging.FabricLogger

	mutex    sync.Mutex
	syncBuff *SyncBuffer
}

func (s *BFTSynchronizer) Sync() types.SyncResponse {
	s.Logger.Debug("BFT Sync initiated")
	decision, err := s.synchronize()
	if err != nil {
		s.Logger.Warnf("Could not synchronize with remote orderers due to %s, returning state from local ledger", err)
		block := s.Support.Block(s.Support.Height() - 1)
		config, nodes := s.LatestConfig()
		return types.SyncResponse{
			Latest: *s.BlockToDecision(block),
			Reconfig: types.ReconfigSync{
				InReplicatedDecisions: false, // If we read from ledger we do not need to reconfigure.
				CurrentNodes:          nodes,
				CurrentConfig:         config,
			},
		}
	}

	// After sync has ended, reset the state of the last reconfig.
	defer func() {
		s.lastReconfig = types.Reconfig{}
	}()

	s.Logger.Debugf("reconfig: %+v", s.lastReconfig)
	return types.SyncResponse{
		Latest: *decision,
		Reconfig: types.ReconfigSync{
			InReplicatedDecisions: s.lastReconfig.InLatestDecision,
			CurrentConfig:         s.lastReconfig.CurrentConfig,
			CurrentNodes:          s.lastReconfig.CurrentNodes,
		},
	}
}

// Buffer return the internal SyncBuffer for testability.
func (s *BFTSynchronizer) Buffer() *SyncBuffer {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	return s.syncBuff
}

func (s *BFTSynchronizer) synchronize() (*types.Decision, error) {
	// === We probe all the endpoints and establish a target height, as well as detect the self endpoint.
	targetHeight, myEndpoint, err := s.detectTargetHeight()
	if err != nil {
		return nil, errors.Wrapf(err, "cannot get detect target height")
	}

	startHeight := s.Support.Height()
	if startHeight >= targetHeight {
		return nil, errors.Errorf("already at target height of %d", targetHeight)
	}

	// === Create a buffer to accept the blocks delivered from the BFTDeliverer.
	capacityBlocks := uint(s.LocalConfigCluster.ReplicationBufferSize) / uint(s.Support.SharedConfig().BatchSize().AbsoluteMaxBytes)
	if capacityBlocks < 100 {
		capacityBlocks = 100
	}
	s.mutex.Lock()
	s.syncBuff = NewSyncBuffer(capacityBlocks)
	s.mutex.Unlock()

	// === Create the BFT block deliverer and start a go-routine that fetches block and inserts them into the syncBuffer.
	bftDeliverer, err := s.createBFTDeliverer(startHeight, myEndpoint)
	if err != nil {
		return nil, errors.Wrapf(err, "cannot create BFT block deliverer")
	}

	go bftDeliverer.DeliverBlocks()
	defer bftDeliverer.Stop()

	// === Loop on sync-buffer and pull blocks, writing them to the ledger, returning the last block pulled.
	lastPulledBlock, err := s.getBlocksFromSyncBuffer(startHeight, targetHeight)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get any blocks from SyncBuffer")
	}

	decision := s.BlockToDecision(lastPulledBlock)
	s.Logger.Infof("Returning decision from block [%d], decision: %+v", lastPulledBlock.GetHeader().GetNumber(), decision)
	return decision, nil
}

// detectTargetHeight probes remote endpoints and detects what is the target height this node needs to reach. It also
// detects the self-endpoint.
//
// In BFT it is highly recommended that the channel/orderer-endpoints (for delivery & broadcast) map 1:1 to the
// channel/orderers/consenters (for cluster consensus), that is, every consenter should be represented by a
// delivery endpoint. This important for Sync to work properly.
func (s *BFTSynchronizer) detectTargetHeight() (uint64, string, error) {
	blockPuller, err := s.BlockPullerFactory.CreateBlockPuller(s.Support, s.ClusterDialer, s.LocalConfigCluster, s.CryptoProvider)
	if err != nil {
		return 0, "", errors.Wrap(err, "cannot get create BlockPuller")
	}
	defer blockPuller.Close()

	heightByEndpoint, myEndpoint, err := blockPuller.HeightsByEndpoints()
	if err != nil {
		return 0, "", errors.Wrap(err, "cannot get HeightsByEndpoints")
	}

	s.Logger.Infof("HeightsByEndpoints: %+v, my endpoint: %s", heightByEndpoint, myEndpoint)

	delete(heightByEndpoint, myEndpoint)
	var heights []uint64
	for _, value := range heightByEndpoint {
		heights = append(heights, value)
	}

	if len(heights) == 0 {
		return 0, "", errors.New("no cluster members to synchronize with")
	}

	targetHeight := s.computeTargetHeight(heights)
	s.Logger.Infof("Detected target height: %d", targetHeight)
	return targetHeight, myEndpoint, nil
}

// computeTargetHeight compute the target height to synchronize to.
//
// heights: a slice containing the heights of accessible peers, length must be >0.
// clusterSize: the cluster size, must be >0.
func (s *BFTSynchronizer) computeTargetHeight(heights []uint64) uint64 {
	sort.Slice(heights, func(i, j int) bool { return heights[i] > heights[j] }) // Descending
	clusterSize := len(s.Support.SharedConfig().Consenters())
	f := uint64(clusterSize-1) / 3 // The number of tolerated byzantine faults
	lenH := uint64(len(heights))

	s.Logger.Debugf("Cluster size: %d, F: %d, Heights: %v", clusterSize, f, heights)

	if lenH < f+1 {
		s.Logger.Debugf("Returning %d", heights[0])
		return heights[int(lenH)-1]
	}
	s.Logger.Debugf("Returning %d", heights[f])
	return heights[f]
}

// createBFTDeliverer creates and initializes the BFT block deliverer.
func (s *BFTSynchronizer) createBFTDeliverer(startHeight uint64, myEndpoint string) (BFTBlockDeliverer, error) {
	lastBlock := s.Support.Block(startHeight - 1)
	lastConfigBlock, err := cluster.LastConfigBlock(lastBlock, s.Support)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to retrieve last config block")
	}
	lastConfigEnv, err := deliverclient.ConfigFromBlock(lastConfigBlock)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to retrieve last config envelope")
	}

	var updatableVerifier deliverclient.CloneableUpdatableBlockVerifier
	updatableVerifier, err = s.VerifierFactory.CreateBlockVerifier(lastConfigBlock, lastBlock, s.CryptoProvider, s.Logger)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create BlockVerificationAssistant")
	}

	clientConfig := s.ClusterDialer.Config // The cluster and block puller use slightly different options
	clientConfig.AsyncConnect = false
	clientConfig.SecOpts.VerifyCertificate = nil

	// The maximal amount of time to wait before retrying to connect.
	maxRetryInterval := s.LocalConfigCluster.ReplicationRetryTimeout
	// The minimal amount of time to wait before retrying. The retry interval doubles after every unsuccessful attempt.
	minRetryInterval := maxRetryInterval / 50
	// The maximal duration of a Sync. After this time Sync returns with whatever it had pulled until that point.
	maxRetryDuration := s.LocalConfigCluster.ReplicationPullTimeout * time.Duration(s.LocalConfigCluster.ReplicationMaxRetries)
	// If a remote orderer does not deliver blocks for this amount of time, even though it can do so, it is replaced as the block deliverer.
	blockCesorshipTimeOut := maxRetryDuration / 3

	bftDeliverer := s.BFTDelivererFactory.CreateBFTDeliverer(
		s.Support.ChannelID(),
		s.syncBuff,
		&ledgerInfoAdapter{s.Support},
		updatableVerifier,
		blocksprovider.DialerAdapter{ClientConfig: clientConfig},
		&orderers.ConnectionSourceFactory{}, // no overrides in the orderer
		s.CryptoProvider,
		make(chan struct{}),
		s.Support,
		blocksprovider.DeliverAdapter{},
		&blocksprovider.BFTCensorshipMonitorFactory{},
		flogging.MustGetLogger("orderer.blocksprovider").With("channel", s.Support.ChannelID()),
		minRetryInterval,
		maxRetryInterval,
		blockCesorshipTimeOut,
		maxRetryDuration,
		func() (stopRetries bool) {
			s.syncBuff.Stop()
			return true // In the orderer we must limit the time we try to do Synch()
		},
	)

	s.Logger.Infof("Created a BFTDeliverer: %+v", bftDeliverer)
	bftDeliverer.Initialize(lastConfigEnv.GetConfig(), myEndpoint)

	return bftDeliverer, nil
}

func (s *BFTSynchronizer) getBlocksFromSyncBuffer(startHeight, targetHeight uint64) (*common.Block, error) {
	targetSeq := targetHeight - 1
	seq := startHeight
	var blocksFetched int
	s.Logger.Debugf("Will fetch sequences [%d-%d]", seq, targetSeq)

	var lastPulledBlock *common.Block
	for seq <= targetSeq {
		block := s.syncBuff.PullBlock(seq)
		if block == nil {
			s.Logger.Debugf("Failed to fetch block [%d] from cluster", seq)
			break
		}
		if protoutil.IsConfigBlock(block) {
			s.Support.WriteConfigBlock(block, nil)
			s.Logger.Debugf("Fetched and committed config block [%d] from cluster", seq)
		} else {
			s.Support.WriteBlockSync(block, nil)
			s.Logger.Debugf("Fetched and committed block [%d] from cluster", seq)
		}
		lastPulledBlock = block

		prevInLatestDecision := s.lastReconfig.InLatestDecision
		s.lastReconfig = s.OnCommit(lastPulledBlock)
		s.lastReconfig.InLatestDecision = s.lastReconfig.InLatestDecision || prevInLatestDecision
		s.Logger.Debugf("Last reconfig %+v", s.lastReconfig)
		seq++
		blocksFetched++
	}

	s.syncBuff.Stop()

	if lastPulledBlock == nil {
		return nil, errors.Errorf("failed pulling block %d", seq)
	}

	s.Logger.Infof("Finished synchronizing with cluster, fetched %d blocks, starting from block [%d], up until and including block [%d]",
		blocksFetched, startHeight, lastPulledBlock.Header.Number)

	return lastPulledBlock, nil
}
