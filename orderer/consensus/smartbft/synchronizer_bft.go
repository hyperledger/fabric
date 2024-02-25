/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package smartbft

import (
	"sort"
	"time"

	"github.com/SmartBFT-Go/consensus/pkg/types"
	"github.com/SmartBFT-Go/consensus/smartbftprotos"
	"github.com/hyperledger/fabric-lib-go/bccsp"
	"github.com/hyperledger/fabric-lib-go/common/flogging"
	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric/common/deliverclient"
	"github.com/hyperledger/fabric/internal/pkg/peer/blocksprovider"
	"github.com/hyperledger/fabric/internal/pkg/peer/orderers"
	"github.com/hyperledger/fabric/orderer/common/cluster"
	"github.com/hyperledger/fabric/orderer/common/localconfig"
	"github.com/hyperledger/fabric/orderer/consensus"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
)

type BFTSynchronizer struct {
	lastReconfig       types.Reconfig
	selfID             uint64
	LatestConfig       func() (types.Configuration, []uint64)
	BlockToDecision    func(*common.Block) *types.Decision
	OnCommit           func(*common.Block) types.Reconfig
	Support            consensus.ConsenterSupport
	CryptoProvider     bccsp.BCCSP
	BlockPuller        BlockPuller              // TODO improve - this only an endpoint prober - detect self EP
	clusterDialer      *cluster.PredicateDialer // TODO make bft-synchro
	localConfigCluster localconfig.Cluster      // TODO make bft-synchro
	Logger             *flogging.FabricLogger
}

func (s *BFTSynchronizer) Sync() types.SyncResponse {
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
	return types.SyncResponse{
		Latest: *decision,
		Reconfig: types.ReconfigSync{
			InReplicatedDecisions: s.lastReconfig.InLatestDecision,
			CurrentConfig:         s.lastReconfig.CurrentConfig,
			CurrentNodes:          s.lastReconfig.CurrentNodes,
		},
	}
}

