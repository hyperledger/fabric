// Copyright IBM Corp. All Rights Reserved.
//
// SPDX-License-Identifier: Apache-2.0

package smartbft_test

import (
	"bytes"
	"encoding/binary"
	"testing"

	cb "github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric-protos-go/msp"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/stretchr/testify/require"
)

var nonce uint64 = 0

// Scenario:
// 1. Start a network of 4 nodes
// 2. Submit a TX
// 3. Wait for the TX to be received by all nodes
func TestSuccessfulTxPropagation(t *testing.T) {
	dir := t.TempDir()
	channelId := "testchannel"

	networkSetupInfo := NewNetworkSetupInfo(t, channelId, dir)
	nodeMap := networkSetupInfo.CreateNodes(4)
	networkSetupInfo.StartAllNodes()

	for _, node := range nodeMap {
		node.State.WaitLedgerHeightToBe(1)
	}

	env := createEndorserTxEnvelope("TEST_MESSAGE #1", channelId)
	err := networkSetupInfo.SendTxToAllAvailableNodes(env)
	require.NoError(t, err)
	for _, node := range nodeMap {
		node.State.WaitLedgerHeightToBe(2)
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
