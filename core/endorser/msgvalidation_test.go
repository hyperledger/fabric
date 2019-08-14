/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package endorser_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/hyperledger/fabric/core/endorser"
	cb "github.com/hyperledger/fabric/protos/common"
	pb "github.com/hyperledger/fabric/protos/peer"
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
			Expect(err).To(MatchError("could not unmarshal proposal bytes: error unmarshaling Proposal: proto: can't skip unknown wire type 7"))
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
			Expect(err).To(MatchError("could not unmarshal header: error unmarshaling Header: proto: can't skip unknown wire type 7"))
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
			Expect(err).To(MatchError("could not unmarshal channel header: error unmarshaling ChannelHeader: proto: can't skip unknown wire type 7"))
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
			Expect(err).To(MatchError("could not unmarshal signature header: error unmarshaling SignatureHeader: proto: can't skip unknown wire type 7"))
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
			Expect(err).To(MatchError("could not unmarshal header extension: error unmarshaling ChaincodeHeaderExtension: proto: can't skip unknown wire type 7"))
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

	Context("when the payload visibility is set", func() {
		BeforeEach(func() {
			chaincodeHeaderExtension.PayloadVisibility = []byte("anything")
		})

		It("wraps and returns an error", func() {
			_, err := endorser.UnpackProposal(signedProposal)
			Expect(err).To(MatchError("invalid payload visibility field"))
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
			Expect(err).To(MatchError("could not get invocation spec: error unmarshaling ChaincodeInvocationSpec: proto: can't skip unknown wire type 7"))
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
