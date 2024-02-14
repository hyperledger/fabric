// Copyright IBM Corp. All Rights Reserved.
//
// SPDX-License-Identifier: Apache-2.0

package smartbft_test

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"testing"
	"time"

	cb "github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric/common/crypto/tlsgen"
	"github.com/hyperledger/fabric/core/config/configtest"
	"github.com/hyperledger/fabric/internal/configtxgen/encoder"
	"github.com/hyperledger/fabric/internal/configtxgen/genesisconfig"
	smartBFTMocks "github.com/hyperledger/fabric/orderer/consensus/smartbft/mocks"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/stretchr/testify/mock"

	"github.com/SmartBFT-Go/consensus/pkg/api"
	"github.com/SmartBFT-Go/consensus/pkg/types"
	"github.com/SmartBFT-Go/consensus/pkg/wal"
	"github.com/hyperledger/fabric/bccsp/sw"
	"github.com/hyperledger/fabric/common/metrics/disabled"
	"github.com/hyperledger/fabric/orderer/consensus/smartbft"
	"github.com/stretchr/testify/require"
)

// TestNewChain create a new bft chain which will be in the future exposed to all nodes in the network.
// the chain is created using mocks
func TestNewChain(t *testing.T) {
	nodeId := uint64(1)
	channelId := "testchannel"
	block := createConfigBlock(t, channelId)

	// In the future we will have a node that holds a ptr to the chain, so SelfID should be changed to the corresponding node id.
	config := types.Configuration{
		SelfID:                        nodeId,
		RequestBatchMaxCount:          100,
		RequestBatchMaxBytes:          10485760,
		RequestBatchMaxInterval:       50 * time.Millisecond,
		IncomingMessageBufferSize:     200,
		RequestPoolSize:               400,
		RequestForwardTimeout:         2 * time.Second,
		RequestComplainTimeout:        20 * time.Second,
		RequestAutoRemoveTimeout:      3 * time.Minute,
		ViewChangeResendInterval:      5 * time.Second,
		ViewChangeTimeout:             20 * time.Second,
		LeaderHeartbeatTimeout:        1 * time.Minute,
		LeaderHeartbeatCount:          10,
		NumOfTicksBehindBeforeSyncing: types.DefaultConfig.NumOfTicksBehindBeforeSyncing,
		CollectTimeout:                1 * time.Second,
		SyncOnStart:                   types.DefaultConfig.SyncOnStart,
		SpeedUpViewChange:             types.DefaultConfig.SpeedUpViewChange,
		LeaderRotation:                types.DefaultConfig.LeaderRotation,
		DecisionsPerLeader:            types.DefaultConfig.DecisionsPerLeader,
		RequestMaxBytes:               types.DefaultConfig.RequestMaxBytes,
		RequestPoolSubmitTimeout:      types.DefaultConfig.RequestPoolSubmitTimeout,
	}

	rootDir := t.TempDir()
	walDir := filepath.Join(rootDir, "wal")
	err := os.Mkdir(walDir, os.ModePerm)
	require.NoError(t, err)

	comm := smartBFTMocks.NewCommunicator(t)
	comm.EXPECT().Configure(mock.Anything, mock.Anything)

	// the number of blocks should be determined by the number of blocks in the ledger, for now lets assume we have one block
	supportMock := smartBFTMocks.NewConsenterSupport(t)
	supportMock.EXPECT().ChannelID().Return(channelId)
	supportMock.EXPECT().Height().Return(uint64(1))
	supportMock.EXPECT().Block(mock.AnythingOfType("uint64")).Return(block)
	supportMock.EXPECT().Sequence().Return(uint64(1))

	mpc := &smartbft.MetricProviderConverter{MetricsProvider: &disabled.Provider{}}
	metricsBFT := api.NewMetrics(mpc, channelId)
	metricsWalBFT := wal.NewMetrics(mpc, channelId)

	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	require.NoError(t, err)

	bftChain, err := smartbft.NewChain(
		nil,
		nodeId,
		config,
		walDir,
		nil,
		comm,
		nil,
		nil,
		supportMock,
		smartbft.NewMetrics(&disabled.Provider{}),
		metricsBFT,
		metricsWalBFT,
		cryptoProvider)

	require.NoError(t, err)
	require.NotNil(t, bftChain)
}

func createConfigBlock(t *testing.T, channelId string) *cb.Block {
	certDir := t.TempDir()
	tlsCA, err := tlsgen.NewCA()
	require.NoError(t, err)
	configProfile := genesisconfig.Load(genesisconfig.SampleAppChannelSmartBftProfile, configtest.GetDevConfigDir())
	tlsCertPath := filepath.Join(configtest.GetDevConfigDir(), "msp", "tlscacerts", "tlsroot.pem")

	// make all BFT nodes belong to the same MSP ID
	for _, consenter := range configProfile.Orderer.ConsenterMapping {
		consenter.MSPID = "SampleOrg"
		consenter.Identity = tlsCertPath
		consenter.ClientTLSCert = tlsCertPath
		consenter.ServerTLSCert = tlsCertPath
	}

	generateCertificatesSmartBFT(t, configProfile, tlsCA, certDir)

	channelGroup, err := encoder.NewChannelGroup(configProfile)
	require.NoError(t, err)
	require.NotNil(t, channelGroup)

	_, err = sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	require.NoError(t, err)

	block := blockWithGroups(channelGroup, channelId, 1)
	return block
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

func blockWithGroups(groups *cb.ConfigGroup, channelID string, blockNumber uint64) *cb.Block {
	block := protoutil.NewBlock(blockNumber, nil)
	block.Data = &cb.BlockData{
		Data: [][]byte{
			protoutil.MarshalOrPanic(&cb.Envelope{
				Payload: protoutil.MarshalOrPanic(&cb.Payload{
					Data: protoutil.MarshalOrPanic(&cb.ConfigEnvelope{
						Config: &cb.Config{
							ChannelGroup: groups,
						},
					}),
					Header: &cb.Header{
						ChannelHeader: protoutil.MarshalOrPanic(&cb.ChannelHeader{
							Type:      int32(cb.HeaderType_CONFIG),
							ChannelId: channelID,
						}),
					},
				}),
			}),
		},
	}
	block.Header.DataHash = protoutil.ComputeBlockDataHash(block.Data)
	block.Metadata.Metadata[cb.BlockMetadataIndex_SIGNATURES] = protoutil.MarshalOrPanic(&cb.Metadata{
		Value: protoutil.MarshalOrPanic(&cb.OrdererBlockMetadata{
			LastConfig: &cb.LastConfig{
				Index: uint64(blockNumber),
			},
		}),
	})

	return block
}
