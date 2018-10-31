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
		fakePolicyChecker *mock.PolicyChecker
		fakeMarshaler     *mock.Marshaler
		fakeIssuer        *mock.Issuer
		fakeTransactor    *mock.Transactor
		fakeTMSManager    *mock.TMSManager

		prover *server.Prover

		importRequest     *token.ImportRequest
		command           *token.Command
		marshaledCommand  []byte
		signedCommand     *token.SignedCommand
		tokenTransaction  *token.TokenTransaction
		marshaledResponse *token.SignedCommandResponse

		listRequest      *token.ListRequest
		unspentTokens    *token.UnspentTokens
		transactorTokens []*token.TokenOutput
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

		transactorTokens = []*token.TokenOutput{
			{Id: []byte("idaz"), Type: "typeaz", Quantity: 135},
			{Id: []byte("idby"), Type: "typeby", Quantity: 79},
		}
		unspentTokens = &token.UnspentTokens{Tokens: transactorTokens}

		fakeTransactor = &mock.Transactor{}
		fakeTransactor.ListTokensReturns(unspentTokens, nil)

		fakeTMSManager = &mock.TMSManager{}
		fakeTMSManager.GetIssuerReturns(fakeIssuer, nil)
		fakeTMSManager.GetTransactorReturns(fakeTransactor, nil)

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

		listRequest = &token.ListRequest{
			Credential: []byte("credential"),
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

	Describe("Issue tokens by a plain issuer", func() {
		var (
			manager         *server.Manager
			expectedTokenTx *token.TokenTransaction
		)

		BeforeEach(func() {
			prover = &server.Prover{
				PolicyChecker: fakePolicyChecker,
				Marshaler:     fakeMarshaler,
				TMSManager:    manager,
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

				prover = &server.Prover{
					PolicyChecker: fakePolicyChecker,
					Marshaler:     marshaler,
					TMSManager:    manager,
				}

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

		fakeLedgerReader := &mock2.LedgerReader{}
		fakeLedgerManager := &mock2.LedgerManager{}
		fakeLedgerManager.GetLedgerReaderReturns(fakeLedgerReader, nil)

		manager := &server.Manager{LedgerManager: fakeLedgerManager}
		marshaler = &server.ResponseMarshaler{Signer: fakeSigner, Creator: []byte("Alice"), Time: clock}

		prover = &server.Prover{
			Marshaler:     marshaler,
			PolicyChecker: fakePolicyChecker,
			TMSManager:    manager,
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
