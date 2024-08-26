/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package deliverclient

import (
	"encoding/hex"
	"fmt"
	"os"
	"path"
	"testing"

	"github.com/hyperledger/fabric-lib-go/bccsp/sw"
	"github.com/hyperledger/fabric-lib-go/common/flogging"
	"github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/hyperledger/fabric/common/crypto/tlsgen"
	"github.com/hyperledger/fabric/core/config/configtest"
	"github.com/hyperledger/fabric/internal/configtxgen/encoder"
	"github.com/hyperledger/fabric/internal/configtxgen/genesisconfig"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
)

func TestNewBlockVerificationAssistantFromConfig(t *testing.T) {
	certDir := t.TempDir()
	tlsCA, err := tlsgen.NewCA()
	require.NoError(t, err)
	configProfile := genesisconfig.Load(genesisconfig.SampleAppChannelSmartBftProfile, configtest.GetDevConfigDir())
	generateCertificatesSmartBFT(t, configProfile, tlsCA, certDir)
	group, err := encoder.NewChannelGroup(configProfile)
	require.NoError(t, err)
	require.NotNil(t, group)
	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	require.NoError(t, err)

	block := blockWithGroups(group, "channel1", 1)
	blockHeaderHash := protoutil.BlockHeaderHash(block.Header)

	config := &common.Config{ChannelGroup: group}
	logger := flogging.MustGetLogger("logger")
	blockVerifierAssembler := &BlockVerifierAssembler{Logger: logger, BCCSP: cryptoProvider}
	require.NoError(t, err)

	t.Run("creating new block verification assistant succeed", func(t *testing.T) {
		assistant, err := NewBlockVerificationAssistantFromConfig(config, 1, blockHeaderHash, "channel1", cryptoProvider, logger)
		require.NoError(t, err)
		require.Equal(t, "channel1", assistant.channelID)
		require.Equal(t, blockVerifierAssembler, assistant.verifierAssembler)
		require.Nil(t, assistant.configBlockHeader)
		require.Equal(t, &common.BlockHeader{Number: 1}, assistant.lastBlockHeader)
		require.Equal(t, blockHeaderHash, assistant.lastBlockHeaderHash)
		require.Equal(t, logger, assistant.logger)
		err = assistant.sigVerifierFunc(block.Header, block.Metadata)
		require.EqualError(t, err, "signature set did not satisfy policy")
	})

	t.Run("config is nil", func(t *testing.T) {
		assistant, err := NewBlockVerificationAssistantFromConfig(nil, 1, blockHeaderHash, "channel1", cryptoProvider, logger)
		require.Error(t, err)
		require.Nil(t, assistant)
		require.Contains(t, err.Error(), "config is nil")
	})

	t.Run("last block header hash is nil", func(t *testing.T) {
		assistant, err := NewBlockVerificationAssistantFromConfig(config, 1, nil, "channel1", cryptoProvider, logger)
		require.Error(t, err)
		require.Nil(t, assistant)
		require.Contains(t, err.Error(), "last block header hash is missing")
	})
}

