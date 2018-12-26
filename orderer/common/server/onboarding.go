/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package server

import (
	"github.com/hyperledger/fabric/common/crypto"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/ledger/blockledger"
	"github.com/hyperledger/fabric/core/comm"
	"github.com/hyperledger/fabric/orderer/common/cluster"
	"github.com/hyperledger/fabric/orderer/common/localconfig"
	"github.com/hyperledger/fabric/orderer/consensus/etcdraft"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/utils"
)

type replicationInitiator struct {
	logger  *flogging.FabricLogger
	secOpts *comm.SecureOptions
	conf    *localconfig.TopLevel
	lf      cluster.LedgerFactory
	signer  crypto.LocalSigner
}

func (ri *replicationInitiator) replicateIfNeeded(bootstrapBlock *common.Block) {
	if bootstrapBlock.Header.Number == 0 {
		ri.logger.Debug("Booted with a genesis block, replication isn't an option")
		return
	}
	ri.replicateNeededChannels(bootstrapBlock)
}

func (ri *replicationInitiator) createReplicator(bootstrapBlock *common.Block, filter func(string) bool) *cluster.Replicator {
	consenterCert := etcdraft.ConsenterCertificate(ri.secOpts.Certificate)
	systemChannelName, err := utils.GetChainIDFromBlock(bootstrapBlock)
	if err != nil {
		ri.logger.Panicf("Failed extracting system channel name from bootstrap block: %v", err)
	}
	pullerConfig := cluster.PullerConfigFromTopLevelConfig(systemChannelName, ri.conf, ri.secOpts.Key, ri.secOpts.Certificate, ri.signer)
	puller, err := cluster.BlockPullerFromConfigBlock(pullerConfig, bootstrapBlock)
	if err != nil {
		ri.logger.Panicf("Failed creating puller config from bootstrap block: %v", err)
	}

	pullerLogger := flogging.MustGetLogger("orderer.common.cluster")

	replicator := &cluster.Replicator{
		Filter:           filter,
		LedgerFactory:    ri.lf,
		SystemChannel:    systemChannelName,
		BootBlock:        bootstrapBlock,
		Logger:           pullerLogger,
		AmIPartOfChannel: consenterCert.IsConsenterOfChannel,
		Puller:           puller,
		ChannelLister: &cluster.ChainInspector{
			Logger:          pullerLogger,
			Puller:          puller,
			LastConfigBlock: bootstrapBlock,
		},
	}

	return replicator
}

func (ri *replicationInitiator) replicateNeededChannels(bootstrapBlock *common.Block) {
	replicator := ri.createReplicator(bootstrapBlock, cluster.AnyChannel)
	defer replicator.Puller.Close()
	replicationNeeded, err := replicator.IsReplicationNeeded()
	if err != nil {
		ri.logger.Panicf("Failed determining whether replication is needed: %v", err)
	}

	if !replicationNeeded {
		ri.logger.Info("Replication isn't needed")
		return
	}

	ri.logger.Info("Will now replicate chains")
	replicator.ReplicateChains()
}

func (ri *replicationInitiator) replicateChains(lastConfigBlock *common.Block, chains []string) {
	ri.logger.Info("Will now replicate chains", chains)
	wantedChannels := make(map[string]struct{})
	for _, chain := range chains {
		wantedChannels[chain] = struct{}{}
	}
	filter := func(channelName string) bool {
		_, exists := wantedChannels[channelName]
		return exists
	}
	replicator := ri.createReplicator(lastConfigBlock, filter)
	defer replicator.Puller.Close()
	replicator.ReplicateChains()
}

type ledgerFactory struct {
	blockledger.Factory
}

func (lf *ledgerFactory) GetOrCreate(chainID string) (cluster.LedgerWriter, error) {
	return lf.Factory.GetOrCreate(chainID)
}
