// Copyright IBM Corp. All Rights Reserved.
//
// SPDX-License-Identifier: Apache-2.0

package smartbft_test

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/common/crypto/tlsgen"
	"github.com/hyperledger/fabric/internal/configtxlator/update"
	smartBFTMocks "github.com/hyperledger/fabric/orderer/consensus/smartbft/mocks"
	"github.com/stretchr/testify/mock"

	cb "github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric-protos-go/msp"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/stretchr/testify/require"
)

var nonce uint64 = 0

// Scenario:
// 1. Start a network of 4 nodes
// 2. Submit a TX and wait for the TX to be received by all nodes
// 3. Restart all nodes
// 4. Submit a TX and wait for the TX to be received by all nodes
func TestSuccessfulTxPropagation(t *testing.T) {
	dir := t.TempDir()
	channelId := "testchannel"

	// start a network
	networkSetupInfo := NewNetworkSetupInfo(t, channelId, dir)
	nodeMap := networkSetupInfo.CreateNodes(4)
	networkSetupInfo.StartAllNodes()

	// wait until all nodes have the genesis block in their ledger
	for _, node := range nodeMap {
		node.State.WaitLedgerHeightToBe(1)
	}

	// send a tx to all nodes and wait the tx will be added to each ledger
	env := createEndorserTxEnvelope("TEST_MESSAGE #1", channelId)
	err := networkSetupInfo.SendTxToAllAvailableNodes(env)
	require.NoError(t, err)
	for _, node := range nodeMap {
		node.State.WaitLedgerHeightToBe(2)
	}

	// restarting all nodes
	err = networkSetupInfo.RestartAllNodes()
	require.NoError(t, err)

	// send a tx to all nodes again and wait the tx will be added to each ledger
	env = createEndorserTxEnvelope("TEST_MESSAGE #2", channelId)
	err = networkSetupInfo.SendTxToAllAvailableNodes(env)
	require.NoError(t, err)
	for _, node := range nodeMap {
		node.State.WaitLedgerHeightToBe(3)
	}
}

// Scenario:
// 1. Start a network of 4 nodes
// 2. Submit a TX and wait for the TX to be received by all nodes
// 3. Stop the leader and wait for a new one to be elected
// 4. Submit a TX and wait for the TX to be received by all nodes
func TestStopLeaderAndSuccessfulTxPropagation(t *testing.T) {
	dir := t.TempDir()
	channelId := "testchannel"

	// start a network
	networkSetupInfo := NewNetworkSetupInfo(t, channelId, dir)
	nodeMap := networkSetupInfo.CreateNodes(4)
	networkSetupInfo.StartAllNodes()

	// wait until all nodes have the genesis block in their ledger
	for _, node := range nodeMap {
		node.State.WaitLedgerHeightToBe(1)
	}

	// send a tx to all nodes and wait the tx will be added to each ledger
	env := createEndorserTxEnvelope("TEST_MESSAGE #1", channelId)
	err := networkSetupInfo.SendTxToAllAvailableNodes(env)
	require.NoError(t, err)
	for _, node := range nodeMap {
		node.State.WaitLedgerHeightToBe(2)
	}

	// get leader id
	leaderId := networkSetupInfo.GetAgreedLeader()

	// stop the leader
	nodeMap[leaderId].Stop()

	// wait for a new leader to be elected
	require.Eventually(t, func() bool {
		newLeaderId := networkSetupInfo.GetAgreedLeader()
		return leaderId != newLeaderId
	}, 1*time.Minute, 100*time.Millisecond)

	// send a tx to all nodes again and wait the tx will be added to each ledger, except the ledger of the old leader which is stopped
	env = createEndorserTxEnvelope("TEST_MESSAGE #2", channelId)
	err = networkSetupInfo.SendTxToAllAvailableNodes(env)
	require.NoError(t, err)
	for _, node := range nodeMap {
		if node.NodeId == leaderId {
			continue
		}
		node.State.WaitLedgerHeightToBe(3)
	}
}