func TestNewBlockVerificationAssistantFromConfigBlock(t *testing.T) {
	certDir := t.TempDir()
	tlsCA, err := tlsgen.NewCA()
	require.NoError(t, err)
	configProfile := genesisconfig.Load(genesisconfig.SampleAppChannelSmartBftProfile, configtest.GetDevConfigDir())
	generateCertificatesSmartBFT(t, configProfile, tlsCA, certDir)
	group, err := encoder.NewChannelGroup(configProfile)
	require.NoError(t, err)
	require.NotNil(t, group)
	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	require.NoError(t, err)

	configBlock := blockWithGroups(group, "channel1", 0)
	lastBlock := nonConfigBlock("channel1", 10)
	nextBlock := nonConfigBlock("channel1", 11)
	logger := flogging.MustGetLogger("logger")

	blockVerifierAssembler := &BlockVerifierAssembler{Logger: logger, BCCSP: cryptoProvider}
	require.NoError(t, err)

	t.Run("success: creating new block verification assistant succeed", func(t *testing.T) {
		assistant, err := NewBlockVerificationAssistant(configBlock, lastBlock, cryptoProvider, logger)
		require.NoError(t, err)
		require.Equal(t, "channel1", assistant.channelID)
		require.Equal(t, blockVerifierAssembler, assistant.verifierAssembler)
		require.Equal(t, configBlock.Header, assistant.configBlockHeader)
		require.Equal(t, lastBlock.Header, assistant.lastBlockHeader)
		require.Equal(t, protoutil.BlockHeaderHash(lastBlock.Header), assistant.lastBlockHeaderHash)
		require.Equal(t, logger, assistant.logger)
		err = assistant.sigVerifierFunc(nextBlock.Header, nextBlock.Metadata)
		require.EqualError(t, err, "signature set did not satisfy policy")
	})

	t.Run("config block is nil", func(t *testing.T) {
		assistant, err := NewBlockVerificationAssistant(nil, lastBlock, cryptoProvider, logger)
		require.Error(t, err)
		require.Nil(t, assistant)
		require.EqualError(t, err, "config block is nil")
	})

	t.Run("last block header is nil", func(t *testing.T) {
		lastBlock2 := nonConfigBlock("channel1", 10)
		lastBlock2.Header = nil
		assistant, err := NewBlockVerificationAssistant(configBlock, lastBlock2, cryptoProvider, logger)
		require.Error(t, err)
		require.Nil(t, assistant)
		require.EqualError(t, err, "last verified block header is nil")
	})

	t.Run("last block config index mismatch", func(t *testing.T) {
		configBlock1 := blockWithGroups(group, "channel1", 3)
		assistant, err := NewBlockVerificationAssistant(configBlock1, lastBlock, cryptoProvider, logger)
		require.Error(t, err)
		require.Nil(t, assistant)
		require.EqualError(t, err, "last verified block [10] config index [0] is different than the config block number [3]")
	})

	t.Run("last block number mismatch", func(t *testing.T) {
		configBlock1 := blockWithGroups(group, "channel1", 3)
		lastBlock2 := nonConfigBlock("channel1", 2)
		assistant, err := NewBlockVerificationAssistant(configBlock1, lastBlock2, cryptoProvider, logger)
		require.Error(t, err)
		require.Nil(t, assistant)
		require.EqualError(t, err, "last verified block number [2] is smaller than the config block number [3]")
	})
}

