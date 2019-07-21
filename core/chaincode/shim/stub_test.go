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
	. "github.com/onsi/gomega"
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
		gt := NewGomegaWithT(t)

		stub, err := newChaincodeStub(
			&Handler{},
			"channel-id",
			"transaction-id",
			&pb.ChaincodeInput{Args: expectedArgs, Decorations: expectedDecorations},
			tt.signedProposal,
		)
		if tt.expectedErr != "" {
			gt.Expect(err).To(HaveOccurred())
			gt.Expect(err).To(MatchError(tt.expectedErr))
			continue
		}
		gt.Expect(err).NotTo(HaveOccurred())
		gt.Expect(stub).NotTo(BeNil())

		gt.Expect(stub.handler).To(Equal(&Handler{}))
		gt.Expect(stub.ChannelId).To(Equal("channel-id"))
		gt.Expect(stub.TxID).To(Equal("transaction-id"))
		gt.Expect(stub.args).To(Equal(expectedArgs))
		gt.Expect(stub.decorations).To(Equal(expectedDecorations))
		gt.Expect(stub.validationParameterMetakey).To(Equal("VALIDATION_PARAMETER"))
		if tt.signedProposal == nil {
			gt.Expect(stub.proposal).To(BeNil())
			gt.Expect(stub.creator).To(BeNil())
			gt.Expect(stub.transient).To(BeNil())
			gt.Expect(stub.binding).To(BeNil())
			continue
		}

		prop := &pb.Proposal{}
		err = proto.Unmarshal(tt.signedProposal.ProposalBytes, prop)
		gt.Expect(err).NotTo(HaveOccurred())
		gt.Expect(stub.proposal).To(Equal(prop))

		gt.Expect(stub.creator).To(Equal(expectedCreator))
		gt.Expect(stub.transient).To(Equal(expectedTransient))

		epoch := make([]byte, 8)
		binary.LittleEndian.PutUint64(epoch, expectedEpoch)
		shdr := &common.SignatureHeader{}
		digest := sha256.Sum256(append(append(shdr.GetNonce(), expectedCreator...), epoch...))
		gt.Expect(stub.binding).To(Equal(digest[:]))
	}
}

func TestChaincodeStubSetEvent(t *testing.T) {
	gt := NewGomegaWithT(t)

	stub := &ChaincodeStub{}
	err := stub.SetEvent("", []byte("event payload"))
	gt.Expect(err).To(MatchError("event name can not be empty string"))
	gt.Expect(stub.chaincodeEvent).To(BeNil())

	stub = &ChaincodeStub{}
	err = stub.SetEvent("name", []byte("payload"))
	gt.Expect(err).NotTo(HaveOccurred())
	gt.Expect(stub.chaincodeEvent).To(Equal(&pb.ChaincodeEvent{
		EventName: "name",
		Payload:   []byte("payload"),
	}))
}

func TestChaincodeStubAccessors(t *testing.T) {
	gt := NewGomegaWithT(t)

	stub := &ChaincodeStub{TxID: "transaction-id"}
	gt.Expect(stub.GetTxID()).To(Equal("transaction-id"))

	stub = &ChaincodeStub{ChannelId: "channel-id"}
	gt.Expect(stub.GetChannelID()).To(Equal("channel-id"))

	stub = &ChaincodeStub{decorations: map[string][]byte{"key": []byte("value")}}
	gt.Expect(stub.GetDecorations()).To(Equal(map[string][]byte{"key": []byte("value")}))

	stub = &ChaincodeStub{args: [][]byte{[]byte("function"), []byte("arg1"), []byte("arg2")}}
	gt.Expect(stub.GetArgs()).To(Equal([][]byte{[]byte("function"), []byte("arg1"), []byte("arg2")}))
	gt.Expect(stub.GetStringArgs()).To(Equal([]string{"function", "arg1", "arg2"}))

	f, a := stub.GetFunctionAndParameters()
	gt.Expect(f).To(Equal("function"))
	gt.Expect(a).To(Equal([]string{"arg1", "arg2"}))

	as, err := stub.GetArgsSlice()
	gt.Expect(err).NotTo(HaveOccurred())
	gt.Expect(as).To(Equal([]byte("functionarg1arg2")))

	stub = &ChaincodeStub{}
	f, a = stub.GetFunctionAndParameters()
	gt.Expect(f).To(Equal(""))
	gt.Expect(a).To(BeEmpty())

	stub = &ChaincodeStub{creator: []byte("creator")}
	creator, err := stub.GetCreator()
	gt.Expect(err).NotTo(HaveOccurred())
	gt.Expect(creator).To(Equal([]byte("creator")))

	stub = &ChaincodeStub{transient: map[string][]byte{"key": []byte("value")}}
	transient, err := stub.GetTransient()
	gt.Expect(err).NotTo(HaveOccurred())
	gt.Expect(transient).To(Equal(map[string][]byte{"key": []byte("value")}))

	stub = &ChaincodeStub{binding: []byte("binding")}
	binding, err := stub.GetBinding()
	gt.Expect(err).NotTo(HaveOccurred())
	gt.Expect(binding).To(Equal([]byte("binding")))

	stub = &ChaincodeStub{signedProposal: &pb.SignedProposal{ProposalBytes: []byte("proposal-bytes")}}
	sp, err := stub.GetSignedProposal()
	gt.Expect(err).NotTo(HaveOccurred())
	gt.Expect(sp).To(Equal(&pb.SignedProposal{ProposalBytes: []byte("proposal-bytes")}))
}

func TestChaincodeStubGetTxTimestamp(t *testing.T) {
	gt := NewGomegaWithT(t)

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
			gt.Expect(err).To(MatchError(tt.expectedErr))
			continue
		}

		gt.Expect(err).NotTo(HaveOccurred())
		gt.Expect(proto.Equal(ts, tt.ts)).To(BeTrue())
	}
}
