/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package endorser_test

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	cb "github.com/hyperledger/fabric-protos-go/common"
	mspproto "github.com/hyperledger/fabric-protos-go/msp"
	pb "github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/core/endorser"
	"github.com/hyperledger/fabric/core/endorser/fake"
	"github.com/hyperledger/fabric/protoutil"

	"github.com/golang/protobuf/proto"
)

var _ = Describe("UnpackProposal", func() {
	var (
		signedProposal           *pb.SignedProposal
		proposal                 *pb.Proposal
		header                   *cb.Header
		channelHeader            *cb.ChannelHeader
		signatureHeader          *cb.SignatureHeader
		chaincodeHeaderExtension *pb.ChaincodeHeaderExtension
		chaincodeProposalPayload *pb.ChaincodeProposalPayload
		chaincodeInvocationSpec  *pb.ChaincodeInvocationSpec
		chaincodeSpec            *pb.ChaincodeSpec
		chaincodeInput           *pb.ChaincodeInput
		chaincodeID              *pb.ChaincodeID

		marshalProposal                 func() []byte
		marshalHeader                   func() []byte
		marshalSignatureHeader          func() []byte
		marshalChannelHeader            func() []byte
		marshalChaincodeHeaderExtension func() []byte
		marshalChaincodeProposalPayload func() []byte
		marshalChaincodeInvocationSpec  func() []byte
	)

	BeforeEach(func() {
		/*
			// This is the natural signed proposal structure, however, for test, we need
			// to be able to control each of the elements, including the nested ones, so
			// we build it awkwardly through function pointers

			signedProposal = &pb.SignedProposal{
				ProposalBytes: protoutil.MarshalOrPanic(&pb.Proposal{
					Header: protoutil.MarshalOrPanic(&cb.Header{
						ChannelHeader: protoutil.MarshalOrPanic(&cb.ChannelHeader{
							Type: int32(cb.HeaderType_ENDORSER_TRANSACTION),
							Extension: protoutil.MarshalOrPanic(&pb.ChaincodeHeaderExtension{
								ChaincodeId: &pb.ChaincodeID{
									Name: "chaincode-name",
								},
							}),
						}),
						SignatureHeader: protoutil.MarshalOrPanic(&cb.SignatureHeader{}),
					}),
					Payload: protoutil.MarshalOrPanic(&pb.ChaincodeProposalPayload{
						Input: protoutil.MarshalOrPanic(&pb.ChaincodeInvocationSpec{
							ChaincodeSpec: &pb.ChaincodeSpec{
								Input: &pb.ChaincodeInput{
									Args: [][]byte{[]byte("arg1"), []byte("arg2"), []byte("arg3")},
								},
							},
						}),
					}),
				}),
			}
		*/
		chaincodeID = &pb.ChaincodeID{
			Name: "chaincode-name",
		}

		chaincodeHeaderExtension = &pb.ChaincodeHeaderExtension{
			ChaincodeId: chaincodeID,
		}

		marshalChaincodeHeaderExtension = func() []byte {
			return protoutil.MarshalOrPanic(chaincodeHeaderExtension)
		}

		channelHeader = &cb.ChannelHeader{
			Type: int32(cb.HeaderType_ENDORSER_TRANSACTION),
		}

		marshalChannelHeader = func() []byte {
			channelHeader.Extension = marshalChaincodeHeaderExtension()
			return protoutil.MarshalOrPanic(channelHeader)
		}

		signatureHeader = &cb.SignatureHeader{
			Creator: []byte("creator"),
			Nonce:   []byte("nonce"),
		}

		marshalSignatureHeader = func() []byte {
			return protoutil.MarshalOrPanic(signatureHeader)
		}

		header = &cb.Header{}

		marshalHeader = func() []byte {
			header.ChannelHeader = marshalChannelHeader()
			header.SignatureHeader = marshalSignatureHeader()
			return protoutil.MarshalOrPanic(header)
		}

		chaincodeInput = &pb.ChaincodeInput{
			Args: [][]byte{[]byte("arg1"), []byte("arg2"), []byte("arg3")},
		}

		chaincodeSpec = &pb.ChaincodeSpec{
			Input: chaincodeInput,
		}

		chaincodeInvocationSpec = &pb.ChaincodeInvocationSpec{
			ChaincodeSpec: chaincodeSpec,
		}

		marshalChaincodeInvocationSpec = func() []byte {
			return protoutil.MarshalOrPanic(chaincodeInvocationSpec)
		}

		chaincodeProposalPayload = &pb.ChaincodeProposalPayload{}

		marshalChaincodeProposalPayload = func() []byte {
			chaincodeProposalPayload.Input = marshalChaincodeInvocationSpec()
			return protoutil.MarshalOrPanic(chaincodeProposalPayload)
		}

		proposal = &pb.Proposal{}

		marshalProposal = func() []byte {
			proposal.Header = marshalHeader()
			proposal.Payload = marshalChaincodeProposalPayload()
			return protoutil.MarshalOrPanic(proposal)
		}
	})

	JustBeforeEach(func() {
		signedProposal = &pb.SignedProposal{
			ProposalBytes: marshalProposal(),
		}
	})

	It("unmarshals the signed proposal into the interesting bits and returns them as a struct", func() {
		up, err := endorser.UnpackProposal(signedProposal)
		Expect(err).NotTo(HaveOccurred())
		Expect(up.ChaincodeName).To(Equal("chaincode-name"))
		Expect(up.SignedProposal).To(Equal(signedProposal))
		Expect(proto.Equal(up.Proposal, proposal)).To(BeTrue())
		Expect(proto.Equal(up.Input, chaincodeInput)).To(BeTrue())
		Expect(proto.Equal(up.SignatureHeader, signatureHeader)).To(BeTrue())
		Expect(proto.Equal(up.ChannelHeader, channelHeader)).To(BeTrue())
	})

	Context("when the proposal bytes are invalid", func() {
		BeforeEach(func() {
			marshalProposal = func() []byte {
				return []byte("garbage")
			}
		})

		It("wraps and returns an error", func() {
			_, err := endorser.UnpackProposal(signedProposal)
			Expect(err).ToNot(BeNil())
			Expect(err.Error()).To(HavePrefix("error unmarshalling Proposal"))
		})
	})

	Context("when the header bytes are invalid", func() {
		BeforeEach(func() {
			marshalHeader = func() []byte {
				return []byte("garbage")
			}
		})

		It("wraps and returns the error", func() {
			_, err := endorser.UnpackProposal(signedProposal)
			Expect(err).ToNot(BeNil())
			Expect(err.Error()).To(HavePrefix("error unmarshalling Header"))
		})
	})

	Context("when the channel header bytes are invalid", func() {
		BeforeEach(func() {
			marshalChannelHeader = func() []byte {
				return []byte("garbage")
			}
		})

		It("wraps and returns an error", func() {
			_, err := endorser.UnpackProposal(signedProposal)
			Expect(err).ToNot(BeNil())
			Expect(err.Error()).To(HavePrefix("error unmarshalling ChannelHeader"))
		})
	})

	Context("when the signature header bytes are invalid", func() {
		BeforeEach(func() {
			marshalSignatureHeader = func() []byte {
				return []byte("garbage")
			}
		})

		It("wraps and returns an error", func() {
			_, err := endorser.UnpackProposal(signedProposal)
			Expect(err).ToNot(BeNil())
			Expect(err.Error()).To(HavePrefix("error unmarshalling SignatureHeader"))
		})
	})

	Context("when the chaincode header extension bytes are invalid", func() {
		BeforeEach(func() {
			marshalChaincodeHeaderExtension = func() []byte {
				return []byte("garbage")
			}
		})

		It("wraps and returns an error", func() {
			_, err := endorser.UnpackProposal(signedProposal)
			Expect(err).ToNot(BeNil())
			Expect(err.Error()).To(HavePrefix("error unmarshalling ChaincodeHeaderExtension"))
		})
	})

	Context("when the chaincode proposal payload is invalid", func() {
		BeforeEach(func() {
			marshalChaincodeProposalPayload = func() []byte {
				return []byte("garbage")
			}
		})

		It("wraps and returns an error", func() {
			_, err := endorser.UnpackProposal(signedProposal)
			Expect(err).ToNot(BeNil())
			Expect(err.Error()).To(HavePrefix("error unmarshalling ChaincodeProposalPayload"))
		})
	})

	Context("when the chaincode id is empty", func() {
		BeforeEach(func() {
			chaincodeHeaderExtension.ChaincodeId = nil
		})

		It("wraps and returns an error", func() {
			_, err := endorser.UnpackProposal(signedProposal)
			Expect(err).To(MatchError("ChaincodeHeaderExtension.ChaincodeId is nil"))
		})
	})

	Context("when the chaincode id name is empty", func() {
		BeforeEach(func() {
			chaincodeHeaderExtension.ChaincodeId.Name = ""
		})

		It("wraps and returns an error", func() {
			_, err := endorser.UnpackProposal(signedProposal)
			Expect(err).To(MatchError("ChaincodeHeaderExtension.ChaincodeId.Name is empty"))
		})
	})

	Context("when the chaincode invocation spec is invalid", func() {
		BeforeEach(func() {
			marshalChaincodeInvocationSpec = func() []byte {
				return []byte("garbage")
			}
		})

		It("wraps and returns an error", func() {
			_, err := endorser.UnpackProposal(signedProposal)
			Expect(err).ToNot(BeNil())
			Expect(err.Error()).To(HavePrefix("error unmarshalling ChaincodeInvocationSpec"))
		})
	})

	Context("when the chaincode invocation spec is has a nil chaincodespec", func() {
		BeforeEach(func() {
			chaincodeInvocationSpec.ChaincodeSpec = nil
		})

		It("wraps and returns an error", func() {
			_, err := endorser.UnpackProposal(signedProposal)
			Expect(err).To(MatchError("chaincode invocation spec did not contain chaincode spec"))
		})
	})

	Context("when the input is missing", func() {
		BeforeEach(func() {
			chaincodeSpec.Input = nil
		})

		It("wraps and returns an error", func() {
			_, err := endorser.UnpackProposal(signedProposal)
			Expect(err).To(MatchError("chaincode input did not contain any input"))
		})
	})
})

