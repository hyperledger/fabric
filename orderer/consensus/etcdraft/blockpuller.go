/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package etcdraft

import (
	"encoding/pem"

	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric/bccsp"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/orderer/common/cluster"
	"github.com/hyperledger/fabric/orderer/common/localconfig"
	"github.com/hyperledger/fabric/orderer/consensus"
	"github.com/pkg/errors"
)

// LedgerBlockPuller pulls blocks upon demand, or fetches them from the ledger
type LedgerBlockPuller struct {
	BlockPuller
	BlockRetriever cluster.BlockRetriever
	Height         func() uint64
}

func (lp *LedgerBlockPuller) PullBlock(seq uint64) *common.Block {
	lastSeq := lp.Height() - 1
	if lastSeq >= seq {
		return lp.BlockRetriever.Block(seq)
	}
	return lp.BlockPuller.PullBlock(seq)
}

// EndpointconfigFromSupport extracts TLS CA certificates and endpoints from the ConsenterSupport
func EndpointconfigFromSupport(support consensus.ConsenterSupport, bccsp bccsp.BCCSP) ([]cluster.EndpointCriteria, error) {
	lastConfigBlock, err := lastConfigBlockFromSupport(support)
	if err != nil {
		return nil, err
	}
	endpointconf, err := cluster.EndpointconfigFromConfigBlock(lastConfigBlock, bccsp)
	if err != nil {
		return nil, err
	}
	return endpointconf, nil
}

func lastConfigBlockFromSupport(support consensus.ConsenterSupport) (*common.Block, error) {
	lastBlockSeq := support.Height() - 1
	lastBlock := support.Block(lastBlockSeq)
	if lastBlock == nil {
		return nil, errors.Errorf("unable to retrieve block [%d]", lastBlockSeq)
	}
	lastConfigBlock, err := cluster.LastConfigBlock(lastBlock, support)
	if err != nil {
		return nil, err
	}
	return lastConfigBlock, nil
}

// NewBlockPuller creates a new block puller
func NewBlockPuller(support consensus.ConsenterSupport,
	baseDialer *cluster.PredicateDialer,
	clusterConfig localconfig.Cluster,
	bccsp bccsp.BCCSP,
) (BlockPuller, error) {
	verifyBlockSequence := func(blocks []*common.Block, _ string) error {
		vb := cluster.BlockVerifierBuilder(bccsp)
		return cluster.VerifyBlocksBFT(blocks, support.SignatureVerifier(), vb)
	}

	stdDialer := &cluster.StandardDialer{
		Config: baseDialer.Config,
	}
	stdDialer.Config.AsyncConnect = false
	stdDialer.Config.SecOpts.VerifyCertificate = nil

	// Extract the TLS CA certs and endpoints from the configuration,
	endpoints, err := EndpointconfigFromSupport(support, bccsp)
	if err != nil {
		return nil, err
	}

	der, _ := pem.Decode(stdDialer.Config.SecOpts.Certificate)
	if der == nil {
		return nil, errors.Errorf("client certificate isn't in PEM format: %v",
			string(stdDialer.Config.SecOpts.Certificate))
	}

	bp := &cluster.BlockPuller{
		VerifyBlockSequence: verifyBlockSequence,
		Logger:              flogging.MustGetLogger("orderer.common.cluster.puller").With("channel", support.ChannelID()),
		RetryTimeout:        clusterConfig.ReplicationRetryTimeout,
		MaxTotalBufferBytes: clusterConfig.ReplicationBufferSize,
		FetchTimeout:        clusterConfig.ReplicationPullTimeout,
		Endpoints:           endpoints,
		Signer:              support,
		TLSCert:             der.Bytes,
		Channel:             support.ChannelID(),
		Dialer:              stdDialer,
		StopChannel:         make(chan struct{}),
	}

	return &LedgerBlockPuller{
		Height:         support.Height,
		BlockRetriever: support,
		BlockPuller:    bp,
	}, nil
}
