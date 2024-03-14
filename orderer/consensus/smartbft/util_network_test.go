// Copyright IBM Corp. All Rights Reserved.
//
// SPDX-License-Identifier: Apache-2.0

package smartbft_test

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"sort"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hyperledger-labs/SmartBFT/pkg/api"
	"github.com/hyperledger-labs/SmartBFT/pkg/types"
	"github.com/hyperledger-labs/SmartBFT/pkg/wal"
	"github.com/hyperledger-labs/SmartBFT/smartbftprotos"
	"github.com/hyperledger/fabric-lib-go/bccsp/sw"
	"github.com/hyperledger/fabric-lib-go/common/metrics/disabled"
	cb "github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric/common/crypto/tlsgen"
	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/core/config/configtest"
	"github.com/hyperledger/fabric/internal/configtxgen/encoder"
	"github.com/hyperledger/fabric/internal/configtxgen/genesisconfig"
	"github.com/hyperledger/fabric/orderer/common/cluster"
	"github.com/hyperledger/fabric/orderer/common/localconfig"
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
	genesisBlock := createConfigBlock(ns.t, ns.channelId)
	for _, nodeId := range nodeIds {
		nodeIdToNode[nodeId] = NewNode(ns.t, nodeId, ns.dir, ns.channelId, genesisBlock)
	}
	ns.nodeIdToNode = nodeIdToNode

	// update all nodes about the nodes map
	for _, nodeId := range nodeIds {
		nodeIdToNode[nodeId].nodesMap = nodeIdToNode
	}
	return nodeIdToNode
}

func (ns *NetworkSetupInfo) StartAllNodes() {
	ns.t.Logf("Starting nodes in the network")
	for _, node := range ns.nodeIdToNode {
		node.Start()
	}
}

func (ns *NetworkSetupInfo) SendTxToAllAvailableNodes(tx *cb.Envelope) error {
	var errorsArr []error
	for idx, node := range ns.nodeIdToNode {
		if !node.IsAvailable() {
			continue
		}
		ns.t.Logf("Sending tx to node %v", idx)
		err := node.SendTx(tx)
		if err != nil {
			ns.t.Logf("Error occurred during sending tx to node %v: %v", idx, err)
		}
		errorsArr = append(errorsArr, err)
	}
	return errors.Join(errorsArr...)
}

func (ns *NetworkSetupInfo) RestartAllNodes() error {
	var errorsArr []error
	for _, node := range ns.nodeIdToNode {
		err := node.Restart()
		if err != nil {
			ns.t.Logf("Restarting node %v fail: %v", node.NodeId, err)
		}
		errorsArr = append(errorsArr, err)
	}
	return errors.Join(errorsArr...)
}

