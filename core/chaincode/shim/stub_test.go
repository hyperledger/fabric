/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package shim

import (
	"crypto/sha256"
	"encoding/binary"
	"testing"

	"github.com/gogo/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/timestamp"
	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/protos/common"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/stretchr/testify/assert"
)

func TestNewChaincodeStub(t *testing.T) {
	expectedArgs := util.ToChaincodeArgs("function", "arg1", "arg2")
	expectedDecorations := map[string][]byte{"decoration-key": []byte("decoration-value")}
	expectedCreator := []byte("signature-header-creator")
	expectedTransient := map[string][]byte{"key": []byte("value")}
	expectedEpoch := uint64(999)

	validSignedProposal := &pb.SignedProposal{
		ProposalBytes: marshalOrPanic(&pb.Proposal{
			Header: marshalOrPanic(&common.Header{
				ChannelHeader: marshalOrPanic(&common.ChannelHeader{
					Type:  int32(common.HeaderType_ENDORSER_TRANSACTION),
					Epoch: expectedEpoch,
				}),
				SignatureHeader: marshalOrPanic(&common.SignatureHeader{
					Creator: expectedCreator,
				}),
			}),
			Payload: marshalOrPanic(&pb.ChaincodeProposalPayload{
				Input:        []byte("chaincode-proposal-input"),
				TransientMap: expectedTransient,
			}),
		}),
	}

	tests := []struct {
		signedProposal *pb.SignedProposal
		expectedErr    string
	}{
		{signedProposal: nil},
		{signedProposal: proto.Clone(validSignedProposal).(*pb.SignedProposal)},
		{
			signedProposal: &pb.SignedProposal{ProposalBytes: []byte("garbage")},
			expectedErr:    "failed to extract Proposal from SignedProposal: proto: can't skip unknown wire type 7",
		},
		{
			signedProposal: &pb.SignedProposal{},
			expectedErr:    "failed to extract Proposal fields: proposal header is nil",
		},
		{
			signedProposal: &pb.SignedProposal{},
			expectedErr:    "failed to extract Proposal fields: proposal header is nil",
		},
		{
			signedProposal: &pb.SignedProposal{
				ProposalBytes: marshalOrPanic(&pb.Proposal{
					Header: marshalOrPanic(&common.Header{
						ChannelHeader: marshalOrPanic(&common.ChannelHeader{
							Type:  int32(common.HeaderType_CONFIG_UPDATE),
							Epoch: expectedEpoch,
						}),
					}),
				}),
			},
			expectedErr: "invalid channel header type. Expected ENDORSER_TRANSACTION or CONFIG, received CONFIG_UPDATE",
		},
	}

	for _, tt := range tests {
		stub, err := newChaincodeStub(
			&Handler{},
			"channel-id",
			"transaction-id",
			&pb.ChaincodeInput{Args: expectedArgs[:], Decorations: expectedDecorations},
			tt.signedProposal,
		)
		if tt.expectedErr != "" {
			assert.Error(t, err)
			assert.EqualError(t, err, tt.expectedErr)
			continue
		}
		assert.NoError(t, err)
		assert.NotNil(t, stub)

		assert.Equal(t, &Handler{}, stub.handler, "expected empty handler")
		assert.Equal(t, "channel-id", stub.ChannelId)
		assert.Equal(t, "transaction-id", stub.TxID)
		assert.Equal(t, expectedArgs, stub.args)
		assert.Equal(t, expectedDecorations, stub.decorations)
		assert.Equal(t, "VALIDATION_PARAMETER", stub.validationParameterMetakey)
		if tt.signedProposal == nil {
			assert.Nil(t, stub.proposal, "expected nil proposal")
			assert.Nil(t, stub.creator, "expected nil creator")
			assert.Nil(t, stub.transient, "expected nil transient")
			assert.Nil(t, stub.binding, "expected nil binding")
			continue
		}

		prop := &pb.Proposal{}
		err = proto.Unmarshal(tt.signedProposal.ProposalBytes, prop)
		assert.NoError(t, err)
		assert.Equal(t, prop, stub.proposal)

		assert.Equal(t, expectedCreator, stub.creator)
		assert.Equal(t, expectedTransient, stub.transient)

		epoch := make([]byte, 8)
		binary.LittleEndian.PutUint64(epoch, expectedEpoch)
		shdr := &common.SignatureHeader{}
		digest := sha256.Sum256(append(append(shdr.GetNonce(), expectedCreator...), epoch...))
		assert.Equal(t, digest[:], stub.binding)
	}
}

