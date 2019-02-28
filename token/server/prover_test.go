/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package server_test

import (
	"context"
	"net"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/protos/ledger/queryresult"
	"github.com/hyperledger/fabric/protos/token"
	mock3 "github.com/hyperledger/fabric/token/identity/mock"
	mock2 "github.com/hyperledger/fabric/token/ledger/mock"
	"github.com/hyperledger/fabric/token/server"
	"github.com/hyperledger/fabric/token/server/mock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
)

func clock() time.Time {
	return time.Time{}
}

var _ = Describe("Prover", func() {
	var (
		fakeCapabilityChecker *mock.CapabilityChecker
		fakePolicyChecker     *mock.PolicyChecker
		fakeMarshaler         *mock.Marshaler
		fakeIssuer            *mock.Issuer
		fakeTransactor        *mock.Transactor
		fakeTMSManager        *mock.TMSManager

		prover *server.Prover

		importRequest     *token.IssueRequest
		command           *token.Command
		marshaledCommand  []byte
		signedCommand     *token.SignedCommand
		tokenTransaction  *token.TokenTransaction
		marshaledResponse *token.SignedCommandResponse

		transferRequest    *token.TransferRequest
		trTokenTransaction *token.TokenTransaction

		redeemRequest          *token.RedeemRequest
		redeemTokenTransaction *token.TokenTransaction

		listRequest      *token.ListRequest
		unspentTokens    *token.UnspentTokens
		transactorTokens []*token.UnspentToken

		importExpectationRequest     *token.TokenOperationRequest
		importExpectationTransaction *token.TokenTransaction

		transferExpectationRequest     *token.TokenOperationRequest
		transferExpectationTransaction *token.TokenTransaction
	)

	BeforeEach(func() {
		fakeCapabilityChecker = &mock.CapabilityChecker{}
		fakeCapabilityChecker.FabTokenReturns(true, nil)
		fakePolicyChecker = &mock.PolicyChecker{}

		tokenTransaction = &token.TokenTransaction{
			Action: &token.TokenTransaction_TokenAction{
				TokenAction: &token.TokenAction{
					Data: &token.TokenAction_Issue{
						Issue: &token.Issue{
							Outputs: []*token.Token{{
								Owner:    &token.TokenOwner{Raw: []byte("token-owner")},
								Type:     "PDQ",
								Quantity: ToHex(777),
							}},
						},
					},
				},
			},
		}
		fakeIssuer = &mock.Issuer{}
		fakeIssuer.RequestIssueReturns(tokenTransaction, nil)

		trTokenTransaction = &token.TokenTransaction{
			Action: &token.TokenTransaction_TokenAction{
				TokenAction: &token.TokenAction{
					Data: &token.TokenAction_Transfer{
						Transfer: &token.Transfer{
							Inputs: []*token.TokenId{{
								TxId:  "txid",
								Index: 0,
							}},
							Outputs: []*token.Token{{
								Owner:    &token.TokenOwner{Raw: []byte("token-owner")},
								Type:     "PDQ",
								Quantity: ToHex(777),
							}},
						},
					},
				},
			},
		}
		fakeTransactor = &mock.Transactor{}
		fakeTransactor.RequestTransferReturns(trTokenTransaction, nil)

		redeemTokenTransaction = &token.TokenTransaction{
			Action: &token.TokenTransaction_TokenAction{
				TokenAction: &token.TokenAction{
					Data: &token.TokenAction_Redeem{
						Redeem: &token.Transfer{
							Inputs: []*token.TokenId{{
								TxId:  "txid",
								Index: 0,
							}},
							Outputs: []*token.Token{{
								Type:     "PDQ",
								Quantity: ToHex(50),
							}},
						},
					},
				},
			},
		}
		fakeTransactor.RequestRedeemReturns(redeemTokenTransaction, nil)

		transactorTokens = []*token.UnspentToken{
			{Id: &token.TokenId{TxId: "idaz", Index: 0}, Type: "typeaz", Quantity: ToHex(135)},
			{Id: &token.TokenId{TxId: "idby", Index: 0}, Type: "typeby", Quantity: ToHex(79)},
		}
		unspentTokens = &token.UnspentTokens{Tokens: transactorTokens}

		fakeTransactor.ListTokensReturns(unspentTokens, nil)

		fakeTMSManager = &mock.TMSManager{}
		fakeTMSManager.GetIssuerReturns(fakeIssuer, nil)
		fakeTMSManager.GetTransactorReturns(fakeTransactor, nil)

		marshaledResponse = &token.SignedCommandResponse{Response: []byte("signed-command-response")}
		fakeMarshaler = &mock.Marshaler{}
		fakeMarshaler.MarshalCommandResponseReturns(marshaledResponse, nil)

		prover = &server.Prover{
			CapabilityChecker: fakeCapabilityChecker,
			PolicyChecker:     fakePolicyChecker,
			Marshaler:         fakeMarshaler,
			TMSManager:        fakeTMSManager,
		}

		importRequest = &token.IssueRequest{
			Credential: []byte("credential"),
			TokensToIssue: []*token.Token{{
				Owner:    &token.TokenOwner{Raw: []byte("recipient")},
				Type:     "XYZ",
				Quantity: ToHex(99),
			}},
		}
		command = &token.Command{
			Header: &token.Header{
				ChannelId: "channel-id",
				Creator:   []byte("creator"),
				Nonce:     []byte("nonce"),
			},
			Payload: &token.Command_IssueRequest{
				IssueRequest: importRequest,
			},
		}
		marshaledCommand = ProtoMarshal(command)
		signedCommand = &token.SignedCommand{
			Command:   marshaledCommand,
			Signature: []byte("command-signature"),
		}

		transferRequest = &token.TransferRequest{
			Credential: []byte("credential"),
			TokenIds:   []*token.TokenId{},
			Shares: []*token.RecipientShare{{
				Recipient: &token.TokenOwner{Raw: []byte("recipient")},
				Quantity:  ToHex(99),
			}},
		}

		redeemRequest = &token.RedeemRequest{
			Credential: []byte("credential"),
			TokenIds:   []*token.TokenId{},
			Quantity:   ToHex(50),
		}

		listRequest = &token.ListRequest{
			Credential: []byte("credential"),
		}

		importExpectationRequest = &token.TokenOperationRequest{
			Credential: []byte("credential"),
			Operations: []*token.TokenOperation{{
				Operation: &token.TokenOperation_Action{
					Action: &token.TokenOperationAction{
						Payload: &token.TokenOperationAction_Issue{
							Issue: &token.TokenActionTerms{
								Sender: &token.TokenOwner{Raw: []byte("credential")},
								Outputs: []*token.Token{{
									Owner:    &token.TokenOwner{Raw: []byte("recipient")},
									Type:     "XYZ",
									Quantity: ToHex(99),
								}},
							},
						},
					},
				},
			},
			},
		}

		transferExpectationRequest = &token.TokenOperationRequest{
			Credential: []byte("credential"),
			TokenIds:   []*token.TokenId{{TxId: "robert", Index: uint32(0)}},
			Operations: []*token.TokenOperation{{
				Operation: &token.TokenOperation_Action{
					Action: &token.TokenOperationAction{
						Payload: &token.TokenOperationAction_Transfer{
							Transfer: &token.TokenActionTerms{
								Sender: &token.TokenOwner{Raw: []byte("credential")},
								Outputs: []*token.Token{{
									Owner:    &token.TokenOwner{Raw: []byte("token-owner")},
									Type:     "PDQ",
									Quantity: ToHex(777),
								}},
							},
						},
					},
				},
			},
			},
		}

		importExpectationTransaction = tokenTransaction
		transferExpectationTransaction = trTokenTransaction
		fakeIssuer.RequestTokenOperationReturns(tokenTransaction, nil)
		fakeTransactor.RequestTokenOperationReturns(trTokenTransaction, 1, nil)
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

		Context("when fabtoken capability is not enabled", func() {
			BeforeEach(func() {
				fakeCapabilityChecker.FabTokenReturns(false, nil)
			})

			It("returns a response with error", func() {
				resp, err := prover.ProcessCommand(context.Background(), signedCommand)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp).To(Equal(marshaledResponse))

				Expect(fakeMarshaler.MarshalCommandResponseCallCount()).To(Equal(1))
				cmd, payload := fakeMarshaler.MarshalCommandResponseArgsForCall(0)
				Expect(cmd).To(Equal(ProtoMarshal(command)))
				Expect(payload).To(Equal(&token.CommandResponse_Err{
					Err: &token.Error{Message: "FabToken capability not enabled for channel channel-id"},
				}))
			})
		})

		Context("when a panic is thrown", func() {

			It("returns nil response and no error", func() {
				resp, err := prover.ProcessCommand(context.Background(), nil)
				Expect(err).To(MatchError("ProcessCommand triggered panic: runtime error: invalid memory address or nil pointer dereference"))
				Expect(resp).To(BeNil())
			})
		})
	})

	Describe("ProcessCommand_RequestIssue", func() {
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
	})

	Describe("ProcessCommand_RequestTransfer", func() {
		BeforeEach(func() {
			command = &token.Command{
				Header: &token.Header{
					ChannelId: "channel-id",
					Creator:   []byte("creator"),
					Nonce:     []byte("nonce"),
				},
				Payload: &token.Command_TransferRequest{
					TransferRequest: transferRequest,
				},
			}
			marshaledCommand = ProtoMarshal(command)
			signedCommand = &token.SignedCommand{
				Command:   marshaledCommand,
				Signature: []byte("command-signature"),
			}
			fakeMarshaler.MarshalCommandResponseReturns(marshaledResponse, nil)
		})

		It("returns a signed command response", func() {
			resp, err := prover.ProcessCommand(context.Background(), signedCommand)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp).To(Equal(marshaledResponse))

			Expect(fakeMarshaler.MarshalCommandResponseCallCount()).To(Equal(1))
			cmd, payload := fakeMarshaler.MarshalCommandResponseArgsForCall(0)
			Expect(cmd).To(Equal(marshaledCommand))
			Expect(payload).To(Equal(&token.CommandResponse_TokenTransaction{
				TokenTransaction: trTokenTransaction,
			}))
		})
	})

	Describe("ProcessCommand_RequestRedeem", func() {
		BeforeEach(func() {
			command = &token.Command{
				Header: &token.Header{
					ChannelId: "channel-id",
					Creator:   []byte("creator"),
					Nonce:     []byte("nonce"),
				},
				Payload: &token.Command_RedeemRequest{
					RedeemRequest: redeemRequest,
				},
			}
			marshaledCommand = ProtoMarshal(command)
			signedCommand = &token.SignedCommand{
				Command:   marshaledCommand,
				Signature: []byte("command-signature"),
			}
			fakeMarshaler.MarshalCommandResponseReturns(marshaledResponse, nil)
		})

		It("returns a signed command response", func() {
			resp, err := prover.ProcessCommand(context.Background(), signedCommand)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp).To(Equal(marshaledResponse))

			Expect(fakeMarshaler.MarshalCommandResponseCallCount()).To(Equal(1))
			cmd, payload := fakeMarshaler.MarshalCommandResponseArgsForCall(0)
			Expect(cmd).To(Equal(marshaledCommand))
			Expect(payload).To(Equal(&token.CommandResponse_TokenTransaction{
				TokenTransaction: redeemTokenTransaction,
			}))
		})
	})

	Describe("Process RequestIssue command", func() {
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
	})

	Describe("Process ListUnspentTokens command", func() {
		BeforeEach(func() {
			command = &token.Command{
				Header: &token.Header{
					ChannelId: "channel-id",
					Creator:   []byte("creator"),
					Nonce:     []byte("nonce"),
				},
				Payload: &token.Command_ListRequest{
					ListRequest: listRequest,
				},
			}
			marshaledCommand = ProtoMarshal(command)
			signedCommand = &token.SignedCommand{
				Command:   marshaledCommand,
				Signature: []byte("command-signature"),
			}
		})

		It("returns a signed command response", func() {
			resp, err := prover.ProcessCommand(context.Background(), signedCommand)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp).To(Equal(marshaledResponse))

			Expect(fakeMarshaler.MarshalCommandResponseCallCount()).To(Equal(1))
			cmd, payload := fakeMarshaler.MarshalCommandResponseArgsForCall(0)
			Expect(cmd).To(Equal(marshaledCommand))
			Expect(payload).To(Equal(&token.CommandResponse_UnspentTokens{
				UnspentTokens: unspentTokens,
			}))
		})
	})

	Describe("RequestIssue", func() {
		It("gets an issuer", func() {
			_, err := prover.RequestIssue(context.Background(), command.Header, importRequest)
			Expect(err).NotTo(HaveOccurred())

			Expect(fakeTMSManager.GetIssuerCallCount()).To(Equal(1))
			channel, cred, creator := fakeTMSManager.GetIssuerArgsForCall(0)
			Expect(channel).To(Equal("channel-id"))
			Expect(cred).To(Equal([]byte("credential")))
			Expect(creator).To(Equal([]byte("creator")))
		})

		It("uses the issuer to request an import", func() {
			resp, err := prover.RequestIssue(context.Background(), command.Header, importRequest)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp).To(Equal(&token.CommandResponse_TokenTransaction{
				TokenTransaction: tokenTransaction,
			}))

			Expect(fakeIssuer.RequestIssueCallCount()).To(Equal(1))
			tti := fakeIssuer.RequestIssueArgsForCall(0)
			Expect(tti).To(Equal(importRequest.TokensToIssue))
		})

		Context("when the TMS manager fails to get an issuer", func() {
			BeforeEach(func() {
				fakeTMSManager.GetIssuerReturns(nil, errors.New("boing boing"))
			})

			It("returns the error", func() {
				_, err := prover.RequestIssue(context.Background(), command.Header, importRequest)
				Expect(err).To(MatchError("boing boing"))
			})
		})

		Context("when the issuer fails to import", func() {
			BeforeEach(func() {
				fakeIssuer.RequestIssueReturns(nil, errors.New("watermelon"))
			})

			It("returns the error", func() {
				_, err := prover.RequestIssue(context.Background(), command.Header, importRequest)
				Expect(err).To(MatchError("watermelon"))
			})
		})
	})

	Describe("RequestTransfer", func() {
		It("gets a transactor", func() {
			_, err := prover.RequestTransfer(context.Background(), command.Header, transferRequest)
			Expect(err).NotTo(HaveOccurred())

			Expect(fakeTMSManager.GetTransactorCallCount()).To(Equal(1))
			channel, cred, creator := fakeTMSManager.GetTransactorArgsForCall(0)
			Expect(channel).To(Equal("channel-id"))
			Expect(cred).To(Equal([]byte("credential")))
			Expect(creator).To(Equal([]byte("creator")))
		})

		It("uses the transactor to request a transfer", func() {
			resp, err := prover.RequestTransfer(context.Background(), command.Header, transferRequest)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp).To(Equal(&token.CommandResponse_TokenTransaction{
				TokenTransaction: trTokenTransaction,
			}))

			Expect(fakeTransactor.RequestTransferCallCount()).To(Equal(1))
			tr := fakeTransactor.RequestTransferArgsForCall(0)
			Expect(tr).To(Equal(transferRequest))
		})

		Context("when the TMS manager fails to get a transactor", func() {
			BeforeEach(func() {
				fakeTMSManager.GetTransactorReturns(nil, errors.New("boing boing"))
			})

			It("returns the error", func() {
				_, err := prover.RequestTransfer(context.Background(), command.Header, transferRequest)
				Expect(err).To(MatchError("boing boing"))
			})
		})

		Context("when the transactor fails to transfer", func() {
			BeforeEach(func() {
				fakeTransactor.RequestTransferReturns(nil, errors.New("watermelon"))
			})

			It("returns the error", func() {
				_, err := prover.RequestTransfer(context.Background(), command.Header, transferRequest)
				Expect(err).To(MatchError("watermelon"))
			})
		})
	})

	Describe("RequestRedeem", func() {
		It("gets a transactor", func() {
			_, err := prover.RequestRedeem(context.Background(), command.Header, redeemRequest)
			Expect(err).NotTo(HaveOccurred())

			Expect(fakeTMSManager.GetTransactorCallCount()).To(Equal(1))
			channel, cred, creator := fakeTMSManager.GetTransactorArgsForCall(0)
			Expect(channel).To(Equal("channel-id"))
			Expect(cred).To(Equal([]byte("credential")))
			Expect(creator).To(Equal([]byte("creator")))
		})

		It("uses the transactor to request a redemption", func() {
			resp, err := prover.RequestRedeem(context.Background(), command.Header, redeemRequest)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp).To(Equal(&token.CommandResponse_TokenTransaction{
				TokenTransaction: redeemTokenTransaction,
			}))

			Expect(fakeTransactor.RequestRedeemCallCount()).To(Equal(1))
			tr := fakeTransactor.RequestRedeemArgsForCall(0)
			Expect(tr).To(Equal(redeemRequest))
		})

		Context("when the TMS manager fails to get a transactor", func() {
			BeforeEach(func() {
				fakeTMSManager.GetTransactorReturns(nil, errors.New("boing boing"))
			})

			It("returns the error", func() {
				_, err := prover.RequestRedeem(context.Background(), command.Header, redeemRequest)
				Expect(err).To(MatchError("boing boing"))
			})
		})

		Context("when the transactor fails to redeem", func() {
			BeforeEach(func() {
				fakeTransactor.RequestRedeemReturns(nil, errors.New("watermelon"))
			})

			It("returns the error", func() {
				_, err := prover.RequestRedeem(context.Background(), command.Header, redeemRequest)
				Expect(err).To(MatchError("watermelon"))
			})
		})
	})

	Describe("RequestTokenOperation import", func() {
		It("gets an issuer", func() {
			_, err := prover.RequestTokenOperations(context.Background(), command.Header, importExpectationRequest)
			Expect(err).NotTo(HaveOccurred())

			Expect(fakeTMSManager.GetIssuerCallCount()).To(Equal(1))
			channel, cred, creator := fakeTMSManager.GetIssuerArgsForCall(0)
			Expect(channel).To(Equal("channel-id"))
			Expect(cred).To(Equal([]byte("credential")))
			Expect(creator).To(Equal([]byte("creator")))
		})

		It("uses the issuer to request an import", func() {
			resp, err := prover.RequestTokenOperations(context.Background(), command.Header, importExpectationRequest)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp).To(Equal(&token.CommandResponse_TokenTransactions{
				TokenTransactions: &token.TokenTransactions{
					Txs: []*token.TokenTransaction{importExpectationTransaction},
				},
			}))

			Expect(fakeIssuer.RequestTokenOperationCallCount()).To(Equal(1))
			request := fakeIssuer.RequestTokenOperationArgsForCall(0)
			Expect(request).To(Equal(importExpectationRequest.Operations[0]))
		})

		Context("when the TMS manager fails to get an issuer", func() {
			BeforeEach(func() {
				fakeTMSManager.GetIssuerReturns(nil, errors.New("boing boing"))
			})

			It("returns the error", func() {
				_, err := prover.RequestTokenOperations(context.Background(), command.Header, importExpectationRequest)
				Expect(err).To(MatchError("boing boing"))
			})
		})

		Context("when the issuer fails to import", func() {
			BeforeEach(func() {
				fakeIssuer.RequestTokenOperationReturns(nil, errors.New("watermelon"))
			})

			It("returns the error", func() {
				_, err := prover.RequestTokenOperations(context.Background(), command.Header, importExpectationRequest)
				Expect(err).To(MatchError("watermelon"))
			})
		})

		Context("when ExpectationRequest has nil Expectation", func() {
			BeforeEach(func() {
				importExpectationRequest = &token.TokenOperationRequest{
					Credential: []byte("credential"),
				}
			})

			It("returns the error", func() {
				_, err := prover.RequestTokenOperations(context.Background(), command.Header, importExpectationRequest)
				Expect(err).To(MatchError("no token operations requested"))
			})
		})

		Context("when ExpectationRequest has nil PlainExpectation", func() {
			BeforeEach(func() {
				importExpectationRequest = &token.TokenOperationRequest{
					Credential: []byte("credential"),
					Operations: []*token.TokenOperation{},
				}
			})

			It("returns the error", func() {
				_, err := prover.RequestTokenOperations(context.Background(), command.Header, importExpectationRequest)
				Expect(err).To(MatchError("no token operations requested"))
			})
		})
	})

	Describe("RequestTokenOperation transfer", func() {
		It("gets a transactor", func() {
			_, err := prover.RequestTokenOperations(context.Background(), command.Header, transferExpectationRequest)
			Expect(err).NotTo(HaveOccurred())

			Expect(fakeTMSManager.GetTransactorCallCount()).To(Equal(1))
			channel, cred, creator := fakeTMSManager.GetTransactorArgsForCall(0)
			Expect(channel).To(Equal("channel-id"))
			Expect(cred).To(Equal([]byte("credential")))
			Expect(creator).To(Equal([]byte("creator")))
		})

		It("uses the transactor to request a transfer operation request", func() {
			resp, err := prover.RequestTokenOperations(context.Background(), command.Header, transferExpectationRequest)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp).To(Equal(&token.CommandResponse_TokenTransactions{
				TokenTransactions: &token.TokenTransactions{
					Txs: []*token.TokenTransaction{transferExpectationTransaction},
				},
			}))

			Expect(fakeTransactor.RequestTokenOperationCallCount()).To(Equal(1))
			_, tr := fakeTransactor.RequestTokenOperationArgsForCall(0)
			Expect(tr).To(Equal(transferExpectationRequest.Operations[0]))
		})

		Context("when the TMS manager fails to get a transactor", func() {
			BeforeEach(func() {
				fakeTMSManager.GetTransactorReturns(nil, errors.New("boing boing"))
			})

			It("returns the error", func() {
				_, err := prover.RequestTokenOperations(context.Background(), command.Header, transferExpectationRequest)
				Expect(err).To(MatchError("boing boing"))
			})
		})

		Context("when the transactor fails to transfer", func() {
			BeforeEach(func() {
				fakeTransactor.RequestTokenOperationReturns(nil, 0, errors.New("watermelon"))
			})

			It("returns the error", func() {
				_, err := prover.RequestTokenOperations(context.Background(), command.Header, transferExpectationRequest)
				Expect(err).To(MatchError("watermelon"))
			})
		})
	})

	Describe("Issue tokens by a plain issuer", func() {
		var (
			manager         *server.Manager
			expectedTokenTx *token.TokenTransaction
		)

		BeforeEach(func() {
			fakeTokenOwnerValidatorManager := &mock3.TokenOwnerValidatorManager{}
			fakeTokenOwnerValidatorManager.GetReturns(&TestTokenOwnerValidator{}, nil)
			manager = &server.Manager{TokenOwnerValidatorManager: fakeTokenOwnerValidatorManager}

			prover = &server.Prover{
				CapabilityChecker: fakeCapabilityChecker,
				PolicyChecker:     fakePolicyChecker,
				Marshaler:         fakeMarshaler,
				TMSManager:        manager,
			}
			importRequest = &token.IssueRequest{
				Credential: []byte("credential"),
				TokensToIssue: []*token.Token{
					{
						Owner:    &token.TokenOwner{Raw: []byte("recipient1")},
						Type:     "XYZ1",
						Quantity: ToHex(10),
					},
					{
						Owner:    &token.TokenOwner{Raw: []byte("recipient2")},
						Type:     "XYZ2",
						Quantity: ToHex(200),
					},
				},
			}

			outputs := []*token.Token{
				{
					Owner:    &token.TokenOwner{Raw: []byte("recipient1")},
					Type:     "XYZ1",
					Quantity: ToHex(10),
				},
				{
					Owner:    &token.TokenOwner{Raw: []byte("recipient2")},
					Type:     "XYZ2",
					Quantity: ToHex(200),
				},
			}
			expectedTokenTx = &token.TokenTransaction{
				Action: &token.TokenTransaction_TokenAction{
					TokenAction: &token.TokenAction{
						Data: &token.TokenAction_Issue{
							Issue: &token.Issue{
								Outputs: outputs,
							},
						},
					},
				},
			}
		})

		Describe("RequestIssue", func() {
			It("returns a TokenTransaction response", func() {
				resp, err := prover.RequestIssue(context.Background(), command.Header, importRequest)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp).To(Equal(&token.CommandResponse_TokenTransaction{
					TokenTransaction: expectedTokenTx,
				}))
				Expect(fakeIssuer.RequestIssueCallCount()).To(Equal(0))
			})
		})

		Describe("ProcessCommand", func() {
			It("marshals a TokenTransaction response", func() {
				command = &token.Command{
					Header: &token.Header{
						ChannelId: "channel-id",
						Creator:   []byte("creator"),
						Nonce:     []byte("nonce"),
					},
					Payload: &token.Command_IssueRequest{
						IssueRequest: importRequest,
					},
				}
				marshaledCommand = ProtoMarshal(command)
				signedCommand = &token.SignedCommand{
					Command:   marshaledCommand,
					Signature: []byte("command-signature"),
				}

				resp, err := prover.ProcessCommand(context.Background(), signedCommand)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp).To(Equal(marshaledResponse))

				Expect(fakeIssuer.RequestIssueCallCount()).To(Equal(0))
				Expect(fakeMarshaler.MarshalCommandResponseCallCount()).To(Equal(1))
				cmd, payload := fakeMarshaler.MarshalCommandResponseArgsForCall(0)
				Expect(cmd).To(Equal(marshaledCommand))
				Expect(payload).To(Equal(&token.CommandResponse_TokenTransaction{
					TokenTransaction: expectedTokenTx,
				}))
			})
		})

		Describe("ProcessCommand via prover client", func() {
			var (
				fakeSignerIdentity *mock.SignerIdentity
				marshaler          *server.ResponseMarshaler
				proverEndpoint     string
				grpcSrv            *grpc.Server
			)

			BeforeEach(func() {
				fakeSignerIdentity = &mock.SignerIdentity{}
				fakeSignerIdentity.SerializeReturns([]byte("response_creator"), nil)
				fakeSignerIdentity.SignReturns([]byte("response_signature"), nil)
				marshaler, _ = server.NewResponseMarshaler(fakeSignerIdentity)

				prover.Marshaler = marshaler

				// start grpc server for prover
				listener, err := net.Listen("tcp", "127.0.0.1:")
				Expect(err).To(BeNil())
				grpcSrv = grpc.NewServer()
				token.RegisterProverServer(grpcSrv, prover)
				go grpcSrv.Serve(listener)

				proverEndpoint = listener.Addr().String()

				// prepare SignedCommand for grpc request
				command = &token.Command{
					Header: &token.Header{
						ChannelId: "channel-id",
						Creator:   []byte("response_creator"),
						Nonce:     []byte("nonce"),
					},
					Payload: &token.Command_IssueRequest{
						IssueRequest: importRequest,
					},
				}
				signedCommand = &token.SignedCommand{
					Command:   ProtoMarshal(command),
					Signature: []byte("command-signature"),
				}
			})

			AfterEach(func() {
				grpcSrv.Stop()
			})

			It("returns expected response", func() {
				// create grpc client
				clientConn, err := grpc.Dial(proverEndpoint, grpc.WithInsecure())
				Expect(err).To(BeNil())
				defer clientConn.Close()
				proverClient := token.NewProverClient(clientConn)

				resp, err := proverClient.ProcessCommand(context.Background(), signedCommand)
				Expect(err).NotTo(HaveOccurred())

				// cannot compare entire response because Timestamp field has dynamic value
				// compare TokenTransanction, header field and signature individually
				commandResp := &token.CommandResponse{}
				err = proto.Unmarshal(resp.Response, commandResp)
				Expect(err).NotTo(HaveOccurred())
				Expect(commandResp.GetTokenTransaction()).To(Equal(expectedTokenTx))
				Expect(commandResp.Header.Creator).To(Equal([]byte("response_creator")))
				Expect(commandResp.Header.CommandHash).To(Equal(util.ComputeSHA256(ProtoMarshal(command))))
				Expect(resp.Signature).To(Equal([]byte("response_signature")))
			})
		})
	})

	Describe("ListUnspentTokens", func() {
		It("gets a transactor", func() {
			_, err := prover.ListUnspentTokens(context.Background(), command.Header, listRequest)
			Expect(err).NotTo(HaveOccurred())

			Expect(fakeTMSManager.GetTransactorCallCount()).To(Equal(1))
			channel, cred, creator := fakeTMSManager.GetTransactorArgsForCall(0)
			Expect(channel).To(Equal("channel-id"))
			Expect(cred).To(Equal([]byte("credential")))
			Expect(creator).To(Equal([]byte("creator")))
		})

		It("uses the transactor to list unspent tokens", func() {
			resp, err := prover.ListUnspentTokens(context.Background(), command.Header, listRequest)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp).To(Equal(&token.CommandResponse_UnspentTokens{
				UnspentTokens: unspentTokens,
			}))

			Expect(fakeTransactor.ListTokensCallCount()).To(Equal(1))
		})

		Context("when the TMS manager fails to get a transactor", func() {
			BeforeEach(func() {
				fakeTMSManager.GetTransactorReturns(nil, errors.New("pineapple"))
			})

			It("returns the error", func() {
				_, err := prover.ListUnspentTokens(context.Background(), command.Header, listRequest)
				Expect(err).To(MatchError("pineapple"))
			})
		})

		Context("when the transactor fails to list tokens", func() {
			BeforeEach(func() {
				fakeTransactor.ListTokensReturns(nil, errors.New("pineapple"))
			})

			It("returns the error", func() {
				_, err := prover.ListUnspentTokens(context.Background(), command.Header, listRequest)
				Expect(err).To(MatchError("pineapple"))
			})
		})
	})

	Describe("ProcessCommand_RequestExpection for import", func() {
		BeforeEach(func() {
			command = &token.Command{
				Header: &token.Header{
					ChannelId: "channel-id",
					Creator:   []byte("creator"),
					Nonce:     []byte("nonce"),
				},
				Payload: &token.Command_TokenOperationRequest{
					TokenOperationRequest: importExpectationRequest,
				},
			}
			marshaledCommand = ProtoMarshal(command)
			signedCommand = &token.SignedCommand{
				Command:   marshaledCommand,
				Signature: []byte("command-signature"),
			}
			fakeMarshaler.MarshalCommandResponseReturns(marshaledResponse, nil)
		})

		It("returns a signed command response", func() {
			resp, err := prover.ProcessCommand(context.Background(), signedCommand)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp).To(Equal(marshaledResponse))

			Expect(fakeMarshaler.MarshalCommandResponseCallCount()).To(Equal(1))
			cmd, payload := fakeMarshaler.MarshalCommandResponseArgsForCall(0)
			Expect(cmd).To(Equal(marshaledCommand))
			Expect(payload).To(Equal(&token.CommandResponse_TokenTransactions{
				TokenTransactions: &token.TokenTransactions{
					Txs: []*token.TokenTransaction{importExpectationTransaction},
				},
			}))
		})
	})

	Describe("ProcessCommand_RequestExpection for transfer", func() {
		BeforeEach(func() {
			command = &token.Command{
				Header: &token.Header{
					ChannelId: "channel-id",
					Creator:   []byte("creator"),
					Nonce:     []byte("nonce"),
				},
				Payload: &token.Command_TokenOperationRequest{
					TokenOperationRequest: transferExpectationRequest,
				},
			}
			marshaledCommand = ProtoMarshal(command)
			signedCommand = &token.SignedCommand{
				Command:   marshaledCommand,
				Signature: []byte("command-signature"),
			}
			fakeMarshaler.MarshalCommandResponseReturns(marshaledResponse, nil)
		})

		It("returns a signed command response", func() {
			resp, err := prover.ProcessCommand(context.Background(), signedCommand)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp).To(Equal(marshaledResponse))

			Expect(fakeMarshaler.MarshalCommandResponseCallCount()).To(Equal(1))
			cmd, payload := fakeMarshaler.MarshalCommandResponseArgsForCall(0)
			Expect(cmd).To(Equal(marshaledCommand))
			Expect(payload).To(Equal(&token.CommandResponse_TokenTransactions{
				TokenTransactions: &token.TokenTransactions{
					Txs: []*token.TokenTransaction{transferExpectationTransaction},
				},
			}))
		})
	})
})

