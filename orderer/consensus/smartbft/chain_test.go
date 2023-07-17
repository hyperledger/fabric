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
	. "github.com/onsi/gomega"
)

const eventuallyTimeout = 10 * time.Second

var (
	nonce     uint64 = 0
	channelId        = "channel-test"
)

func Test_StartingNodes_SubmittingTx_RestartingAllNodes_SubmittingTx(t *testing.T) {
	// Scenario:
	// 1. Start 4 nodes
	// 2. Submit a TX
	// 3. Wait for the TX to append to all the nodes' ledgers
	// 4. Restart all the nodes
	// 5. Submit a second TX
	// 6. Wait for the second TX to append to all the nodes' ledgers

	g := NewWithT(t)

	// Creating nodes
	testingFramework := NewChainTestingFramework(t, channelId)
	nodeIdToNode := testingFramework.CreateNodes(1, defaultConfigFactory)
	testingFramework.StartAllNodes()

	// Sending TX #1 to each chain
	env := createEndorserTxEnvelope("TEST_MESSAGE #1", channelId)
	g.Expect(testingFramework.SendTxToAllAvailableNodes(env)).NotTo(HaveOccurred())
	// Asserting each chain have added the block to the ledger
	for _, node := range nodeIdToNode {
		node.State.WaitLedgerHeightToBe(2, eventuallyTimeout)
	}

	// Restarting all the chains
	testingFramework.RestartAllNodes()

	// Sending TX #2 to each chain
	env = createEndorserTxEnvelope("TEST_MESSAGE #2", channelId)
	g.Expect(testingFramework.SendTxToAllAvailableNodes(env)).NotTo(HaveOccurred())
	// Asserting each chain have added the block to the ledger
	for _, node := range nodeIdToNode {
		node.State.WaitLedgerHeightToBe(3, eventuallyTimeout)
	}
}

func Test_StartingNodes_SubmittingTx_KillingLeader_SubmittingTx(t *testing.T) {
	// Scenario:
	// 1. Start 4 nodes
	// 2. Submit a TX
	// 3. Wait for the TX to append to all the nodes' ledgers
	// 4. Kill the leader
	// 5. Wait for a new leader to be elected
	// 6. Submit a second TX
	// 7. Wait for the second TX to append to all the remaining nodes' ledgers

	g := NewWithT(t)
	configMaxTimeout := 1 * time.Second

	// Creating nodes
	testingFramework := NewChainTestingFramework(t, channelId)
	nodeIdToNode := testingFramework.CreateNodes(4, maxTimeConfigFactory(configMaxTimeout))
	testingFramework.StartAllNodes()

	leaderId := testingFramework.GetAgreedLeader()
	t.Logf("Leader is : %d", leaderId)

	// Sending TX #1 to each chain
	env := createEndorserTxEnvelope("TEST_MESSAGE #1", channelId)
	g.Expect(testingFramework.SendTxToAllAvailableNodes(env)).NotTo(HaveOccurred())
	// Asserting each chain have added the block to the ledger
	for _, node := range nodeIdToNode {
		node.State.WaitLedgerHeightToBe(2, eventuallyTimeout)
	}

	// Stop the leader
	nodeIdToNode[leaderId].Stop()

	// Wait for new leader to be elected
	g.Eventually(func() uint64 {
		return testingFramework.GetAgreedLeader()
	}, 3*configMaxTimeout).Should(Satisfy(func(newLeader uint64) bool { return leaderId != newLeader }))

	env = createEndorserTxEnvelope("TEST_MESSAGE #2", channelId)
	// Sending TX #2 to each chain
	g.Expect(testingFramework.SendTxToAllAvailableNodes(env)).NotTo(HaveOccurred())

	// Asserting each chain have added the block to the ledger
	for nodeId, node := range nodeIdToNode {
		if nodeId == leaderId {
			// This chain is stopped
			continue
		}
		node.State.WaitLedgerHeightToBe(3, eventuallyTimeout)
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
	}
}
