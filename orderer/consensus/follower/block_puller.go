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

//go:generate counterfeiter -o mocks/block_puller_support.go -fake-name BlockPullerSupport . BlockPullerSupport

type BlockPullerSupport interface {
	cluster.BlockVerifier
	identity.SignerSerializer
	ChannelID() string
}

//go:generate counterfeiter -o mocks/channel_puller.go -fake-name ChannelPuller . ChannelPuller

// ChannelPuller pulls blocks for a channel
type ChannelPuller interface {
	PullBlock(seq uint64) *common.Block
	HeightsByEndpoints() (map[string]uint64, error)
	Close()
}

// BlockPullerFromJoinBlock creates a new block puller from a join-block.
// This block puller is used for on-boarding, when join-block.Number >= ledger.Height().
// This block puller can fetch the genesis block and skip verifying it.
func BlockPullerFromJoinBlock(
	joinBlock *common.Block,
	support BlockPullerSupport,
	baseDialer *cluster.PredicateDialer,
	clusterConfig localconfig.Cluster,
	bccsp bccsp.BCCSP,
) (*cluster.BlockPuller, error) {
	if baseDialer == nil {
		return nil, errors.New("base dialer is nil")
	}

	verifyBlockSequence := func(blocks []*common.Block, _ string) error {
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
			return cluster.VerifyBlocks(blocksAfterGenesis, support)
		}

		return cluster.VerifyBlocks(blocks, support)
	}

	stdDialer := &cluster.StandardDialer{
		Config: baseDialer.Config.Clone(),
	}
	stdDialer.Config.AsyncConnect = false
	stdDialer.Config.SecOpts.VerifyCertificate = nil

	// Extract the TLS CA certs and endpoints from the join-block
	endpoints, err := cluster.EndpointconfigFromConfigBlock(joinBlock, bccsp)
	if err != nil {
		return nil, errors.Wrap(err, "error extracting endpoints from config block")
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
	}

	return bp, nil
}
