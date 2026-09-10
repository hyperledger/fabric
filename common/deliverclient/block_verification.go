/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package deliverclient

import (
	"bytes"
	"encoding/hex"

	"github.com/hyperledger/fabric-lib-go/bccsp"
	"github.com/hyperledger/fabric-lib-go/common/flogging"
	"github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/hyperledger/fabric/common/configtx"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
)

type CloneableUpdatableBlockVerifier interface {
	// VerifyBlock checks block integrity and its relation to the chain, and verifies the signatures.
	VerifyBlock(block *common.Block) error

	// VerifyBlockAttestation does the same as VerifyBlock, except it assumes block.Data = nil. It therefore does not
	// compute the block.Data.Hash() and compares it to the block.Header.DataHash. This is used when the orderer
	// delivers a block with header & metadata only, as an attestation of block existence.
	VerifyBlockAttestation(block *common.Block) error

	// UpdateConfig sets the config by which blocks are verified. It is assumed that this config block had already been
	// verified using the VerifyBlock method immediately prior to calling this method.
	UpdateConfig(configBlock *common.Block) error

	// UpdateBlockHeader saves the last block header that was verified and handled successfully.
	// This must be called after VerifyBlock and VerifyBlockAttestation and successfully handling the block.
	UpdateBlockHeader(block *common.Block)

	// Clone makes a copy from the current verifier, a copy that can keep being updated independently.
	Clone() CloneableUpdatableBlockVerifier
}

// BlockVerificationAssistant verifies the integrity and signatures of a block stream, while keeping a copy of the
// latest configuration.
//
// Every time a config block arrives, it must first be verified using VerifyBlock and then
// used as an argument to the UpdateConfig method.
// The block stream could be composed of either:
// - full blocks, which are verified using the VerifyBlock method, or
// - block attestations (a header+metadata, with nil data) which are verified using the VerifyBlockAttestation method.
// In both cases, config blocks must arrive in full.
type BlockVerificationAssistant struct {
	channelID string

	// Creates the sigVerifierFunc whenever the configuration is set or updated.
	verifierAssembler *BlockVerifierAssembler
	// Verifies block signature(s). Recreated whenever the configuration is set or updated.
	sigVerifierFunc protoutil.BlockVerifierFunc
	// The current config block header.
	// It may be nil if the BlockVerificationAssistant is created from common.Config and not a config block.
	// After 'UpdateConfig(*common.Block) error' this field is always set.
	configBlockHeader *common.BlockHeader
	// The last block header may include the number only, in case we start from common.Config.
	lastBlockHeader *common.BlockHeader
	// The last block header hash is given when we start from common.Config, and is computed otherwise.
	lastBlockHeaderHash []byte

	logger *flogging.FabricLogger
}

// NewBlockVerificationAssistant creates a new BlockVerificationAssistant from a config block.
// This is used in the orderer, where we always have access to the last config block.
func NewBlockVerificationAssistant(configBlock *common.Block, lastBlock *common.Block, cryptoProvider bccsp.BCCSP, lg *flogging.FabricLogger) (*BlockVerificationAssistant, error) {
	if configBlock == nil {
		return nil, errors.Errorf("config block is nil")
	}
	if configBlock.GetHeader() == nil {
		return nil, errors.Errorf("config block header is nil")
	}
	if !protoutil.IsConfigBlock(configBlock) {
		return nil, errors.New("config block parameter does not carry a config block")
	}
	configIndex, err := protoutil.GetLastConfigIndexFromBlock(configBlock)
	if err != nil {
		return nil, errors.WithMessage(err, "error getting config index from config block")
	}
	if configIndex != configBlock.GetHeader().GetNumber() {
		return nil, errors.Errorf("config block number [%d] is different than its own config index [%d]", configBlock.GetHeader().GetNumber(), configIndex)
	}

	if lastBlock == nil {
		return nil, errors.New("last block is nil")
	}
	if lastBlock.GetHeader() == nil {
		return nil, errors.New("last verified block header is nil")
	}
	if lastBlock.GetHeader().GetNumber() < configBlock.GetHeader().GetNumber() {
		return nil, errors.Errorf("last verified block number [%d] is smaller than the config block number [%d]", lastBlock.GetHeader().GetNumber(), configBlock.GetHeader().GetNumber())
	}
	lastBlockConfigIndex, err := protoutil.GetLastConfigIndexFromBlock(lastBlock)
	if err != nil {
		return nil, errors.WithMessage(err, "error getting config index from last verified block")
	}
	if lastBlockConfigIndex != configBlock.GetHeader().GetNumber() {
		return nil, errors.Errorf("last verified block [%d] config index [%d] is different than the config block number [%d]", lastBlock.GetHeader().GetNumber(), lastBlockConfigIndex, configBlock.GetHeader().GetNumber())
	}

	configTx, err := protoutil.ExtractEnvelope(configBlock, 0)
	if err != nil {
		return nil, errors.WithMessage(err, "error extracting envelope")
	}
	payload, err := protoutil.UnmarshalPayload(configTx.GetPayload())
	if err != nil {
		return nil, errors.WithMessage(err, "error umarshaling envelope to payload")
	}

	if payload.GetHeader() == nil {
		return nil, errors.New("missing channel header")
	}

	chdr, err := protoutil.UnmarshalChannelHeader(payload.GetHeader().GetChannelHeader())
	if err != nil {
		return nil, errors.WithMessage(err, "error unmarshalling channel header")
	}

	configEnvelope, err := configtx.UnmarshalConfigEnvelope(payload.GetData())
	if err != nil {
		return nil, errors.WithMessage(err, "error umarshaling config envelope from payload data")
	}

	bva := &BlockVerifierAssembler{
		Logger: lg,
		BCCSP:  cryptoProvider,
	}
	verifierFunc, err := bva.VerifierFromConfig(configEnvelope, chdr.GetChannelId())
	if err != nil {
		return nil, errors.WithMessage(err, "error creating verifier function")
	}

	a := &BlockVerificationAssistant{
		channelID:           chdr.GetChannelId(),
		verifierAssembler:   bva,
		sigVerifierFunc:     verifierFunc,
		configBlockHeader:   configBlock.GetHeader(),
		lastBlockHeader:     lastBlock.GetHeader(),
		lastBlockHeaderHash: protoutil.BlockHeaderHash(lastBlock.GetHeader()),
		logger:              lg,
	}

	return a, nil
}

