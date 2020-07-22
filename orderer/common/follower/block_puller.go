/*
Copyright IBM Corp. 2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package follower

import (
	"encoding/pem"

	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric/bccsp"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/internal/pkg/identity"
	"github.com/hyperledger/fabric/orderer/common/cluster"
	"github.com/hyperledger/fabric/orderer/common/localconfig"
	"github.com/pkg/errors"
)

//go:generate counterfeiter -o mocks/channel_puller.go -fake-name ChannelPuller . ChannelPuller

// ChannelPuller pulls blocks for a channel
type ChannelPuller interface {
	PullBlock(seq uint64) *common.Block
	HeightsByEndpoints() (map[string]uint64, error)
	Close()
}

type BlockPullerFactory interface {
	BlockPuller(configBlock *common.Block) (ChannelPuller, error)
}

type BlockPullerCreator struct {
	channelID           string
	bccsp               bccsp.BCCSP
	blockVerifier       cluster.BlockVerifier
	clusterConfig       localconfig.Cluster
	signer              identity.SignerSerializer
	der                 *pem.Block
	stdDialer           *cluster.StandardDialer
	clusterVerifyBlocks ClusterVerifyBlocksFunc
}

type ClusterVerifyBlocksFunc func(blockBuff []*common.Block, signatureVerifier cluster.BlockVerifier) error

func NewBlockPullerFactory(
	channelID string,
	signer identity.SignerSerializer,
	blockVerifier cluster.BlockVerifier,
	baseDialer *cluster.PredicateDialer,
	clusterConfig localconfig.Cluster,
	bccsp bccsp.BCCSP,
	alternativeVerifyBlocksFunc ...ClusterVerifyBlocksFunc, //Allow to optionally replace cluster.VerifyBlocks
) (BlockPullerFactory, error) {
	if baseDialer == nil {
		return nil, errors.New("base dialer is nil")
	}

	stdDialer := &cluster.StandardDialer{
		Config: baseDialer.Config.Clone(),
	}
	stdDialer.Config.AsyncConnect = false
	stdDialer.Config.SecOpts.VerifyCertificate = nil

	der, _ := pem.Decode(stdDialer.Config.SecOpts.Certificate)
	if der == nil {
		return nil, errors.Errorf("client certificate isn't in PEM format: %v",
			string(stdDialer.Config.SecOpts.Certificate))
	}

	factory := &BlockPullerCreator{
		channelID:           channelID,
		bccsp:               bccsp,
		blockVerifier:       blockVerifier,
		clusterConfig:       clusterConfig,
		signer:              signer,
		stdDialer:           stdDialer,
		der:                 der,
		clusterVerifyBlocks: cluster.VerifyBlocks, // The default block sequence verification method.
	}

	if len(alternativeVerifyBlocksFunc) > 0 { // Provide an alternative, mainly for tests.
		factory.clusterVerifyBlocks = alternativeVerifyBlocksFunc[0]
	}

	return factory, nil
}

func (factory *BlockPullerCreator) BlockPuller(configBlock *common.Block) (ChannelPuller, error) {
	// Extract the TLS CA certs and endpoints from the join-block
	endpoints, err := cluster.EndpointconfigFromConfigBlock(configBlock, factory.bccsp)
	if err != nil {
		return nil, errors.Wrap(err, "error extracting endpoints from config block")
	}

	bp := &cluster.BlockPuller{
		VerifyBlockSequence: factory.VerifyBlockSequence,
		Logger:              flogging.MustGetLogger("orderer.common.cluster.puller").With("channel", factory.channelID),
		RetryTimeout:        factory.clusterConfig.ReplicationRetryTimeout,
		MaxTotalBufferBytes: factory.clusterConfig.ReplicationBufferSize,
		FetchTimeout:        factory.clusterConfig.ReplicationPullTimeout,
		Endpoints:           endpoints,
		Signer:              factory.signer,
		TLSCert:             factory.der.Bytes,
		Channel:             factory.channelID,
		Dialer:              factory.stdDialer,
	}

	return bp, nil
}

func (factory *BlockPullerCreator) VerifyBlockSequence(blocks []*common.Block, _ string) error {
	if len(blocks) == 0 {
		return errors.New("buffer is empty")
	}
	if blocks[0] == nil {
		return errors.New("first block is nil")
	}
	if blocks[0].Header == nil {
		return errors.New("first block header is nil")
	}
	if blocks[0].Header.Number == 0 {
		blocksAfterGenesis := blocks[1:]
		return factory.clusterVerifyBlocks(blocksAfterGenesis, factory.blockVerifier)
	}

	return factory.clusterVerifyBlocks(blocks, factory.blockVerifier)
}
