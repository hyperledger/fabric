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
	"slices"
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
	"github.com/hyperledger/fabric-lib-go/common/flogging"
	"github.com/hyperledger/fabric-lib-go/common/metrics/disabled"
	cb "github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/hyperledger/fabric/common/channelconfig"
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
	"google.golang.org/protobuf/proto"
)

// ConfigInfo stores the block numbers which are configuration blocks
type ConfigInfo struct {
	t                      *testing.T
	numsOfConfigBlocks     []uint64
	lock                   sync.RWMutex
	leaderHeartbeatTimeout time.Duration
}

func NewConfigInfo(t *testing.T) *ConfigInfo {
	return &ConfigInfo{
		t: t,
	}
}

type NetworkSetupInfo struct {
	t            *testing.T
	nodeIdToNode map[uint64]*Node
	dir          string
	channelId    string
	genesisBlock *cb.Block
	tlsCA        tlsgen.CA
	configInfo   *ConfigInfo
	Logger       *flogging.FabricLogger
}

func NewNetworkSetupInfo(t *testing.T, channelId string, rootDir string) *NetworkSetupInfo {
	return &NetworkSetupInfo{
		t:            t,
		nodeIdToNode: map[uint64]*Node{},
		dir:          rootDir,
		channelId:    channelId,
		configInfo:   NewConfigInfo(t),
		Logger:       flogging.MustGetLogger("orderer.consensus.smartbft").With("channel", channelId),
	}
}

func (ns *NetworkSetupInfo) CreateNodes(numberOfNodes int) map[uint64]*Node {
	var nodeIds []uint64
	for nodeId := uint64(1); nodeId <= uint64(numberOfNodes); nodeId++ {
		nodeIds = append(nodeIds, nodeId)
	}
	nodeIdToNode := map[uint64]*Node{}
	genesisBlock, tlsCA := createConfigBlock(ns.t, ns.channelId)
	clusterService := &cluster.ClusterService{
		StreamCountReporter:              &cluster.StreamCountReporter{},
		Logger:                           flogging.MustGetLogger("orderer.common.cluster"),
		StepLogger:                       flogging.MustGetLogger("orderer.common.cluster.step"),
		MinimumExpirationWarningInterval: cluster.MinimumExpirationWarningInterval,
		MembershipByChannel:              make(map[string]*cluster.ChannelMembersConfig),
		RequestHandler:                   &smartbft.Ingress{},
	}

	for _, nodeId := range nodeIds {
		nodeIdToNode[nodeId] = NewNode(ns.t, nodeId, ns.dir, ns.channelId, genesisBlock, ns.configInfo, nil)
		// update each node's chain to recognize the cluster service
		nodeIdToNode[nodeId].Chain.ClusterService = clusterService
	}
	ns.nodeIdToNode = nodeIdToNode
	ns.genesisBlock = genesisBlock
	ns.tlsCA = tlsCA

	// update all nodes about the nodes map
	for _, nodeId := range nodeIds {
		nodeIdToNode[nodeId].nodesMap = nodeIdToNode
	}
	return nodeIdToNode
}

func (ns *NetworkSetupInfo) AddNewNode() (map[uint64]*Node, *Node) {
	numberOfNodes := uint64(len(ns.nodeIdToNode))
	numberOfNodes = numberOfNodes + 1
	newNodeId := numberOfNodes

	ns.Logger.Infof("Adding node %v to the network", newNodeId)

	ledgerToSyncWith := findLargestDifferentLedger(ns.nodeIdToNode, newNodeId)

	ns.nodeIdToNode[newNodeId] = NewNode(ns.t, newNodeId, ns.dir, ns.channelId, ns.genesisBlock, ns.configInfo, ledgerToSyncWith)

	// update all nodes about the nodes map
	for nodeId := uint64(1); nodeId <= numberOfNodes; nodeId++ {
		ns.nodeIdToNode[nodeId].nodesMap = ns.nodeIdToNode
	}

	return ns.nodeIdToNode, ns.nodeIdToNode[newNodeId]
}

func (ns *NetworkSetupInfo) RemoveNode(num uint64) map[uint64]*Node {
	ns.Logger.Infof("Removing node %v from the network", num)
	delete(ns.nodeIdToNode, num)

	// update all nodes about the nodes map
	for nodeId := range ns.nodeIdToNode {
		ns.nodeIdToNode[nodeId].nodesMap = ns.nodeIdToNode
	}

	return ns.nodeIdToNode
}

