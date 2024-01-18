/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package smartbft

import (
	"sort"
	"time"

	"github.com/SmartBFT-Go/consensus/pkg/types"
	"github.com/hyperledger/fabric-lib-go/common/flogging"
	cb "github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric/internal/pkg/peer/blocksprovider"
	"github.com/hyperledger/fabric/internal/pkg/peer/orderers"
	"github.com/hyperledger/fabric/orderer/consensus"
	"github.com/pkg/errors"
)

type BFTSynchronizer struct {
	lastReconfig    types.Reconfig
	selfID          uint64
	LatestConfig    func() (types.Configuration, []uint64)
	BlockToDecision func(*cb.Block) *types.Decision
	OnCommit        func(*cb.Block) types.Reconfig
	Support         consensus.ConsenterSupport
	BlockPuller     BlockPuller // TODO make bft-synchro - this only an endpoint prober
	ClusterSize     uint64      // TODO this can be taken from the channel/orderer config
	Logger          *flogging.FabricLogger
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
	defer s.BlockPuller.Close()

	//=== we use the BlockPuller to probe all the endpoints and establish a target height
	//TODO in BFT it is important that the channel/orderer-endpoints (for delivery & broadcast) map 1:1 to the
	// channel/orderers/consenters (for cluster consensus), that is, every consenter should be represented by a
	// delivery endpoint.
	heightByEndpoint, _, err := s.BlockPuller.HeightsByEndpoints()
	if err != nil {
		return nil, errors.Wrap(err, "cannot get HeightsByEndpoints")
	}

	s.Logger.Infof("HeightsByEndpoints: %v", heightByEndpoint)

	if len(heightByEndpoint) == 0 {
		return nil, errors.New("no cluster members to synchronize with")
	}

	var heights []uint64
	for _, value := range heightByEndpoint {
		heights = append(heights, value)
	}

	targetHeight := s.computeTargetHeight(heights)
	startHeight := s.Support.Height()
	if startHeight >= targetHeight {
		return nil, errors.Errorf("already at target height of %d", targetHeight)
	}

	//====
	//TODO create a buffer to accept the blocks delivered from the BFTDeliverer

	//===
	//TODO create the deliverer
	bftDeliverer := &blocksprovider.BFTDeliverer{
		ChannelID:    s.Support.ChannelID(),
		BlockHandler: nil, // TODO handle the block into a buffer
		//&GossipBlockHandler{
		//	gossip:              d.conf.Gossip,
		//	blockGossipDisabled: true, // Block gossip is deprecated since in v2.2 and is no longer supported in v3.x
		//	logger:              flogging.MustGetLogger("peer.blocksprovider").With("channel", chainID),},
		Ledger:                 nil, // ledgerInfo,
		UpdatableBlockVerifier: nil, // ubv,
		Dialer:                 blocksprovider.DialerAdapter{
			//ClientConfig: comm.ClientConfig{
			//	DialTimeout: d.conf.DeliverServiceConfig.ConnectionTimeout,
			//	KaOpts:      d.conf.DeliverServiceConfig.KeepaliveOptions,
			//	SecOpts:     d.conf.DeliverServiceConfig.SecOpts,
			//},
		},
		OrderersSourceFactory:     &orderers.ConnectionSourceFactory{}, // no overrides in the orderer
		CryptoProvider:            nil,                                 // d.conf.CryptoProvider,
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
			return true // In the orderer we must limit the time we try to do Synch()
		},
	}

	s.Logger.Infof("Created a BFTDeliverer: %+v", bftDeliverer)

	return nil, errors.New("not implemented")
}

// computeTargetHeight compute the target height to synchronize to.
//
// heights: a slice containing the heights of accessible peers, length must be >0.
// clusterSize: the cluster size, must be >0.
func (s *BFTSynchronizer) computeTargetHeight(heights []uint64) uint64 {
	sort.Slice(heights, func(i, j int) bool { return heights[i] > heights[j] }) // Descending
	f := uint64(s.ClusterSize-1) / 3                                            // The number of tolerated byzantine faults
	lenH := uint64(len(heights))

	s.Logger.Debugf("Heights: %v", heights)

	if lenH < f+1 {
		s.Logger.Debugf("Returning %d", heights[0])
		return heights[int(lenH)-1]
	}
	s.Logger.Debugf("Returning %d", heights[f])
	return heights[f]
}
