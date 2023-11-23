/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package deliverclient

import (
	"fmt"
	"os"
	"path"
	"testing"

	"github.com/pkg/errors"

	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric/common/flogging"

	"github.com/hyperledger/fabric/bccsp/sw"
	"github.com/hyperledger/fabric/common/crypto/tlsgen"
	"github.com/hyperledger/fabric/core/config/configtest"
	"github.com/hyperledger/fabric/internal/configtxgen/encoder"
	"github.com/hyperledger/fabric/internal/configtxgen/genesisconfig"
	"github.com/hyperledger/fabric/protoutil"
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
	verifierFunc, err := blockVerifierAssembler.VerifierFromConfig(&common.ConfigEnvelope{Config: config}, "channel1")
	require.NoError(t, err)

	t.Run("creating new block verification assistant succeed", func(t *testing.T) {
		expectedAssistant := BlockVerificationAssistant{
			channelID:           "channel1",
			verifierAssembler:   blockVerifierAssembler,
			sigVerifierFunc:     verifierFunc,
			configBlockHeader:   nil,
			lastBlockHeader:     &common.BlockHeader{Number: 1},
			lastBlockHeaderHash: blockHeaderHash,
			logger:              logger,
		}
		assistant, err := NewBlockVerificationAssistantFromConfig(config, 1, blockHeaderHash, "channel1", cryptoProvider, logger)
		require.NoError(t, err)
		require.Equal(t, expectedAssistant.channelID, assistant.channelID)
		require.Equal(t, expectedAssistant.verifierAssembler, assistant.verifierAssembler)
		require.Equal(t, expectedAssistant.configBlockHeader, assistant.configBlockHeader)
		require.Equal(t, expectedAssistant.lastBlockHeader, assistant.lastBlockHeader)
		require.Equal(t, expectedAssistant.lastBlockHeaderHash, assistant.lastBlockHeaderHash)
		require.Equal(t, assistant.logger, expectedAssistant.logger)
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

func TestBlockVerificationAssistant_VerifyBlock(t *testing.T) {
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

	t.Run("verify block succeed", func(t *testing.T) {
		block := blockWithGroups(group, "channel1", 1)
		lastBlockHeaderHash := []byte{1, 2, 3}
		config := &common.Config{ChannelGroup: group}
		logger := flogging.MustGetLogger("logger")

		assistant, err := NewBlockVerificationAssistantFromConfig(config, 0, lastBlockHeaderHash, "channel1", cryptoProvider, logger)
		require.NoError(t, err)
		assistant.sigVerifierFunc = sigVerifierFuncReturnNoError()

		err = assistant.VerifyBlock(block)
		require.NoError(t, err)
		require.Equal(t, assistant.lastBlockHeader, block.Header)
		require.Equal(t, assistant.lastBlockHeaderHash, protoutil.BlockHeaderHash(block.Header))
	})

	t.Run("channel id is missing in the block", func(t *testing.T) {
		block := blockWithGroups(group, "channel1", 1)
		lastBlockHeaderHash := []byte{1, 2, 3}
		config := &common.Config{ChannelGroup: group}
		logger := flogging.MustGetLogger("logger")

		assistant, err := NewBlockVerificationAssistantFromConfig(config, 0, lastBlockHeaderHash, "channel1", cryptoProvider, logger)
		require.NoError(t, err)
		assistant.sigVerifierFunc = sigVerifierFuncReturnNoError()

		block.Data.Data = nil

		err = assistant.VerifyBlock(block)
		require.Error(t, err)
		require.EqualError(t, err, errors.Errorf("failed getting channel id from block number [%d] on channel [%s]: failed to retrieve channel id - block is empty", block.Header.Number, assistant.channelID).Error())
	})

	t.Run("invalid block's channel id", func(t *testing.T) {
		block := blockWithGroups(group, "channel1", 1)
		lastBlockHeaderHash := []byte{1, 2, 3}
		config := &common.Config{ChannelGroup: group}
		logger := flogging.MustGetLogger("logger")

		assistant, err := NewBlockVerificationAssistantFromConfig(config, 0, lastBlockHeaderHash, "channel2", cryptoProvider, logger)
		require.NoError(t, err)
		assistant.sigVerifierFunc = sigVerifierFuncReturnNoError()

		err = assistant.VerifyBlock(block)
		require.Error(t, err)
		require.EqualError(t, err, errors.Errorf("invalid block's channel id. Expected [%s]. Given [%s]", assistant.channelID, "channel1").Error())
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

		err = assistant.VerifyBlock(block)
		require.Error(t, err)
		require.EqualError(t, err, errors.Errorf("block with id [%d] on channel [%s] does not have metadata", block.Header.Number, assistant.channelID).Error())
	})

	t.Run("config index different from the block number", func(t *testing.T) {
		block := blockWithGroups(group, "channel1", 1)
		lastBlockHeaderHash := []byte{1, 2, 3}
		config := &common.Config{ChannelGroup: group}
		logger := flogging.MustGetLogger("logger")

		assistant, err := NewBlockVerificationAssistantFromConfig(config, 0, lastBlockHeaderHash, "channel1", cryptoProvider, logger)
		require.NoError(t, err)
		assistant.sigVerifierFunc = sigVerifierFuncReturnNoError()

		block.Metadata.Metadata[common.BlockMetadataIndex_SIGNATURES] = protoutil.MarshalOrPanic(&common.Metadata{
			Value: protoutil.MarshalOrPanic(&common.OrdererBlockMetadata{
				LastConfig: &common.LastConfig{
					Index: uint64(0),
				},
			}),
		})
		configIndex, err := protoutil.GetLastConfigIndexFromBlock(block)
		require.NoError(t, err)

		err = assistant.VerifyBlock(block)
		require.Error(t, err)
		require.EqualError(t, err, errors.Errorf("block [%d] is a config block but has config index [%d] that is different than expected (own number), on channel [%s]",
			block.Header.Number, configIndex, assistant.channelID).Error())
	})

	t.Run("Header.DataHash is different from Hash(block.Data)", func(t *testing.T) {
		block := blockWithGroups(group, "channel1", 1)
		lastBlockHeaderHash := []byte{1, 2, 3}
		config := &common.Config{ChannelGroup: group}
		logger := flogging.MustGetLogger("logger")

		assistant, err := NewBlockVerificationAssistantFromConfig(config, 0, lastBlockHeaderHash, "channel1", cryptoProvider, logger)
		require.NoError(t, err)
		assistant.sigVerifierFunc = sigVerifierFuncReturnNoError()

		block.Header.DataHash = []byte{4, 5, 6}

		err = assistant.VerifyBlock(block)
		require.Error(t, err)
		require.EqualError(t, err, errors.Errorf("Header.DataHash is different from Hash(block.Data) for block with id [%d] on channel [%s]", block.Header.Number, assistant.channelID).Error())
	})

	t.Run("sig verifier func return policy error", func(t *testing.T) {
		block := blockWithGroups(group, "channel1", 1)
		lastBlockHeaderHash := []byte{1, 2, 3}
		config := &common.Config{ChannelGroup: group}
		logger := flogging.MustGetLogger("logger")

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
		config := &common.Config{ChannelGroup: group}
		logger := flogging.MustGetLogger("logger")

		assistant, err := NewBlockVerificationAssistantFromConfig(config, 0, lastBlockHeaderHash, "channel1", cryptoProvider, logger)
		require.NoError(t, err)
		assistant.sigVerifierFunc = sigVerifierFuncReturnNoError()

		err = assistant.VerifyBlock(block)
		require.Error(t, err)
		require.Equal(t, err.Error(), errors.Errorf("failed to verify transactions are well formed for block with id [%d] on channel [%s]: transaction 0 has no signature", block.Header.Number, assistant.channelID).Error())
	})

	t.Run("verify non config block succeed", func(t *testing.T) {
		block := nonConfigBlock("channel1", 1)
		lastBlockHeaderHash := []byte{1, 2, 3}
		config := &common.Config{ChannelGroup: group}
		logger := flogging.MustGetLogger("logger")

		assistant, err := NewBlockVerificationAssistantFromConfig(config, 0, lastBlockHeaderHash, "channel1", cryptoProvider, logger)
		require.NoError(t, err)
		assistant.sigVerifierFunc = sigVerifierFuncReturnNoError()

		// add signature and update the block Header.DataHash
		env, err := protoutil.ExtractEnvelope(block, 0)
		require.NoError(t, err)
		env.Signature = []byte{1, 2, 3}
		block.Data.Data[0] = protoutil.MarshalOrPanic(env)
		block.Header.DataHash, err = protoutil.BlockDataHash(block.Data)
		require.NoError(t, err)

		err = assistant.VerifyBlock(block)
		require.NoError(t, err)
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

	t.Run("block data is nil", func(t *testing.T) {
		configBlock := blockWithGroups(group, "channel1", 1)
		lastBlockHeaderHash := []byte{1, 2, 3}
		config := &common.Config{ChannelGroup: group}
		logger := flogging.MustGetLogger("logger")

		assistant, err := NewBlockVerificationAssistantFromConfig(config, 0, lastBlockHeaderHash, "channel1", cryptoProvider, logger)
		require.NoError(t, err)

		configBlock.Data = nil

		err = assistant.UpdateConfig(configBlock)
		require.Error(t, err)
		require.Equal(t, err.Error(), "error extracting envelope: block data is nil")
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
		require.Equal(t, err.Error(), "missing channel header")
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
		require.EqualError(t, err, errors.Errorf("block with id [%d] on channel [%s] does not have metadata", block.Header.Number, assistant.channelID).Error())
	})

	t.Run("config index different from the block number", func(t *testing.T) {
		block := blockWithGroups(group, "channel1", 1)
		lastBlockHeaderHash := []byte{1, 2, 3}
		config := &common.Config{ChannelGroup: group}
		logger := flogging.MustGetLogger("logger")

		assistant, err := NewBlockVerificationAssistantFromConfig(config, 0, lastBlockHeaderHash, "channel1", cryptoProvider, logger)
		require.NoError(t, err)
		assistant.sigVerifierFunc = sigVerifierFuncReturnNoError()

		block.Metadata.Metadata[common.BlockMetadataIndex_SIGNATURES] = protoutil.MarshalOrPanic(&common.Metadata{
			Value: protoutil.MarshalOrPanic(&common.OrdererBlockMetadata{
				LastConfig: &common.LastConfig{
					Index: uint64(0),
				},
			}),
		})
		configIndex, err := protoutil.GetLastConfigIndexFromBlock(block)
		require.NoError(t, err)

		err = assistant.VerifyBlockAttestation(block)
		require.Error(t, err)
		require.EqualError(t, err, errors.Errorf("block [%d] is a config block but has config index [%d] that is different than expected (own number), on channel [%s]",
			block.Header.Number, configIndex, assistant.channelID).Error())
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
				Index: uint64(1),
			},
		}),
	})

	return block
}

func nonConfigBlock(channelID string, blockNumber uint64) *common.Block {
	block := protoutil.NewBlock(blockNumber, []byte{1, 2, 3})
	block.Data = &common.BlockData{
		Data: [][]byte{
			protoutil.MarshalOrPanic(&common.Envelope{
				Payload: protoutil.MarshalOrPanic(&common.Payload{
					Header: &common.Header{
						ChannelHeader: protoutil.MarshalOrPanic(&common.ChannelHeader{
							Type:      int32(common.HeaderType_ENDORSER_TRANSACTION),
							ChannelId: channelID,
						}),
					},
				}),
			}),
		},
	}
	block.Header.DataHash, _ = protoutil.BlockDataHash(block.Data)
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
