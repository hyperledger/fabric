/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package smartbft

import (
	"crypto/x509"
	"encoding/pem"

	"github.com/hyperledger/fabric-lib-go/bccsp"
	"github.com/hyperledger/fabric-lib-go/common/flogging"
	cb "github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/hyperledger/fabric/orderer/common/cluster"
	"github.com/hyperledger/fabric/orderer/common/localconfig"
	"github.com/hyperledger/fabric/orderer/consensus"
	"github.com/hyperledger/fabric/orderer/consensus/etcdraft"
	"github.com/pkg/errors"
)

//go:generate counterfeiter -o mocks/block_puller.go . BlockPuller
//go:generate mockery --dir . --name BlockPuller --case underscore --with-expecter=true --output mocks/mock_block_puller.go

// BlockPuller is used to pull blocks from other OSN
type BlockPuller interface {
	PullBlock(seq uint64) *cb.Block
	HeightsByEndpoints() (map[string]uint64, string, error)
	Close()
}

//go:generate counterfeiter -o mocks/block_puller_factory.go . BlockPullerFactory

type BlockPullerFactory interface {
	// CreateBlockPuller creates a new block puller.
	CreateBlockPuller(
		support consensus.ConsenterSupport,
		baseDialer *cluster.PredicateDialer,
		clusterConfig localconfig.Cluster,
		bccsp bccsp.BCCSP,
	) (BlockPuller, error)
}

type blockPullerCreator struct{}

func (*blockPullerCreator) CreateBlockPuller(
	support consensus.ConsenterSupport,
	baseDialer *cluster.PredicateDialer,
	clusterConfig localconfig.Cluster,
	bccsp bccsp.BCCSP,
) (BlockPuller, error) {
	return newBlockPuller(support, baseDialer, clusterConfig, bccsp)
}

// newBlockPuller creates a new block puller
func newBlockPuller(
	support consensus.ConsenterSupport,
	baseDialer *cluster.PredicateDialer,
	clusterConfig localconfig.Cluster,
	bccsp bccsp.BCCSP,
) (BlockPuller, error) {
	verifyBlockSequence := func(blocks []*cb.Block, _ string) error {
		vb := cluster.BlockVerifierBuilder(bccsp)
		return cluster.VerifyBlocksBFT(blocks, support.SignatureVerifier(), vb)
	}

	stdDialer := &cluster.StandardDialer{
		Config: baseDialer.Config.Clone(),
	}
	stdDialer.Config.AsyncConnect = false
	stdDialer.Config.SecOpts.VerifyCertificate = nil

	// Extract the TLS CA certs and endpoints from the configuration,
	endpoints, err := etcdraft.EndpointconfigFromSupport(support, bccsp)
	if err != nil {
		return nil, err
	}

	logger := flogging.MustGetLogger("orderer.common.cluster.puller")

	der, _ := pem.Decode(stdDialer.Config.SecOpts.Certificate)
	if der == nil {
		return nil, errors.Errorf("client certificate isn't in PEM format: %v",
			string(stdDialer.Config.SecOpts.Certificate))
	}

	myCert, err := x509.ParseCertificate(der.Bytes)
	if err != nil {
		logger.Warnf("Failed parsing my own TLS certificate: %v, therefore we may connect to our own endpoint when pulling blocks", err)
	}

	bp := &cluster.BlockPuller{
		MyOwnTLSCert:        myCert,
		VerifyBlockSequence: verifyBlockSequence,
		Logger:              logger,
		RetryTimeout:        clusterConfig.ReplicationRetryTimeout,
		MaxTotalBufferBytes: clusterConfig.ReplicationBufferSize,
		FetchTimeout:        clusterConfig.ReplicationPullTimeout,
		Endpoints:           endpoints,
		Signer:              support,
		TLSCert:             der.Bytes,
		Channel:             support.ChannelID(),
		Dialer:              stdDialer,
	}

	logger.Infof("Built new block puller with cluster config: %+v, endpoints: %+v", clusterConfig, endpoints)

	return bp, nil
}