func TestBlockVerificationAssistant_VerifyBlock(t *testing.T) {
	certDir := t.TempDir()
	tlsCA, err := tlsgen.NewCA()
	require.NoError(t, err)
	configProfile := genesisconfig.Load(genesisconfig.SampleAppChannelSmartBftProfile, configtest.GetDevConfigDir())
	generateCertificatesSmartBFT(t, configProfile, tlsCA, certDir)
	group, err := encoder.NewChannelGroup(configProfile)
	require.NoError(t, err)
	require.NotNil(t, group)
	config := &common.Config{ChannelGroup: group}
	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	require.NoError(t, err)
	logger := flogging.MustGetLogger("logger")

	t.Run("success: verify config block ", func(t *testing.T) {
		block := blockWithGroups(group, "channel1", 1)
		lastBlockHeaderHash := []byte{1, 2, 3}

		assistant, err := NewBlockVerificationAssistantFromConfig(config, 0, lastBlockHeaderHash, "channel1", cryptoProvider, logger)
		require.NoError(t, err)
		assistant.sigVerifierFunc = sigVerifierFuncReturnNoError()

		err = assistant.VerifyBlock(block)
		require.NoError(t, err)
		require.Equal(t, assistant.lastBlockHeader, block.Header)
		require.Equal(t, assistant.lastBlockHeaderHash, protoutil.BlockHeaderHash(block.Header))
	})

	t.Run("success: verify non config block", func(t *testing.T) {
		block := nonConfigBlock("channel1", 1)
		lastBlockHeaderHash := []byte{1, 2, 3}

		assistant, err := NewBlockVerificationAssistantFromConfig(config, 0, lastBlockHeaderHash, "channel1", cryptoProvider, logger)
		require.NoError(t, err)
		assistant.sigVerifierFunc = sigVerifierFuncReturnNoError()

		err = assistant.VerifyBlock(block)
		require.NoError(t, err)
	})

	t.Run("invalid block: empty", func(t *testing.T) {
		block := blockWithGroups(group, "channel1", 1)
		lastBlockHeaderHash := []byte{1, 2, 3}

		assistant, err := NewBlockVerificationAssistantFromConfig(config, 0, lastBlockHeaderHash, "channel1", cryptoProvider, logger)
		require.NoError(t, err)
		assistant.sigVerifierFunc = sigVerifierFuncReturnNoError()

		block.Data.Data = [][]byte{}

		err = assistant.VerifyBlock(block)
		require.Error(t, err)
		require.EqualError(t, err, "failed to verify transactions are well formed for block with id [1] on channel [channel1]: empty block")

		block.Data.Data = nil

		err = assistant.VerifyBlock(block)
		require.Error(t, err)
		require.EqualError(t, err, "failed to verify transactions are well formed for block with id [1] on channel [channel1]: empty block")

		block.Data = nil

		err = assistant.VerifyBlock(block)
		require.Error(t, err)
		require.EqualError(t, err, "failed to verify transactions are well formed for block with id [1] on channel [channel1]: empty block")

		err = assistant.VerifyBlock(nil)
		require.Error(t, err)
		require.EqualError(t, err, "block must be different from nil, channel=channel1")
	})

	t.Run("invalid block TXs: invalid TXs", func(t *testing.T) {
		block := blockWithGroups(group, "channel1", 1)
		lastBlockHeaderHash := []byte{1, 2, 3}

		assistant, err := NewBlockVerificationAssistantFromConfig(config, 0, lastBlockHeaderHash, "channel1", cryptoProvider, logger)
		require.NoError(t, err)
		assistant.sigVerifierFunc = sigVerifierFuncReturnNoError()

		block.Data.Data = [][]byte{{0x66, 0x77}, {0x88, 0x99}}

		err = assistant.VerifyBlock(block)
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed to verify transactions are well formed for block with id [1] on channel [channel1]: transaction 0 is invalid:")
	})

	t.Run("bad metadata", func(t *testing.T) {
		block := blockWithGroups(group, "channel1", 1)
		lastBlockHeaderHash := []byte{1, 2, 3}

		assistant, err := NewBlockVerificationAssistantFromConfig(config, 0, lastBlockHeaderHash, "channel1", cryptoProvider, logger)
		require.NoError(t, err)
		assistant.sigVerifierFunc = sigVerifierFuncReturnNoError()

		block.Metadata = nil

		err = assistant.VerifyBlock(block)
		require.Error(t, err)
		require.EqualError(t, err, "block with id [1] on channel [channel1] does not have metadata or contains too few entries")

		block.Metadata = &common.BlockMetadata{Metadata: nil}
		require.Error(t, err)
		require.EqualError(t, err, "block with id [1] on channel [channel1] does not have metadata or contains too few entries")
	})

	t.Run("invalid header: empty", func(t *testing.T) {
		block := nonConfigBlock("channel1", 1)
		lastBlockHeaderHash := []byte{1, 2, 3}
		assistant, err := NewBlockVerificationAssistantFromConfig(config, 0, lastBlockHeaderHash, "channel1", cryptoProvider, logger)
		require.NoError(t, err)
		assistant.sigVerifierFunc = sigVerifierFuncReturnNoError()

		block.Header.Number = 2
		err = assistant.VerifyBlock(block)
		require.Error(t, err)
		require.EqualError(t, err, "expected block number is [1] but actual block number inside block is [2]")

		block.Header = nil
		err = assistant.VerifyBlock(block)
		require.Error(t, err)
		require.EqualError(t, err, "invalid block, header must be different from nil, channel=channel1")
	})

	t.Run("invalid header: wrong number", func(t *testing.T) {
		block := nonConfigBlock("channel1", 1)
		lastBlockHeaderHash := []byte{1, 2, 3}
		assistant, err := NewBlockVerificationAssistantFromConfig(config, 0, lastBlockHeaderHash, "channel1", cryptoProvider, logger)
		require.NoError(t, err)
		assistant.sigVerifierFunc = sigVerifierFuncReturnNoError()

		block.Header.Number = 2
		err = assistant.VerifyBlock(block)
		require.Error(t, err)
		require.EqualError(t, err, "expected block number is [1] but actual block number inside block is [2]")
	})

	t.Run("Header.DataHash is different from Hash(block.Data)", func(t *testing.T) {
		block := blockWithGroups(group, "channel1", 1)
		lastBlockHeaderHash := []byte{1, 2, 3}
		assistant, err := NewBlockVerificationAssistantFromConfig(config, 0, lastBlockHeaderHash, "channel1", cryptoProvider, logger)
		require.NoError(t, err)
		assistant.sigVerifierFunc = sigVerifierFuncReturnNoError()

		expectedDataHash := block.Header.DataHash
		block.Header.DataHash = []byte{4, 5, 6}

		err = assistant.VerifyBlock(block)
		require.Error(t, err)
		require.EqualError(t, err, fmt.Sprintf("Header.DataHash is different from Hash(block.Data) for block with id [1] on channel [channel1]; Header: 040506, Data: %s", hex.EncodeToString(expectedDataHash)))
	})

	t.Run("Header.PreviousHash is different from expected", func(t *testing.T) {
		block := blockWithGroups(group, "channel1", 1)
		lastBlockHeaderHash := []byte{2, 3, 4}

		assistant, err := NewBlockVerificationAssistantFromConfig(config, 0, lastBlockHeaderHash, "channel1", cryptoProvider, logger)
		require.NoError(t, err)
		assistant.sigVerifierFunc = sigVerifierFuncReturnNoError()

		block.Header.DataHash = []byte{4, 5, 6}

		err = assistant.VerifyBlock(block)
		require.Error(t, err)
		require.EqualError(t, err, "Header.PreviousHash of block [1] is different from Hash(block.Header) of previous block, on channel [channel1], received: 010203, expected: 020304")
	})

	t.Run("sig verifier func return policy error", func(t *testing.T) {
		block := blockWithGroups(group, "channel1", 1)
		lastBlockHeaderHash := []byte{1, 2, 3}

		assistant, err := NewBlockVerificationAssistantFromConfig(config, 0, lastBlockHeaderHash, "channel1", cryptoProvider, logger)
		require.NoError(t, err)
		assistant.sigVerifierFunc = sigVerifierFuncReturnPolicyError()

		err = assistant.VerifyBlock(block)
		require.Error(t, err)
		require.EqualError(t, err, errors.Errorf("signature set did not satisfy policy").Error())
	})

	t.Run("non config block with tx not well formed", func(t *testing.T) {
		block := nonConfigBlock("channel1", 1)
		lastBlockHeaderHash := []byte{1, 2, 3}

		assistant, err := NewBlockVerificationAssistantFromConfig(config, 0, lastBlockHeaderHash, "channel1", cryptoProvider, logger)
		require.NoError(t, err)
		assistant.sigVerifierFunc = sigVerifierFuncReturnNoError()

		txEnv := &common.Envelope{
			Payload: protoutil.MarshalOrPanic(&common.Payload{
				Header: &common.Header{
					ChannelHeader: protoutil.MarshalOrPanic(&common.ChannelHeader{
						Type:      int32(common.HeaderType_ENDORSER_TRANSACTION),
						ChannelId: "channel1",
					}),
				},
			}),
		}
		block.Data.Data[0] = protoutil.MarshalOrPanic(txEnv)

		err = assistant.VerifyBlock(block)
		require.Error(t, err)
		require.Equal(t, err.Error(), errors.Errorf("failed to verify transactions are well formed for block with id [%d] on channel [%s]: transaction 0 has no signature", block.Header.Number, assistant.channelID).Error())
	})
}

