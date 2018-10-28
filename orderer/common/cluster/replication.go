/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package cluster

import (
	"encoding/base64"
	"encoding/pem"
	"time"

	"github.com/hyperledger/fabric/common/crypto"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/core/comm"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/pkg/errors"
)

const (
	// RetryTimeout is the time the block puller retries
	RetryTimeout = time.Second * 10
)

// PullerConfig configures a BlockPuller.
type PullerConfig struct {
	TLSKey              []byte
	TLSCert             []byte
	Timeout             time.Duration
	Signer              crypto.LocalSigner
	Channel             string
	MaxTotalBufferBytes int
}

// BlockPullerFromConfigBlock returns a BlockPuller that doesn't verify signatures on blocks.
func BlockPullerFromConfigBlock(conf PullerConfig, block *common.Block) (*BlockPuller, error) {
	if block == nil {
		return nil, errors.New("nil block")
	}

	endpointconfig, err := EndpointconfigFromConfigBlock(block)
	if err != nil {
		return nil, err
	}

	dialer := &StandardDialer{
		Dialer: NewTLSPinningDialer(comm.ClientConfig{
			Timeout: conf.Timeout,
			SecOpts: &comm.SecureOptions{
				ServerRootCAs:     endpointconfig.TLSRootCAs,
				Certificate:       conf.TLSCert,
				Key:               conf.TLSKey,
				RequireClientCert: true,
				UseTLS:            true,
			},
		})}

	tlsCertAsDER, _ := pem.Decode(conf.TLSCert)
	if tlsCertAsDER == nil {
		return nil, errors.Errorf("unable to decode TLS certificate PEM: %s", base64.StdEncoding.EncodeToString(conf.TLSCert))
	}

	return &BlockPuller{
		Logger:  flogging.MustGetLogger("orderer/common/cluster/replication"),
		Dialer:  dialer,
		TLSCert: tlsCertAsDER.Bytes,
		VerifyBlockSequence: func(blocks []*common.Block) error {
			return VerifyBlocks(blocks, &NoopBlockVerifier{})
		},
		MaxTotalBufferBytes: conf.MaxTotalBufferBytes,
		Endpoints:           endpointconfig.Endpoints,
		RetryTimeout:        RetryTimeout,
		FetchTimeout:        conf.Timeout,
		Channel:             conf.Channel,
		Signer:              conf.Signer,
	}, nil
}

// NoopBlockVerifier doesn't verify block signatures
type NoopBlockVerifier struct{}

// VerifyBlockSignature accepts all signatures over blocks.
func (*NoopBlockVerifier) VerifyBlockSignature(sd []*common.SignedData, config *common.ConfigEnvelope) error {
	return nil
}

//go:generate mockery -dir . -name ChainPuller -case underscore -output mocks/

// ChainPuller pulls blocks from a chain
type ChainPuller interface {
	// PullBlock pulls the given block from some orderer node
	PullBlock(seq uint64) *common.Block
	// Close closes the ChainPuller
	Close()
}

// ChainInspector walks over a chain
type ChainInspector struct {
	Logger          *flogging.FabricLogger
	Puller          ChainPuller
	LastConfigBlock *common.Block
}

// Channels returns the list of channels
// that exist in the chain
func (cw *ChainInspector) Channels() []string {
	channels := make(map[string]struct{})
	lastConfigBlockNum := cw.LastConfigBlock.Header.Number
	var block *common.Block
	for seq := uint64(1); seq < lastConfigBlockNum; seq++ {
		block = cw.Puller.PullBlock(seq)
		channel, err := IsNewChannelBlock(block)
		if err != nil {
			// If we failed to classify a block, something is wrong in the system chain
			// we're trying to pull, so abort.
			cw.Logger.Panic("Failed classifying block", seq, ":", err)
			continue
		}
		if channel == "" {
			cw.Logger.Info("Block", seq, "doesn't contain a new channel")
			continue
		}
		cw.Logger.Info("Block", seq, "contains channel", channel)
		channels[channel] = struct{}{}
	}
	// At this point, block holds reference to the last block pulled.
	// We ensure that the hash of the last block pulled, is the previous hash
	// of the LastConfigBlock we were initialized with.
	// We don't need to verify the entire chain of all blocks we pulled,
	// because the block puller calls VerifyBlockHash on all blocks it pulls.
	last2Blocks := []*common.Block{block, cw.LastConfigBlock}
	if err := VerifyBlockHash(1, last2Blocks); err != nil {
		cw.Logger.Panic("System channel pulled doesn't match the boot last config block:", err)
	}

	return flattenChannelMap(channels)
}

func flattenChannelMap(m map[string]struct{}) []string {
	var res []string
	for channel := range m {
		res = append(res, channel)
	}
	return res
}

// IsNewChannelBlock returns a name of the channel in case
// it holds a channel create transaction, or empty string otherwise.
func IsNewChannelBlock(block *common.Block) (string, error) {
	if block == nil {
		return "", errors.New("nil block")
	}
	env, err := utils.ExtractEnvelope(block, 0)
	if err != nil {
		return "", err
	}
	payload, err := utils.ExtractPayload(env)
	if err != nil {
		return "", err
	}
	if payload.Header == nil {
		return "", errors.New("nil header in payload")
	}
	chdr, err := utils.UnmarshalChannelHeader(payload.Header.ChannelHeader)
	if err != nil {
		return "", err
	}
	// The transaction is an orderer transaction
	if common.HeaderType(chdr.Type) != common.HeaderType_ORDERER_TRANSACTION {
		return "", nil
	}
	systemChannelName := chdr.ChannelId
	innerEnvelope, err := utils.UnmarshalEnvelope(payload.Data)
	if err != nil {
		return "", err
	}
	innerPayload, err := utils.UnmarshalPayload(innerEnvelope.Payload)
	if err != nil {
		return "", err
	}
	if innerPayload.Header == nil {
		return "", errors.New("inner payload's header is nil")
	}
	chdr, err = utils.UnmarshalChannelHeader(innerPayload.Header.ChannelHeader)
	if err != nil {
		return "", err
	}
	// The inner payload's header is a config transaction
	if common.HeaderType(chdr.Type) != common.HeaderType_CONFIG {
		return "", nil
	}
	// In any case, exclude all system channel transactions
	if chdr.ChannelId == systemChannelName {
		return "", nil
	}
	return chdr.ChannelId, nil
}
