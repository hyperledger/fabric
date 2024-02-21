// Copyright IBM Corp. All Rights Reserved.
//
// SPDX-License-Identifier: Apache-2.0

package smartbft_test

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/SmartBFT-Go/consensus/pkg/api"
	"github.com/SmartBFT-Go/consensus/pkg/types"
	"github.com/SmartBFT-Go/consensus/pkg/wal"
	"github.com/hyperledger/fabric-lib-go/bccsp/sw"
	"github.com/hyperledger/fabric-lib-go/common/metrics/disabled"
	cb "github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric/common/crypto/tlsgen"
	"github.com/hyperledger/fabric/core/config/configtest"
	"github.com/hyperledger/fabric/internal/configtxgen/encoder"
	"github.com/hyperledger/fabric/internal/configtxgen/genesisconfig"
	"github.com/hyperledger/fabric/orderer/consensus/smartbft"
	smartBFTMocks "github.com/hyperledger/fabric/orderer/consensus/smartbft/mocks"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type NetworkSetupInfo struct {
	t            *testing.T
	nodeIdToNode map[uint64]*Node
	dir          string
	channelId    string
}

func NewNetworkSetupInfo(t *testing.T, channelId string, rootDir string) *NetworkSetupInfo {
	return &NetworkSetupInfo{
		t:            t,
		nodeIdToNode: map[uint64]*Node{},
		dir:          rootDir,
		channelId:    channelId,
	}
}

func (ns *NetworkSetupInfo) CreateNodes(numberOfNodes int) map[uint64]*Node {
	var nodeIds []uint64
	for nodeId := uint64(1); nodeId <= uint64(numberOfNodes); nodeId++ {
		nodeIds = append(nodeIds, nodeId)
	}
	nodeIdToNode := map[uint64]*Node{}
	for _, nodeId := range nodeIds {
		nodeIdToNode[nodeId] = NewNode(ns.t, nodeId, ns.dir, ns.channelId, ns.nodeIdToNode)
	}
	return nodeIdToNode
}

func (ns *NetworkSetupInfo) StartAllNodes() {
	ns.t.Logf("Starting nodes in the network")
	for _, node := range ns.nodeIdToNode {
		node.Start()
	}
}

type Node struct {
	t                    *testing.T
	NodeId               uint64
	ChannelId            string
	WorkingDir           string
	Chain                *smartbft.BFTChain
	State                *NodeState
	IsStarted            bool
	IsConnectedToNetwork bool
	nodesMap             map[uint64]*Node
	Endpoint             string
	lock                 sync.RWMutex
}

func NewNode(t *testing.T, nodeId uint64, rootDir string, channelId string, nodeIdToNode map[uint64]*Node) *Node {
	t.Logf("Creating node %d", nodeId)
	nodeWorkingDir := filepath.Join(rootDir, fmt.Sprintf("node-%d", nodeId))

	t.Logf("Creating working directory for node %d: %s", nodeId, nodeWorkingDir)
	err := os.Mkdir(nodeWorkingDir, os.ModePerm)
	require.NoError(t, err)

	t.Log("Creating chain")
	node := &Node{
		t:                    t,
		NodeId:               nodeId,
		ChannelId:            channelId,
		WorkingDir:           nodeWorkingDir,
		State:                NewNodeState(t, nodeId, channelId),
		IsStarted:            false,
		IsConnectedToNetwork: false,
		nodesMap:             nodeIdToNode,
		Endpoint:             fmt.Sprintf("%s:%d", "localhost", 9000+nodeId),
	}

	node.Chain, err = createBFTChainUsingMocks(t, node)
	require.NoError(t, err)
	return node
}

func (n *Node) Start() {
	n.lock.Lock()
	defer n.lock.Unlock()
	n.Chain.Start()
	n.IsConnectedToNetwork = true
	n.IsStarted = true
}

type NodeState struct {
	NodeId      uint64
	ledgerArray []*cb.Block
	Sequence    uint64
	lock        sync.RWMutex
	t           *testing.T
}

func NewNodeState(t *testing.T, nodeId uint64, channelId string) *NodeState {
	return &NodeState{
		NodeId:      nodeId,
		ledgerArray: []*cb.Block{createConfigBlock(t, channelId)},
		Sequence:    1,
		t:           t,
	}
}

func (ns *NodeState) AddBlock(block *cb.Block) {
	ns.lock.Lock()
	defer ns.lock.Unlock()
	ns.ledgerArray = append(ns.ledgerArray, block)
}

func (ns *NodeState) GetBlock(idx uint64) *cb.Block {
	ns.lock.RLock()
	defer ns.lock.RUnlock()
	return ns.ledgerArray[idx]
}

func (ns *NodeState) GetLastBlock() *cb.Block {
	ns.lock.RLock()
	defer ns.lock.RUnlock()
	return ns.ledgerArray[len(ns.ledgerArray)-1]
}

func (ns *NodeState) GetLedgerHeight() int {
	ns.lock.Lock()
	defer ns.lock.Unlock()
	return len(ns.ledgerArray)
}

func (ns *NodeState) WaitLedgerHeightToBe(height int) {
	require.Eventually(ns.t, func() bool { return ns.GetLedgerHeight() == height }, 30*time.Second, 100*time.Millisecond)
}

// createBFTChainUsingMocks creates a new bft chain which is exposed to all nodes in the network.
// the chain is created using mocks and is useful for testing
func createBFTChainUsingMocks(t *testing.T, node *Node) (*smartbft.BFTChain, error) {
	nodeId := node.NodeId
	channelId := node.ChannelId

	config := createBFTConfiguration(node)

	comm := smartBFTMocks.NewCommunicator(t)
	comm.EXPECT().Configure(mock.Anything, mock.Anything)

	// the number of blocks is determined by the number of blocks in the node's ledger
	supportMock := smartBFTMocks.NewConsenterSupport(t)
	supportMock.EXPECT().ChannelID().Return(channelId)
	supportMock.EXPECT().Height().Return(uint64(node.State.GetLedgerHeight()))
	supportMock.EXPECT().Block(mock.AnythingOfType("uint64")).RunAndReturn(
		func(blockIdx uint64) *cb.Block {
			return node.State.GetBlock(blockIdx)
		})
	supportMock.EXPECT().Sequence().RunAndReturn(
		func() uint64 {
			return node.State.Sequence
		})
	supportMock.EXPECT().WriteBlock(mock.Anything, mock.Anything).Run(
		func(block *cb.Block, encodedMetadataValue []byte) {
			node.State.AddBlock(block)
		}).Maybe()

	supportMock.EXPECT().WriteConfigBlock(mock.Anything, mock.Anything).Run(
		func(block *cb.Block, encodedMetadataValue []byte) {
			node.State.AddBlock(block)
		}).Maybe()

	mpc := &smartbft.MetricProviderConverter{MetricsProvider: &disabled.Provider{}}
	metricsBFT := api.NewMetrics(mpc, channelId)
	metricsWalBFT := wal.NewMetrics(mpc, channelId)

	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	require.NoError(t, err)

	bftChain, err := smartbft.NewChain(
		nil,
		nodeId,
		config,
		node.WorkingDir,
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

	return bftChain, nil
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

	block := blockWithGroups(channelGroup, channelId, 0)
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

func createBFTConfiguration(node *Node) types.Configuration {
	return types.Configuration{
		SelfID:                        node.NodeId,
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
}
