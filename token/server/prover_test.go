/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package server_test

import (
	"context"
	"net"
	"strconv"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/protos/ledger/queryresult"
	"github.com/hyperledger/fabric/protos/token"
	mock2 "github.com/hyperledger/fabric/token/ledger/mock"
	"github.com/hyperledger/fabric/token/server"
	"github.com/hyperledger/fabric/token/server/mock"
	"github.com/hyperledger/fabric/token/tms/plain"
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

		importRequest     *token.ImportRequest
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
		transactorTokens []*token.TokenOutput

		importExpectationRequest     *token.ExpectationRequest
		importExpectationTransaction *token.TokenTransaction

		transferExpectationRequest     *token.ExpectationRequest
		transferExpectationTransaction *token.TokenTransaction
	)

	BeforeEach(func() {
		fakeCapabilityChecker = &mock.CapabilityChecker{}
		fakeCapabilityChecker.FabTokenReturns(true, nil)
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

		trTokenTransaction = &token.TokenTransaction{
			Action: &token.TokenTransaction_PlainAction{
				PlainAction: &token.PlainTokenAction{
					Data: &token.PlainTokenAction_PlainTransfer{
						PlainTransfer: &token.PlainTransfer{
							Inputs: []*token.InputId{{
								TxId:  "txid",
								Index: 0,
							}},
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
		fakeTransactor = &mock.Transactor{}
		fakeTransactor.RequestTransferReturns(trTokenTransaction, nil)

		redeemTokenTransaction = &token.TokenTransaction{
			Action: &token.TokenTransaction_PlainAction{
				PlainAction: &token.PlainTokenAction{
					Data: &token.PlainTokenAction_PlainRedeem{
						PlainRedeem: &token.PlainTransfer{
							Inputs: []*token.InputId{{
								TxId:  "txid",
								Index: 0,
							}},
							Outputs: []*token.PlainOutput{{
								Type:     "PDQ",
								Quantity: 50,
							}},
						},
					},
				},
			},
		}
		fakeTransactor.RequestRedeemReturns(redeemTokenTransaction, nil)

		transactorTokens = []*token.TokenOutput{
			{Id: []byte("idaz"), Type: "typeaz", Quantity: 135},
			{Id: []byte("idby"), Type: "typeby", Quantity: 79},
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

		transferRequest = &token.TransferRequest{
			Credential: []byte("credential"),
			TokenIds:   [][]byte{},
			Shares: []*token.RecipientTransferShare{{
				Recipient: []byte("recipient"),
				Quantity:  99,
			}},
		}

		redeemRequest = &token.RedeemRequest{
			Credential:       []byte("credential"),
			TokenIds:         [][]byte{},
			QuantityToRedeem: 50,
		}

		listRequest = &token.ListRequest{
			Credential: []byte("credential"),
		}

		importExpectationRequest = &token.ExpectationRequest{
			Credential: []byte("credential"),
			Expectation: &token.TokenExpectation{
				Expectation: &token.TokenExpectation_PlainExpectation{
					PlainExpectation: &token.PlainExpectation{
						Payload: &token.PlainExpectation_ImportExpectation{
							ImportExpectation: &token.PlainTokenExpectation{
								Outputs: []*token.PlainOutput{{
									Owner:    []byte("recipient"),
									Type:     "XYZ",
									Quantity: 99,
								}},
							},
						},
					},
				},
			},
		}

		transferExpectationRequest = &token.ExpectationRequest{
			Credential: []byte("credential"),
			TokenIds:   [][]byte{},
			Expectation: &token.TokenExpectation{
				Expectation: &token.TokenExpectation_PlainExpectation{
					PlainExpectation: &token.PlainExpectation{
						Payload: &token.PlainExpectation_TransferExpectation{
							TransferExpectation: &token.PlainTokenExpectation{
								Outputs: []*token.PlainOutput{{
									Owner:    []byte("token-owner"),
									Type:     "PDQ",
									Quantity: 777,
								}},
							},
						},
					},
				},
			},
		}

		importExpectationTransaction = tokenTransaction
		transferExpectationTransaction = trTokenTransaction
		fakeIssuer.RequestExpectationReturns(tokenTransaction, nil)
		fakeTransactor.RequestExpectationReturns(trTokenTransaction, nil)
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
	})

	Describe("ProcessCommand_RequestImport", func() {
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

	Describe("Process RequestImport command", func() {
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

			It("returns the error", func() {
				_, err := prover.RequestImport(context.Background(), command.Header, importRequest)
				Expect(err).To(MatchError("boing boing"))
			})
		})

		Context("when the issuer fails to import", func() {
			BeforeEach(func() {
				fakeIssuer.RequestImportReturns(nil, errors.New("watermelon"))
			})

			It("returns the error", func() {
				_, err := prover.RequestImport(context.Background(), command.Header, importRequest)
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

	Describe("RequestExpectation import", func() {
		It("gets an issuer", func() {
			_, err := prover.RequestExpectation(context.Background(), command.Header, importExpectationRequest)
			Expect(err).NotTo(HaveOccurred())

			Expect(fakeTMSManager.GetIssuerCallCount()).To(Equal(1))
			Expect(fakeTMSManager.GetTransactorCallCount()).To(Equal(0))
			channel, cred, creator := fakeTMSManager.GetIssuerArgsForCall(0)
			Expect(channel).To(Equal("channel-id"))
			Expect(cred).To(Equal([]byte("credential")))
			Expect(creator).To(Equal([]byte("creator")))
		})

		It("uses the issuer to request an import", func() {
			resp, err := prover.RequestExpectation(context.Background(), command.Header, importExpectationRequest)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp).To(Equal(&token.CommandResponse_TokenTransaction{
				TokenTransaction: importExpectationTransaction,
			}))

			Expect(fakeIssuer.RequestExpectationCallCount()).To(Equal(1))
			request := fakeIssuer.RequestExpectationArgsForCall(0)
			Expect(request).To(Equal(importExpectationRequest))
		})

		Context("when the TMS manager fails to get an issuer", func() {
			BeforeEach(func() {
				fakeTMSManager.GetIssuerReturns(nil, errors.New("boing boing"))
			})

			It("returns the error", func() {
				_, err := prover.RequestExpectation(context.Background(), command.Header, importExpectationRequest)
				Expect(err).To(MatchError("boing boing"))
			})
		})

		Context("when the issuer fails to import", func() {
			BeforeEach(func() {
				fakeIssuer.RequestExpectationReturns(nil, errors.New("watermelon"))
			})

			It("returns the error", func() {
				_, err := prover.RequestExpectation(context.Background(), command.Header, importExpectationRequest)
				Expect(err).To(MatchError("watermelon"))
			})
		})

		Context("when Expectationrequest has nil Expectation", func() {
			BeforeEach(func() {
				importExpectationRequest = &token.ExpectationRequest{
					Credential: []byte("credential"),
				}
			})

			It("returns the error", func() {
				_, err := prover.RequestExpectation(context.Background(), command.Header, importExpectationRequest)
				Expect(err).To(MatchError("ExpectationRequest has nil Expectation"))
			})
		})

		Context("when Expectationrequest has nil PlainExpectation", func() {
			BeforeEach(func() {
				importExpectationRequest = &token.ExpectationRequest{
					Credential:  []byte("credential"),
					Expectation: &token.TokenExpectation{},
				}
			})

			It("returns the error", func() {
				_, err := prover.RequestExpectation(context.Background(), command.Header, importExpectationRequest)
				Expect(err).To(MatchError("ExpectationRequest has nil PlainExpectation"))
			})
		})
	})

	Describe("RequestExpectation transfer", func() {
		It("gets a transactor", func() {
			_, err := prover.RequestExpectation(context.Background(), command.Header, transferExpectationRequest)
			Expect(err).NotTo(HaveOccurred())

			Expect(fakeTMSManager.GetIssuerCallCount()).To(Equal(0))
			Expect(fakeTMSManager.GetTransactorCallCount()).To(Equal(1))
			channel, cred, creator := fakeTMSManager.GetTransactorArgsForCall(0)
			Expect(channel).To(Equal("channel-id"))
			Expect(cred).To(Equal([]byte("credential")))
			Expect(creator).To(Equal([]byte("creator")))
		})

		It("uses the transactor to request a transfer expectation", func() {
			resp, err := prover.RequestExpectation(context.Background(), command.Header, transferExpectationRequest)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp).To(Equal(&token.CommandResponse_TokenTransaction{
				TokenTransaction: transferExpectationTransaction,
			}))

			Expect(fakeTransactor.RequestExpectationCallCount()).To(Equal(1))
			tr := fakeTransactor.RequestExpectationArgsForCall(0)
			Expect(tr).To(Equal(transferExpectationRequest))
		})

		Context("when the TMS manager fails to get a transactor", func() {
			BeforeEach(func() {
				fakeTMSManager.GetTransactorReturns(nil, errors.New("boing boing"))
			})

			It("returns the error", func() {
				_, err := prover.RequestExpectation(context.Background(), command.Header, transferExpectationRequest)
				Expect(err).To(MatchError("boing boing"))
			})
		})

		Context("when the transactor fails to transfer", func() {
			BeforeEach(func() {
				fakeTransactor.RequestExpectationReturns(nil, errors.New("watermelon"))
			})

			It("returns the error", func() {
				_, err := prover.RequestExpectation(context.Background(), command.Header, transferExpectationRequest)
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
			prover = &server.Prover{
				CapabilityChecker: fakeCapabilityChecker,
				PolicyChecker:     fakePolicyChecker,
				Marshaler:         fakeMarshaler,
				TMSManager:        manager,
			}
			importRequest = &token.ImportRequest{
				Credential: []byte("credential"),
				TokensToIssue: []*token.TokenToIssue{
					{
						Recipient: []byte("recipient1"),
						Type:      "XYZ1",
						Quantity:  10,
					},
					{
						Recipient: []byte("recipient2"),
						Type:      "XYZ2",
						Quantity:  200,
					},
				},
			}

			plainOutputs := []*token.PlainOutput{
				{
					Owner:    []byte("recipient1"),
					Type:     "XYZ1",
					Quantity: 10,
				},
				{
					Owner:    []byte("recipient2"),
					Type:     "XYZ2",
					Quantity: 200,
				},
			}
			expectedTokenTx = &token.TokenTransaction{
				Action: &token.TokenTransaction_PlainAction{
					PlainAction: &token.PlainTokenAction{
						Data: &token.PlainTokenAction_PlainImport{
							PlainImport: &token.PlainImport{
								Outputs: plainOutputs,
							},
						},
					},
				},
			}
		})

		Describe("RequestImport", func() {
			It("returns a TokenTransaction response", func() {
				resp, err := prover.RequestImport(context.Background(), command.Header, importRequest)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp).To(Equal(&token.CommandResponse_TokenTransaction{
					TokenTransaction: expectedTokenTx,
				}))
				Expect(fakeIssuer.RequestImportCallCount()).To(Equal(0))
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
					Payload: &token.Command_ImportRequest{
						ImportRequest: importRequest,
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

				Expect(fakeIssuer.RequestImportCallCount()).To(Equal(0))
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
					Payload: &token.Command_ImportRequest{
						ImportRequest: importRequest,
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
				Payload: &token.Command_ExpectationRequest{
					ExpectationRequest: importExpectationRequest,
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
				TokenTransaction: importExpectationTransaction,
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
				Payload: &token.Command_ExpectationRequest{
					ExpectationRequest: transferExpectationRequest,
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
				TokenTransaction: transferExpectationTransaction,
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

		manager := &server.Manager{LedgerManager: fakeLedgerManager}
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
		outputToken, err := proto.Marshal(&token.PlainOutput{Owner: []byte("Alice"), Type: "XYZ", Quantity: 100})
		Expect(err).NotTo(HaveOccurred())

		key, err := plain.GenerateKeyForTest("1", 0)
		Expect(err).NotTo(HaveOccurred())

		queryResult = &queryresult.KV{Key: key, Value: outputToken}

		unspentTokens := &token.UnspentTokens{Tokens: []*token.TokenOutput{{Type: "XYZ", Quantity: 100, Id: []byte(key)}}}
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

		var err error
		keys := make([]string, 2)
		keys[0], err = plain.GenerateKeyForTest("1", 0)
		Expect(err).NotTo(HaveOccurred())
		keys[1], err = plain.GenerateKeyForTest("2", 1)
		Expect(err).NotTo(HaveOccurred())

		tokenIDs := [][]byte{[]byte(keys[0]), []byte(keys[1])}

		shares := []*token.RecipientTransferShare{
			{Recipient: []byte("Alice"), Quantity: 20},
			{Recipient: []byte("Bob"), Quantity: 250},
			{Recipient: []byte("Charlie"), Quantity: 30},
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

		inputIDs, err := getInputIDs(keys)

		outTokens := make([]*token.PlainOutput, 3)
		outTokens[0] = &token.PlainOutput{Owner: []byte("Alice"), Type: "XYZ", Quantity: 20}
		outTokens[1] = &token.PlainOutput{Owner: []byte("Bob"), Type: "XYZ", Quantity: 250}
		outTokens[2] = &token.PlainOutput{Owner: []byte("Charlie"), Type: "XYZ", Quantity: 30}

		tokenTx := &token.TokenTransaction{
			Action: &token.TokenTransaction_PlainAction{
				PlainAction: &token.PlainTokenAction{
					Data: &token.PlainTokenAction_PlainTransfer{
						PlainTransfer: &token.PlainTransfer{
							Inputs:  inputIDs,
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
		inTokens[0], err = proto.Marshal(&token.PlainOutput{Owner: []byte("Alice"), Type: "XYZ", Quantity: 100})
		Expect(err).NotTo(HaveOccurred())

		inTokens[1], err = proto.Marshal(&token.PlainOutput{Owner: []byte("Alice"), Type: "XYZ", Quantity: 200})
		Expect(err).NotTo(HaveOccurred())

		fakeCapabilityChecker = &mock.CapabilityChecker{}
		fakeCapabilityChecker.FabTokenReturns(true, nil)
		fakePolicyChecker := &mock.PolicyChecker{}
		fakeSigner := &mock.SignerIdentity{}
		fakeLedgerReader := &mock2.LedgerReader{}
		fakeLedgerManager := &mock2.LedgerManager{}

		manager := &server.Manager{LedgerManager: fakeLedgerManager}
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
			Expect(response).To(Equal(expectedResponse))
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

var _ = Describe("Prover Approve using mock TMS", func() {

	var (
		fakeMarshaler         *mock.Marshaler
		fakeTransactor        *mock.Transactor
		fakeTMSManager        *mock.TMSManager
		fakePolicyChecker     *mock.PolicyChecker
		fakeCapabilityChecker *mock.CapabilityChecker
		prover                *server.Prover
		command               *token.Command
		approveRequest        *token.ApproveRequest
		tokenTransaction      *token.TokenTransaction
	)

	It("initializes variables and expected responses", func() {
		approveRequest = &token.ApproveRequest{
			Credential: []byte("credential"),
			AllowanceShares: []*token.AllowanceRecipientShare{{
				Quantity:  100,
				Recipient: []byte("Alice"),
			}},
		}

		tokenTransaction = &token.TokenTransaction{Action: &token.TokenTransaction_PlainAction{
			PlainAction: &token.PlainTokenAction{
				Data: &token.PlainTokenAction_PlainApprove{
					PlainApprove: &token.PlainApprove{
						Inputs: []*token.InputId{{
							TxId:  "txid",
							Index: 0,
						}},
						DelegatedOutputs: []*token.PlainDelegatedOutput{{
							Owner:      []byte("credential"),
							Delegatees: [][]byte{[]byte("Alice")},
							Quantity:   100,
							Type:       "XYZ",
						}},
					},
				},
			},
		}}

		command = &token.Command{
			Header: &token.Header{
				ChannelId: "channel-id",
				Creator:   []byte("creator"),
				Nonce:     []byte("nonce"),
			},
			Payload: &token.Command_ApproveRequest{
				ApproveRequest: approveRequest,
			},
		}
	})

	Describe("RequestApprove", func() {
		BeforeEach(func() {
			fakeMarshaler = &mock.Marshaler{}
			fakeTransactor = &mock.Transactor{}
			fakeTMSManager = &mock.TMSManager{}
			fakePolicyChecker = &mock.PolicyChecker{}
			fakeCapabilityChecker = &mock.CapabilityChecker{}

			fakeTransactor.RequestApproveReturns(tokenTransaction, nil)
			fakeTMSManager.GetTransactorReturns(fakeTransactor, nil)
			fakePolicyChecker.CheckReturns(nil)
			fakeCapabilityChecker.FabTokenReturns(true, nil)

			prover = &server.Prover{TMSManager: fakeTMSManager, PolicyChecker: fakePolicyChecker, Marshaler: fakeMarshaler, CapabilityChecker: fakeCapabilityChecker}

		})
		It("gets a transactor", func() {
			_, err := prover.RequestApprove(context.Background(), command.Header, approveRequest)
			Expect(err).NotTo(HaveOccurred())

			Expect(fakeTMSManager.GetTransactorCallCount()).To(Equal(1))
			channel, cred, creator := fakeTMSManager.GetTransactorArgsForCall(0)
			Expect(channel).To(Equal("channel-id"))
			Expect(cred).To(Equal([]byte("credential")))
			Expect(creator).To(Equal([]byte("creator")))
		})

		It("uses the transactor to request an approve", func() {
			resp, err := prover.RequestApprove(context.Background(), command.Header, approveRequest)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp).To(Equal(&token.CommandResponse_TokenTransaction{
				TokenTransaction: tokenTransaction,
			}))

			Expect(fakeTransactor.RequestApproveCallCount()).To(Equal(1))
			tr := fakeTransactor.RequestApproveArgsForCall(0)
			Expect(tr).To(Equal(approveRequest))
		})

		Context("when the TMS manager fails to get a transactor", func() {
			It("returns the error", func() {
				fakeTMSManager.GetTransactorReturns(nil, errors.New("camel"))
				_, err := prover.RequestApprove(context.Background(), command.Header, approveRequest)
				Expect(err).To(MatchError("camel"))
			})
		})

		Context("when the transactor fails to approve", func() {
			It("returns the error", func() {
				fakeTransactor.RequestApproveReturns(nil, errors.New("banana"))
				_, err := prover.RequestApprove(context.Background(), command.Header, approveRequest)
				Expect(err).To(MatchError("banana"))
			})
		})
	})

	Describe("ProcessCommand_RequestApprove", func() {
		var (
			marshaledCommand  []byte
			signedCommand     *token.SignedCommand
			marshaledResponse *token.SignedCommandResponse
		)
		BeforeEach(func() {
			command = &token.Command{
				Header: &token.Header{
					ChannelId: "channel-id",
					Creator:   []byte("creator"),
					Nonce:     []byte("nonce"),
				},
				Payload: &token.Command_ApproveRequest{
					ApproveRequest: approveRequest,
				},
			}
			marshaledResponse = &token.SignedCommandResponse{Response: []byte("signed-command-response")}

			marshaledCommand = ProtoMarshal(command)
			signedCommand = &token.SignedCommand{
				Command:   marshaledCommand,
				Signature: []byte("command-signature"),
			}
			fakeMarshaler.MarshalCommandResponseReturns(marshaledResponse, nil)
			fakeTransactor.RequestApproveReturns(tokenTransaction, nil)
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
	})
})

var _ = Describe("RequestTransferFrom using mock TMS", func() {

	var (
		fakeMarshaler         *mock.Marshaler
		fakeTransactor        *mock.Transactor
		fakeTMSManager        *mock.TMSManager
		fakePolicyChecker     *mock.PolicyChecker
		fakeCapabilityChecker *mock.CapabilityChecker

		prover              *server.Prover
		command             *token.Command
		transferFromRequest *token.TransferRequest
		tokenTransaction    *token.TokenTransaction
	)

	It("initializes variables and expected responses", func() {
		transferFromRequest = &token.TransferRequest{
			Credential: []byte("credential"),
			Shares: []*token.RecipientTransferShare{{
				Quantity:  100,
				Recipient: []byte("Alice"),
			}},
		}

		tokenTransaction = &token.TokenTransaction{Action: &token.TokenTransaction_PlainAction{
			PlainAction: &token.PlainTokenAction{
				Data: &token.PlainTokenAction_PlainTransfer_From{
					PlainTransfer_From: &token.PlainTransferFrom{
						Inputs: []*token.InputId{{
							TxId:  "txid",
							Index: 0,
						}},
						Outputs: []*token.PlainOutput{{Owner: []byte("Bob"), Type: "XYZ", Quantity: 100}},
						DelegatedOutput: &token.PlainDelegatedOutput{
							Owner:      []byte("Alice"),
							Delegatees: [][]byte{[]byte("credential")},
							Quantity:   50,
							Type:       "XYZ",
						},
					},
				},
			},
		}}

		command = &token.Command{
			Header: &token.Header{
				ChannelId: "channel-id",
				Creator:   []byte("creator"),
				Nonce:     []byte("nonce"),
			},
			Payload: &token.Command_TransferFromRequest{
				TransferFromRequest: transferFromRequest,
			},
		}
	})

	Describe("RequestTransferFrom", func() {
		BeforeEach(func() {
			fakeMarshaler = &mock.Marshaler{}
			fakeTransactor = &mock.Transactor{}
			fakeTMSManager = &mock.TMSManager{}
			fakePolicyChecker = &mock.PolicyChecker{}
			fakeCapabilityChecker = &mock.CapabilityChecker{}

			fakeTransactor.RequestTransferFromReturns(tokenTransaction, nil)
			fakeTMSManager.GetTransactorReturns(fakeTransactor, nil)
			fakePolicyChecker.CheckReturns(nil)
			fakeCapabilityChecker.FabTokenReturns(true, nil)

			prover = &server.Prover{TMSManager: fakeTMSManager, PolicyChecker: fakePolicyChecker, Marshaler: fakeMarshaler, CapabilityChecker: fakeCapabilityChecker}

		})
		It("gets a transactor", func() {
			_, err := prover.RequestTransferFrom(context.Background(), command.Header, transferFromRequest)
			Expect(err).NotTo(HaveOccurred())

			Expect(fakeTMSManager.GetTransactorCallCount()).To(Equal(1))
			channel, cred, creator := fakeTMSManager.GetTransactorArgsForCall(0)
			Expect(channel).To(Equal("channel-id"))
			Expect(cred).To(Equal([]byte("credential")))
			Expect(creator).To(Equal([]byte("creator")))
		})

		It("uses the transactor to request a transferFrom", func() {
			resp, err := prover.RequestTransferFrom(context.Background(), command.Header, transferFromRequest)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp).To(Equal(&token.CommandResponse_TokenTransaction{
				TokenTransaction: tokenTransaction,
			}))

			Expect(fakeTransactor.RequestTransferFromCallCount()).To(Equal(1))
			tr := fakeTransactor.RequestTransferFromArgsForCall(0)
			Expect(tr).To(Equal(transferFromRequest))
		})

		Context("when the TMS manager fails to get a transferFrom", func() {
			It("returns the error", func() {
				fakeTMSManager.GetTransactorReturns(nil, errors.New("camel"))
				_, err := prover.RequestTransferFrom(context.Background(), command.Header, transferFromRequest)
				Expect(err).To(MatchError("camel"))
			})
		})

		Context("when the transactor fails to transferFrom", func() {
			It("returns the error", func() {
				fakeTransactor.RequestTransferFromReturns(nil, errors.New("banana"))
				_, err := prover.RequestTransferFrom(context.Background(), command.Header, transferFromRequest)
				Expect(err).To(MatchError("banana"))
			})
		})
	})

	Describe("ProcessCommand_RequestTransferFrom", func() {
		var (
			marshaledCommand  []byte
			signedCommand     *token.SignedCommand
			marshaledResponse *token.SignedCommandResponse
		)
		BeforeEach(func() {
			command = &token.Command{
				Header: &token.Header{
					ChannelId: "channel-id",
					Creator:   []byte("creator"),
					Nonce:     []byte("nonce"),
				},
				Payload: &token.Command_TransferFromRequest{
					TransferFromRequest: transferFromRequest,
				},
			}
			marshaledResponse = &token.SignedCommandResponse{Response: []byte("signed-command-response")}

			marshaledCommand = ProtoMarshal(command)
			signedCommand = &token.SignedCommand{
				Command:   marshaledCommand,
				Signature: []byte("command-signature"),
			}
			fakeMarshaler.MarshalCommandResponseReturns(marshaledResponse, nil)
			fakeTransactor.RequestTransferFromReturns(tokenTransaction, nil)
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

func getInputIDs(keys []string) ([]*token.InputId, error) {
	inputIDs := make([]*token.InputId, len(keys))
	for i := 0; i < len(keys); i++ {
		_, indices, err := splitCompositeKey(keys[i])

		if err != nil {
			return nil, err
		}
		if len(indices) != 2 {
			return nil, errors.New("error splitting composite keys")
		}

		index, err := strconv.Atoi(indices[1])
		if err != nil {
			return nil, err
		}
		inputIDs[i] = &token.InputId{TxId: indices[0], Index: uint32(index)}
	}
	return inputIDs, nil
}