func TestChaincodeStubSetEvent(t *testing.T) {
	stub := &ChaincodeStub{}
	err := stub.SetEvent("", []byte("event payload"))
	assert.EqualError(t, err, "event name can not be empty string")
	assert.Nil(t, stub.chaincodeEvent)

	stub = &ChaincodeStub{}
	err = stub.SetEvent("name", []byte("payload"))
	assert.NoError(t, err)
	assert.Equal(t, &pb.ChaincodeEvent{EventName: "name", Payload: []byte("payload")}, stub.chaincodeEvent)
}

func TestChaincodeStubAccessors(t *testing.T) {
	stub := &ChaincodeStub{TxID: "transaction-id"}
	assert.Equal(t, "transaction-id", stub.GetTxID())

	stub = &ChaincodeStub{ChannelId: "channel-id"}
	assert.Equal(t, "channel-id", stub.GetChannelID())

	stub = &ChaincodeStub{decorations: map[string][]byte{"key": []byte("value")}}
	assert.Equal(t, map[string][]byte{"key": []byte("value")}, stub.GetDecorations())

	stub = &ChaincodeStub{args: [][]byte{[]byte("function"), []byte("arg1"), []byte("arg2")}}
	assert.Equal(t, [][]byte{[]byte("function"), []byte("arg1"), []byte("arg2")}, stub.GetArgs())
	assert.Equal(t, []string{"function", "arg1", "arg2"}, stub.GetStringArgs())

	f, a := stub.GetFunctionAndParameters()
	assert.Equal(t, "function", f)
	assert.Equal(t, []string{"arg1", "arg2"}, a)

	as, err := stub.GetArgsSlice()
	assert.NoError(t, err)
	assert.Equal(t, []byte("functionarg1arg2"), as)

	stub = &ChaincodeStub{}
	f, a = stub.GetFunctionAndParameters()
	assert.Equal(t, "", f)
	assert.Empty(t, a)

	stub = &ChaincodeStub{creator: []byte("creator")}
	creator, err := stub.GetCreator()
	assert.NoError(t, err)
	assert.Equal(t, []byte("creator"), creator)

	stub = &ChaincodeStub{transient: map[string][]byte{"key": []byte("value")}}
	transient, err := stub.GetTransient()
	assert.NoError(t, err)
	assert.Equal(t, map[string][]byte{"key": []byte("value")}, transient)

	stub = &ChaincodeStub{binding: []byte("binding")}
	binding, err := stub.GetBinding()
	assert.NoError(t, err)
	assert.Equal(t, []byte("binding"), binding)

	stub = &ChaincodeStub{signedProposal: &pb.SignedProposal{ProposalBytes: []byte("proposal-bytes")}}
	sp, err := stub.GetSignedProposal()
	assert.NoError(t, err)
	assert.Equal(t, &pb.SignedProposal{ProposalBytes: []byte("proposal-bytes")}, sp)
}

func TestChaincodeStubGetTxTimestamp(t *testing.T) {
	now := ptypes.TimestampNow()
	tests := []struct {
		proposal    *pb.Proposal
		ts          *timestamp.Timestamp
		expectedErr string
	}{
		{
			ts: now,
			proposal: &pb.Proposal{
				Header: marshalOrPanic(&common.Header{
					ChannelHeader: marshalOrPanic(&common.ChannelHeader{
						Timestamp: now,
					}),
				}),
			},
		},
		{
			proposal: &pb.Proposal{
				Header: marshalOrPanic(&common.Header{
					ChannelHeader: []byte("garbage-channel-header"),
				}),
			},
			expectedErr: "error unmarshaling ChannelHeader: proto: can't skip unknown wire type 7",
		},
		{
			proposal:    &pb.Proposal{Header: []byte("garbage-header")},
			expectedErr: "error unmarshaling Header: proto: can't skip unknown wire type 7",
		},
	}

	for _, tt := range tests {
		stub := &ChaincodeStub{proposal: tt.proposal}
		ts, err := stub.GetTxTimestamp()
		if tt.expectedErr != "" {
			assert.EqualError(t, err, tt.expectedErr)
			continue
		}

		assert.NoError(t, err)
		assert.True(t, proto.Equal(ts, tt.ts))
	}
}