var _ = Describe("ProverListUnspentTokens", func() {
	var (
		marshaler        *server.ResponseMarshaler
		prover           *server.Prover
		listRequest      *token.ListRequest
		command          *token.Command
		marshaledCommand []byte
		signedCommand    *token.SignedCommand
		queryResult      *queryresult.KV
		expectedResponse *token.CommandResponse_UnspentTokens
	)
	BeforeEach(func() {
		fakeIterator := &mock2.ResultsIterator{}
		fakePolicyChecker := &mock.PolicyChecker{}
		fakeSigner := &mock.SignerIdentity{}
		fakeCapabilityChecker := &mock.CapabilityChecker{}
		fakeCapabilityChecker.FabTokenReturns(true, nil)

		fakeLedgerReader := &mock2.LedgerReader{}
		fakeLedgerManager := &mock2.LedgerManager{}
		fakeLedgerManager.GetLedgerReaderReturns(fakeLedgerReader, nil)

		fakeTokenOwnerValidatorManager := &mock3.TokenOwnerValidatorManager{}
		fakeTokenOwnerValidatorManager.GetReturns(&TestTokenOwnerValidator{}, nil)

		manager := &server.Manager{LedgerManager: fakeLedgerManager, TokenOwnerValidatorManager: fakeTokenOwnerValidatorManager}
		marshaler = &server.ResponseMarshaler{Signer: fakeSigner, Creator: []byte("Alice"), Time: clock}

		prover = &server.Prover{
			CapabilityChecker: fakeCapabilityChecker,
			Marshaler:         marshaler,
			PolicyChecker:     fakePolicyChecker,
			TMSManager:        manager,
		}

		fakeLedgerReader.GetStateRangeScanIteratorReturns(fakeIterator, nil)

		fakeIterator.NextReturns(queryResult, nil)
		fakeIterator.NextReturnsOnCall(1, nil, nil)

		fakeSigner.SignReturns([]byte("it is a signature"), nil)
		fakeSigner.SerializeReturns([]byte("creator"), nil)

	})
	It("initializes variables and expected responses", func() {
		listRequest = &token.ListRequest{Credential: []byte("creator")}

		command = &token.Command{
			Header: &token.Header{
				ChannelId: "channel-id",
				Creator:   []byte("Alice"),
				Nonce:     []byte("nonce"),
			},
			Payload: &token.Command_ListRequest{
				ListRequest: listRequest,
			},
		}
		marshaledCommand = ProtoMarshal(command)
		signedCommand = &token.SignedCommand{
			Command:   marshaledCommand,
			Signature: []byte("command-signature"),
		}
		outputToken, err := proto.Marshal(&token.Token{Owner: &token.TokenOwner{Raw: []byte("Alice")}, Type: "XYZ", Quantity: ToHex(100)})
		Expect(err).NotTo(HaveOccurred())

		key := generateKey("owner1", "1", "0", "tokenOutput")

		queryResult = &queryresult.KV{Key: key, Value: outputToken}

		unspentTokens := &token.UnspentTokens{Tokens: []*token.UnspentToken{{Type: "XYZ", Quantity: ToDecimal(100), Id: &token.TokenId{TxId: "1", Index: 0}}}}
		expectedResponse = &token.CommandResponse_UnspentTokens{UnspentTokens: unspentTokens}
	})

	Describe("ListUnspentTokens", func() {
		It("returns UnspentTokens", func() {
			response, err := prover.ListUnspentTokens(context.Background(), command.Header, listRequest)
			Expect(err).NotTo(HaveOccurred())
			Expect(response).To(Equal(expectedResponse))
		})
	})

	Describe("ProcessCommand using ResponseMarshaler", func() {
		It("marshals a ListTokens response", func() {

			marshaledResponse, err := marshaler.MarshalCommandResponse(marshaledCommand, expectedResponse)
			Expect(err).NotTo(HaveOccurred())

			resp, err := prover.ProcessCommand(context.Background(), signedCommand)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp).To(Equal(marshaledResponse))
		})
	})

	Describe("ProcessCommand with grpc service activated", func() {
		var (
			proverEndpoint string
			grpcSrv        *grpc.Server
		)

		BeforeEach(func() {

			// start grpc server for prover
			listener, err := net.Listen("tcp", "127.0.0.1:")
			Expect(err).To(BeNil())
			grpcSrv = grpc.NewServer()
			token.RegisterProverServer(grpcSrv, prover)
			go grpcSrv.Serve(listener)

			proverEndpoint = listener.Addr().String()

		})

		AfterEach(func() {
			grpcSrv.Stop()
		})

		It("returns expected response", func() {
			// create grpc client
			clientConn, err := grpc.Dial(proverEndpoint, grpc.WithInsecure())
			Expect(err).To(BeNil())
			defer clientConn.Close()
			proverClient := token.NewProverClient(clientConn)

			response, err := proverClient.ProcessCommand(context.Background(), signedCommand)
			Expect(err).NotTo(HaveOccurred())

			marshaledResponse, err := marshaler.MarshalCommandResponse(marshaledCommand, expectedResponse)
			Expect(err).NotTo(HaveOccurred())

			Expect(err).NotTo(HaveOccurred())
			Expect(response.Signature).To(Equal(marshaledResponse.Signature))
			Expect(response.Response).To(Equal(marshaledResponse.Response))
		})
	})
})

