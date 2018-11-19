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
)

type replicationInitiator struct {
	logger         *flogging.FabricLogger
	secOpts        *comm.SecureOptions
	conf           *localconfig.TopLevel
	bootstrapBlock *common.Block
	lf             cluster.LedgerFactory
	signer         crypto.LocalSigner
}

func (ri *replicationInitiator) replicateIfNeeded() {
	if ri.bootstrapBlock.Header.Number == 0 {
		ri.logger.Debug("Booted with a genesis block, replication isn't an option")
		return
	}

	consenterCert := etcdraft.ConsenterCertificate(ri.secOpts.Certificate)

	pullerConfig := cluster.PullerConfigFromTopLevelConfig(ri.conf, ri.secOpts.Key, ri.secOpts.Certificate, ri.signer)
	puller, err := cluster.BlockPullerFromConfigBlock(pullerConfig, ri.bootstrapBlock)
	if err != nil {
		ri.logger.Panicf("Failed creating puller config from bootstrap block: %v", err)
	}

	pullerLogger := flogging.MustGetLogger("orderer.common.cluster")

	replicator := &cluster.Replicator{
		LedgerFactory:    ri.lf,
		SystemChannel:    ri.conf.General.SystemChannel,
		BootBlock:        ri.bootstrapBlock,
		Logger:           pullerLogger,
		AmIPartOfChannel: consenterCert.IsConsenterOfChannel,
		Puller:           puller,
		ChannelLister: &cluster.ChainInspector{
			Logger:          pullerLogger,
			Puller:          puller,
			LastConfigBlock: ri.bootstrapBlock,
		},
	}

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

type ledgerFactory struct {
	blockledger.Factory
}

func (lf *ledgerFactory) GetOrCreate(chainID string) (cluster.LedgerWriter, error) {
	return lf.Factory.GetOrCreate(chainID)
}