// NewBlockVerificationAssistantFromConfig creates a new BlockVerificationAssistant from a common.Config.
// This is used in the peer, since when the peer starts from a snapshot we may not have access to the last config-block,
// only to the config object.
func NewBlockVerificationAssistantFromConfig(config *common.Config, lastBlockNumber uint64, lastBlockHeaderHash []byte, channelID string, cryptoProvider bccsp.BCCSP, lg *flogging.FabricLogger) (*BlockVerificationAssistant, error) {
	if config == nil {
		return nil, errors.Errorf("config is nil")
	}

	if len(lastBlockHeaderHash) == 0 {
		return nil, errors.Errorf("last block header hash is missing")
	}

	bva := &BlockVerifierAssembler{
		Logger: lg,
		BCCSP:  cryptoProvider,
	}
	verifierFunc, err := bva.VerifierFromConfig(&common.ConfigEnvelope{Config: config}, channelID)
	if err != nil {
		return nil, errors.WithMessage(err, "error creating verifier function")
	}

	a := &BlockVerificationAssistant{
		channelID:           channelID,
		verifierAssembler:   bva,
		sigVerifierFunc:     verifierFunc,
		lastBlockHeader:     &common.BlockHeader{Number: lastBlockNumber},
		lastBlockHeaderHash: lastBlockHeaderHash,
		logger:              lg,
	}

	return a, nil
}

func (a *BlockVerificationAssistant) Clone() CloneableUpdatableBlockVerifier {
	c := &BlockVerificationAssistant{
		channelID:           a.channelID,
		verifierAssembler:   a.verifierAssembler,
		sigVerifierFunc:     a.sigVerifierFunc,
		configBlockHeader:   a.configBlockHeader,
		lastBlockHeader:     a.lastBlockHeader,
		lastBlockHeaderHash: a.lastBlockHeaderHash,
		logger:              a.logger,
	}
	return c
}

// UpdateConfig sets the config by which blocks are verified. It is assumed that this config block had already been
// verified using the VerifyBlock method immediately prior to calling this method.
func (a *BlockVerificationAssistant) UpdateConfig(configBlock *common.Block) error {
	configTx, err := protoutil.ExtractEnvelope(configBlock, 0)
	if err != nil {
		return errors.WithMessage(err, "error extracting envelope")
	}

	payload, err := protoutil.UnmarshalPayload(configTx.GetPayload())
	if err != nil {
		return errors.WithMessage(err, "error unmarshalling envelope to payload")
	}

	if payload.GetHeader() == nil {
		return errors.New("missing channel header")
	}

	chdr, err := protoutil.UnmarshalChannelHeader(payload.GetHeader().GetChannelHeader())
	if err != nil {
		return errors.WithMessage(err, "error unmarshalling channel header")
	}

	if chdr.GetChannelId() != a.channelID {
		return errors.Errorf("config block channel ID [%s] does not match expected: [%s]", chdr.GetChannelId(), a.channelID)
	}

	configEnvelope, err := configtx.UnmarshalConfigEnvelope(payload.GetData())
	if err != nil {
		return errors.WithMessage(err, "error unmarshalling config envelope from payload data")
	}

	verifierFunc, err := a.verifierAssembler.VerifierFromConfig(configEnvelope, chdr.GetChannelId())
	if err != nil {
		return errors.WithMessage(err, "error creating verifier function")
	}

	a.configBlockHeader = configBlock.GetHeader()
	a.lastBlockHeader = configBlock.GetHeader()
	a.lastBlockHeaderHash = protoutil.BlockHeaderHash(configBlock.GetHeader())
	a.sigVerifierFunc = verifierFunc

	return nil
}

