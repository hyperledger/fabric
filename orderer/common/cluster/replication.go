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
		Logger:  flogging.MustGetLogger("orderer.common.cluster.replication"),
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

	// HeightsByEndpoints returns the block heights by endpoints of orderers
	HeightsByEndpoints() map[string]uint64

	// Close closes the ChainPuller
	Close()
}

// ChainInspector walks over a chain
type ChainInspector struct {
	Logger          *flogging.FabricLogger
	Puller          ChainPuller
	LastConfigBlock *common.Block
}

// NotInChannelError denotes that an ordering node is not in the channel
var NotInChannelError = errors.New("not in the channel")

// selfMembershipPredicate determines whether the caller is found in the given config block
type selfMembershipPredicate func(configBlock *common.Block) error

// Participant returns whether the caller participates in the chain.
// It receives a ChainPuller that should already be calibrated for the chain,
// and a selfMembershipPredicate that is used to detect whether the caller should service the chain.
// It returns nil if the caller participates in the chain.
// It may return notInChannelError error in case the caller doesn't participate in the chain.
func Participant(puller ChainPuller, analyzeLastConfBlock selfMembershipPredicate) error {
	endpoint, latestHeight := latestHeightAndEndpoint(puller)
	if endpoint == "" {
		return errors.New("no available orderer")
	}

	lastBlock := puller.PullBlock(latestHeight - 1)
	lastConfNumber, err := lastConfigFromBlock(lastBlock)
	if err != nil {
		return err
	}

	lastConfigBlock := puller.PullBlock(lastConfNumber)
	return analyzeLastConfBlock(lastConfigBlock)
}

func latestHeightAndEndpoint(puller ChainPuller) (string, uint64) {
	var maxHeight uint64
	var mostUpToDateEndpoint string
	for endpoint, height := range puller.HeightsByEndpoints() {
		if height > maxHeight {
			maxHeight = height
			mostUpToDateEndpoint = endpoint
		}
	}
	return mostUpToDateEndpoint, maxHeight
}

func lastConfigFromBlock(block *common.Block) (uint64, error) {
	if block.Metadata == nil || len(block.Metadata.Metadata) <= int(common.BlockMetadataIndex_LAST_CONFIG) {
		return 0, errors.New("no metadata in block")
	}
	return utils.GetLastConfigIndexFromBlock(block)
}

// Channels returns the list of channels
// that exist in the chain
func (ci *ChainInspector) Channels() []string {
	channels := make(map[string]struct{})
	lastConfigBlockNum := ci.LastConfigBlock.Header.Number
	var block *common.Block
	for seq := uint64(1); seq < lastConfigBlockNum; seq++ {
		block = ci.Puller.PullBlock(seq)
		channel, err := IsNewChannelBlock(block)
		if err != nil {
			// If we failed to classify a block, something is wrong in the system chain
			// we're trying to pull, so abort.
			ci.Logger.Panic("Failed classifying block", seq, ":", err)
			continue
		}
		if channel == "" {
			ci.Logger.Info("Block", seq, "doesn't contain a new channel")
			continue
		}
		ci.Logger.Info("Block", seq, "contains channel", channel)
		channels[channel] = struct{}{}
	}
	// At this point, block holds reference to the last block pulled.
	// We ensure that the hash of the last block pulled, is the previous hash
	// of the LastConfigBlock we were initialized with.
	// We don't need to verify the entire chain of all blocks we pulled,
	// because the block puller calls VerifyBlockHash on all blocks it pulls.
	last2Blocks := []*common.Block{block, ci.LastConfigBlock}
	if err := VerifyBlockHash(1, last2Blocks); err != nil {
		ci.Logger.Panic("System channel pulled doesn't match the boot last config block:", err)
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