func TestBlockVerificationAssistant_UpdateConfig(t *testing.T) {
	certDir := t.TempDir()
	tlsCA, err := tlsgen.NewCA()
	require.NoError(t, err)
	configProfile := genesisconfig.Load(genesisconfig.SampleAppChannelSmartBftProfile, configtest.GetDevConfigDir())
	generateCertificatesSmartBFT(t, configProfile, tlsCA, certDir)
	group, err := encoder.NewChannelGroup(configProfile)
	require.NoError(t, err)
	require.NotNil(t, group)
	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	require.NoError(t, err)

	t.Run("update config succeed", func(t *testing.T) {
		configBlock := blockWithGroups(group, "channel1", 1)
		lastBlockHeaderHash := []byte{1, 2, 3}
		config := &common.Config{ChannelGroup: group}
		logger := flogging.MustGetLogger("logger")

		assistant, err := NewBlockVerificationAssistantFromConfig(config, 0, lastBlockHeaderHash, "channel1", cryptoProvider, logger)
		require.NoError(t, err)

		expectedVerifierFunc, err := assistant.verifierAssembler.VerifierFromConfig(&common.ConfigEnvelope{Config: config}, "channel1")
		require.NoError(t, err)

		err = assistant.UpdateConfig(configBlock)
		require.NoError(t, err)
		require.Equal(t, assistant.configBlockHeader, configBlock.Header)
		require.Equal(t, assistant.lastBlockHeader, configBlock.Header)
		require.Equal(t, assistant.lastBlockHeaderHash, protoutil.BlockHeaderHash(configBlock.Header))
		require.Equal(t, assistant.sigVerifierFunc(configBlock.Header, configBlock.Metadata).Error(), expectedVerifierFunc(configBlock.Header, configBlock.Metadata).Error())
	})

	t.Run("block data is corrupt", func(t *testing.T) {
		configBlock := blockWithGroups(group, "channel1", 1)
		lastBlockHeaderHash := []byte{1, 2, 3}
		config := &common.Config{ChannelGroup: group}
		logger := flogging.MustGetLogger("logger")

		assistant, err := NewBlockVerificationAssistantFromConfig(config, 0, lastBlockHeaderHash, "channel1", cryptoProvider, logger)
		require.NoError(t, err)

		configTx, err := protoutil.ExtractEnvelope(configBlock, 0)
		require.NoError(t, err)
		configTx.Payload = []byte{1, 2, 3, 4}
		configBlock.Data.Data[0] = protoutil.MarshalOrPanic(configTx)
		err = assistant.UpdateConfig(configBlock)
		require.Error(t, err)
		require.Contains(t, err.Error(), "error unmarshalling envelope to payload: error unmarshalling Payload: proto:")

		configBlock.Data.Data = nil

		err = assistant.UpdateConfig(configBlock)
		require.Error(t, err)
		require.EqualError(t, err, "error extracting envelope: envelope index out of bounds")

		configBlock.Data = nil

		err = assistant.UpdateConfig(configBlock)
		require.Error(t, err)
		require.EqualError(t, err, "error extracting envelope: block data is nil")
	})

	t.Run("missing channel header", func(t *testing.T) {
		configBlock := blockWithGroups(group, "channel1", 1)
		lastBlockHeaderHash := []byte{1, 2, 3}
		config := &common.Config{ChannelGroup: group}
		logger := flogging.MustGetLogger("logger")

		assistant, err := NewBlockVerificationAssistantFromConfig(config, 0, lastBlockHeaderHash, "channel1", cryptoProvider, logger)
		require.NoError(t, err)

		configBlock.Data.Data[0] = protoutil.MarshalOrPanic(&common.Envelope{
			Payload: protoutil.MarshalOrPanic(&common.Payload{
				Header: nil,
			}),
		})

		err = assistant.UpdateConfig(configBlock)
		require.Error(t, err)
		require.EqualError(t, err, "missing channel header")
	})

	t.Run("config block channel id is different from the assistant channel id", func(t *testing.T) {
		configBlock := blockWithGroups(group, "channel1", 1)
		lastBlockHeaderHash := []byte{1, 2, 3}
		config := &common.Config{ChannelGroup: group}
		logger := flogging.MustGetLogger("logger")

		assistant, err := NewBlockVerificationAssistantFromConfig(config, 0, lastBlockHeaderHash, "channel2", cryptoProvider, logger)
		require.NoError(t, err)

		blockChannelId, err := protoutil.GetChannelIDFromBlock(configBlock)
		require.NoError(t, err)

		err = assistant.UpdateConfig(configBlock)
		require.Error(t, err)
		require.Equal(t, err.Error(), errors.Errorf("config block channel ID [%s] does not match expected: [%s]", blockChannelId, assistant.channelID).Error())
	})
}

