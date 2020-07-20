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

// BlockPullerFromJoinBlock creates a new block puller from a join-block.
// This block puller is used for on-boarding, when join-block.Number >= ledger.Height().
func BlockPullerFromJoinBlock(
	joinBlock *common.Block,
	channelID string,
	signer identity.SignerSerializer,
	blockVerifier cluster.BlockVerifier,
	baseDialer *cluster.PredicateDialer,
	clusterConfig localconfig.Cluster,
	bccsp bccsp.BCCSP,
) (*cluster.BlockPuller, error) {
	if baseDialer == nil {
		return nil, errors.New("base dialer is nil")
	}

	verifyBlockSequence := func(blocks []*common.Block, _ string) error {
		return cluster.VerifyBlocks(blocks, blockVerifier)
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
		Logger:              flogging.MustGetLogger("orderer.common.cluster.puller").With("channel", channelID),
		RetryTimeout:        clusterConfig.ReplicationRetryTimeout,
		MaxTotalBufferBytes: clusterConfig.ReplicationBufferSize,
		FetchTimeout:        clusterConfig.ReplicationPullTimeout,
		Endpoints:           endpoints,
		Signer:              signer,
		TLSCert:             der.Bytes,
		Channel:             channelID,
		Dialer:              stdDialer,
	}

	return bp, nil
}