func (ns *NetworkSetupInfo) GetAgreedLeader() uint64 {
	var ids []uint64
	for _, node := range ns.nodeIdToNode {
		if !node.IsAvailable() {
			continue
		}
		id := node.Chain.GetLeaderID()
		ids = append(ids, id)
	}

	// check all nodes see the same leader
	for _, element := range ids {
		if element != ids[0] {
			return 0
		}
	}

	return ids[0]
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

func NewNode(t *testing.T, nodeId uint64, rootDir string, channelId string, genesisBlock *cb.Block) *Node {
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
		State:                NewNodeState(t, nodeId, channelId, genesisBlock),
		IsStarted:            false,
		IsConnectedToNetwork: false,
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

func (n *Node) Restart() error {
	n.t.Logf("Restarting node %d", n.NodeId)
	n.lock.Lock()
	defer n.lock.Unlock()
	n.Chain.Halt()
	newChain, err := createBFTChainUsingMocks(n.t, n)
	if err != nil {
		return err
	}
	n.Chain = newChain
	n.Chain.Start()
	return nil
}

func (n *Node) Stop() {
	n.t.Logf("Stoping node %d", n.NodeId)
	n.lock.Lock()
	defer n.lock.Unlock()
	n.IsStarted = false
	n.IsConnectedToNetwork = false
	n.Chain.Halt()
}

func (n *Node) IsAvailable() bool {
	n.lock.RLock()
	defer n.lock.RUnlock()
	return n.IsStarted && n.IsConnectedToNetwork
}

func (n *Node) SendTx(tx *cb.Envelope) error {
	n.lock.RLock()
	defer n.lock.RUnlock()
	return n.Chain.Order(tx, n.State.Sequence)
}

func (n *Node) sendMessage(sender uint64, target uint64, message *smartbftprotos.Message) error {
	n.lock.RLock()
	targetNode, exists := n.nodesMap[target]
	n.lock.RUnlock()
	if !exists {
		return fmt.Errorf("target node %d does not exist", target)
	}
	targetNode.receiveMessage(sender, message)
	return nil
}

func (n *Node) sendRequest(sender uint64, target uint64, request []byte) error {
	n.lock.RLock()
	targetNode, exists := n.nodesMap[target]
	n.lock.RUnlock()
	if !exists {
		return fmt.Errorf("target node %d does not exist", target)
	}
	targetNode.receiveRequest(sender, request)
	return nil
}

func (n *Node) receiveMessage(sender uint64, message *smartbftprotos.Message) {
	n.lock.Lock()
	defer n.lock.Unlock()
	n.Chain.HandleMessage(sender, message)
}

func (n *Node) receiveRequest(sender uint64, request []byte) {
	n.lock.Lock()
	defer n.lock.Unlock()
	n.Chain.HandleRequest(sender, request)
}

func (n *Node) HeightsByEndpoints() map[string]uint64 {
	n.lock.RLock()
	defer n.lock.RUnlock()
	nodeEndpointToHeight := map[string]uint64{}
	for _, node := range n.nodesMap {
		if !node.IsAvailable() {
			continue
		}
		nodeEndpointToHeight[node.Endpoint] = uint64(node.State.GetLedgerHeight())
	}
	return nodeEndpointToHeight
}

type NodeState struct {
	NodeId      uint64
	ledgerArray []*cb.Block
	Sequence    uint64
	lock        sync.RWMutex
	t           *testing.T
}

func NewNodeState(t *testing.T, nodeId uint64, channelId string, genesisBlock *cb.Block) *NodeState {
	return &NodeState{
		NodeId:      nodeId,
		ledgerArray: []*cb.Block{genesisBlock},
		Sequence:    0,
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
	require.Eventually(ns.t, func() bool { return ns.GetLedgerHeight() == height }, 10*time.Second, 100*time.Millisecond)
}

// createBFTChainUsingMocks creates a new bft chain which is exposed to all nodes in the network.
// the chain is created using mocks and is useful for testing
func createBFTChainUsingMocks(t *testing.T, node *Node) (*smartbft.BFTChain, error) {
	nodeId := node.NodeId
	channelId := node.ChannelId

	config := createBFTConfiguration(node)

	blockPuller := smartBFTMocks.NewBlockPuller(t)
	blockPuller.EXPECT().Close().Maybe()
	blockPuller.EXPECT().HeightsByEndpoints().RunAndReturn(
		func() (map[string]uint64, string, error) {
			return node.HeightsByEndpoints(), node.Endpoint, nil
		}).Maybe()
	blockPuller.EXPECT().PullBlock(mock.Anything).RunAndReturn(
		func(seq uint64) *cb.Block {
			t.Logf("Node %d reqested PullBlock %d, returning nil", node.NodeId, seq)
			return nil
		}).Maybe()

	configValidatorMock := smartBFTMocks.NewConfigValidator(t)

	comm := smartBFTMocks.NewCommunicator(t)
	comm.EXPECT().Configure(mock.Anything, mock.Anything)

	signerSerializerMock := smartBFTMocks.NewSignerSerializer(t)
	signerSerializerMock.EXPECT().Sign(mock.Anything).RunAndReturn(
		func(message []byte) ([]byte, error) {
			return message, nil
		}).Maybe()

	policyManagerMock := smartBFTMocks.NewPolicyManager(t)
	noErrorPolicyMock := smartBFTMocks.NewPolicy(t)
	noErrorPolicyMock.EXPECT().EvaluateSignedData(mock.Anything).Return(nil).Maybe()
	noErrorPolicyMock.EXPECT().EvaluateSignedData(mock.Anything).Return(nil).Maybe()
	policyManagerMock.EXPECT().GetPolicy(mock.AnythingOfType("string")).RunAndReturn(
		func(s string) (policies.Policy, bool) {
			return noErrorPolicyMock, true
		}).Maybe()

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
			t.Logf("Node %d appended block number %v to ledger", node.NodeId, block.Header.Number)
		}).Maybe()

	supportMock.EXPECT().WriteConfigBlock(mock.Anything, mock.Anything).Run(
		func(block *cb.Block, encodedMetadataValue []byte) {
			node.State.AddBlock(block)
			t.Logf("Node %d appended block number %v to ledger", node.NodeId, block.Header.Number)
		}).Maybe()

	mpc := &smartbft.MetricProviderConverter{MetricsProvider: &disabled.Provider{}}
	metricsBFT := api.NewMetrics(mpc, channelId)
	metricsWalBFT := wal.NewMetrics(mpc, channelId)

	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	require.NoError(t, err)

	egressCommMock := smartBFTMocks.NewEgressComm(t)
	egressCommMock.EXPECT().Nodes().RunAndReturn(
		func() []uint64 {
			var nodeIds []uint64
			for nodeId := range node.nodesMap {
				nodeIds = append(nodeIds, nodeId)
			}

			sort.Slice(nodeIds, func(i, j int) bool {
				return nodeIds[i] < nodeIds[j]
			})

			return nodeIds
		}).Maybe()
	egressCommMock.EXPECT().SendTransaction(mock.Anything, mock.Anything).Run(
		func(targetNodeId uint64, message []byte) {
			t.Logf("Node %d requested SendTransaction to node %d", node.NodeId, targetNodeId)
			err := node.sendRequest(node.NodeId, targetNodeId, message)
			require.NoError(t, err)
		}).Maybe()
	egressCommMock.EXPECT().SendConsensus(mock.Anything, mock.Anything).Run(
		func(targetNodeId uint64, message *smartbftprotos.Message) {
			t.Logf("Node %d requested SendConsensus to node %d of type <%s>", node.NodeId, targetNodeId, reflect.TypeOf(message.GetContent()))
			err = node.sendMessage(node.NodeId, targetNodeId, message)
			require.NoError(t, err)
		}).Maybe()

	egressCommFactory := func(runtimeConfig *atomic.Value, channelId string, comm cluster.Communicator) smartbft.EgressComm {
		return egressCommMock
	}

	localConfigCluster := localconfig.Cluster{ReplicationPolicy: "simple"}

	bftChain, err := smartbft.NewChain(
		configValidatorMock,
		nodeId,
		config,
		node.WorkingDir,
		nil,
		localConfigCluster,
		comm,
		signerSerializerMock,
		policyManagerMock,
		supportMock,
		smartbft.NewMetrics(&disabled.Provider{}),
		metricsBFT,
		metricsWalBFT,
		cryptoProvider,
		egressCommFactory)

	require.NoError(t, err)
	require.NotNil(t, bftChain)

	return bftChain, nil
}

// createConfigBlock creates the genesis block. This is the first block in the ledger of all nodes.
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
							Sequence:     uint64(0),
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
		LeaderHeartbeatTimeout:        30 * time.Second,
		LeaderHeartbeatCount:          10,
		NumOfTicksBehindBeforeSyncing: types.DefaultConfig.NumOfTicksBehindBeforeSyncing,
		CollectTimeout:                1 * time.Second,
		SyncOnStart:                   types.DefaultConfig.SyncOnStart,
		SpeedUpViewChange:             types.DefaultConfig.SpeedUpViewChange,
		LeaderRotation:                false,
		DecisionsPerLeader:            types.DefaultConfig.DecisionsPerLeader,
		RequestMaxBytes:               types.DefaultConfig.RequestMaxBytes,
		RequestPoolSubmitTimeout:      types.DefaultConfig.RequestPoolSubmitTimeout,
	}
}