func TestBlockVerificationAssistant_UpdateBlockHeader(t *testing.T) {
	certDir := t.TempDir()
	tlsCA, err := tlsgen.NewCA()
	require.NoError(t, err)
	configProfile := genesisconfig.Load(genesisconfig.SampleAppChannelSmartBftProfile, configtest.GetDevConfigDir())
	generateCertificatesSmartBFT(t, configProfile, tlsCA, certDir)
	group, err := encoder.NewChannelGroup(configProfile)
	require.NoError(t, err)
	require.NotNil(t, group)
	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	require.NoError(t, err)

	block := blockWithGroups(group, "channel1", 1)
	lastBlockHeaderHash := []byte{1, 2, 3}
	config := &common.Config{ChannelGroup: group}
	logger := flogging.MustGetLogger("logger")

	assistant, err := NewBlockVerificationAssistantFromConfig(config, 0, lastBlockHeaderHash, "channel2", cryptoProvider, logger)
	require.NoError(t, err)

	assistant.UpdateBlockHeader(block)
	require.Equal(t, assistant.lastBlockHeader, block.Header)
	require.Equal(t, assistant.lastBlockHeaderHash, protoutil.BlockHeaderHash(block.Header))
}

func TestBlockVerificationAssistant_VerifyBlockAttestation(t *testing.T) {
	certDir := t.TempDir()
	tlsCA, err := tlsgen.NewCA()
	require.NoError(t, err)
	configProfile := genesisconfig.Load(genesisconfig.SampleAppChannelSmartBftProfile, configtest.GetDevConfigDir())
	generateCertificatesSmartBFT(t, configProfile, tlsCA, certDir)
	group, err := encoder.NewChannelGroup(configProfile)
	require.NoError(t, err)
	require.NotNil(t, group)
	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	require.NoError(t, err)

	t.Run("verify non config block attestation with data succeed", func(t *testing.T) {
		block := nonConfigBlock("channel1", 1)
		lastBlockHeaderHash := []byte{1, 2, 3}
		config := &common.Config{ChannelGroup: group}
		logger := flogging.MustGetLogger("logger")

		assistant, err := NewBlockVerificationAssistantFromConfig(config, 0, lastBlockHeaderHash, "channel1", cryptoProvider, logger)
		require.NoError(t, err)
		assistant.sigVerifierFunc = sigVerifierFuncReturnNoError()

		err = assistant.VerifyBlockAttestation(block)
		require.NoError(t, err)
		require.Equal(t, assistant.lastBlockHeader, block.Header)
		require.Equal(t, assistant.lastBlockHeaderHash, protoutil.BlockHeaderHash(block.Header))
	})

	t.Run("verify non block attestation with nil data succeed", func(t *testing.T) {
		block := nonConfigBlock("channel1", 1)
		lastBlockHeaderHash := []byte{1, 2, 3}
		config := &common.Config{ChannelGroup: group}
		logger := flogging.MustGetLogger("logger")

		assistant, err := NewBlockVerificationAssistantFromConfig(config, 0, lastBlockHeaderHash, "channel1", cryptoProvider, logger)
		require.NoError(t, err)
		assistant.sigVerifierFunc = sigVerifierFuncReturnNoError()

		block.Data = nil

		err = assistant.VerifyBlockAttestation(block)
		require.NoError(t, err)
	})

	t.Run("nil metadata", func(t *testing.T) {
		block := blockWithGroups(group, "channel1", 1)
		lastBlockHeaderHash := []byte{1, 2, 3}
		config := &common.Config{ChannelGroup: group}
		logger := flogging.MustGetLogger("logger")

		assistant, err := NewBlockVerificationAssistantFromConfig(config, 0, lastBlockHeaderHash, "channel1", cryptoProvider, logger)
		require.NoError(t, err)
		assistant.sigVerifierFunc = sigVerifierFuncReturnNoError()

		block.Metadata = nil

		err = assistant.VerifyBlockAttestation(block)
		require.Error(t, err)
		require.EqualError(t, err, "block with id [1] on channel [channel1] does not have metadata or contains too few entries")
	})

	t.Run("sig verifier func return policy error", func(t *testing.T) {
		block := blockWithGroups(group, "channel1", 1)
		lastBlockHeaderHash := []byte{1, 2, 3}
		config := &common.Config{ChannelGroup: group}
		logger := flogging.MustGetLogger("logger")

		assistant, err := NewBlockVerificationAssistantFromConfig(config, 0, lastBlockHeaderHash, "channel1", cryptoProvider, logger)
		require.NoError(t, err)
		assistant.sigVerifierFunc = sigVerifierFuncReturnPolicyError()

		err = assistant.VerifyBlockAttestation(block)
		require.Error(t, err)
		require.EqualError(t, err, errors.Errorf("signature set did not satisfy policy").Error())
	})
}

