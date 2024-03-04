// Copyright IBM Corp. All Rights Reserved.
//
// SPDX-License-Identifier: Apache-2.0

package smartbft_test

import (
	"bytes"
	"encoding/binary"
	"testing"
	"time"

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
