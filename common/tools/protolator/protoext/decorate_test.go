/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package protoext

import (
	"testing"

	"github.com/golang/protobuf/proto"

	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric-protos-go/ledger/rwset"
	"github.com/hyperledger/fabric-protos-go/msp"
	"github.com/hyperledger/fabric-protos-go/orderer"
	"github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/common/tools/protolator/protoext/commonext"
	"github.com/hyperledger/fabric/common/tools/protolator/protoext/ledger/rwsetext"
	"github.com/hyperledger/fabric/common/tools/protolator/protoext/mspext"
	"github.com/hyperledger/fabric/common/tools/protolator/protoext/ordererext"
	"github.com/hyperledger/fabric/common/tools/protolator/protoext/peerext"

	. "github.com/onsi/gomega"
)

type GenericProtoMessage struct {
	GenericField string
}

func (g *GenericProtoMessage) Reset() {
	panic("not implemented")
}

func (g *GenericProtoMessage) String() string {
	return "not implemented"
}

func (g *GenericProtoMessage) ProtoMessage() {
	panic("not implemented")
}

func TestDecorate(t *testing.T) {
	tests := []struct {
		testSpec       string
		msg            proto.Message
		expectedReturn proto.Message
	}{
		{
			testSpec: "common.BlockData",
			msg: &common.BlockData{
				Data: [][]byte{
					[]byte("data-bytes"),
				}},
			expectedReturn: &commonext.BlockData{
				BlockData: &common.BlockData{
					Data: [][]byte{
						[]byte("data-bytes"),
					},
				},
			},
		},
		{
			testSpec: "common.Config",
			msg: &common.Config{
				Sequence: 5,
			},
			expectedReturn: &commonext.Config{
				Config: &common.Config{
					Sequence: 5,
				},
			},
		},
		{
			testSpec: "common.ConfigSignature",
			msg: &common.ConfigSignature{
				SignatureHeader: []byte("signature-header-bytes"),
			},
			expectedReturn: &commonext.ConfigSignature{
				ConfigSignature: &common.ConfigSignature{
					SignatureHeader: []byte("signature-header-bytes"),
				},
			},
		},
		{
			testSpec: "common.ConfigUpdate",
			msg: &common.ConfigUpdate{
				ChannelId: "testchannel",
			},
			expectedReturn: &commonext.ConfigUpdate{
				ConfigUpdate: &common.ConfigUpdate{
					ChannelId: "testchannel",
				},
			},
		},
		{
			testSpec: "common.ConfigUpdateEnvelope",
			msg: &common.ConfigUpdateEnvelope{
				ConfigUpdate: []byte("config-update-bytes"),
			},
			expectedReturn: &commonext.ConfigUpdateEnvelope{
				ConfigUpdateEnvelope: &common.ConfigUpdateEnvelope{
					ConfigUpdate: []byte("config-update-bytes"),
				},
			},
		},
		{
			testSpec: "common.Envelope",
			msg: &common.Envelope{
				Payload: []byte("payload-bytes"),
			},
			expectedReturn: &commonext.Envelope{
				Envelope: &common.Envelope{
					Payload: []byte("payload-bytes"),
				},
			},
		},
		{
			testSpec: "common.Header",
			msg: &common.Header{
				ChannelHeader: []byte("channel-header-bytes"),
			},
			expectedReturn: &commonext.Header{
				Header: &common.Header{
					ChannelHeader: []byte("channel-header-bytes"),
				},
			},
		},
		{
			testSpec: "common.ChannelHeader",
			msg: &common.ChannelHeader{
				Type: 5,
			},
			expectedReturn: &commonext.ChannelHeader{
				ChannelHeader: &common.ChannelHeader{
					Type: 5,
				},
			},
		},
		{
			testSpec: "common.SignatureHeader",
			msg: &common.SignatureHeader{
				Creator: []byte("creator-bytes"),
			},
			expectedReturn: &commonext.SignatureHeader{
				SignatureHeader: &common.SignatureHeader{
					Creator: []byte("creator-bytes"),
				},
			},
		},
		{
			testSpec: "common.Payload",
			msg: &common.Payload{
				Header: &common.Header{ChannelHeader: []byte("channel-header-bytes")},
			},
			expectedReturn: &commonext.Payload{
				Payload: &common.Payload{
					Header: &common.Header{ChannelHeader: []byte("channel-header-bytes")},
				},
			},
		},
		{
			testSpec: "common.Policy",
			msg: &common.Policy{
				Type: 5,
			},
			expectedReturn: &commonext.Policy{
				Policy: &common.Policy{
					Type: 5,
				},
			},
		},
		{
			testSpec: "msp.MSPConfig",
			msg: &msp.MSPConfig{
				Type: 5,
			},
			expectedReturn: &mspext.MSPConfig{
				MSPConfig: &msp.MSPConfig{
					Type: 5,
				},
			},
		},
		{
			testSpec: "msp.MSPPrincipal",
			msg: &msp.MSPPrincipal{
				Principal: []byte("principal-bytes"),
			},
			expectedReturn: &mspext.MSPPrincipal{
				MSPPrincipal: &msp.MSPPrincipal{
					Principal: []byte("principal-bytes"),
				},
			},
		},
		{
			testSpec: "orderer.ConsensusType",
			msg: &orderer.ConsensusType{
				Type: "etcdraft",
			},
			expectedReturn: &ordererext.ConsensusType{
				ConsensusType: &orderer.ConsensusType{
					Type: "etcdraft",
				},
			},
		},
		{
			testSpec: "peer.ChaincodeAction",
			msg: &peer.ChaincodeAction{
				Results: []byte("results-bytes"),
			},
			expectedReturn: &peerext.ChaincodeAction{
				ChaincodeAction: &peer.ChaincodeAction{
					Results: []byte("results-bytes"),
				},
			},
		},
		{
			testSpec: "peer.ChaincodeActionPayload",
			msg: &peer.ChaincodeActionPayload{
				ChaincodeProposalPayload: []byte("chaincode-proposal-payload-bytes"),
			},
			expectedReturn: &peerext.ChaincodeActionPayload{
				ChaincodeActionPayload: &peer.ChaincodeActionPayload{
					ChaincodeProposalPayload: []byte("chaincode-proposal-payload-bytes"),
				},
			},
		},
		{
			testSpec: "peer.ChaincodeEndorsedAction",
			msg: &peer.ChaincodeEndorsedAction{
				ProposalResponsePayload: []byte("proposal-response-payload-bytes"),
			},
			expectedReturn: &peerext.ChaincodeEndorsedAction{
				ChaincodeEndorsedAction: &peer.ChaincodeEndorsedAction{
					ProposalResponsePayload: []byte("proposal-response-payload-bytes"),
				},
			},
		},
		{
			testSpec: "peer.ChaincodeProposalPayload",
			msg: &peer.ChaincodeProposalPayload{
				Input: []byte("input-bytes"),
			},
			expectedReturn: &peerext.ChaincodeProposalPayload{
				ChaincodeProposalPayload: &peer.ChaincodeProposalPayload{
					Input: []byte("input-bytes"),
				},
			},
		},
		{
			testSpec: "peer.ProposalResponsePayload",
			msg: &peer.ProposalResponsePayload{
				ProposalHash: []byte("proposal-hash-bytes"),
			},
			expectedReturn: &peerext.ProposalResponsePayload{
				ProposalResponsePayload: &peer.ProposalResponsePayload{
					ProposalHash: []byte("proposal-hash-bytes"),
				},
			},
		},
		{
			testSpec: "peer.TransactionAction",
			msg: &peer.TransactionAction{
				Header: []byte("header-bytes"),
			},
			expectedReturn: &peerext.TransactionAction{
				TransactionAction: &peer.TransactionAction{
					Header: []byte("header-bytes"),
				},
			},
		},
		{
			testSpec: "rwset.TxReadWriteSet",
			msg: &rwset.TxReadWriteSet{
				NsRwset: []*rwset.NsReadWriteSet{
					{
						Namespace: "namespace",
					},
				},
			},
			expectedReturn: &rwsetext.TxReadWriteSet{
				TxReadWriteSet: &rwset.TxReadWriteSet{
					NsRwset: []*rwset.NsReadWriteSet{
						{
							Namespace: "namespace",
						},
					},
				},
			},
		},
		{
			testSpec: "default",
			msg: &GenericProtoMessage{
				GenericField: "test",
			},
			expectedReturn: &GenericProtoMessage{
				GenericField: "test",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.testSpec, func(t *testing.T) {
			gt := NewGomegaWithT(t)
			decoratedMsg := Decorate(tt.msg)
			gt.Expect(proto.Equal(decoratedMsg, tt.expectedReturn)).To(BeTrue())
		})
	}
}