func TestBlockVerificationAssistant_Clone(t *testing.T) {
	certDir := t.TempDir()
	tlsCA, err := tlsgen.NewCA()
	require.NoError(t, err)
	configProfile := genesisconfig.Load(genesisconfig.SampleAppChannelSmartBftProfile, configtest.GetDevConfigDir())
	generateCertificatesSmartBFT(t, configProfile, tlsCA, certDir)
	group, err := encoder.NewChannelGroup(configProfile)
	require.NoError(t, err)
	require.NotNil(t, group)
	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	require.NoError(t, err)

	lastBlockHeaderHash := []byte{1, 2, 3}
	config := &common.Config{ChannelGroup: group}
	logger := flogging.MustGetLogger("logger")

	assistant, err := NewBlockVerificationAssistantFromConfig(config, 0, lastBlockHeaderHash, "channel1", cryptoProvider, logger)
	require.NoError(t, err)
	cloned := assistant.Clone().(*BlockVerificationAssistant)

	require.Equal(t, assistant.channelID, cloned.channelID)
	require.Equal(t, assistant.verifierAssembler, cloned.verifierAssembler)
	require.Equal(t, assistant.configBlockHeader, cloned.configBlockHeader)
	require.Equal(t, assistant.lastBlockHeader, cloned.lastBlockHeader)
	require.Equal(t, assistant.lastBlockHeaderHash, cloned.lastBlockHeaderHash)
	require.Equal(t, assistant.logger, cloned.logger)
}

