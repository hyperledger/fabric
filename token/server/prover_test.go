/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package server_test

import (
	"context"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/protos/token"
	"github.com/hyperledger/fabric/token/server"
	"github.com/hyperledger/fabric/token/server/mock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
)

var _ = Describe("Prover", func() {
	var (
		fakePolicyChecker *mock.PolicyChecker
		fakeMarshaler     *mock.Marshaler
		fakeIssuer        *mock.Issuer
		fakeTMSManager    *mock.TMSManager

		prover *server.Prover

		importRequest    *token.ImportRequest
		command          *token.Command
		marshaledCommand []byte
		signedCommand    *token.SignedCommand

		tokenTransaction  *token.TokenTransaction
		marshaledResponse *token.SignedCommandResponse
	)

	BeforeEach(func() {
		fakePolicyChecker = &mock.PolicyChecker{}

		tokenTransaction = &token.TokenTransaction{
			Action: &token.TokenTransaction_PlainAction{
				PlainAction: &token.PlainTokenAction{
					Data: &token.PlainTokenAction_PlainImport{
						PlainImport: &token.PlainImport{
							Outputs: []*token.PlainOutput{{
								Owner:    []byte("token-owner"),
								Type:     "PDQ",
								Quantity: 777,
							}},
						},
					},
				},
			},
		}
		fakeIssuer = &mock.Issuer{}
		fakeIssuer.RequestImportReturns(tokenTransaction, nil)

		fakeTMSManager = &mock.TMSManager{}
		fakeTMSManager.GetIssuerReturns(fakeIssuer, nil)

		marshaledResponse = &token.SignedCommandResponse{Response: []byte("signed-command-response")}
		fakeMarshaler = &mock.Marshaler{}
		fakeMarshaler.MarshalCommandResponseReturns(marshaledResponse, nil)

		prover = &server.Prover{
			PolicyChecker: fakePolicyChecker,
			Marshaler:     fakeMarshaler,
			TMSManager:    fakeTMSManager,
		}

		importRequest = &token.ImportRequest{
			Credential: []byte("credential"),
			TokensToIssue: []*token.TokenToIssue{{
				Recipient: []byte("recipient"),
				Type:      "XYZ",
				Quantity:  99,
			}},
		}
		command = &token.Command{
			Header: &token.Header{
				ChannelId: "channel-id",
				Creator:   []byte("creator"),
				Nonce:     []byte("nonce"),
			},
			Payload: &token.Command_ImportRequest{
				ImportRequest: importRequest,
			},
		}
		marshaledCommand = ProtoMarshal(command)
		signedCommand = &token.SignedCommand{
			Command:   marshaledCommand,
			Signature: []byte("command-signature"),
		}
	})

	Describe("ProcessCommand", func() {
		It("performs access control checks", func() {
			_, err := prover.ProcessCommand(context.Background(), signedCommand)
			Expect(err).NotTo(HaveOccurred())

			Expect(fakePolicyChecker.CheckCallCount()).To(Equal(1))
			sc, c := fakePolicyChecker.CheckArgsForCall(0)
			Expect(sc).To(Equal(signedCommand))
			Expect(proto.Equal(c, command)).To(BeTrue())
		})

		It("returns a signed command response", func() {
			resp, err := prover.ProcessCommand(context.Background(), signedCommand)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp).To(Equal(marshaledResponse))

			Expect(fakeMarshaler.MarshalCommandResponseCallCount()).To(Equal(1))
			cmd, payload := fakeMarshaler.MarshalCommandResponseArgsForCall(0)
			Expect(cmd).To(Equal(marshaledCommand))
			Expect(payload).To(Equal(&token.CommandResponse_TokenTransaction{
				TokenTransaction: tokenTransaction,
			}))
		})

		Context("when the access control check fails", func() {
			BeforeEach(func() {
				fakePolicyChecker.CheckReturns(errors.New("banana-time"))
			})

			It("returns an error response", func() {
				resp, err := prover.ProcessCommand(context.Background(), signedCommand)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp).To(Equal(marshaledResponse))

				Expect(fakeMarshaler.MarshalCommandResponseCallCount()).To(Equal(1))
				cmd, payload := fakeMarshaler.MarshalCommandResponseArgsForCall(0)
				Expect(cmd).To(Equal(marshaledCommand))
				Expect(payload).To(Equal(&token.CommandResponse_Err{
					Err: &token.Error{Message: "banana-time"},
				}))
			})

			It("does not perform the operation", func() {
				_, err := prover.ProcessCommand(context.Background(), signedCommand)
				Expect(err).NotTo(HaveOccurred())

				Expect(fakeTMSManager.Invocations()).To(BeEmpty())
			})
		})

		Context("when the unmarshaling the command fails", func() {
			BeforeEach(func() {
				signedCommand.Command = []byte("garbage-in")
			})

			It("returns an error response", func() {
				resp, err := prover.ProcessCommand(context.Background(), signedCommand)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp).To(Equal(marshaledResponse))

				Expect(fakeMarshaler.MarshalCommandResponseCallCount()).To(Equal(1))
				cmd, payload := fakeMarshaler.MarshalCommandResponseArgsForCall(0)
				Expect(cmd).To(Equal([]byte("garbage-in")))
				Expect(payload).To(Equal(&token.CommandResponse_Err{
					Err: &token.Error{Message: "proto: can't skip unknown wire type 7"},
				}))
			})
		})

		Context("when the command header is missing", func() {
			BeforeEach(func() {
				command.Header = nil
				signedCommand.Command = ProtoMarshal(command)
			})

			It("returns an error response", func() {
				resp, err := prover.ProcessCommand(context.Background(), signedCommand)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp).To(Equal(marshaledResponse))

				Expect(fakeMarshaler.MarshalCommandResponseCallCount()).To(Equal(1))
				cmd, payload := fakeMarshaler.MarshalCommandResponseArgsForCall(0)
				Expect(cmd).To(Equal(ProtoMarshal(command)))
				Expect(payload).To(Equal(&token.CommandResponse_Err{
					Err: &token.Error{Message: "command header is required"},
				}))
			})
		})

		Context("when the channel is missing from the command header", func() {
			BeforeEach(func() {
				command.Header.ChannelId = ""
				signedCommand.Command = ProtoMarshal(command)
			})

			It("returns an error response", func() {
				resp, err := prover.ProcessCommand(context.Background(), signedCommand)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp).To(Equal(marshaledResponse))

				Expect(fakeMarshaler.MarshalCommandResponseCallCount()).To(Equal(1))
				cmd, payload := fakeMarshaler.MarshalCommandResponseArgsForCall(0)
				Expect(cmd).To(Equal(ProtoMarshal(command)))
				Expect(payload).To(Equal(&token.CommandResponse_Err{
					Err: &token.Error{Message: "channel ID is required in header"},
				}))
			})
		})

		Context("when the nonce is missing from the command header", func() {
			BeforeEach(func() {
				command.Header.Nonce = nil
				signedCommand.Command = ProtoMarshal(command)
			})

			It("returns an error response", func() {
				resp, err := prover.ProcessCommand(context.Background(), signedCommand)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp).To(Equal(marshaledResponse))

				Expect(fakeMarshaler.MarshalCommandResponseCallCount()).To(Equal(1))
				cmd, payload := fakeMarshaler.MarshalCommandResponseArgsForCall(0)
				Expect(cmd).To(Equal(ProtoMarshal(command)))
				Expect(payload).To(Equal(&token.CommandResponse_Err{
					Err: &token.Error{Message: "nonce is required in header"},
				}))
			})
		})

		Context("when the creator is missing from the command header", func() {
			BeforeEach(func() {
				command.Header.Creator = nil
				signedCommand.Command = ProtoMarshal(command)
			})

			It("returns an error response", func() {
				resp, err := prover.ProcessCommand(context.Background(), signedCommand)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp).To(Equal(marshaledResponse))

				Expect(fakeMarshaler.MarshalCommandResponseCallCount()).To(Equal(1))
				cmd, payload := fakeMarshaler.MarshalCommandResponseArgsForCall(0)
				Expect(cmd).To(Equal(ProtoMarshal(command)))
				Expect(payload).To(Equal(&token.CommandResponse_Err{
					Err: &token.Error{Message: "creator is required in header"},
				}))
			})
		})

		Context("when an unknown command is received", func() {
			BeforeEach(func() {
				command.Payload = nil
				signedCommand.Command = ProtoMarshal(command)
			})

			It("returns an error response", func() {
				resp, err := prover.ProcessCommand(context.Background(), signedCommand)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp).To(Equal(marshaledResponse))

				Expect(fakeMarshaler.MarshalCommandResponseCallCount()).To(Equal(1))
				cmd, payload := fakeMarshaler.MarshalCommandResponseArgsForCall(0)
				Expect(cmd).To(Equal(ProtoMarshal(command)))
				Expect(payload).To(Equal(&token.CommandResponse_Err{
					Err: &token.Error{Message: "command type not recognized: <nil>"},
				}))
			})
		})
	})

	Describe("RequestImport", func() {
		It("gets an issuer", func() {
			_, err := prover.RequestImport(context.Background(), command.Header, importRequest)
			Expect(err).NotTo(HaveOccurred())

			Expect(fakeTMSManager.GetIssuerCallCount()).To(Equal(1))
			channel, cred, creator := fakeTMSManager.GetIssuerArgsForCall(0)
			Expect(channel).To(Equal("channel-id"))
			Expect(cred).To(Equal([]byte("credential")))
			Expect(creator).To(Equal([]byte("creator")))
		})

		It("uses the issuer to request an import", func() {
			resp, err := prover.RequestImport(context.Background(), command.Header, importRequest)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp).To(Equal(&token.CommandResponse_TokenTransaction{
				TokenTransaction: tokenTransaction,
			}))

			Expect(fakeIssuer.RequestImportCallCount()).To(Equal(1))
			tti := fakeIssuer.RequestImportArgsForCall(0)
			Expect(tti).To(Equal(importRequest.TokensToIssue))
		})

		Context("when the TMS manager fails to get an issuer", func() {
			BeforeEach(func() {
				fakeTMSManager.GetIssuerReturns(nil, errors.New("boing boing"))
			})

			It("retuns the error", func() {
				_, err := prover.RequestImport(context.Background(), command.Header, importRequest)
				Expect(err).To(MatchError("boing boing"))
			})
		})

		Context("when the issuer fails to import", func() {
			BeforeEach(func() {
				fakeIssuer.RequestImportReturns(nil, errors.New("watermelon"))
			})

			It("retuns the error", func() {
				_, err := prover.RequestImport(context.Background(), command.Header, importRequest)
				Expect(err).To(MatchError("watermelon"))
			})
		})
	})
})