var _ = Describe("Prover Transfer using TMS", func() {
	var (
		fakeCapabilityChecker *mock.CapabilityChecker
		prover                *server.Prover
		marshaler             *server.ResponseMarshaler
		expectedResponse      *token.CommandResponse_TokenTransaction
		command               *token.Command
		signedCommand         *token.SignedCommand
		marshaledCommand      []byte
		transferRequest       *token.TransferRequest
	)
	It("initializes variables and expected responses", func() {
		tokenIDs := []*token.TokenId{{TxId: "1", Index: 0}, {TxId: "2", Index: 1}}

		shares := []*token.RecipientShare{
			{Recipient: &token.TokenOwner{Raw: []byte("Alice")}, Quantity: ToHex(20)},
			{Recipient: &token.TokenOwner{Raw: []byte("Bob")}, Quantity: ToHex(250)},
			{Recipient: &token.TokenOwner{Raw: []byte("Charlie")}, Quantity: ToHex(30)},
		}
		transferRequest = &token.TransferRequest{Credential: []byte("Alice"), TokenIds: tokenIDs, Shares: shares}

		command = &token.Command{
			Header: &token.Header{
				ChannelId: "channel-id",
				Creator:   []byte("Alice"),
				Nonce:     []byte("nonce"),
			},
			Payload: &token.Command_TransferRequest{
				TransferRequest: transferRequest,
			},
		}
		marshaledCommand = ProtoMarshal(command)
		signedCommand = &token.SignedCommand{
			Command:   marshaledCommand,
			Signature: []byte("command-signature"),
		}

		outTokens := make([]*token.Token, 3)
		outTokens[0] = &token.Token{Owner: &token.TokenOwner{Raw: []byte("Alice")}, Type: "XYZ", Quantity: ToHex(20)}
		outTokens[1] = &token.Token{Owner: &token.TokenOwner{Raw: []byte("Bob")}, Type: "XYZ", Quantity: ToHex(250)}
		outTokens[2] = &token.Token{Owner: &token.TokenOwner{Raw: []byte("Charlie")}, Type: "XYZ", Quantity: ToHex(30)}

		tokenTx := &token.TokenTransaction{
			Action: &token.TokenTransaction_TokenAction{
				TokenAction: &token.TokenAction{
					Data: &token.TokenAction_Transfer{
						Transfer: &token.Transfer{
							Inputs:  tokenIDs,
							Outputs: outTokens,
						},
					},
				},
			},
		}
		expectedResponse = &token.CommandResponse_TokenTransaction{TokenTransaction: tokenTx}
	})
	BeforeEach(func() {
		var err error
		inTokens := make([][]byte, 2)
		inTokens[0], err = proto.Marshal(&token.Token{
			Owner: &token.TokenOwner{Raw: []byte("Alice")}, Type: "XYZ", Quantity: ToHex(100)})
		Expect(err).NotTo(HaveOccurred())

		inTokens[1], err = proto.Marshal(&token.Token{
			Owner: &token.TokenOwner{Raw: []byte("Alice")}, Type: "XYZ", Quantity: ToHex(200)})
		Expect(err).NotTo(HaveOccurred())

		fakeCapabilityChecker = &mock.CapabilityChecker{}
		fakeCapabilityChecker.FabTokenReturns(true, nil)
		fakePolicyChecker := &mock.PolicyChecker{}
		fakeSigner := &mock.SignerIdentity{}
		fakeLedgerReader := &mock2.LedgerReader{}
		fakeLedgerManager := &mock2.LedgerManager{}

		fakeTokenOwnerValidatorManager := &mock3.TokenOwnerValidatorManager{}
		fakeTokenOwnerValidatorManager.GetReturns(&TestTokenOwnerValidator{}, nil)

		manager := &server.Manager{LedgerManager: fakeLedgerManager, TokenOwnerValidatorManager: fakeTokenOwnerValidatorManager}
		marshaler = &server.ResponseMarshaler{Signer: fakeSigner, Creator: []byte("creator"), Time: clock}

		prover = &server.Prover{
			CapabilityChecker: fakeCapabilityChecker,
			Marshaler:         marshaler,
			PolicyChecker:     fakePolicyChecker,
			TMSManager:        manager,
		}

		fakeSigner.SignReturns([]byte("it is a signature"), nil)
		fakeSigner.SerializeReturns([]byte("Alice"), nil)

		fakeLedgerManager.GetLedgerReaderReturns(fakeLedgerReader, nil)
		fakeLedgerReader.GetStateReturns(inTokens[0], nil)
		fakeLedgerReader.GetStateReturnsOnCall(1, inTokens[1], nil)
	})

	Describe("Transfer ", func() {
		It("returns token.CommandResponse_TokenTransaction for transfer", func() {
			response, err := prover.RequestTransfer(context.Background(), command.Header, transferRequest)
			Expect(err).NotTo(HaveOccurred())
			Expect(proto.Equal(response.TokenTransaction, expectedResponse.TokenTransaction)).To(BeTrue())
		})
	})

	Describe("ProcessCommand using ResponseMarshaler", func() {
		It("marshals a Transfer response", func() {

			marshaledResponse, err := marshaler.MarshalCommandResponse(marshaledCommand, expectedResponse)
			Expect(err).NotTo(HaveOccurred())

			response, err := prover.ProcessCommand(context.Background(), signedCommand)
			Expect(err).NotTo(HaveOccurred())
			Expect(response).To(Equal(marshaledResponse))
		})
	})
	Describe("ProcessCommand with grpc service activated", func() {
		var (
			proverEndpoint string
			grpcSrv        *grpc.Server
		)

		BeforeEach(func() {

			// start grpc server for prover
			listener, err := net.Listen("tcp", "127.0.0.1:")
			Expect(err).To(BeNil())
			grpcSrv = grpc.NewServer()
			token.RegisterProverServer(grpcSrv, prover)
			go grpcSrv.Serve(listener)

			proverEndpoint = listener.Addr().String()

		})

		AfterEach(func() {
			grpcSrv.Stop()
		})

		It("returns expected response", func() {
			// create grpc client
			clientConn, err := grpc.Dial(proverEndpoint, grpc.WithInsecure())
			Expect(err).To(BeNil())
			defer clientConn.Close()
			proverClient := token.NewProverClient(clientConn)

			response, err := proverClient.ProcessCommand(context.Background(), signedCommand)
			Expect(err).NotTo(HaveOccurred())

			marshaledResponse, err := marshaler.MarshalCommandResponse(marshaledCommand, expectedResponse)
			Expect(err).NotTo(HaveOccurred())

			Expect(err).NotTo(HaveOccurred())
			Expect(response.Signature).To(Equal(marshaledResponse.Signature))
			Expect(response.Response).To(Equal(marshaledResponse.Response))
		})
	})
})

const minUnicodeRuneValue = 0 //U+0000

func splitCompositeKey(compositeKey string) (string, []string, error) {
	componentIndex := 1
	components := []string{}
	for i := 1; i < len(compositeKey); i++ {
		if compositeKey[i] == minUnicodeRuneValue {
			components = append(components, compositeKey[componentIndex:i])
			componentIndex = i + 1
		}
	}
	return components[0], components[1:], nil
}

func generateKey(ownerString, txID, index, namespace string) string {
	return "\x00" + namespace + "\x00" + ownerString + "\x00" + txID + "\x00" + index + "\x00"
}

type TestTokenOwnerValidator struct {
}

func (TestTokenOwnerValidator) Validate(owner *token.TokenOwner) error {
	if owner == nil {
		return errors.New("owner is nil")
	}

	if len(owner.Raw) == 0 {
		return errors.New("raw is emptyr")
	}
	return nil
}