func generateCertificatesSmartBFT(t *testing.T, confAppSmartBFT *genesisconfig.Profile, tlsCA tlsgen.CA, certDir string) {
	for i, c := range confAppSmartBFT.Orderer.ConsenterMapping {
		t.Logf("BFT Consenter: %+v", c)
		srvC, err := tlsCA.NewServerCertKeyPair(c.Host)
		require.NoError(t, err)
		srvP := path.Join(certDir, fmt.Sprintf("server%d.crt", i))
		err = os.WriteFile(srvP, srvC.Cert, 0o644)
		require.NoError(t, err)

		clnC, err := tlsCA.NewClientCertKeyPair()
		require.NoError(t, err)
		clnP := path.Join(certDir, fmt.Sprintf("client%d.crt", i))
		err = os.WriteFile(clnP, clnC.Cert, 0o644)
		require.NoError(t, err)

		c.Identity = srvP
		c.ServerTLSCert = srvP
		c.ClientTLSCert = clnP
	}
}

func blockWithGroups(groups *common.ConfigGroup, channelID string, blockNumber uint64) *common.Block {
	block := protoutil.NewBlock(blockNumber, []byte{1, 2, 3})
	block.Data = &common.BlockData{
		Data: [][]byte{
			protoutil.MarshalOrPanic(&common.Envelope{
				Payload: protoutil.MarshalOrPanic(&common.Payload{
					Data: protoutil.MarshalOrPanic(&common.ConfigEnvelope{
						Config: &common.Config{
							ChannelGroup: groups,
						},
					}),
					Header: &common.Header{
						ChannelHeader: protoutil.MarshalOrPanic(&common.ChannelHeader{
							Type:      int32(common.HeaderType_CONFIG),
							ChannelId: channelID,
						}),
					},
				}),
			}),
		},
	}
	block.Header.DataHash = protoutil.ComputeBlockDataHash(block.Data)
	block.Metadata.Metadata[common.BlockMetadataIndex_SIGNATURES] = protoutil.MarshalOrPanic(&common.Metadata{
		Value: protoutil.MarshalOrPanic(&common.OrdererBlockMetadata{
			LastConfig: &common.LastConfig{
				Index: uint64(blockNumber),
			},
		}),
	})

	return block
}

func nonConfigBlock(channelID string, blockNumber uint64) *common.Block {
	block := protoutil.NewBlock(blockNumber, []byte{1, 2, 3})

	txEnv := &common.Envelope{
		Payload: protoutil.MarshalOrPanic(&common.Payload{
			Header: &common.Header{
				ChannelHeader: protoutil.MarshalOrPanic(&common.ChannelHeader{
					Type:      int32(common.HeaderType_ENDORSER_TRANSACTION),
					ChannelId: channelID,
				}),
			},
		}),
		Signature: []byte{1, 2, 3},
	}

	block.Data = &common.BlockData{
		Data: [][]byte{
			protoutil.MarshalOrPanic(txEnv),
		},
	}

	block.Header.DataHash = protoutil.ComputeBlockDataHash(block.Data)
	protoutil.InitBlockMetadata(block)

	return block
}

func sigVerifierFuncReturnNoError() func(header *common.BlockHeader, metadata *common.BlockMetadata) error {
	return func(header *common.BlockHeader, metadata *common.BlockMetadata) error {
		return nil
	}
}

func sigVerifierFuncReturnPolicyError() func(header *common.BlockHeader, metadata *common.BlockMetadata) error {
	return func(header *common.BlockHeader, metadata *common.BlockMetadata) error {
		return errors.New("signature set did not satisfy policy")
	}
}