func (s *BFTSynchronizer) synchronize() (*types.Decision, error) {
	//=== We use the BlockPuller to probe all the endpoints and establish a target height, as well as detect
	// the self endpoint.

	// In BFT it is highly recommended that the channel/orderer-endpoints (for delivery & broadcast) map 1:1 to the
	// channel/orderers/consenters (for cluster consensus), that is, every consenter should be represented by a
	// delivery endpoint.
	blockPuller, err := newBlockPuller(s.Support, s.clusterDialer, s.localConfigCluster, s.CryptoProvider)
	if err != nil {
		return nil, errors.Wrap(err, "cannot get create BlockPuller")
	}
	defer blockPuller.Close()

	heightByEndpoint, myEndpoint, err := blockPuller.HeightsByEndpoints()
	if err != nil {
		return nil, errors.Wrap(err, "cannot get HeightsByEndpoints")
	}

	s.Logger.Infof("HeightsByEndpoints: %+v, my endpoint: %s", heightByEndpoint, myEndpoint)

	var heights []uint64
	for ep, value := range heightByEndpoint {
		if ep == myEndpoint {
			continue
		}
		heights = append(heights, value)
	}

	if len(heights) == 0 {
		return nil, errors.New("no cluster members to synchronize with")
	}

	targetHeight := s.computeTargetHeight(heights)
	startHeight := s.Support.Height()
	if startHeight >= targetHeight {
		return nil, errors.Errorf("already at target height of %d", targetHeight)
	}

	//====
	// create a buffer to accept the blocks delivered from the BFTDeliverer
	syncBuffer := NewSyncBuffer()

	//===
	// create the deliverer
	lastBlock := s.Support.Block(startHeight - 1)
	lastConfigBlock, err := cluster.LastConfigBlock(lastBlock, s.Support)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to retrieve last config block")
	}
	lastConfigEnv, err := deliverclient.ConfigFromBlock(lastConfigBlock)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to retrieve last config envelope")
	}
	verifier, err := deliverclient.NewBlockVerificationAssistant(lastConfigBlock, lastBlock, s.CryptoProvider, s.Logger)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create BlockVerificationAssistant")
	}

	clientConfig := s.clusterDialer.Config // The cluster and block puller use slightly different options
	clientConfig.AsyncConnect = false
	clientConfig.SecOpts.VerifyCertificate = nil

	bftDeliverer := &blocksprovider.BFTDeliverer{
		ChannelID:                 s.Support.ChannelID(),
		BlockHandler:              syncBuffer,
		Ledger:                    &ledgerInfoAdapter{s.Support},
		UpdatableBlockVerifier:    verifier,
		Dialer:                    blocksprovider.DialerAdapter{ClientConfig: clientConfig},
		OrderersSourceFactory:     &orderers.ConnectionSourceFactory{}, // no overrides in the orderer
		CryptoProvider:            s.CryptoProvider,
		DoneC:                     make(chan struct{}),
		Signer:                    s.Support,
		DeliverStreamer:           blocksprovider.DeliverAdapter{},
		CensorshipDetectorFactory: &blocksprovider.BFTCensorshipMonitorFactory{},
		Logger:                    flogging.MustGetLogger("orderer.blocksprovider").With("channel", s.Support.ChannelID()),
		InitialRetryInterval:      10 * time.Millisecond, // TODO get it from config.
		MaxRetryInterval:          2 * time.Second,       // TODO get it from config.
		BlockCensorshipTimeout:    20 * time.Second,      // TODO get it from config.
		MaxRetryDuration:          time.Minute,           // TODO get it from config.
		MaxRetryDurationExceededHandler: func() (stopRetries bool) {
			syncBuffer.Stop()
			return true // In the orderer we must limit the time we try to do Synch()
		},
	}

	s.Logger.Infof("Created a BFTDeliverer: %+v", bftDeliverer)
	bftDeliverer.Initialize(lastConfigEnv.GetConfig(), myEndpoint)

	go bftDeliverer.DeliverBlocks()
	defer bftDeliverer.Stop()

	//===
	// Loop on sync-buffer

	targetSeq := targetHeight - 1
	seq := startHeight
	var blocksFetched int

	s.Logger.Debugf("Will fetch sequences [%d-%d]", seq, targetSeq)

	var lastPulledBlock *common.Block
	for seq <= targetSeq {
		block := syncBuffer.PullBlock(seq)
		if block == nil {
			s.Logger.Debugf("Failed to fetch block [%d] from cluster", seq)
			break
		}
		if protoutil.IsConfigBlock(block) {
			s.Support.WriteConfigBlock(block, nil)
		} else {
			s.Support.WriteBlock(block, nil)
		}
		s.Logger.Debugf("Fetched and committed block [%d] from cluster", seq)
		lastPulledBlock = block

		prevInLatestDecision := s.lastReconfig.InLatestDecision
		s.lastReconfig = s.OnCommit(lastPulledBlock)
		s.lastReconfig.InLatestDecision = s.lastReconfig.InLatestDecision || prevInLatestDecision
		seq++
		blocksFetched++
	}

	syncBuffer.Stop()

	if lastPulledBlock == nil {
		return nil, errors.Errorf("failed pulling block %d", seq)
	}

	startSeq := startHeight
	s.Logger.Infof("Finished synchronizing with cluster, fetched %d blocks, starting from block [%d], up until and including block [%d]",
		blocksFetched, startSeq, lastPulledBlock.Header.Number)

	viewMetadata, lastConfigSqn := s.getViewMetadataLastConfigSqnFromBlock(lastPulledBlock)

	s.Logger.Infof("Returning view metadata of %v, lastConfigSeq %d", viewMetadata, lastConfigSqn)
	return s.BlockToDecision(lastPulledBlock), nil
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

	s.Logger.Debugf("Heights: %v", heights)

	if lenH < f+1 {
		s.Logger.Debugf("Returning %d", heights[0])
		return heights[int(lenH)-1]
	}
	s.Logger.Debugf("Returning %d", heights[f])
	return heights[f]
}

func (s *BFTSynchronizer) getViewMetadataLastConfigSqnFromBlock(block *common.Block) (*smartbftprotos.ViewMetadata, uint64) {
	viewMetadata, err := getViewMetadataFromBlock(block)
	if err != nil {
		return nil, 0
	}

	lastConfigSqn := s.Support.Sequence()

	return viewMetadata, lastConfigSqn
}
