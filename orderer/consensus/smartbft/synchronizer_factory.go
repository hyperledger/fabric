/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package smartbft

import (
	"github.com/hyperledger-labs/SmartBFT/pkg/api"
	"github.com/hyperledger-labs/SmartBFT/pkg/types"
	"github.com/hyperledger/fabric-lib-go/bccsp"
	"github.com/hyperledger/fabric-lib-go/common/flogging"
	cb "github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/hyperledger/fabric/orderer/common/cluster"
	"github.com/hyperledger/fabric/orderer/common/localconfig"
	"github.com/hyperledger/fabric/orderer/consensus"
)

//go:generate mockery --dir . --name synchronizer --case underscore --with-expecter=true --exported --output mocks
type synchronizer interface {
	api.Synchronizer
}

//go:generate mockery --dir . --name SynchronizerFactory --case underscore --with-expecter=true --output mocks
type SynchronizerFactory interface {
	// CreateSynchronizer creates a new Synchronizer.
	CreateSynchronizer(
		logger *flogging.FabricLogger,
		localConfigCluster localconfig.Cluster,
		rtc RuntimeConfig,
		blockToDecision func(block *cb.Block) *types.Decision,
		pruneCommittedRequests func(block *cb.Block),
		updateRuntimeConfig func(block *cb.Block) types.Reconfig,
		support consensus.ConsenterSupport,
		bccsp bccsp.BCCSP,
		clusterDialer *cluster.PredicateDialer,
	) api.Synchronizer
}

type synchronizerCreator struct{}

func (*synchronizerCreator) CreateSynchronizer(
	logger *flogging.FabricLogger,
	localConfigCluster localconfig.Cluster,
	rtc RuntimeConfig,
	blockToDecision func(block *cb.Block) *types.Decision,
	pruneCommittedRequests func(block *cb.Block),
	updateRuntimeConfig func(block *cb.Block) types.Reconfig,
	support consensus.ConsenterSupport,
	bccsp bccsp.BCCSP,
	clusterDialer *cluster.PredicateDialer,
) api.Synchronizer {
	return newSynchronizer(logger, localConfigCluster, rtc, blockToDecision, pruneCommittedRequests, updateRuntimeConfig, support, bccsp, clusterDialer)
}

// newSynchronizer creates a new synchronizer
func newSynchronizer(
	logger *flogging.FabricLogger,
	localConfigCluster localconfig.Cluster,
	rtc RuntimeConfig,
	blockToDecision func(block *cb.Block) *types.Decision,
	pruneCommittedRequests func(block *cb.Block),
	updateRuntimeConfig func(block *cb.Block) types.Reconfig,
	support consensus.ConsenterSupport,
	bccsp bccsp.BCCSP,
	clusterDialer *cluster.PredicateDialer,
) api.Synchronizer {
	switch localConfigCluster.ReplicationPolicy {
	case "consensus":
		logger.Debug("Creating a BFTSynchronizer")
		return &BFTSynchronizer{
			selfID: rtc.id,
			LatestConfig: func() (types.Configuration, []uint64) {
				return rtc.BFTConfig, rtc.Nodes
			},
			BlockToDecision: blockToDecision,
			OnCommit: func(block *cb.Block) types.Reconfig {
				pruneCommittedRequests(block)
				return updateRuntimeConfig(block)
			},
			Support:             support,
			CryptoProvider:      bccsp,
			ClusterDialer:       clusterDialer,
			LocalConfigCluster:  localConfigCluster,
			BlockPullerFactory:  &blockPullerCreator{},
			VerifierFactory:     &verifierCreator{},
			BFTDelivererFactory: &bftDelivererCreator{},
			Logger:              logger,
		}
	case "simple":
		logger.Debug("Creating simple Synchronizer")
		return &Synchronizer{
			selfID:          rtc.id,
			BlockToDecision: blockToDecision,
			OnCommit: func(block *cb.Block) types.Reconfig {
				pruneCommittedRequests(block)
				return updateRuntimeConfig(block)
			},
			Support:            support,
			CryptoProvider:     bccsp,
			ClusterDialer:      clusterDialer,
			LocalConfigCluster: localConfigCluster,
			BlockPullerFactory: &blockPullerCreator{},
			Logger:             logger,
			LatestConfig: func() (types.Configuration, []uint64) {
				return rtc.BFTConfig, rtc.Nodes
			},
		}
	default:
		logger.Panicf("Unsupported Cluster.ReplicationPolicy: %s", localConfigCluster.ReplicationPolicy)
		return nil
	}
}
