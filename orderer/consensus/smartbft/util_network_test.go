// Copyright IBM Corp. All Rights Reserved.
//
// SPDX-License-Identifier: Apache-2.0

package smartbft_test

import (
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/hyperledger/fabric/common/policies"

	"github.com/SmartBFT-Go/consensus/pkg/types"
	"github.com/SmartBFT-Go/consensus/smartbftprotos"
	cb "github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric/bccsp"
	"github.com/hyperledger/fabric/bccsp/sw"
	"github.com/hyperledger/fabric/common/metrics/disabled"
	"github.com/hyperledger/fabric/orderer/common/cluster"
	"github.com/hyperledger/fabric/orderer/consensus/smartbft"
	smartbftMocks "github.com/hyperledger/fabric/orderer/consensus/smartbft/mocks"
	"github.com/hyperledger/fabric/protoutil"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"
)

// factory for creating a configuration for a given node
type ChainConfigFactory func(nodeId uint64) *types.Configuration

// defaultConfigFactory creates default BFT configuration
func defaultConfigFactory(nodeId uint64) *types.Configuration {
	return &types.Configuration{
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
}

// maxTimeConfigFactory creates a default BFT and switches all the timeouts and intervals to the given maxTime
func maxTimeConfigFactory(maxTime time.Duration) ChainConfigFactory {
	return func(nodeId uint64) *types.Configuration {
		config := defaultConfigFactory(nodeId)
		config.RequestForwardTimeout = maxTime
		config.RequestComplainTimeout = maxTime
		config.RequestAutoRemoveTimeout = maxTime
		config.ViewChangeResendInterval = maxTime
		config.ViewChangeTimeout = maxTime
		config.LeaderHeartbeatTimeout = maxTime
		config.CollectTimeout = maxTime
		return config
	}
}

// createMockGenesisBlock creates mock genesis block which can be used by the testing framework
func createMockGenesisBlock() *cb.Block {
	blockData := &cb.BlockData{
		Data: [][]byte{
			protoutil.MarshalOrPanic(&cb.Envelope{
				Payload: protoutil.MarshalOrPanic(&cb.Payload{
					Header: &cb.Header{
						ChannelHeader: protoutil.MarshalOrPanic(&cb.ChannelHeader{
							Type: int32(cb.HeaderType_CONFIG),
						}),
					},
				}),
			}),
		},
	}

	return &cb.Block{
		Header: &cb.BlockHeader{
			// The genesis block is the first block in the ledger
			Number:       0,
			PreviousHash: nil,
			DataHash:     protoutil.BlockDataHash(blockData),
		},
		Data: blockData,
		Metadata: &cb.BlockMetadata{
			Metadata: [][]byte{
				// BlockMetadataIndex_SIGNATURES
				protoutil.MarshalOrPanic(&cb.Metadata{
					Value: protoutil.MarshalOrPanic(&cb.OrdererBlockMetadata{
						LastConfig: &cb.LastConfig{
							// The last config index is the genesis blockZ
							Index: 0,
						},
						ConsenterMetadata: nil,
					}),
					Signatures: nil,
				}),
			},
		},
	}
}

// createNoErrorPolicyMock creates a mock for Policy which will not return errors on
// identities and signed-data evaluations
func createNoErrorPolicyMock(t *testing.T) smartbft.Policy {
	policyMock := smartbftMocks.NewPolicy(t)
	policyMock.EXPECT().EvaluateIdentities(mock.Anything).Return(nil).Maybe()
	policyMock.EXPECT().EvaluateSignedData(mock.Anything).Return(nil).Maybe()
	return policyMock
}

// updateRuntimeConfigWithBlock updates the rtc according to the last block which was added to the ledger
func updateRuntimeConfigWithBlock(rtc *smartbft.RuntimeConfig, lastBlock *cb.Block) {
	rtc.LastBlock = lastBlock
	rtc.LastCommittedBlockHash = hex.EncodeToString(protoutil.BlockHeaderHash(lastBlock.Header))
	if protoutil.IsConfigBlock(lastBlock) {
		rtc.LastConfigBlock = lastBlock
	}
}

// allSameAndNotEmpty checks if all the elements in the array are the same
func allSameAndNotEmpty(arr []uint64) bool {
	if len(arr) == 0 {
		return false
	}
	for _, element := range arr {
		if element != arr[0] {
			return false
		}
	}
	return true
}

// networkConfig is the network configuration of the cluster.
// This config should be known prior to creating a network.
type networkConfig struct {
	nodeIdToRemoteNode map[uint64]*cluster.RemoteNode
	lock               sync.RWMutex
}

func newNetworkConfig(nodeIds []uint64) *networkConfig {
	nodeIdToRemoteNode := map[uint64]*cluster.RemoteNode{}
	for _, nodeId := range nodeIds {
		nodeIdToRemoteNode[nodeId] = &cluster.RemoteNode{
			NodeAddress: cluster.NodeAddress{
				ID:       nodeId,
				Endpoint: fmt.Sprintf("%s:%d", "localhost", 9000+nodeId),
			},
			NodeCerts: cluster.NodeCerts{},
		}
	}
	return &networkConfig{
		nodeIdToRemoteNode: nodeIdToRemoteNode,
	}
}

func (nc *networkConfig) AddNode(nodeId uint64) {
	nc.lock.RLock()
	_, nodeExists := nc.nodeIdToRemoteNode[nodeId]
	nc.lock.RUnlock()
	if nodeExists {
		return
	}
	nc.lock.Lock()
	defer nc.lock.Unlock()
	nc.nodeIdToRemoteNode[nodeId] = &cluster.RemoteNode{
		NodeAddress: cluster.NodeAddress{
			ID:       nodeId,
			Endpoint: fmt.Sprintf("%s:%d", "localhost", 9000+nodeId),
		},
		NodeCerts: cluster.NodeCerts{},
	}
}

func (nc *networkConfig) RemoteNodes() []cluster.RemoteNode {
	nc.lock.RLock()
	defer nc.lock.RUnlock()
	var remoteNodes []cluster.RemoteNode
	for _, remoteNode := range nc.nodeIdToRemoteNode {
		remoteNodes = append(remoteNodes, *remoteNode)
	}
	return remoteNodes
}

func (nc *networkConfig) NodeIds() []uint64 {
	nc.lock.RLock()
	defer nc.lock.RUnlock()
	var nodeIds []uint64
	for nodeId := range nc.nodeIdToRemoteNode {
		nodeIds = append(nodeIds, nodeId)
	}
	return nodeIds
}

type ChainTestingFramework struct {
	t                    *testing.T
	g                    *WithT
	network              *Network
	rootDir              string
	channelId            string
	defaultNetworkConfig *networkConfig
	// defaultNetworkConfig *networkConfig
}

func NewChainTestingFramework(
	t *testing.T,
	channelId string,
) *ChainTestingFramework {
	rootDir := t.TempDir()
	return &ChainTestingFramework{
		t:         t,
		g:         NewWithT(t),
		network:   NewNetwork(t),
		rootDir:   rootDir,
		channelId: channelId,
	}
}

func (ctf *ChainTestingFramework) CreateNodes(
	numberOfNodes int,
	chainConfigFactory ChainConfigFactory,
) map[uint64]*Node {
	var nodeIds []uint64
	for nodeId := uint64(1); nodeId <= uint64(numberOfNodes); nodeId++ {
		nodeIds = append(nodeIds, nodeId)
	}
	ctf.defaultNetworkConfig = newNetworkConfig(nodeIds)
	nodeIdToNode := map[uint64]*Node{}
	for _, nodeId := range nodeIds {
		nodeIdToNode[nodeId] = ctf.createNode(nodeId, chainConfigFactory)
	}
	return nodeIdToNode
}

func (ctf *ChainTestingFramework) RestartAllNodes() {
	for _, node := range ctf.network.Nodes() {
		ctf.g.Expect(node.Restart()).To(Succeed())
	}
}

func (ctf *ChainTestingFramework) createNode(
	nodeId uint64,
	chainConfigFactory ChainConfigFactory,
) *Node {
	node, err := NewNode(
		ctf.t,
		nodeId,
		ctf.rootDir,
		ctf.network,
		ctf.channelId,
		chainConfigFactory,
		ctf.defaultNetworkConfig,
	)
	ctf.g.Expect(err).ShouldNot(HaveOccurred())
	ctf.network.AddOrUpdateNode(node)
	// ctf.g.Expect().To(Succeed())
	return node
}

func (ctf *ChainTestingFramework) StartAllNodes() {
	for _, node := range ctf.network.nodeIdToNode {
		node.Start()
	}
}

func (ctf *ChainTestingFramework) SendTxToAllAvailableNodes(tx *cb.Envelope) error {
	var errorsArr []error
	for _, node := range ctf.network.Nodes() {
		if !node.IsAvailable() {
			continue
		}
		errorsArr = append(errorsArr, node.SendTx(tx))
	}
	return errors.Join(errorsArr...)
}

func (ctf *ChainTestingFramework) GetAgreedLeader() uint64 {
	nodes := ctf.network.Nodes()
	var ids []uint64
	for _, node := range nodes {
		if !node.IsAvailable() {
			continue
		}
		id := node.Chain.GetLeaderID()
		ids = append(ids, id)
	}
	if allSameAndNotEmpty(ids) {
		return ids[0]
	}
	return 0
}

// Network is responsible to transfer messages between nodes in the cluster
type Network struct {
	t            *testing.T
	g            *WithT
	nodeIdToNode map[uint64]*Node
	lock         sync.RWMutex
}

func NewNetwork(t *testing.T) *Network {
	return &Network{
		t:            t,
		g:            NewWithT(t),
		nodeIdToNode: map[uint64]*Node{},
	}
}

func (n *Network) sendMessage(sender uint64, target uint64, message *smartbftprotos.Message) error {
	n.lock.RLock()
	node, exists := n.nodeIdToNode[target]
	n.lock.RUnlock()
	if !exists {
		return fmt.Errorf("target node %d does not exist", target)
	}
	node.receiveMessage(sender, message)
	return nil
}

func (n *Network) sendRequest(sender uint64, target uint64, request []byte) error {
	n.lock.RLock()
	node, exists := n.nodeIdToNode[target]
	n.lock.RUnlock()
	if !exists {
		return fmt.Errorf("target node %d does not exist", target)
	}
	node.receiveRequest(sender, request)
	return nil
}

func (n *Network) AddOrUpdateNode(node *Node) {
	n.lock.Lock()
	defer n.lock.Unlock()
	n.nodeIdToNode[node.NodeId] = node
}

func (n *Network) HeightsByEndpoints() map[string]uint64 {
	n.lock.RLock()
	defer n.lock.RUnlock()
	nodeEndpointToHeight := map[string]uint64{}
	for _, node := range n.nodeIdToNode {
		if !node.IsAvailable() {
			continue
		}
		nodeEndpointToHeight[node.Endpoint] = uint64(node.State.GetLedgerHeight())
	}
	return nodeEndpointToHeight
}

func (n *Network) Nodes() []*Node {
	n.lock.RLock()
	defer n.lock.RUnlock()
	var nodes []*Node
	for _, node := range n.nodeIdToNode {
		nodes = append(nodes, node)
	}
	return nodes
}

// NodeState holds the ledger and the sequence
type NodeState struct {
	NodeId      uint64
	ledgerArray []*cb.Block
	Sequence    uint64
	lock        sync.RWMutex
	t           *testing.T
}

func NewNodeState(t *testing.T, nodeId uint64) *NodeState {
	return &NodeState{
		NodeId:      nodeId,
		ledgerArray: []*cb.Block{createMockGenesisBlock()},
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

func (ns *NodeState) WaitLedgerHeightToBe(height int, timeout time.Duration) {
	NewWithT(ns.t).Eventually(func() int {
		return ns.GetLedgerHeight()
	}, timeout).Should(Equal(height))
}

// Node is a wrapper around the BFT chain, enables us to test it conveniently
type Node struct {
	t                    *testing.T
	NodeId               uint64
	ChannelId            string
	WorkingDir           string
	Chain                *smartbft.BFTChain
	RuntimeConfig        *smartbft.RuntimeConfig
	State                *NodeState
	IsStarted            bool
	IsConnectedToNetwork bool
	network              *Network
	networkConfig        *networkConfig
	Endpoint             string
	config               *types.Configuration
	lock                 sync.RWMutex
}

func NewNode(
	t *testing.T,
	nodeId uint64,
	rootDir string,
	network *Network,
	channelId string,
	chainConfigFactory ChainConfigFactory,
	networkConfig *networkConfig,
) (*Node, error) {
	t.Logf("Creating node %d", nodeId)
	nodeWorkingDir := filepath.Join(rootDir, fmt.Sprintf("node-%d", nodeId))
	t.Logf("Creating working directory for node %d: %s", nodeId, nodeWorkingDir)
	err := os.Mkdir(nodeWorkingDir, os.ModePerm)
	if err != nil {
		return nil, err
	}
	t.Log("Creating chain")
	node := &Node{
		t:         t,
		NodeId:    nodeId,
		ChannelId: channelId,
		// Chain:              nil,
		WorkingDir:           nodeWorkingDir,
		RuntimeConfig:        &smartbft.RuntimeConfig{},
		State:                NewNodeState(t, nodeId),
		networkConfig:        networkConfig,
		IsStarted:            false,
		IsConnectedToNetwork: false,
		network:              network,
		config:               chainConfigFactory(nodeId),
		Endpoint:             networkConfig.nodeIdToRemoteNode[nodeId].Endpoint,
	}
	node.Chain, err = newBftChain(node)
	if err != nil {
		return nil, err
	}
	return node, nil
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

func (n *Node) IsAvailable() bool {
	n.lock.RLock()
	defer n.lock.RUnlock()
	return n.IsStarted && n.IsConnectedToNetwork
}

func (n *Node) Start() {
	n.lock.Lock()
	defer n.lock.Unlock()
	n.Chain.Start()
	n.IsConnectedToNetwork = true
	n.IsStarted = true
}

func (n *Node) Stop() {
	n.t.Logf("Stoping node %d", n.NodeId)
	n.lock.Lock()
	defer n.lock.Unlock()
	n.IsStarted = false
	n.IsConnectedToNetwork = false
	n.Chain.Halt()
}

func (n *Node) Restart() error {
	n.t.Logf("Restarting node %d", n.NodeId)
	n.lock.Lock()
	defer n.lock.Unlock()
	n.Chain.Halt()
	newChain, err := newBftChain(n)
	if err != nil {
		return err
	}
	n.Chain = newChain
	n.Chain.Start()
	return nil
}

func (n *Node) SendTx(tx *cb.Envelope) error {
	n.lock.RLock()
	defer n.lock.RUnlock()
	return n.Chain.Order(tx, n.State.Sequence)
}

// newBftChain creates the BFT chain instance using mocks
func newBftChain(node *Node) (*smartbft.BFTChain, error) {
	t := node.t
	g := NewWithT(t)
	// Mock runtime config manager
	rtmMock := smartbftMocks.NewRuntimeConfigManager(t)
	rtmMock.EXPECT().GetConfig().RunAndReturn(
		func() smartbft.RuntimeConfig {
			return *node.RuntimeConfig
		})
	rtmMock.EXPECT().UpdateUsingBlock(mock.Anything, mock.Anything).RunAndReturn(
		func(block *cb.Block, b bccsp.BCCSP) (smartbft.RuntimeConfig, error) {
			t.Logf("Node %d called UpdateUsingBlock %d", node.NodeId, block.Header.Number)
			updateRuntimeConfigWithBlock(node.RuntimeConfig, block)
			return *node.RuntimeConfig, nil
		})
	newRuntimeConfigManagerMockFactory := func(initialRtc smartbft.RuntimeConfig) smartbft.RuntimeConfigManager {
		node.RuntimeConfig = &initialRtc
		node.RuntimeConfig.RemoteNodes = node.networkConfig.RemoteNodes()
		node.RuntimeConfig.Nodes = node.networkConfig.NodeIds()
		updateRuntimeConfigWithBlock(node.RuntimeConfig, node.State.GetLastBlock())
		node.RuntimeConfig.ID2Identities = map[uint64][]byte{}
		for _, id := range node.networkConfig.NodeIds() {
			node.RuntimeConfig.ID2Identities[id] = []byte{uint8(id)}
		}
		return rtmMock
	}

	configValidatorMock := smartbftMocks.NewConfigValidator(t)

	ledgerMock := smartbftMocks.NewLedger(t)
	ledgerMock.EXPECT().Height().Return(uint64(node.State.GetLedgerHeight()))
	ledgerMock.EXPECT().Block(mock.AnythingOfType("uint64")).RunAndReturn(
		func(blockIdx uint64) *cb.Block {
			return node.State.GetBlock(blockIdx)
		})
	ledgerMock.EXPECT().WriteBlock(mock.Anything, mock.Anything).Run(
		func(block *cb.Block, encodedMetadataValue []byte) {
			node.State.AddBlock(block)
			t.Logf("Node %d appended block to ledger !!!", node.NodeId)
		}).Maybe()

	sequencerMock := smartbftMocks.NewSequencer(t)
	sequencerMock.EXPECT().Sequence().RunAndReturn(
		func() uint64 {
			return node.State.Sequence
		})

	blockPuller := smartbftMocks.NewBlockPuller(t)
	blockPuller.EXPECT().Close().Maybe()
	blockPuller.EXPECT().HeightsByEndpoints().RunAndReturn(
		func() (map[string]uint64, error) {
			return node.network.HeightsByEndpoints(), nil
		}).Maybe()
	blockPuller.EXPECT().PullBlock(mock.Anything).RunAndReturn(
		func(seq uint64) *cb.Block {
			t.Logf("Node %d reqested PullBlock %d, returning nil", node.NodeId, seq)
			return nil
		}).Maybe()

	communicator := smartbftMocks.NewCommunicator(t)
	communicator.EXPECT().Configure(mock.Anything, mock.Anything)

	signerSerializer := smartbftMocks.NewSignerSerializer(t)
	signerSerializer.EXPECT().Sign(mock.Anything).RunAndReturn(
		func(message []byte) ([]byte, error) {
			return message, nil
		}).Maybe()

	policyManager := smartbftMocks.NewPolicyManager(t)
	allGoodPolicy := createNoErrorPolicyMock(t)
	policyManager.EXPECT().GetPolicy(mock.AnythingOfType("string")).RunAndReturn(
		func(s string) (policies.Policy, bool) {
			t.Log(s)
			return allGoodPolicy, true
		}).Maybe()

	msgprocessor := smartbftMocks.NewProcessor(t)

	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	if err != nil {
		return nil, err
	}

	egressCommMock := smartbftMocks.NewEgressComm(t)
	egressCommMock.EXPECT().Nodes().RunAndReturn(
		func() []uint64 {
			return node.networkConfig.NodeIds()
		}).Maybe()
	egressCommMock.EXPECT().SendTransaction(mock.Anything, mock.Anything).Run(
		func(targetNodeId uint64, message []byte) {
			t.Logf("Node %d requested SendTransaction to node %d", node.NodeId, targetNodeId)
			g.Expect(node.network.sendRequest(node.NodeId, targetNodeId, message)).To(Succeed())
		}).Maybe()
	egressCommMock.EXPECT().SendConsensus(mock.Anything, mock.Anything).Run(
		func(targetNodeId uint64, message *smartbftprotos.Message) {
			t.Logf("Node %d requested SendConsensus to node %d of type <%s>", node.NodeId, targetNodeId, reflect.TypeOf(message.GetContent()))
			g.Expect(node.network.sendMessage(node.NodeId, targetNodeId, message)).To(Succeed())
		}).Maybe()

	egressCommFactory := func(rtcm smartbft.RuntimeConfigManager, channelId string, comm smartbft.Communicator) smartbft.EgressComm {
		return egressCommMock
	}

	bftChain, err := smartbft.NewChain(
		node.ChannelId,
		configValidatorMock,
		node.NodeId,
		*node.config,
		node.WorkingDir,
		blockPuller,
		communicator,
		signerSerializer,
		policyManager,
		smartbft.NewMetrics(&disabled.Provider{}),
		cryptoProvider,
		newRuntimeConfigManagerMockFactory,
		msgprocessor,
		ledgerMock,
		sequencerMock,
		egressCommFactory,
	)
	if err != nil {
		return nil, err
	}
	return bftChain, nil
}