// VerifyBlock checks block integrity and its relation to the chain, and verifies the signatures.
func (a *BlockVerificationAssistant) VerifyBlock(block *common.Block) error {
	if err := a.verifyHeader(block); err != nil {
		return err
	}

	if err := a.verifyMetadata(block); err != nil {
		return err
	}

	dataHash, err := protoutil.BlockDataHash(block.GetData())
	if err != nil {
		return errors.Wrapf(err, "failed to verify transactions are well formed for block with id [%d] on channel [%s]", block.GetHeader().GetNumber(), a.channelID)
	}

	// Verify that Header.DataHash is equal to the hash of block.Data
	// This is to ensure that the header is consistent with the data carried by this block
	if !bytes.Equal(dataHash, block.GetHeader().GetDataHash()) {
		return errors.Errorf("Header.DataHash is different from Hash(block.Data) for block with id [%d] on channel [%s]; Header: %s, Data: %s",
			block.GetHeader().GetNumber(), a.channelID, hex.EncodeToString(block.GetHeader().GetDataHash()), hex.EncodeToString(dataHash))
	}

	err = a.sigVerifierFunc(block.GetHeader(), block.GetMetadata())
	if err != nil {
		return err
	}

	a.lastBlockHeader = block.GetHeader()
	a.lastBlockHeaderHash = protoutil.BlockHeaderHash(block.GetHeader())

	return nil
}

// VerifyBlockAttestation does the same as VerifyBlock, except it assumes block.Data = nil. It therefore does not
// compute the block.Data.Hash() and compare it to the block.Header.DataHash. This is used when the orderer
// delivers a block with header & metadata only, as an attestation of block existence.
func (a *BlockVerificationAssistant) VerifyBlockAttestation(block *common.Block) error {
	if err := a.verifyHeader(block); err != nil {
		return err
	}

	if err := a.verifyMetadata(block); err != nil {
		return err
	}

	err := a.sigVerifierFunc(block.GetHeader(), block.GetMetadata())
	if err == nil {
		a.lastBlockHeader = block.GetHeader()
		a.lastBlockHeaderHash = protoutil.BlockHeaderHash(block.GetHeader())
	}

	return err
}

// UpdateBlockHeader saves the last block header that was verified and handled successfully.
// This must be called after VerifyBlock and VerifyBlockAttestation and successfully handling the block.
func (a *BlockVerificationAssistant) UpdateBlockHeader(block *common.Block) {
	a.lastBlockHeader = block.GetHeader()
	a.lastBlockHeaderHash = protoutil.BlockHeaderHash(block.GetHeader())
}

func (a *BlockVerificationAssistant) verifyMetadata(block *common.Block) error {
	if block.GetMetadata() == nil || len(block.GetMetadata().GetMetadata()) < len(common.BlockMetadataIndex_name) {
		return errors.Errorf("block with id [%d] on channel [%s] does not have metadata or contains too few entries", block.GetHeader().GetNumber(), a.channelID)
	}

	return nil
}

func (a *BlockVerificationAssistant) verifyHeader(block *common.Block) error {
	if block == nil {
		return errors.Errorf("block must be different from nil, channel=%s", a.channelID)
	}
	if block.GetHeader() == nil {
		return errors.Errorf("invalid block, header must be different from nil, channel=%s", a.channelID)
	}

	expectedBlockNum := a.lastBlockHeader.GetNumber() + 1
	if expectedBlockNum != block.GetHeader().GetNumber() {
		return errors.Errorf("expected block number is [%d] but actual block number inside block is [%d]", expectedBlockNum, block.GetHeader().GetNumber())
	}

	if len(a.lastBlockHeaderHash) != 0 {
		if !bytes.Equal(block.GetHeader().GetPreviousHash(), a.lastBlockHeaderHash) {
			return errors.Errorf("Header.PreviousHash of block [%d] is different from Hash(block.Header) of previous block, on channel [%s], received: %s, expected: %s",
				block.GetHeader().GetNumber(), a.channelID, hex.EncodeToString(block.GetHeader().GetPreviousHash()), hex.EncodeToString(a.lastBlockHeaderHash))
		}
	}
	return nil
}
