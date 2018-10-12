/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package server_test

import (
	"crypto/sha256"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"github.com/hyperledger/fabric/protos/token"
	"github.com/hyperledger/fabric/token/server"
	"github.com/hyperledger/fabric/token/server/mock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
)

var _ = Describe("Marshal", func() {
	Describe("UnmarshalCommand", func() {
		var (
			expectedCommand *token.Command
			encodedCommand  []byte
		)

		BeforeEach(func() {
			ts := ptypes.TimestampNow()

			expectedCommand = &token.Command{
				Header: &token.Header{
					Timestamp: ts,
					ChannelId: "channel-id",
					Nonce:     []byte{1, 2, 3, 4, 5},
					Creator:   []byte("creator"),
				},
				Payload: &token.Command_ImportRequest{
					ImportRequest: &token.ImportRequest{
						Credential: []byte("credential"),
						TokensToIssue: []*token.TokenToIssue{{
							Recipient: []byte("recipient"),
							Type:      "TYPE",
							Quantity:  999,
						}},
					},
				},
			}

			var err error
			encodedCommand, err = proto.Marshal(expectedCommand)
			Expect(err).NotTo(HaveOccurred())
		})

		It("unmarshals a protobuf encoded token command", func() {
			cmd, err := server.UnmarshalCommand(encodedCommand)
			Expect(err).NotTo(HaveOccurred())
			Expect(proto.Equal(cmd, expectedCommand)).To(BeTrue())
		})

		Context("when the encoded token cannot be unmarshaled", func() {
			It("returns an error", func() {
				_, err := server.UnmarshalCommand([]byte("garbage-command"))
				Expect(err).To(MatchError("proto: can't skip unknown wire type 7"))
			})
		})
	})

	Describe("NewResponseMarshaler", func() {
		var fakeSignerID *mock.SignerIdentity

		BeforeEach(func() {
			fakeSignerID = &mock.SignerIdentity{}
			fakeSignerID.SerializeReturns([]byte("creator"), nil)
		})

		It("returns a usable response marshaler", func() {
			rm, err := server.NewResponseMarshaler(fakeSignerID)
			Expect(err).NotTo(HaveOccurred())

			Expect(time.Since(rm.Time())).To(BeNumerically("<", time.Second))
			rm.Time = nil

			Expect(fakeSignerID.SerializeCallCount()).To(Equal(1))
			Expect(rm).To(Equal(&server.ResponseMarshaler{
				Signer:  fakeSignerID,
				Creator: []byte("creator"),
			}))
		})

		Context("when retrieving the serialized signer ID fails", func() {
			BeforeEach(func() {
				fakeSignerID.SerializeReturns(nil, errors.New("boing"))
			})

			It("returns an error", func() {
				_, err := server.NewResponseMarshaler(fakeSignerID)
				Expect(err).To(MatchError("boing"))
			})
		})
	})

	Describe("ResponseMarshaler", func() {
		var (
			fakeSignerID           *mock.SignerIdentity
			expectedResponseHeader *token.CommandResponseHeader

			rm *server.ResponseMarshaler
		)

		BeforeEach(func() {
			now := time.Now()
			expectedTimestamp, err := ptypes.TimestampProto(now)
			Expect(err).NotTo(HaveOccurred())

			commandHash := sha256.Sum256([]byte("command"))
			expectedResponseHeader = &token.CommandResponseHeader{
				Creator:     []byte("creator"),
				CommandHash: commandHash[:],
				Timestamp:   expectedTimestamp,
			}

			fakeSignerID = &mock.SignerIdentity{}
			fakeSignerID.SignReturns([]byte("signature"), nil)
			rm = &server.ResponseMarshaler{
				Signer:  fakeSignerID,
				Creator: []byte("creator"),
				Time:    func() time.Time { return now },
			}
		})

		It("marshals and signs TokenTransaction responses", func() {
			tokenTransactionResponse := &token.CommandResponse_TokenTransaction{
				TokenTransaction: &token.TokenTransaction{
					Action: &token.TokenTransaction_PlainAction{
						PlainAction: &token.PlainTokenAction{
							Data: &token.PlainTokenAction_PlainImport{
								PlainImport: &token.PlainImport{
									Outputs: []*token.PlainOutput{
										{Owner: []byte("owner-1"), Type: "TOK1", Quantity: 888},
										{Owner: []byte("owner-2"), Type: "TOK2", Quantity: 999},
									},
								},
							},
						},
					},
				},
			}
			marshaledCommandResponse, err := proto.Marshal(&token.CommandResponse{
				Header:  expectedResponseHeader,
				Payload: tokenTransactionResponse,
			})
			Expect(err).NotTo(HaveOccurred())

			scr, err := rm.MarshalCommandResponse([]byte("command"), tokenTransactionResponse)
			Expect(err).NotTo(HaveOccurred())

			Expect(scr).To(Equal(&token.SignedCommandResponse{
				Response:  marshaledCommandResponse,
				Signature: []byte("signature"),
			}))
		})

		It("marshals and signs Err responses", func() {
			errResponse := &token.CommandResponse_Err{
				Err: &token.Error{
					Message: "error-message",
					Payload: []byte("payload"),
				},
			}

			marshaledCommandResponse, err := proto.Marshal(&token.CommandResponse{
				Header:  expectedResponseHeader,
				Payload: errResponse,
			})
			Expect(err).NotTo(HaveOccurred())

			scr, err := rm.MarshalCommandResponse([]byte("command"), errResponse)
			Expect(err).NotTo(HaveOccurred())
			Expect(scr).To(Equal(&token.SignedCommandResponse{
				Response:  marshaledCommandResponse,
				Signature: []byte("signature"),
			}))
		})

		Context("when marshal is called with an unexpected response payload type", func() {
			It("returns an error", func() {
				_, err := rm.MarshalCommandResponse([]byte("command"), nil)
				Expect(err).To(MatchError("command type not recognized: <nil>"))
			})
		})

		Context("when constructing the protobuf timestamp fails", func() {
			BeforeEach(func() {
				rm.Time = func() time.Time {
					return time.Date(-9999, 1, 1, 1, 1, 1, 0, time.UTC)
				}
			})

			It("returns an error", func() {
				_, err := rm.MarshalCommandResponse([]byte("command"), &token.CommandResponse_Err{})
				Expect(err).To(MatchError("timestamp: seconds:-377705113139  before 0001-01-01"))
			})
		})

		Context("when marshaling the response fails", func() {
			It("returns an error", func() {
				_, err := rm.MarshalCommandResponse([]byte("command"), &token.CommandResponse_Err{Err: nil})
				Expect(err).To(MatchError("proto: oneof field has nil value"))
			})
		})

		Context("when signing the response fails", func() {
			BeforeEach(func() {
				fakeSignerID.SignReturns(nil, errors.New("welp"))
			})

			It("returns an error", func() {
				_, err := rm.MarshalCommandResponse([]byte("command"), &token.CommandResponse_Err{
					Err: &token.Error{},
				})
				Expect(err).To(MatchError("welp"))
			})
		})
	})
})