// Scenario:
// 1. Start a network of 4 nodes
// 2. Submit a TX and wait for the TX to be received by all nodes
// 3. Stop the leader and wait for a new one to be elected
// 4. Submit some more TXs and wait for the TXs to be received by all available nodes
// 5. Start the old leader
// 6. Make sure the old leader has synced with the nodes in the network
// 7. Submit a TX and wait for the TX to be received by all nodes
func TestSyncNode(t *testing.T) {
	dir := t.TempDir()
	channelId := "testchannel"

	// start a network
	networkSetupInfo := NewNetworkSetupInfo(t, channelId, dir)
	nodeMap := networkSetupInfo.CreateNodes(4)
	networkSetupInfo.StartAllNodes()

	// wait until all nodes have the genesis block in their ledger
	for _, node := range nodeMap {
		node.State.WaitLedgerHeightToBe(1)
	}

	// send a tx to all nodes and wait the tx will be added to each ledger
	env := createEndorserTxEnvelope("TEST_MESSAGE #1", channelId)
	err := networkSetupInfo.SendTxToAllAvailableNodes(env)
	require.NoError(t, err)
	for _, node := range nodeMap {
		node.State.WaitLedgerHeightToBe(2)
	}

	// get leader id
	leaderId := networkSetupInfo.GetAgreedLeader()

	// stop the leader
	nodeMap[leaderId].Stop()

	// wait for a new leader to be elected
	require.Eventually(t, func() bool {
		newLeaderId := networkSetupInfo.GetAgreedLeader()
		return leaderId != newLeaderId
	}, 1*time.Minute, 100*time.Millisecond)

	// send 5 txs to all available nodes and wait the tx will be added to each ledger except the ledger of the old leader
	numberTxs := 5
	for i := 0; i < numberTxs; i++ {
		message := "TEST_MESSAGE #" + fmt.Sprintf("%d", i+2)
		env = createEndorserTxEnvelope(message, channelId)
		err = networkSetupInfo.SendTxToAllAvailableNodes(env)
		require.NoError(t, err)
		for _, node := range nodeMap {
			if node.NodeId == leaderId {
				continue
			}
			node.State.WaitLedgerHeightToBe(i + 3)
		}
	}

	// restart the old leader
	nodeMap[leaderId].Restart(networkSetupInfo.configInfo)

	// make sure the old leader has synced with the nodes in the network
	nodeMap[leaderId].State.WaitLedgerHeightToBe(7)

	// send a tx to all nodes and wait the tx will be added to each ledger
	env = createEndorserTxEnvelope("TEST_MESSAGE #7", channelId)
	err = networkSetupInfo.SendTxToAllAvailableNodes(env)
	require.NoError(t, err)
	for _, node := range nodeMap {
		node.State.WaitLedgerHeightToBe(8)
	}
}

// Scenario:
// 1. Start a network of 4 nodes
// 2. Submit a TX and wait for the TX to be received by all nodes
// 3. Add new node to the network
// 4. Submit a TX and wait for the TX to be received by all nodes
func TestAddNode(t *testing.T) {
	dir := t.TempDir()
	channelId := "testchannel"

	// start a network
	networkSetupInfo := NewNetworkSetupInfo(t, channelId, dir)
	nodeMap := networkSetupInfo.CreateNodes(4)
	networkSetupInfo.StartAllNodes()

	// wait until all nodes have the genesis block in their ledger
	for _, node := range nodeMap {
		node.State.WaitLedgerHeightToBe(1)
	}

	// send a tx to all nodes and wait the tx will be added to each ledger
	env := createEndorserTxEnvelope("TEST_MESSAGE #1", channelId)
	err := networkSetupInfo.SendTxToAllAvailableNodes(env)
	require.NoError(t, err)
	for _, node := range nodeMap {
		node.State.WaitLedgerHeightToBe(2)
	}

	// send a new config block to all nodes, to notice them about the new node
	env = addNodeToConfig(t, networkSetupInfo.genesisBlock, 5, networkSetupInfo.tlsCA, networkSetupInfo.dir, networkSetupInfo.channelId)
	err = networkSetupInfo.SendTxToAllAvailableNodes(env)
	require.NoError(t, err)
	for _, node := range nodeMap {
		node.State.WaitLedgerHeightToBe(3)
	}

	// add new node to the network
	nodesMap, newNode := networkSetupInfo.AddNewNode()
	newNode.Start()
	require.Equal(t, len(nodesMap), 5)
	require.Equal(t, len(networkSetupInfo.nodeIdToNode), 5)

	// send a tx to all nodes again and wait the tx will be added to each ledger
	env = createEndorserTxEnvelope("TEST_ADDITION_OF_NODE", channelId)
	err = networkSetupInfo.SendTxToAllAvailableNodes(env)
	require.NoError(t, err)
	for _, node := range nodeMap {
		node.State.WaitLedgerHeightToBe(4)
	}
}

func createEndorserTxEnvelope(message string, channelId string) *cb.Envelope {
	return &cb.Envelope{
		Payload: protoutil.MarshalOrPanic(&cb.Payload{
			Header: &cb.Header{
				ChannelHeader: protoutil.MarshalOrPanic(&cb.ChannelHeader{
					Type:      int32(cb.HeaderType_ENDORSER_TRANSACTION),
					ChannelId: channelId,
				}),
				SignatureHeader: protoutil.MarshalOrPanic(&cb.SignatureHeader{
					Creator: protoutil.MarshalOrPanic(&msp.SerializedIdentity{
						Mspid:   "mockMSP",
						IdBytes: []byte("mockClient"),
					}),
					Nonce: generateNonce(),
				}),
			},
			Data: []byte(message),
		}),
		Signature: []byte{1, 2, 3},
	}
}

