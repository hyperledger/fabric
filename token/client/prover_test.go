/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package client_test

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"io"
	"net"
	"strings"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"github.com/hyperledger/fabric/protos/token"
	"github.com/hyperledger/fabric/token/client"
	"github.com/hyperledger/fabric/token/client/mock"
	"github.com/hyperledger/fabric/token/server"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

//go:generate counterfeiter -o mock/prover_client.go -fake-name ProverClient . proverClient

type proverClient interface {
	token.ProverClient
}

var _ = Describe("TokenClient", func() {
	var (
		channelID         string
		commandHeader     *token.Header
		serializedTokenTx []byte
		signedCommandResp *token.SignedCommandResponse

		fakeSigningIdentity  *mock.SigningIdentity
		fakeRandomnessReader io.Reader
		fakeProverPeerClient *mock.ProverPeerClient
		fakeProverClient     *mock.ProverClient

		prover *client.ProverPeer
	)

	BeforeEach(func() {
		channelID = "mychannel"

		nonce := make([]byte, 32)
		ts, _ := ptypes.TimestampProto(clock())
		commandHeader = &token.Header{
			Timestamp: ts,
			Nonce:     nonce,
			Creator:   []byte("Alice"),
			ChannelId: channelID,
		}

		fakeSigningIdentity = &mock.SigningIdentity{}
		fakeRandomnessReader = strings.NewReader(string(nonce))
		fakeProverClient = &mock.ProverClient{}
		fakeProverPeerClient = &mock.ProverPeerClient{}

		tokenTx := &token.TokenTransaction{
			Action: &token.TokenTransaction_TokenAction{
				TokenAction: &token.TokenAction{
					Data: &token.TokenAction_Issue{
						Issue: &token.Issue{
							Outputs: []*token.Token{},
						},
					},
				},
			},
		}
		commandResponse := &token.CommandResponse{
			Payload: &token.CommandResponse_TokenTransaction{
				TokenTransaction: tokenTx,
			},
		}
		signedCommandResp = &token.SignedCommandResponse{
			Response:  ProtoMarshal(commandResponse),
			Signature: []byte("response-signature"),
		}
		serializedTokenTx = ProtoMarshal(tokenTx)

		fakeSigningIdentity.SerializeReturns([]byte("Alice"), nil)
		fakeSigningIdentity.SignReturns([]byte("pineapple"), nil)
		fakeProverPeerClient.CreateProverClientReturns(nil, fakeProverClient, nil)
		fakeProverClient.ProcessCommandReturns(signedCommandResp, nil)
		prover = &client.ProverPeer{RandomnessReader: fakeRandomnessReader, ProverPeerClient: fakeProverPeerClient, ChannelID: channelID, Time: clock}
	})

	Describe("RequestIssue", func() {
		var (
			tokensToIssue     []*token.Token
			marshalledCommand []byte
			signedCommand     *token.SignedCommand
		)

		BeforeEach(func() {
			tokensToIssue = []*token.Token{{
				Type:     "type",
				Quantity: ToHex(10),
				Owner:    &token.TokenOwner{Raw: []byte("Alice")},
			}}

			command := &token.Command{
				Header: commandHeader,
				Payload: &token.Command_IssueRequest{
					IssueRequest: &token.IssueRequest{
						TokensToIssue: tokensToIssue,
					},
				},
			}
			marshalledCommand = ProtoMarshal(command)
			signedCommand = &token.SignedCommand{
				Command:   marshalledCommand,
				Signature: []byte("pineapple"),
			}
		})

		It("returns serialized token transaction", func() {
			response, err := prover.RequestIssue(tokensToIssue, fakeSigningIdentity)
			Expect(err).NotTo(HaveOccurred())
			Expect(response).To(Equal(serializedTokenTx))

			Expect(fakeSigningIdentity.SerializeCallCount()).To(Equal(1))
			Expect(fakeSigningIdentity.SignCallCount()).To(Equal(1))
			raw := fakeSigningIdentity.SignArgsForCall(0)
			Expect(raw).To(Equal(marshalledCommand))

			Expect(fakeProverPeerClient.CreateProverClientCallCount()).To(Equal(1))
			Expect(fakeProverClient.ProcessCommandCallCount()).To(Equal(1))
			_, sc, _ := fakeProverClient.ProcessCommandArgsForCall(0)
			Expect(sc).To(Equal(signedCommand))
		})

		Context("when SigningIdentity serialize fails", func() {
			BeforeEach(func() {
				fakeSigningIdentity.SerializeReturns(nil, errors.New("wild-banana"))
			})

			It("returns an error", func() {
				_, err := prover.RequestIssue(tokensToIssue, fakeSigningIdentity)
				Expect(err).To(MatchError("wild-banana"))
				Expect(fakeSigningIdentity.SignCallCount()).To(Equal(0))
				Expect(fakeProverPeerClient.CreateProverClientCallCount()).To(Equal(0))
				Expect(fakeProverClient.ProcessCommandCallCount()).To(Equal(0))
			})
		})

		Context("when SigningIdentity sign fails", func() {
			BeforeEach(func() {
				fakeSigningIdentity.SignReturns(nil, errors.New("wild-banana"))
			})

			It("returns an error", func() {
				_, err := prover.RequestIssue(tokensToIssue, fakeSigningIdentity)
				Expect(err).To(MatchError("wild-banana"))

				Expect(fakeProverClient.ProcessCommandCallCount()).To(Equal(0))
				Expect(fakeSigningIdentity.SerializeCallCount()).To(Equal(1))
				Expect(fakeSigningIdentity.SignCallCount()).To(Equal(1))
				raw := fakeSigningIdentity.SignArgsForCall(0)
				Expect(raw).To(Equal(marshalledCommand))
			})
		})

		Context("when processcommand fails", func() {
			BeforeEach(func() {
				fakeProverClient.ProcessCommandReturns(nil, errors.New("wild-banana"))
			})

			It("returns an error", func() {
				_, err := prover.RequestIssue(tokensToIssue, fakeSigningIdentity)
				Expect(err).To(MatchError("wild-banana"))
				Expect(fakeSigningIdentity.SignCallCount()).To(Equal(1))
				Expect(fakeProverPeerClient.CreateProverClientCallCount()).To(Equal(1))
				Expect(fakeProverClient.ProcessCommandCallCount()).To(Equal(1))
			})
		})

		Context("when ProcessCommand returns an error response", func() {
			BeforeEach(func() {
				commandResponse := &token.CommandResponse{
					Payload: &token.CommandResponse_Err{
						Err: &token.Error{
							Message: "flying-pineapple",
							Payload: []byte("payload"),
						},
					},
				}
				signedCommandResp = &token.SignedCommandResponse{
					Response:  ProtoMarshal(commandResponse),
					Signature: []byte("signature"),
				}
				fakeProverClient.ProcessCommandReturns(signedCommandResp, nil)
			})

			It("returns an error", func() {
				_, err := prover.RequestIssue(tokensToIssue, fakeSigningIdentity)
				Expect(err).To(MatchError("error from prover: flying-pineapple"))

				Expect(fakeSigningIdentity.SerializeCallCount()).To(Equal(1))
				Expect(fakeSigningIdentity.SignCallCount()).To(Equal(1))
				Expect(fakeProverClient.ProcessCommandCallCount()).To(Equal(1))
			})
		})
	})

	Describe("RequestTransfer", func() {
		var (
			tokenIDs          []*token.TokenId
			transferShares    []*token.RecipientShare
			marshalledCommand []byte
			signedCommand     *token.SignedCommand
		)

		BeforeEach(func() {
			// input data for Transfer
			tokenIDs = []*token.TokenId{
				{TxId: "id1", Index: 0},
				{TxId: "id2", Index: 0},
			}
			transferShares = []*token.RecipientShare{
				{Recipient: &token.TokenOwner{Raw: []byte("alice")}, Quantity: ToHex(100)},
				{Recipient: &token.TokenOwner{Raw: []byte("bob")}, Quantity: ToHex(50)},
			}

			command := &token.Command{
				Header: commandHeader,
				Payload: &token.Command_TransferRequest{
					TransferRequest: &token.TransferRequest{
						TokenIds: tokenIDs,
						Shares:   transferShares,
					},
				},
			}
			marshalledCommand = ProtoMarshal(command)
			signedCommand = &token.SignedCommand{
				Command:   marshalledCommand,
				Signature: []byte("pineapple"),
			}
		})

		It("returns serialized token transaction", func() {
			response, err := prover.RequestTransfer(tokenIDs, transferShares, fakeSigningIdentity)
			Expect(err).NotTo(HaveOccurred())
			Expect(response).To(Equal(serializedTokenTx))

			Expect(fakeSigningIdentity.SerializeCallCount()).To(Equal(1))
			Expect(fakeSigningIdentity.SignCallCount()).To(Equal(1))
			raw := fakeSigningIdentity.SignArgsForCall(0)
			Expect(raw).To(Equal(marshalledCommand))

			Expect(fakeProverPeerClient.CreateProverClientCallCount()).To(Equal(1))
			Expect(fakeProverClient.ProcessCommandCallCount()).To(Equal(1))
			_, sc, _ := fakeProverClient.ProcessCommandArgsForCall(0)
			Expect(sc).To(Equal(signedCommand))
		})

		Context("when Identity serialize fails", func() {
			BeforeEach(func() {
				fakeSigningIdentity.SerializeReturns(nil, errors.New("wild-banana"))
			})

			It("returns an error", func() {
				_, err := prover.RequestTransfer(tokenIDs, transferShares, fakeSigningIdentity)
				Expect(err).To(MatchError("wild-banana"))
				Expect(fakeSigningIdentity.SerializeCallCount()).To(Equal(1))
				Expect(fakeSigningIdentity.SignCallCount()).To(Equal(0))
				Expect(fakeProverClient.ProcessCommandCallCount()).To(Equal(0))
			})
		})

		Context("when SigningIdentity sign fails", func() {
			BeforeEach(func() {
				fakeSigningIdentity.SignReturns(nil, errors.New("wild-banana"))
			})

			It("returns an error", func() {
				_, err := prover.RequestTransfer(tokenIDs, transferShares, fakeSigningIdentity)
				Expect(err).To(MatchError("wild-banana"))

				Expect(fakeProverPeerClient.CreateProverClientCallCount()).To(Equal(0))
				Expect(fakeProverClient.ProcessCommandCallCount()).To(Equal(0))
				Expect(fakeSigningIdentity.SerializeCallCount()).To(Equal(1))
				Expect(fakeSigningIdentity.SignCallCount()).To(Equal(1))
				raw := fakeSigningIdentity.SignArgsForCall(0)
				Expect(raw).To(Equal(marshalledCommand))
			})
		})

		Context("when processcommand fails", func() {
			BeforeEach(func() {
				fakeProverClient.ProcessCommandReturns(nil, errors.New("wild-banana"))
			})

			It("returns an error", func() {
				_, err := prover.RequestTransfer(tokenIDs, transferShares, fakeSigningIdentity)
				Expect(err).To(MatchError("wild-banana"))
				Expect(fakeSigningIdentity.SignCallCount()).To(Equal(1))
				Expect(fakeProverPeerClient.CreateProverClientCallCount()).To(Equal(1))
				Expect(fakeProverClient.ProcessCommandCallCount()).To(Equal(1))
			})
		})

		Context("when ProcessCommand returns an error response", func() {
			BeforeEach(func() {
				commandResponse := &token.CommandResponse{
					Payload: &token.CommandResponse_Err{
						Err: &token.Error{
							Message: "flying-pineapple",
							Payload: []byte("payload"),
						},
					},
				}
				signedCommandResp = &token.SignedCommandResponse{
					Response:  ProtoMarshal(commandResponse),
					Signature: []byte("signature"),
				}
				fakeProverClient.ProcessCommandReturns(signedCommandResp, nil)
			})

			It("returns an error", func() {
				_, err := prover.RequestTransfer(tokenIDs, transferShares, fakeSigningIdentity)
				Expect(err).To(MatchError("error from prover: flying-pineapple"))

				Expect(fakeSigningIdentity.SerializeCallCount()).To(Equal(1))
				Expect(fakeSigningIdentity.SignCallCount()).To(Equal(1))
				Expect(fakeProverClient.ProcessCommandCallCount()).To(Equal(1))
			})
		})
	})

	Describe("RequestRedeem", func() {
		var (
			tokenIDs          []*token.TokenId
			quantity          uint64
			marshalledCommand []byte
			signedCommand     *token.SignedCommand
		)

		BeforeEach(func() {
			// input data for redeem
			tokenIDs = []*token.TokenId{
				{TxId: "id1", Index: 0},
				{TxId: "id2", Index: 0},
			}

			quantity = 100

			command := &token.Command{
				Header: commandHeader,
				Payload: &token.Command_RedeemRequest{
					RedeemRequest: &token.RedeemRequest{
						TokenIds: tokenIDs,
						Quantity: ToHex(quantity),
					},
				},
			}
			marshalledCommand = ProtoMarshal(command)
			signedCommand = &token.SignedCommand{
				Command:   marshalledCommand,
				Signature: []byte("pineapple"),
			}
		})

		It("returns serialized token transaction", func() {
			response, err := prover.RequestRedeem(tokenIDs, ToHex(quantity), fakeSigningIdentity)
			Expect(err).NotTo(HaveOccurred())
			Expect(response).To(Equal(serializedTokenTx))

			Expect(fakeSigningIdentity.SerializeCallCount()).To(Equal(1))
			Expect(fakeSigningIdentity.SignCallCount()).To(Equal(1))
			raw := fakeSigningIdentity.SignArgsForCall(0)
			Expect(raw).To(Equal(marshalledCommand))

			Expect(fakeProverClient.ProcessCommandCallCount()).To(Equal(1))
			_, sc, _ := fakeProverClient.ProcessCommandArgsForCall(0)
			Expect(sc).To(Equal(signedCommand))
		})

		Context("when Identity serialize fails", func() {
			BeforeEach(func() {
				fakeSigningIdentity.SerializeReturns(nil, errors.New("wild-banana"))
			})

			It("returns an error", func() {
				_, err := prover.RequestRedeem(tokenIDs, ToHex(quantity), fakeSigningIdentity)
				Expect(err).To(MatchError("wild-banana"))
				Expect(fakeSigningIdentity.SerializeCallCount()).To(Equal(1))
				Expect(fakeSigningIdentity.SignCallCount()).To(Equal(0))
				Expect(fakeProverClient.ProcessCommandCallCount()).To(Equal(0))
			})
		})

		Context("when SigningIdentity sign fails", func() {
			BeforeEach(func() {
				fakeSigningIdentity.SignReturns(nil, errors.New("wild-banana"))
			})

			It("returns an error", func() {
				_, err := prover.RequestRedeem(tokenIDs, ToHex(quantity), fakeSigningIdentity)
				Expect(err).To(MatchError("wild-banana"))
				Expect(fakeProverClient.ProcessCommandCallCount()).To(Equal(0))
				Expect(fakeSigningIdentity.SerializeCallCount()).To(Equal(1))
				Expect(fakeSigningIdentity.SignCallCount()).To(Equal(1))
				raw := fakeSigningIdentity.SignArgsForCall(0)
				Expect(raw).To(Equal(marshalledCommand))
			})
		})

		Context("when processcommand fails", func() {
			BeforeEach(func() {
				fakeProverClient.ProcessCommandReturns(nil, errors.New("wild-banana"))
			})

			It("returns an error", func() {
				_, err := prover.RequestRedeem(tokenIDs, ToHex(quantity), fakeSigningIdentity)
				Expect(err).To(MatchError("wild-banana"))
				Expect(fakeSigningIdentity.SignCallCount()).To(Equal(1))
				Expect(fakeProverClient.ProcessCommandCallCount()).To(Equal(1))
			})
		})

		Context("when ProcessCommand returns an error response", func() {
			BeforeEach(func() {
				commandResponse := &token.CommandResponse{
					Payload: &token.CommandResponse_Err{
						Err: &token.Error{
							Message: "flying-pineapple",
							Payload: []byte("payload"),
						},
					},
				}
				signedCommandResp = &token.SignedCommandResponse{
					Response:  ProtoMarshal(commandResponse),
					Signature: []byte("signature"),
				}
				fakeProverClient.ProcessCommandReturns(signedCommandResp, nil)
			})

			It("returns an error", func() {
				_, err := prover.RequestRedeem(tokenIDs, ToHex(quantity), fakeSigningIdentity)
				Expect(err).To(MatchError("error from prover: flying-pineapple"))
				Expect(fakeSigningIdentity.SerializeCallCount()).To(Equal(1))
				Expect(fakeSigningIdentity.SignCallCount()).To(Equal(1))
				Expect(fakeProverClient.ProcessCommandCallCount()).To(Equal(1))
			})
		})
	})

	Describe("ListTokens", func() {
		var (
			marshalledCommand []byte
			signedCommand     *token.SignedCommand
			expectedTokens    []*token.UnspentToken
		)
		BeforeEach(func() {
			command := &token.Command{
				Header: commandHeader,
				Payload: &token.Command_ListRequest{
					ListRequest: &token.ListRequest{},
				},
			}
			marshalledCommand = ProtoMarshal(command)
			signedCommand = &token.SignedCommand{
				Command:   marshalledCommand,
				Signature: []byte("pineapple"),
			}

			// prepare SignedCommandResponse for fakeProverClient to return
			expectedTokens = []*token.UnspentToken{
				{Id: &token.TokenId{TxId: "idaz", Index: 0}, Type: "typeaz", Quantity: ToHex(135)},
				{Id: &token.TokenId{TxId: "idby", Index: 0}, Type: "typeby", Quantity: ToHex(79)},
			}
			commandResp := &token.CommandResponse{
				Payload: &token.CommandResponse_UnspentTokens{
					UnspentTokens: &token.UnspentTokens{
						Tokens: expectedTokens,
					},
				},
			}
			signedCommandResp = &token.SignedCommandResponse{
				Response:  ProtoMarshal(commandResp),
				Signature: []byte("response-signature"),
			}

			fakeProverClient.ProcessCommandReturns(signedCommandResp, nil)
		})

		It("returns unspent tokens", func() {
			tokens, err := prover.ListTokens(fakeSigningIdentity)
			Expect(err).NotTo(HaveOccurred())
			Expect(len(tokens)).To(Equal(len(expectedTokens)))
			for i := range tokens {
				Expect(proto.Equal(tokens[i], expectedTokens[i])).To(BeTrue())
			}

			Expect(fakeSigningIdentity.SerializeCallCount()).To(Equal(1))
			Expect(fakeSigningIdentity.SignCallCount()).To(Equal(1))
			raw := fakeSigningIdentity.SignArgsForCall(0)
			Expect(raw).To(Equal(marshalledCommand))

			Expect(fakeProverClient.ProcessCommandCallCount()).To(Equal(1))
			_, sc, _ := fakeProverClient.ProcessCommandArgsForCall(0)
			Expect(sc).To(Equal(signedCommand))
		})

		Context("when Identity serialize fails", func() {
			BeforeEach(func() {
				fakeSigningIdentity.SerializeReturns(nil, errors.New("banana-seesaw"))
			})

			It("returns an error", func() {
				_, err := prover.ListTokens(fakeSigningIdentity)
				Expect(err).To(MatchError("banana-seesaw"))
				Expect(fakeSigningIdentity.SerializeCallCount()).To(Equal(1))
				Expect(fakeSigningIdentity.SignCallCount()).To(Equal(0))
				Expect(fakeProverClient.ProcessCommandCallCount()).To(Equal(0))
			})
		})

		Context("when SigningIdentity fails to sign", func() {
			BeforeEach(func() {
				fakeSigningIdentity.SignReturns(nil, errors.New("banana-seesaw"))
			})

			It("returns an error", func() {
				_, err := prover.ListTokens(fakeSigningIdentity)
				Expect(err).To(MatchError("banana-seesaw"))

				Expect(fakeSigningIdentity.SerializeCallCount()).To(Equal(1))
				Expect(fakeSigningIdentity.SignCallCount()).To(Equal(1))
				Expect(fakeProverClient.ProcessCommandCallCount()).To(Equal(0))
			})
		})

		Context("when ProcessCommand returns an error", func() {
			BeforeEach(func() {
				fakeProverClient.ProcessCommandReturns(nil, errors.New("banana-loop"))
			})

			It("returns an error", func() {
				_, err := prover.ListTokens(fakeSigningIdentity)
				Expect(err).To(MatchError("banana-loop"))

				Expect(fakeSigningIdentity.SerializeCallCount()).To(Equal(1))
				Expect(fakeSigningIdentity.SignCallCount()).To(Equal(1))
				Expect(fakeProverClient.ProcessCommandCallCount()).To(Equal(1))
			})
		})

		Context("when ProcessCommand returns an error response", func() {
			BeforeEach(func() {
				commandResponse := &token.CommandResponse{
					Payload: &token.CommandResponse_Err{
						Err: &token.Error{
							Message: "flying-pineapple",
							Payload: []byte("payload"),
						},
					},
				}
				signedCommandResp = &token.SignedCommandResponse{
					Response:  ProtoMarshal(commandResponse),
					Signature: []byte("signature"),
				}
				fakeProverClient.ProcessCommandReturns(signedCommandResp, nil)
			})

			It("returns an error", func() {
				_, err := prover.ListTokens(fakeSigningIdentity)
				Expect(err).To(MatchError("error from prover: flying-pineapple"))

				Expect(fakeSigningIdentity.SerializeCallCount()).To(Equal(1))
				Expect(fakeSigningIdentity.SignCallCount()).To(Equal(1))
				Expect(fakeProverClient.ProcessCommandCallCount()).To(Equal(1))
			})
		})

		Context("when ProcessCommand does not return UnspentTokens", func() {
			BeforeEach(func() {
				commandResp := &token.CommandResponse{}
				signedCommandResp = &token.SignedCommandResponse{
					Response:  ProtoMarshal(commandResp),
					Signature: []byte("response-signature"),
				}

				fakeProverClient.ProcessCommandReturns(signedCommandResp, nil)
			})

			It("returns an error", func() {
				_, err := prover.ListTokens(fakeSigningIdentity)
				Expect(err).To(MatchError("no UnspentTokens in command response"))

				Expect(fakeSigningIdentity.SerializeCallCount()).To(Equal(1))
				Expect(fakeSigningIdentity.SignCallCount()).To(Equal(1))
				Expect(fakeProverClient.ProcessCommandCallCount()).To(Equal(1))
			})
		})
	})

	Describe("SendCommand", func() {
		var (
			signedCommand *token.SignedCommand
		)

		BeforeEach(func() {
			signedCommand = &token.SignedCommand{
				Command:   []byte("token-command"),
				Signature: []byte("pineapple"),
			}
		})

		It("returns serialized token transaction", func() {
			txBytes, err := prover.SendCommand(context.Background(), signedCommand)
			Expect(err).NotTo(HaveOccurred())
			Expect(txBytes).To(Equal(serializedTokenTx))

			Expect(fakeProverPeerClient.CreateProverClientCallCount()).To(Equal(1))
			Expect(fakeProverClient.ProcessCommandCallCount()).To(Equal(1))
			_, sc, _ := fakeProverClient.ProcessCommandArgsForCall(0)
			Expect(sc).To(Equal(signedCommand))
		})

		Context("when CreateProverClient fails", func() {
			BeforeEach(func() {
				fakeProverPeerClient.CreateProverClientReturns(nil, nil, errors.New("wild-banana"))
			})

			It("returns an error", func() {
				_, err := prover.SendCommand(context.Background(), signedCommand)
				Expect(err).To(MatchError("wild-banana"))
				Expect(fakeProverPeerClient.CreateProverClientCallCount()).To(Equal(1))
				Expect(fakeProverClient.ProcessCommandCallCount()).To(Equal(0))
			})
		})

		Context("when ProverClient.ProcessCommand fails", func() {
			BeforeEach(func() {
				fakeProverClient.ProcessCommandReturns(nil, errors.New("wild-banana"))
			})

			It("returns an error", func() {
				_, err := prover.SendCommand(context.Background(), signedCommand)
				Expect(err).To(MatchError("wild-banana"))
				Expect(fakeProverPeerClient.CreateProverClientCallCount()).To(Equal(1))
				Expect(fakeProverClient.ProcessCommandCallCount()).To(Equal(1))
			})
		})

		Context("when ProcessCommand returns error response", func() {
			BeforeEach(func() {
				cmdResponse := &token.CommandResponse{
					Payload: &token.CommandResponse_Err{
						Err: &token.Error{
							Message: "flying-pineapple",
							Payload: []byte("payload"),
						},
					},
				}
				signedCommandResp = &token.SignedCommandResponse{
					Response:  ProtoMarshal(cmdResponse),
					Signature: []byte("response-signature"),
				}
				fakeProverClient.ProcessCommandReturns(signedCommandResp, nil)
			})

			It("returns an error", func() {
				_, err := prover.SendCommand(context.Background(), signedCommand)
				Expect(err).To(MatchError("error from prover: flying-pineapple"))
				Expect(fakeProverPeerClient.CreateProverClientCallCount()).To(Equal(1))
				Expect(fakeProverClient.ProcessCommandCallCount()).To(Equal(1))
			})
		})

		Context("when unmarshalling CommandResponse failed", func() {
			BeforeEach(func() {
				signedCommandResp = &token.SignedCommandResponse{
					Response:  []byte("bad-data"),
					Signature: []byte("response-signature"),
				}
				fakeProverClient.ProcessCommandReturns(signedCommandResp, nil)
			})

			It("returns an error", func() {
				_, err := prover.SendCommand(context.Background(), signedCommand)
				Expect(err).To(MatchError("failed to unmarshal command response: unexpected EOF"))
				Expect(fakeProverPeerClient.CreateProverClientCallCount()).To(Equal(1))
				Expect(fakeProverClient.ProcessCommandCallCount()).To(Equal(1))
			})
		})
	})

	Describe("CreateSignedCommand", func() {
		var (
			payload           interface{}
			marshalledCommand []byte
			signedCommand     *token.SignedCommand
		)

		BeforeEach(func() {
			// create a valid payload
			tokensToIssue := []*token.Token{{
				Type:     "type",
				Quantity: ToHex(10),
				Owner:    &token.TokenOwner{Raw: []byte("alice")},
			}}
			payload = &token.Command_IssueRequest{
				IssueRequest: &token.IssueRequest{
					TokensToIssue: tokensToIssue,
				},
			}

			// create expected SignedCommand
			marshalledCommand = ProtoMarshal(&token.Command{
				Header:  commandHeader,
				Payload: payload.(*token.Command_IssueRequest),
			})
			signedCommand = &token.SignedCommand{
				Command:   marshalledCommand,
				Signature: []byte("pineapple"),
			}
		})

		It("returns a signed command", func() {
			sc, err := prover.CreateSignedCommand(payload, fakeSigningIdentity)
			Expect(err).NotTo(HaveOccurred())
			Expect(sc).To(Equal(signedCommand))

			Expect(fakeSigningIdentity.SerializeCallCount()).To(Equal(1))
			Expect(fakeSigningIdentity.SignCallCount()).To(Equal(1))
			raw := fakeSigningIdentity.SignArgsForCall(0)
			Expect(raw).To(Equal(marshalledCommand))
		})

		Context("when SigningIdentity sign fails", func() {
			BeforeEach(func() {
				fakeSigningIdentity.SignReturns(nil, errors.New("wild-banana"))
			})

			It("returns an error", func() {
				_, err := prover.CreateSignedCommand(payload, fakeSigningIdentity)
				Expect(err).To(MatchError("wild-banana"))

				Expect(fakeSigningIdentity.SerializeCallCount()).To(Equal(1))
				Expect(fakeSigningIdentity.SignCallCount()).To(Equal(1))
				raw := fakeSigningIdentity.SignArgsForCall(0)
				Expect(raw).To(Equal(marshalledCommand))
			})
		})

		Context("when payload is not a valid type", func() {
			BeforeEach(func() {
				payload = []byte("garbage")
			})

			It("returns an error", func() {
				_, err := prover.CreateSignedCommand(payload, fakeSigningIdentity)
				Expect(err).To(MatchError("command type not recognized: []uint8"))

				Expect(fakeSigningIdentity.SerializeCallCount()).To(Equal(0))
				Expect(fakeSigningIdentity.SignCallCount()).To(Equal(0))
			})
		})
	})

	Describe("NewProver", func() {
		var (
			config           *client.ClientConfig
			proverServerCert tls.Certificate
			peerEndpoint     string
			listener         net.Listener
		)

		BeforeEach(func() {
			// create listener to get the endpoint
			var err error
			listener, err = net.Listen("tcp", "127.0.0.1:")
			Expect(err).To(BeNil())
			peerEndpoint = listener.Addr().String()

			config = &client.ClientConfig{
				ChannelID: channelID,
				ProverPeer: client.ConnectionConfig{
					Address:         peerEndpoint,
					TLSEnabled:      true,
					TLSRootCertFile: "./testdata/crypto/peerOrganizations/org1.example.com/peers/peer0.org1.example.com/tls/ca.crt",
				},
			}

			proverServerCert, err = tls.LoadX509KeyPair(
				"./testdata/crypto/peerOrganizations/org1.example.com/peers/peer0.org1.example.com/tls/server.crt",
				"./testdata/crypto/peerOrganizations/org1.example.com/peers/peer0.org1.example.com/tls/server.key",
			)
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			if listener != nil {
				listener.Close()
			}
		})

		It("creates a Prover when TLS is enabled", func() {
			// start grpc servers with TLS
			grpcServer := grpc.NewServer(grpc.Creds(credentials.NewTLS(&tls.Config{
				Certificates: []tls.Certificate{proverServerCert},
			})))
			token.RegisterProverServer(grpcServer, &server.Prover{})
			defer grpcServer.Stop()
			go grpcServer.Serve(listener)

			prover, err := client.NewProverPeer(config)
			Expect(err).NotTo(HaveOccurred())

			Expect(prover.ChannelID).To(Equal(channelID))
			Expect(prover.RandomnessReader).To(Equal(rand.Reader))

			ppClient, ok := prover.ProverPeerClient.(*client.ProverPeerClientImpl)
			Expect(ok).To(Equal(true))
			Expect(ppClient.Address).To(Equal(config.ProverPeer.Address))
			Expect(ppClient.ServerNameOverride).To(Equal(""))
			Expect(ppClient.GRPCClient).NotTo(BeNil())

			conn, proverClient, err := ppClient.CreateProverClient()
			Expect(err).NotTo(HaveOccurred())
			Expect(conn).NotTo(BeNil())
			Expect(proverClient).NotTo(BeNil())
		})

		It("creates a Prover when TLS is disabled", func() {
			grpcServer := grpc.NewServer()
			token.RegisterProverServer(grpcServer, &server.Prover{})
			defer grpcServer.Stop()
			go grpcServer.Serve(listener)

			config.ProverPeer.TLSEnabled = false
			prover, err := client.NewProverPeer(config)
			Expect(err).NotTo(HaveOccurred())

			Expect(prover.ChannelID).To(Equal(channelID))
			Expect(prover.RandomnessReader).To(Equal(rand.Reader))

			ppClient, ok := prover.ProverPeerClient.(*client.ProverPeerClientImpl)
			Expect(ok).To(Equal(true))
			Expect(ppClient.Address).To(Equal(config.ProverPeer.Address))
			Expect(ppClient.ServerNameOverride).To(Equal(""))
			Expect(ppClient.GRPCClient).NotTo(BeNil())

			conn, proverClient, err := ppClient.CreateProverClient()
			Expect(err).NotTo(HaveOccurred())
			Expect(conn).NotTo(BeNil())
			Expect(proverClient).NotTo(BeNil())
		})

		Context("when it fails to connect to prover peer", func() {
			BeforeEach(func() {
				// use a non-existing port and 1 second timeout
				config.ProverPeer.Address = "127.0.0.1:11111"
				config.ProverPeer.ConnectionTimeout = 1 * time.Second
			})

			It("returns an error", func() {
				prover, err := client.NewProverPeer(config)
				Expect(err).NotTo(HaveOccurred())

				ppClient, ok := prover.ProverPeerClient.(*client.ProverPeerClientImpl)
				Expect(ok).To(Equal(true))
				conn, proverClient, err := ppClient.CreateProverClient()
				Expect(err.Error()).To(ContainSubstring("failed to create new connection"))
				Expect(conn).To(BeNil())
				Expect(proverClient).To(BeNil())
			})
		})

		Context("when TLS root cert file is missing in config", func() {
			BeforeEach(func() {
				config.ProverPeer.TLSRootCertFile = ""
			})

			It("returns an error", func() {
				_, err := client.NewProverPeer(config)
				Expect(err).To(MatchError("missing TLSRootCertFile in client config"))
			})
		})

		Context("when it fails to load root cert file", func() {
			BeforeEach(func() {
				config.ProverPeer.TLSRootCertFile = "./testdata/crypto/non-file"
			})

			It("returns an error", func() {
				_, err := client.NewProverPeer(config)
				Expect(err.Error()).To(ContainSubstring("unable to load TLS cert from " + config.ProverPeer.TLSRootCertFile))
			})
		})
	})
})

func clock() time.Time {
	return time.Time{}
}
