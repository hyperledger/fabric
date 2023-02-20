/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package smartbft

import (
	"sort"

	"github.com/SmartBFT-Go/consensus/pkg/types"
	"github.com/SmartBFT-Go/consensus/smartbftprotos"
	cb "github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/orderer/consensus"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
)

// Synchronizer implementation
type Synchronizer struct {
	lastReconfig    types.Reconfig
	selfID          uint64
	LatestConfig    func() (types.Configuration, []uint64)
	BlockToDecision func(*cb.Block) *types.Decision
	OnCommit        func(*cb.Block) types.Reconfig
	Support         consensus.ConsenterSupport
	BlockPuller     BlockPuller
	ClusterSize     uint64
	Logger          *flogging.FabricLogger
}

// Close closes the block puller connection
func (s *Synchronizer) Close() {
	s.BlockPuller.Close()
}

// Sync synchronizes blocks and returns the response
func (s *Synchronizer) Sync() types.SyncResponse {
	decision, err := s.synchronize()
	if err != nil {
		s.Logger.Warnf("Could not synchronize with remote peers due to %s, returning state from local ledger", err)
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

func (s *Synchronizer) getViewMetadataLastConfigSqnFromBlock(block *cb.Block) (*smartbftprotos.ViewMetadata, uint64) {
	viewMetadata, err := getViewMetadataFromBlock(block)
	if err != nil {
		return nil, 0
	}

	lastConfigSqn := s.Support.Sequence()

	return viewMetadata, lastConfigSqn
}

func (s *Synchronizer) synchronize() (*types.Decision, error) {
	defer s.BlockPuller.Close()
	heightByEndpoint, err := s.BlockPuller.HeightsByEndpoints()
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
		return nil, errors.Errorf("already at height of %d", targetHeight)
	}

	targetSeq := targetHeight - 1
	seq := startHeight

	var blocksFetched int

	s.Logger.Debugf("Will fetch sequences [%d-%d]", seq, targetSeq)

	var lastPulledBlock *cb.Block
	for seq <= targetSeq {
		block := s.BlockPuller.PullBlock(seq)
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
func (s *Synchronizer) computeTargetHeight(heights []uint64) uint64 {
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