func generateNonce() []byte {
	nonceBuf := new(bytes.Buffer)
	err := binary.Write(nonceBuf, binary.LittleEndian, nonce)
	if err != nil {
		panic("Cannot generate nonce")
	}
	nonce++
	return nonceBuf.Bytes()
}

// addNodeToConfig creates a config block based on the last config block. It is useful for addition or removal of a node
func addNodeToConfig(t *testing.T, lastConfigBlock *cb.Block, nodeId uint32, tlsCA tlsgen.CA, certDir string, channelId string) *cb.Envelope {
	// copy the last config block
	clonedLastConfigBlock := proto.Clone(lastConfigBlock).(*cb.Block)

	// fetch the ConfigEnvelope from the block
	env := protoutil.UnmarshalEnvelopeOrPanic(clonedLastConfigBlock.Data.Data[0])
	payload := protoutil.UnmarshalPayloadOrPanic(env.Payload)
	configEnv, err := protoutil.UnmarshalConfigEnvelope(payload.Data)
	require.NoError(t, err)
	originalConfigEnv := proto.Clone(configEnv).(*cb.ConfigEnvelope)

	// create the crypto material for the new node
	host := fmt.Sprintf("bft%d.example.com", nodeId-1)
	srvP, clnP := generateSingleCertificateSmartBFT(t, tlsCA, certDir, int(nodeId), host)
	clientCert, err := os.ReadFile(clnP)
	require.NoError(t, err)
	serverCert, err := os.ReadFile(srvP)
	require.NoError(t, err)

	// update the consenter mapping to include the new node
	newOrderer := &cb.Consenter{
		Id:            nodeId,
		Host:          host,
		Port:          7050,
		MspId:         "SampleOrg",
		Identity:      clientCert,
		ClientTlsCert: clientCert,
		ServerTlsCert: serverCert,
	}

	currentOrderers := &cb.Orderers{}
	err = proto.Unmarshal(configEnv.Config.ChannelGroup.Groups[channelconfig.OrdererGroupKey].Values[channelconfig.OrderersKey].Value, currentOrderers)
	require.NoError(t, err)
	currentOrderers.ConsenterMapping = append(currentOrderers.ConsenterMapping, newOrderer)
	configEnv.Config.ChannelGroup.Groups[channelconfig.OrdererGroupKey].Values[channelconfig.OrderersKey] = &cb.ConfigValue{
		Version:   configEnv.Config.ChannelGroup.Groups[channelconfig.OrdererGroupKey].Values[channelconfig.OrderersKey].Version + 1,
		Value:     protoutil.MarshalOrPanic(currentOrderers),
		ModPolicy: channelconfig.AdminsPolicyKey,
	}

	// update organization endpoints
	ordererEndpoints := configEnv.Config.ChannelGroup.Groups[channelconfig.OrdererGroupKey].Groups["SampleOrg"].Values["Endpoints"].Value
	ordererEndpointsVal := &cb.OrdererAddresses{}
	proto.Unmarshal(ordererEndpoints, ordererEndpointsVal)
	ordererAddresses := ordererEndpointsVal.Addresses
	ordererAddresses = append(ordererAddresses, fmt.Sprintf("%s:%d", newOrderer.Host, newOrderer.Port))
	configEnv.Config.ChannelGroup.Groups[channelconfig.OrdererGroupKey].Groups["SampleOrg"].Values["Endpoints"].Value = protoutil.MarshalOrPanic(&cb.OrdererAddresses{
		Addresses: ordererAddresses,
	})

	// increase the sequence
	configEnv.Config.Sequence = configEnv.Config.Sequence + 1

	// calculate config update tx
	configUpdate, err := update.Compute(originalConfigEnv.Config, configEnv.Config)
	require.NoError(t, err)
	signerSerializer := smartBFTMocks.NewSignerSerializer(t)
	signerSerializer.EXPECT().Serialize().RunAndReturn(
		func() ([]byte, error) {
			return []byte{1, 2, 3}, nil
		}).Maybe()
	signerSerializer.EXPECT().Sign(mock.Anything).RunAndReturn(
		func(message []byte) ([]byte, error) {
			return message, nil
		}).Maybe()
	configUpdateTx, err := protoutil.CreateSignedEnvelope(cb.HeaderType_CONFIG_UPDATE, channelId, signerSerializer, configUpdate, 0, 0)
	require.NoError(t, err)

	// return the updated Envelope
	return &cb.Envelope{
		Payload: protoutil.MarshalOrPanic(&cb.Payload{
			Data: protoutil.MarshalOrPanic(&cb.ConfigEnvelope{
				Config:     configEnv.Config,
				LastUpdate: configUpdateTx,
			}),
			Header: &cb.Header{
				ChannelHeader: protoutil.MarshalOrPanic(&cb.ChannelHeader{
					Type:      int32(cb.HeaderType_CONFIG),
					ChannelId: channelId,
				}),
			},
		}),
	}
}