func (ns *NetworkSetupInfo) StartAllNodes() {
	ns.Logger.Infof("Starting nodes in the network")
	for _, node := range ns.nodeIdToNode {
		node.Start()
	}
}

func (ns *NetworkSetupInfo) SendTxToAllAvailableNodes(tx *cb.Envelope) error {
	var errorsArr []error
	for idx, node := range ns.nodeIdToNode {
		if !node.IsAvailable() {
			ns.Logger.Infof("Sending tx to node %v, but the node is not available", idx)
			continue
		}
		ns.Logger.Infof("Sending tx to node %v", idx)
		err := node.SendTx(tx)
		if err != nil {
			errorsArr = append(errorsArr, err)
			ns.Logger.Infof("Error occurred during sending tx to node %v: %v", idx, err)
		} else {
			ns.Logger.Infof("Tx to node %v was sent successfully", idx)
		}
	}
	return errors.Join(errorsArr...)
}

func (ns *NetworkSetupInfo) RestartAllNodes() error {
	var errorsArr []error
	for _, node := range ns.nodeIdToNode {
		err := node.Restart(ns.configInfo)
		if err != nil {
			ns.Logger.Infof("Restarting node %v fail: %v", node.NodeId, err)
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
	Logger               *flogging.FabricLogger
}

func NewNode(t *testing.T, nodeId uint64, rootDir string, channelId string, genesisBlock *cb.Block, configInfo *ConfigInfo, ledgerToSyncWith []*cb.Block) *Node {
	logger := flogging.MustGetLogger("orderer.consensus.smartbft").With("channel", channelId).With("nodeId", nodeId)

	logger.Infof("Creating node %d", nodeId)
	nodeWorkingDir := filepath.Join(rootDir, fmt.Sprintf("node-%d", nodeId))

	logger.Infof("Creating working directory for node %d: %s", nodeId, nodeWorkingDir)
	err := os.Mkdir(nodeWorkingDir, os.ModePerm)
	require.NoError(t, err)

	logger.Info("Creating chain")
	node := &Node{
		t:                    t,
		NodeId:               nodeId,
		ChannelId:            channelId,
		WorkingDir:           nodeWorkingDir,
		State:                NewNodeState(t, nodeId, channelId, genesisBlock),
		IsStarted:            false,
		IsConnectedToNetwork: false,
		Endpoint:             fmt.Sprintf("%s:%d", "localhost", 9000+nodeId),
		Logger:               logger,
	}

	// To test a case in which a new node is added to an existing network, its chain should be aware of his existence.
	// Hence, the node's ledger has to sync with the ledger of the other nodes such that it will include
	// all blocks from the genesis to the config block that adds it to the network. Only then its chain can be created.
	if len(configInfo.numsOfConfigBlocks) >= 1 {
		configBlkNum := configInfo.numsOfConfigBlocks[len(configInfo.numsOfConfigBlocks)-1]
		node.State.UpdateLedger(ledgerToSyncWith[:configBlkNum+1])
	}

	node.Chain, err = createBFTChainUsingMocks(t, node, configInfo)
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

func (n *Node) Restart(configInfo *ConfigInfo) error {
	n.Logger.Infof("Restarting node %d", n.NodeId)
	n.lock.Lock()
	defer n.lock.Unlock()
	newChain, err := createBFTChainUsingMocks(n.t, n, configInfo)
	if err != nil {
		return err
	}
	n.Chain = newChain
	n.Chain.Start()
	n.IsConnectedToNetwork = true
	n.IsStarted = true
	return nil
}

func (n *Node) Stop() {
	n.Logger.Infof("Stoping node %d", n.NodeId)
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
	if isConfigTx(tx) {
		return n.Chain.Configure(tx, n.State.Sequence)
	}
	return n.Chain.Order(tx, n.State.Sequence)
}

func (n *Node) GetConfigBlock(num uint64) *cb.Block {
	n.lock.RLock()
	defer n.lock.RUnlock()
	return n.State.ledgerArray[num]
}

func isConfigTx(envelope *cb.Envelope) bool {
	if envelope == nil {
		return false
	}

	payload, err := protoutil.UnmarshalPayload(envelope.Payload)
	if err != nil {
		return false
	}

	if payload.Header == nil {
		return false
	}

	hdr, err := protoutil.UnmarshalChannelHeader(payload.Header.ChannelHeader)
	if err != nil {
		return false
	}

	return cb.HeaderType(hdr.Type) == cb.HeaderType_CONFIG
}

func (n *Node) sendMessage(sender uint64, target uint64, message *smartbftprotos.Message) error {
	n.lock.RLock()
	targetNode, exists := n.nodesMap[target]
	n.lock.RUnlock()
	if !exists {
		return fmt.Errorf("target node %d does not exist", target)
	}
	targetNode.receiveMessage(sender, message)
	n.Logger.Infof("Node %v received a message of type <%s> from node %v", targetNode.NodeId, reflect.TypeOf(message.GetContent()), sender)
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
	n.lock.RLock()
	defer n.lock.RUnlock()
	n.Chain.HandleMessage(sender, message)
}

func (n *Node) receiveRequest(sender uint64, request []byte) {
	n.lock.RLock()
	defer n.lock.RUnlock()
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
	require.Eventually(ns.t, func() bool { return ns.GetLedgerHeight() == height }, 2*time.Minute, 200*time.Millisecond)
}

func (ns *NodeState) GetLedgerArray() []*cb.Block {
	ns.lock.RLock()
	defer ns.lock.RUnlock()
	return ns.ledgerArray
}

func (ns *NodeState) UpdateLedger(ledgerToSyncWith []*cb.Block) {
	// copy the blocks from ledgerToSyncWith except the genesisBlock which is already exists in the node's ledger
	ns.lock.Lock()
	defer ns.lock.Unlock()
	for i := 1; i < len(ledgerToSyncWith); i++ {
		ns.ledgerArray = append(ns.ledgerArray, proto.Clone(ledgerToSyncWith[i]).(*cb.Block))
	}
}

func findLargestDifferentLedger(nodes map[uint64]*Node, givenNodeId uint64) []*cb.Block {
	max := 0
	var ledgerToCopy []*cb.Block
	for _, n := range nodes {
		if n.NodeId != givenNodeId {
			len := n.State.GetLedgerHeight()
			if len > max {
				max = len
				ledgerToCopy = n.State.GetLedgerArray()
			}
		}
	}
	return ledgerToCopy
}

// createBFTChainUsingMocks creates a new bft chain which is exposed to all nodes in the network.
// the chain is created using mocks and is useful for testing
func createBFTChainUsingMocks(t *testing.T, node *Node, configInfo *ConfigInfo) (*smartbft.BFTChain, error) {
	nodeId := node.NodeId
	channelId := node.ChannelId

	config := createBFTConfiguration(node)
	if configInfo.leaderHeartbeatTimeout != 0 {
		config.LeaderHeartbeatTimeout = configInfo.leaderHeartbeatTimeout
	}

	blockPuller := smartBFTMocks.NewBlockPuller(t)
	blockPuller.EXPECT().Close().Maybe()
	blockPuller.EXPECT().HeightsByEndpoints().RunAndReturn(
		func() (map[string]uint64, string, error) {
			return node.HeightsByEndpoints(), node.Endpoint, nil
		}).Maybe()
	blockPuller.EXPECT().PullBlock(mock.Anything).RunAndReturn(
		func(seq uint64) *cb.Block {
			node.Logger.Infof("Node %d reqested PullBlock %d, returning nil", node.NodeId, seq)
			return nil
		}).Maybe()

	configValidatorMock := smartBFTMocks.NewConfigValidator(t)
	configValidatorMock.EXPECT().ValidateConfig(mock.Anything).Return(nil).Maybe()

	comm := smartBFTMocks.NewCommunicator(t)
	comm.EXPECT().Configure(mock.Anything, mock.Anything).Run(func(channel string, members []cluster.RemoteNode) {
		node.Logger.Infof("Configuring channel with remote nodes")
	})

	signerSerializerMock := smartBFTMocks.NewSignerSerializer(t)
	signerSerializerMock.EXPECT().Sign(mock.Anything).RunAndReturn(
		func(message []byte) ([]byte, error) {
			return message, nil
		}).Maybe()
	signerSerializerMock.EXPECT().Serialize().RunAndReturn(
		func() ([]byte, error) {
			return []byte{1, 2, 3}, nil
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
			node.Logger.Infof("Node %d appended block number %v to ledger", node.NodeId, block.Header.Number)
		}).Maybe()
	supportMock.EXPECT().WriteBlockSync(mock.Anything, mock.Anything).Run(
		func(block *cb.Block, encodedMetadataValue []byte) {
			node.State.AddBlock(block)
			node.Logger.Infof("Node %d appended block number %v to ledger", node.NodeId, block.Header.Number)
		}).Maybe()

	supportMock.EXPECT().WriteConfigBlock(mock.Anything, mock.Anything).Run(
		func(block *cb.Block, encodedMetadataValue []byte) {
			node.State.AddBlock(block)
			node.Logger.Infof("Node %d appended config block number %v to ledger", node.NodeId, block.Header.Number)
			configInfo.lock.Lock()
			defer configInfo.lock.Unlock()
			if !slices.Contains(configInfo.numsOfConfigBlocks, block.Header.Number) {
				configInfo.numsOfConfigBlocks = append(configInfo.numsOfConfigBlocks, block.Header.Number)
			}
		}).Maybe()

	supportMock.EXPECT().Serialize().RunAndReturn(
		func() ([]byte, error) {
			return []byte{1, 2, 3}, nil
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
			if !node.IsConnectedToNetwork {
				node.Logger.Infof("Node %d requested SendTransaction to node %d but is not connected to the network", node.NodeId, targetNodeId)
				return
			}
			node.Logger.Infof("Node %d requested SendTransaction to node %d", node.NodeId, targetNodeId)
			err := node.sendRequest(node.NodeId, targetNodeId, message)
			require.NoError(t, err)
		}).Maybe()
	egressCommMock.EXPECT().SendConsensus(mock.Anything, mock.Anything).Run(
		func(targetNodeId uint64, message *smartbftprotos.Message) {
			if !node.IsConnectedToNetwork {
				node.Logger.Infof("Node %d requested SendConsensus to node %d of type <%s> but is not connected to the network", node.NodeId, targetNodeId, reflect.TypeOf(message.GetContent()))
				return
			}
			node.Logger.Infof("Node %d requested SendConsensus to node %d of type <%s>", node.NodeId, targetNodeId, reflect.TypeOf(message.GetContent()))
			err := node.sendMessage(node.NodeId, targetNodeId, message)
			require.NoError(t, err)
		}).Maybe()

	egressCommFactory := func(runtimeConfig *atomic.Value, channelId string, comm cluster.Communicator) smartbft.EgressComm {
		return egressCommMock
	}

	synchronizerMock := smartBFTMocks.NewSynchronizer(t)
	synchronizerMock.EXPECT().Sync().RunAndReturn(
		func() types.SyncResponse {
			node.Logger.Infof("Sync Called by node %v", node.NodeId)
			// iterate over the ledger of the other nodes and find the highest ledger to sync with
			ledgerToCopy := findLargestDifferentLedger(node.nodesMap, node.NodeId)

			// sync node
			for i := node.State.GetLedgerHeight(); i < len(ledgerToCopy); i++ {
				clonedBlock := proto.Clone(ledgerToCopy[i]).(*cb.Block)
				node.State.AddBlock(clonedBlock)
			}

			// send response
			// at this point the chain exists, so we can use its methods
			nodesMap := node.nodesMap
			var currentNodes []uint64
			for nodeId := range nodesMap {
				currentNodes = append(currentNodes, nodeId)
			}

			return types.SyncResponse{
				Latest: *node.Chain.BlockToDecision(ledgerToCopy[len(ledgerToCopy)-1]),
				Reconfig: types.ReconfigSync{
					InReplicatedDecisions: false,
					CurrentNodes:          currentNodes,
					CurrentConfig:         types.Configuration{SelfID: node.NodeId},
				},
			}
		}).Maybe()
	synchronizerFactory := smartBFTMocks.NewSynchronizerFactory(t)
	synchronizerFactory.EXPECT().CreateSynchronizer(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(synchronizerMock)

	localConfigCluster := localconfig.Cluster{ReplicationPolicy: "consensus"}
	clusterDialer := &cluster.PredicateDialer{}

	bftChain, err := smartbft.NewChain(
		configValidatorMock,
		nodeId,
		config,
		node.WorkingDir,
		clusterDialer,
		localConfigCluster,
		comm,
		signerSerializerMock,
		policyManagerMock,
		supportMock,
		smartbft.NewMetrics(&disabled.Provider{}),
		metricsBFT,
		metricsWalBFT,
		cryptoProvider,
		egressCommFactory,
		synchronizerFactory)

	require.NoError(t, err)
	require.NotNil(t, bftChain)

	return bftChain, nil
}

// createConfigBlock creates the genesis block. This is the first block in the ledger of all nodes.
func createConfigBlock(t *testing.T, channelId string) (*cb.Block, tlsgen.CA) {
	certDir := t.TempDir()
	tlsCA, err := tlsgen.NewCA()
	require.NoError(t, err)
	configProfile := genesisconfig.Load(genesisconfig.SampleAppChannelSmartBftProfile, configtest.GetDevConfigDir())

	// make all BFT nodes belong to the same MSP ID
	for _, consenter := range configProfile.Orderer.ConsenterMapping {
		consenter.MSPID = "SampleOrg"
	}

	generateCertificatesSmartBFT(t, configProfile, tlsCA, certDir)

	channelGroup, err := encoder.NewChannelGroup(configProfile)
	require.NoError(t, err)
	require.NotNil(t, channelGroup)

	// update organization endpoints
	ordererEndpoints := channelGroup.Groups[channelconfig.OrdererGroupKey].Groups["SampleOrg"].Values["Endpoints"].Value
	ordererEndpointsVal := &cb.OrdererAddresses{}
	err = proto.Unmarshal(ordererEndpoints, ordererEndpointsVal)
	require.NoError(t, err)
	ordererAddresses := ordererEndpointsVal.Addresses
	for _, consenter := range configProfile.Orderer.ConsenterMapping {
		ordererAddresses = append(ordererAddresses, fmt.Sprintf("%s:%d", consenter.Host, consenter.Port))
	}
	ordererAddresses = ordererAddresses[1:]
	channelGroup.Groups[channelconfig.OrdererGroupKey].Groups["SampleOrg"].Values["Endpoints"].Value = protoutil.MarshalOrPanic(&cb.OrdererAddresses{
		Addresses: ordererAddresses,
	})

	_, err = sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	require.NoError(t, err)

	block := blockWithGroups(channelGroup, channelId, 0)
	return block, tlsCA
}

func generateCertificatesSmartBFT(t *testing.T, confAppSmartBFT *genesisconfig.Profile, tlsCA tlsgen.CA, certDir string) {
	logger := flogging.MustGetLogger("orderer.consensus.smartbft")

	for i, c := range confAppSmartBFT.Orderer.ConsenterMapping {
		logger.Infof("BFT Consenter: %+v", c)
		srvP, clnP := generateSingleCertificateSmartBFT(t, tlsCA, certDir, i, c.Host)
		c.Identity = srvP
		c.ServerTLSCert = srvP
		c.ClientTLSCert = clnP
	}
}

func generateSingleCertificateSmartBFT(t *testing.T, tlsCA tlsgen.CA, certDir string, nodeId int, host string) (string, string) {
	srvC, err := tlsCA.NewServerCertKeyPair(host)
	require.NoError(t, err)
	srvP := path.Join(certDir, fmt.Sprintf("server%d.crt", nodeId))
	err = os.WriteFile(srvP, srvC.Cert, 0o644)
	require.NoError(t, err)

	clnC, err := tlsCA.NewClientCertKeyPair()
	require.NoError(t, err)
	clnP := path.Join(certDir, fmt.Sprintf("client%d.crt", nodeId))
	err = os.WriteFile(clnP, clnC.Cert, 0o644)
	require.NoError(t, err)

	return srvP, clnP
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
		RequestForwardTimeout:         10 * time.Second,
		RequestComplainTimeout:        20 * time.Second,
		RequestAutoRemoveTimeout:      3 * time.Minute,
		ViewChangeResendInterval:      10 * time.Second,
		ViewChangeTimeout:             20 * time.Second,
		LeaderHeartbeatTimeout:        20 * time.Second,
		LeaderHeartbeatCount:          10,
		NumOfTicksBehindBeforeSyncing: types.DefaultConfig.NumOfTicksBehindBeforeSyncing,
		CollectTimeout:                1 * time.Second,
		SyncOnStart:                   types.DefaultConfig.SyncOnStart,
		SpeedUpViewChange:             types.DefaultConfig.SpeedUpViewChange,
		LeaderRotation:                false,
		DecisionsPerLeader:            0,
		RequestMaxBytes:               types.DefaultConfig.RequestMaxBytes * 10,
		RequestPoolSubmitTimeout:      types.DefaultConfig.RequestPoolSubmitTimeout,
	}
}
