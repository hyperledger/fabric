/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package cluster

// TODO rename the file?

import (
	"encoding/base64"
	"encoding/pem"
	"time"

	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric/bccsp"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/internal/pkg/comm"
	"github.com/hyperledger/fabric/internal/pkg/identity"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
)

const (
	// RetryTimeout is the time the block puller retries.
	RetryTimeout = time.Second * 10
)

//go:generate mockery --dir . --name LedgerWriter --case underscore --output mocks/

// LedgerWriter allows the caller to write blocks and inspect the height
type LedgerWriter interface {
	// Append a new block to the ledger
	Append(block *common.Block) error

	// Height returns the number of blocks on the ledger
	Height() uint64
}

// PullerConfig configures a BlockPuller.
type PullerConfig struct {
	TLSKey              []byte
	TLSCert             []byte
	Timeout             time.Duration
	Signer              identity.SignerSerializer
	Channel             string
	MaxTotalBufferBytes int
}

//go:generate mockery --dir . --name VerifierRetriever --case underscore --output mocks/

// VerifierRetriever retrieves BlockVerifiers for channels.
type VerifierRetriever interface {
	// RetrieveVerifier retrieves a BlockVerifier for the given channel.
	RetrieveVerifier(channel string) protoutil.BlockVerifierFunc
}

// BlockPullerFromConfigBlock returns a BlockPuller that doesn't verify signatures on blocks.
func BlockPullerFromConfigBlock(conf PullerConfig, block *common.Block, verifierRetriever VerifierRetriever, bccsp bccsp.BCCSP) (*BlockPuller, error) {
	if block == nil {
		return nil, errors.New("nil block")
	}

	endpoints, err := EndpointconfigFromConfigBlock(block, bccsp)
	if err != nil {
		return nil, err
	}

	clientConf := comm.ClientConfig{
		DialTimeout: conf.Timeout,
		SecOpts: comm.SecureOptions{
			Certificate:       conf.TLSCert,
			Key:               conf.TLSKey,
			RequireClientCert: true,
			UseTLS:            true,
		},
	}

	dialer := &StandardDialer{
		Config: clientConf,
	}

	tlsCertAsDER, _ := pem.Decode(conf.TLSCert)
	if tlsCertAsDER == nil {
		return nil, errors.Errorf("unable to decode TLS certificate PEM: %s", base64.StdEncoding.EncodeToString(conf.TLSCert))
	}

	// TODO: This code is entire file is going away. I am parking this here for the meantime.
	verifyBlockSequence := func(blocks []*common.Block, _ string) error {
		vb := BlockVerifierBuilder(bccsp)
		verify := func(header *common.BlockHeader, metadata *common.BlockMetadata) error {
			return verifierRetriever.RetrieveVerifier(conf.Channel)(header, metadata)
		}
		return VerifyBlocksBFT(blocks, verify, vb)
	}

	return &BlockPuller{
		Logger:              flogging.MustGetLogger("orderer.common.cluster.replication").With("channel", conf.Channel),
		Dialer:              dialer,
		TLSCert:             tlsCertAsDER.Bytes,
		VerifyBlockSequence: verifyBlockSequence,
		MaxTotalBufferBytes: conf.MaxTotalBufferBytes,
		Endpoints:           endpoints,
		RetryTimeout:        RetryTimeout,
		FetchTimeout:        conf.Timeout,
		Channel:             conf.Channel,
		Signer:              conf.Signer,
		StopChannel:         make(chan struct{}),
	}, nil
}

//go:generate mockery --dir . --name ChainPuller --case underscore --output mocks/

// ChainPuller pulls blocks from a chain
type ChainPuller interface {
	// PullBlock pulls the given block from some orderer node
	PullBlock(seq uint64) *common.Block

	// HeightsByEndpoints returns the block heights by endpoints of orderers
	HeightsByEndpoints() (map[string]uint64, error)

	// Close closes the ChainPuller
	Close()
}

// ChainInspector walks over a chain
type ChainInspector struct {
	Logger          *flogging.FabricLogger
	Puller          ChainPuller
	LastConfigBlock *common.Block
}

// ErrForbidden denotes that an ordering node refuses sending blocks due to access control.
var ErrForbidden = errors.New("forbidden pulling the channel")

// ErrServiceUnavailable denotes that an ordering node is not servicing at the moment.
var ErrServiceUnavailable = errors.New("service unavailable")

// ErrNotInChannel denotes that an ordering node is not in the channel
var ErrNotInChannel = errors.New("not in the channel")

var ErrRetryCountExhausted = errors.New("retry attempts exhausted")

// SelfMembershipPredicate determines whether the caller is found in the given config block
type SelfMembershipPredicate func(configBlock *common.Block) error

// PullLastConfigBlock pulls the last configuration block, or returns an error on failure.
func PullLastConfigBlock(puller ChainPuller) (*common.Block, error) {
	endpoint, latestHeight, err := LatestHeightAndEndpoint(puller)
	if err != nil {
		return nil, err
	}
	if endpoint == "" {
		return nil, ErrRetryCountExhausted
	}
	lastBlock := puller.PullBlock(latestHeight - 1)
	if lastBlock == nil {
		return nil, ErrRetryCountExhausted
	}
	lastConfNumber, err := protoutil.GetLastConfigIndexFromBlock(lastBlock)
	if err != nil {
		return nil, err
	}
	// The last config block is smaller than the latest height,
	// and a block iterator on the server side is a sequenced one.
	// So we need to reset the puller if we wish to pull an earlier block.
	puller.Close()
	lastConfigBlock := puller.PullBlock(lastConfNumber)
	if lastConfigBlock == nil {
		return nil, ErrRetryCountExhausted
	}
	return lastConfigBlock, nil
}

func LatestHeightAndEndpoint(puller ChainPuller) (string, uint64, error) {
	var maxHeight uint64
	var mostUpToDateEndpoint string
	heightsByEndpoints, err := puller.HeightsByEndpoints()
	if err != nil {
		return "", 0, err
	}
	for endpoint, height := range heightsByEndpoints {
		if height >= maxHeight {
			maxHeight = height
			mostUpToDateEndpoint = endpoint
		}
	}
	return mostUpToDateEndpoint, maxHeight, nil
}

// Close closes the ChainInspector
func (ci *ChainInspector) Close() {
	ci.Puller.Close()
}