var _ = Describe("Validate", func() {
	var (
		up *endorser.UnpackedProposal

		fakeIdentity             *fake.Identity
		fakeIdentityDeserializer *fake.IdentityDeserializer
	)

	BeforeEach(func() {
		up = &endorser.UnpackedProposal{
			SignedProposal: &pb.SignedProposal{
				Signature:     []byte("signature"),
				ProposalBytes: []byte("payload"),
			},
			ChannelHeader: &cb.ChannelHeader{
				ChannelId: "channel-id",
				Type:      int32(cb.HeaderType_ENDORSER_TRANSACTION),
				TxId:      "876a1777b78e5e3a6d1aabf8b5a11b893c3838285b2f5eedca7d23e25365fcfd",
			},
			SignatureHeader: &cb.SignatureHeader{
				Nonce: []byte("nonce"),
				Creator: protoutil.MarshalOrPanic(&mspproto.SerializedIdentity{
					Mspid:   "mspid",
					IdBytes: []byte("identity"),
				}),
			},
		}

		fakeIdentity = &fake.Identity{}

		fakeIdentityDeserializer = &fake.IdentityDeserializer{}
		fakeIdentityDeserializer.DeserializeIdentityReturns(fakeIdentity, nil)
	})

	It("validates the proposal", func() {
		err := up.Validate(fakeIdentityDeserializer)
		Expect(err).NotTo(HaveOccurred())

		Expect(fakeIdentityDeserializer.DeserializeIdentityCallCount()).To(Equal(1))
		creator := fakeIdentityDeserializer.DeserializeIdentityArgsForCall(0)
		Expect(creator).To(Equal(protoutil.MarshalOrPanic(&mspproto.SerializedIdentity{
			Mspid:   "mspid",
			IdBytes: []byte("identity"),
		})))

		Expect(fakeIdentity.ValidateCallCount()).To(Equal(1))
		Expect(fakeIdentity.VerifyCallCount()).To(Equal(1))
		payload, sig := fakeIdentity.VerifyArgsForCall(0)
		Expect(payload).To(Equal([]byte("payload")))
		Expect(sig).To(Equal([]byte("signature")))
	})

	Context("when the header type is config", func() {
		BeforeEach(func() {
			up.ChannelHeader = &cb.ChannelHeader{
				Type: int32(cb.HeaderType_CONFIG),
				TxId: "876a1777b78e5e3a6d1aabf8b5a11b893c3838285b2f5eedca7d23e25365fcfd",
			}
		})

		It("preserves buggy behavior and does not error", func() {
			err := up.Validate(fakeIdentityDeserializer)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("when the header type is bad", func() {
		BeforeEach(func() {
			up.ChannelHeader = &cb.ChannelHeader{
				Type: int32(0),
				TxId: "876a1777b78e5e3a6d1aabf8b5a11b893c3838285b2f5eedca7d23e25365fcfd",
			}
		})

		It("returns an error", func() {
			err := up.Validate(fakeIdentityDeserializer)
			Expect(err).To(MatchError("invalid header type MESSAGE"))
		})
	})

	Context("when the signature is missing", func() {
		BeforeEach(func() {
			up.SignedProposal.Signature = nil
		})

		It("returns an error", func() {
			err := up.Validate(fakeIdentityDeserializer)
			Expect(err).To(MatchError("empty signature bytes"))
		})
	})

	Context("when the nonce is missing", func() {
		BeforeEach(func() {
			up.SignatureHeader.Nonce = nil
		})

		It("returns an error", func() {
			err := up.Validate(fakeIdentityDeserializer)
			Expect(err).To(MatchError("nonce is empty"))
		})
	})

	Context("when the creator is missing", func() {
		BeforeEach(func() {
			up.SignatureHeader.Creator = nil
		})

		It("returns an error", func() {
			err := up.Validate(fakeIdentityDeserializer)
			Expect(err).To(MatchError("creator is empty"))
		})
	})

	Context("when the epoch is nonzero", func() {
		BeforeEach(func() {
			up.ChannelHeader.Epoch = 7
		})

		It("returns an error", func() {
			err := up.Validate(fakeIdentityDeserializer)
			Expect(err).To(MatchError("epoch is non-zero"))
		})
	})

	Context("when the txid is wrong", func() {
		BeforeEach(func() {
			up.ChannelHeader.TxId = "fake-txid"
		})

		It("returns an error", func() {
			err := up.Validate(fakeIdentityDeserializer)
			Expect(err).To(MatchError("incorrectly computed txid 'fake-txid' -- expected '876a1777b78e5e3a6d1aabf8b5a11b893c3838285b2f5eedca7d23e25365fcfd'"))
		})
	})

	Context("when the proposal bytes are missing", func() {
		BeforeEach(func() {
			up.SignedProposal.ProposalBytes = nil
		})

		It("returns an error", func() {
			err := up.Validate(fakeIdentityDeserializer)
			Expect(err).To(MatchError("empty proposal bytes"))
		})
	})

	Context("when the identity cannot be deserialized", func() {
		BeforeEach(func() {
			fakeIdentityDeserializer.DeserializeIdentityReturns(nil, fmt.Errorf("fake-deserializing-error"))
		})

		It("returns a generic auth error", func() {
			err := up.Validate(fakeIdentityDeserializer)
			Expect(err).To(MatchError("access denied: channel [channel-id] creator org unknown, creator is malformed"))
		})
	})

	Context("when the identity is not valid", func() {
		BeforeEach(func() {
			fakeIdentity.GetMSPIdentifierReturns("mspid")
			fakeIdentity.ValidateReturns(fmt.Errorf("fake-validate-error"))
		})

		It("returns a generic auth error", func() {
			err := up.Validate(fakeIdentityDeserializer)
			Expect(err).To(MatchError("access denied: channel [channel-id] creator org [mspid]"))
		})
	})

	Context("when the identity signature is not valid", func() {
		BeforeEach(func() {
			fakeIdentity.GetMSPIdentifierReturns("mspid")
			fakeIdentity.VerifyReturns(fmt.Errorf("fake-verify-error"))
		})

		It("returns a generic auth error", func() {
			err := up.Validate(fakeIdentityDeserializer)
			Expect(err).To(MatchError("access denied: channel [channel-id] creator org [mspid]"))
		})
	})
})
