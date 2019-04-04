/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package plain_test

import (
	"errors"
	"fmt"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/protos/ledger/queryresult"
	"github.com/hyperledger/fabric/protos/token"
	"github.com/hyperledger/fabric/token/ledger/mock"
	"github.com/hyperledger/fabric/token/tms/plain"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("RequestListTokens", func() {
	var (
		transactor    *plain.Transactor
		unspentTokens *token.UnspentTokens
		outputs       [][]byte
		keys          []string
		results       []*queryresult.KV
	)

	It("initializes variables for test", func() {
		outputs = make([][]byte, 4)
		keys = make([]string, 4)
		results = make([]*queryresult.KV, 5)

		var err error
		outputs[0], err = proto.Marshal(&token.Token{Owner: &token.TokenOwner{Raw: []byte("Alice")}, Type: "TOK1", Quantity: ToHex(100)})
		Expect(err).NotTo(HaveOccurred())
		outputs[1], err = proto.Marshal(&token.Token{Owner: &token.TokenOwner{Raw: []byte("Bob")}, Type: "TOK2", Quantity: ToHex(200)})
		Expect(err).NotTo(HaveOccurred())
		outputs[2], err = proto.Marshal(&token.Token{Owner: &token.TokenOwner{Raw: []byte("Alice")}, Type: "TOK3", Quantity: ToHex(300)})
		Expect(err).NotTo(HaveOccurred())
		outputs[3], err = proto.Marshal(&token.Token{Owner: &token.TokenOwner{Raw: []byte("Alice")}, Type: "TOK4", Quantity: ToHex(400)})
		Expect(err).NotTo(HaveOccurred())

		// owner should be the same credential for transactor
		ownerString := buildTokenOwnerString([]byte("Alice"))

		keys[0] = generateKey(ownerString, "1", "0", tokenKeyPrefix)
		keys[1] = generateKey(ownerString, "1", "1", tokenKeyPrefix)
		keys[2] = generateKey(ownerString, "2", "0", tokenKeyPrefix)
		keys[3] = generateKey(ownerString, "3", "0", tokenKeyPrefix)

		results[0] = &queryresult.KV{Key: keys[0], Value: outputs[0]}
		results[1] = &queryresult.KV{Key: keys[1], Value: outputs[1]}
		results[2] = &queryresult.KV{Key: keys[2], Value: outputs[2]}
		results[3] = &queryresult.KV{Key: keys[3], Value: outputs[3]}
		results[4] = &queryresult.KV{Key: "123", Value: []byte("not an output")}

		unspentTokens = &token.UnspentTokens{
			Tokens: []*token.UnspentToken{
				{Id: &token.TokenId{TxId: "1", Index: uint32(0)}, Type: "TOK1", Quantity: ToDecimal(100)},
				{Id: &token.TokenId{TxId: "3", Index: uint32(0)}, Type: "TOK4", Quantity: ToDecimal(400)},
			},
		}
	})

	Describe("verify the unspentTokens returned by a list token request", func() {
		var (
			fakeLedger   *mock.LedgerReader
			fakeIterator *mock.ResultsIterator
		)

		BeforeEach(func() {
			fakeLedger = &mock.LedgerReader{}
			fakeIterator = &mock.ResultsIterator{}
			transactor = &plain.Transactor{PublicCredential: []byte("Alice"), Ledger: fakeLedger}
		})

		When("request list tokens does not fail", func() {
			It("returns unspent tokens", func() {
				fakeLedger.GetStateRangeScanIteratorReturns(fakeIterator, nil)
				fakeIterator.NextReturnsOnCall(0, results[0], nil)
				fakeIterator.NextReturnsOnCall(1, results[3], nil)
				fakeIterator.NextReturnsOnCall(2, nil, nil)

				tokens, err := transactor.ListTokens()
				Expect(err).NotTo(HaveOccurred())
				Expect(proto.Equal(tokens, unspentTokens)).To(BeTrue())
			})
		})

		When("request list tokens fails", func() {
			It("returns an error", func() {
				When("GetStateRangeScanIterator fails", func() {
					fakeLedger.GetStateRangeScanIteratorReturns(nil, errors.New("water melon"))
					tokens, err := transactor.ListTokens()

					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(Equal("water melon"))
					Expect(tokens).To(BeNil())
					Expect(fakeIterator.NextCallCount()).To(Equal(0))
				})
				When("Next fails", func() {
					fakeLedger.GetStateRangeScanIteratorReturns(fakeIterator, nil)
					fakeIterator.NextReturnsOnCall(0, results[0], nil)
					fakeIterator.NextReturnsOnCall(1, queryresult.KV{}, errors.New("banana"))

					tokens, err := transactor.ListTokens()
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(Equal("banana"))
					Expect(tokens).To(BeNil())
					Expect(fakeIterator.NextCallCount()).To(Equal(2))
				})
			})
		})
	})
})

var _ = Describe("Transactor", func() {
	var (
		transactor              *plain.Transactor
		recipientTransferShares []*token.RecipientShare
	)

	BeforeEach(func() {
		recipientTransferShares = []*token.RecipientShare{
			{Recipient: &token.TokenOwner{Raw: []byte("R1")}, Quantity: ToHex(1001)},
			{Recipient: &token.TokenOwner{Raw: []byte("R2")}, Quantity: ToHex(1002)},
			{Recipient: &token.TokenOwner{Raw: []byte("R3")}, Quantity: ToHex(1003)},
		}
		transactor = &plain.Transactor{PublicCredential: []byte("Alice")}
	})

	It("converts a transfer request with no inputs into a token transaction", func() {
		transferRequest := &token.TransferRequest{
			Credential: []byte("credential"),
			TokenIds:   []*token.TokenId{},
			Shares:     recipientTransferShares,
		}

		tt, err := transactor.RequestTransfer(transferRequest)
		Expect(tt).To(BeNil())
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(Equal("no token IDs in transfer request"))
	})

	Describe("when no recipient shares are provided", func() {
		It("returns an error", func() {
			transferRequest := &token.TransferRequest{
				Credential: []byte("credential"),
				TokenIds:   []*token.TokenId{{TxId: "george", Index: 0}},
				Shares:     []*token.RecipientShare{},
			}

			tt, err := transactor.RequestTransfer(transferRequest)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal("no shares in transfer request"))
			Expect(tt).To(BeNil())
		})
	})

	Describe("when a transfer request with actual inputs is provided", func() {
		var (
			fakeLedger      *mock.LedgerWriter
			transferRequest *token.TransferRequest
			inputBytes      []byte
		)

		When("transferred quantity is same as quantity in token ids", func() {
			BeforeEach(func() {
				input := &token.Token{
					Owner:    &token.TokenOwner{Raw: []byte("Alice")},
					Type:     "TOK1",
					Quantity: ToHex(3006),
				}
				var err error
				inputBytes, err = proto.Marshal(input)
				Expect(err).NotTo(HaveOccurred())
				fakeLedger = &mock.LedgerWriter{}
				fakeLedger.SetStateReturns(nil)
				fakeLedger.GetStateReturnsOnCall(0, inputBytes, nil)
				transactor.Ledger = fakeLedger
				transactor.TokenOwnerValidator = &TestTokenOwnerValidator{}
			})

			It("creates a valid transfer request", func() {
				transferRequest = &token.TransferRequest{
					Credential: []byte("credential"),
					TokenIds:   []*token.TokenId{{TxId: "george", Index: 0}},
					Shares:     recipientTransferShares,
				}
				tt, err := transactor.RequestTransfer(transferRequest)
				Expect(err).NotTo(HaveOccurred())
				Expect(tt).To(Equal(&token.TokenTransaction{
					Action: &token.TokenTransaction_TokenAction{
						TokenAction: &token.TokenAction{
							Data: &token.TokenAction_Transfer{
								Transfer: &token.Transfer{
									Inputs: []*token.TokenId{
										{TxId: "george", Index: uint32(0)},
									},
									Outputs: []*token.Token{
										{Owner: &token.TokenOwner{Raw: []byte("R1")}, Type: "TOK1", Quantity: ToHex(1001)},
										{Owner: &token.TokenOwner{Raw: []byte("R2")}, Type: "TOK1", Quantity: ToHex(1002)},
										{Owner: &token.TokenOwner{Raw: []byte("R3")}, Type: "TOK1", Quantity: ToHex(1003)},
									},
								},
							},
						},
					},
				}))
			})
		})

		When("quantity in token ids is more than quantity for transfer", func() {
			BeforeEach(func() {
				input := &token.Token{
					Owner:    &token.TokenOwner{Raw: []byte("Alice")},
					Type:     "TOK1",
					Quantity: ToHex(3106),
				}
				var err error
				inputBytes, err = proto.Marshal(input)
				Expect(err).NotTo(HaveOccurred())
				fakeLedger = &mock.LedgerWriter{}
				fakeLedger.SetStateReturns(nil)
				fakeLedger.GetStateReturnsOnCall(0, inputBytes, nil)
				transactor.Ledger = fakeLedger
				transactor.TokenOwnerValidator = &TestTokenOwnerValidator{}
			})

			It("creates a valid transfer request with an output for remaining quantity", func() {
				transferRequest = &token.TransferRequest{
					Credential: []byte("credential"),
					TokenIds:   []*token.TokenId{{TxId: "george", Index: 0}},
					Shares:     recipientTransferShares,
				}
				tt, err := transactor.RequestTransfer(transferRequest)
				Expect(err).NotTo(HaveOccurred())
				Expect(tt).To(Equal(&token.TokenTransaction{
					Action: &token.TokenTransaction_TokenAction{
						TokenAction: &token.TokenAction{
							Data: &token.TokenAction_Transfer{
								Transfer: &token.Transfer{
									Inputs: []*token.TokenId{
										{TxId: "george", Index: uint32(0)},
									},
									Outputs: []*token.Token{
										{Owner: &token.TokenOwner{Raw: []byte("R1")}, Type: "TOK1", Quantity: ToHex(1001)},
										{Owner: &token.TokenOwner{Raw: []byte("R2")}, Type: "TOK1", Quantity: ToHex(1002)},
										{Owner: &token.TokenOwner{Raw: []byte("R3")}, Type: "TOK1", Quantity: ToHex(1003)},
										{Owner: &token.TokenOwner{Raw: []byte("Alice")}, Type: "TOK1", Quantity: ToHex(100)},
									},
								},
							},
						},
					},
				}))
			})
		})

		When("quantity in token ids is less than quantity for transfer", func() {
			BeforeEach(func() {
				input := &token.Token{
					Owner:    &token.TokenOwner{Raw: []byte("Alice")},
					Type:     "TOK1",
					Quantity: ToHex(3000),
				}
				var err error
				inputBytes, err = proto.Marshal(input)
				Expect(err).NotTo(HaveOccurred())
				fakeLedger = &mock.LedgerWriter{}
				fakeLedger.SetStateReturns(nil)
				fakeLedger.GetStateReturnsOnCall(0, inputBytes, nil)
				transactor.Ledger = fakeLedger
				transactor.TokenOwnerValidator = &TestTokenOwnerValidator{}
			})

			It("returns an error ", func() {
				transferRequest = &token.TransferRequest{
					Credential: []byte("credential"),
					TokenIds:   []*token.TokenId{{TxId: "george", Index: 0}},
					Shares:     recipientTransferShares,
				}
				_, err := transactor.RequestTransfer(transferRequest)
				Expect(err).To(MatchError("total quantity [3000] from TokenIds is less than total quantity [3006] for transfer"))
			})
		})
	})

	Describe("when a transfer request with a non-existing input is provided", func() {
		var (
			fakeLedger      *mock.LedgerWriter
			transferRequest *token.TransferRequest
			tokenID         *token.TokenId
		)

		BeforeEach(func() {
			fakeLedger = &mock.LedgerWriter{}
			fakeLedger.SetStateReturns(nil)
			fakeLedger.GetStateReturnsOnCall(0, nil, nil)
			transactor.Ledger = fakeLedger

			tokenID = &token.TokenId{TxId: "george", Index: 0}
		})

		It("returns an invalid transaction error", func() {
			transferRequest = &token.TransferRequest{
				Credential: []byte("credential"),
				TokenIds:   []*token.TokenId{tokenID},
				Shares:     recipientTransferShares,
			}
			_, err := transactor.RequestTransfer(transferRequest)
			Expect(err.Error()).To(Equal(fmt.Sprintf("input TokenId (%s, %d) does not exist or not owned by the user", "george", 0)))
		})
	})

	Describe("when a transfer request with two different input token types is provided", func() {
		var (
			fakeLedger      *mock.LedgerWriter
			transferRequest *token.TransferRequest
			inputBytes1     []byte
			inputBytes2     []byte
			tokenID1        *token.TokenId
			tokenID2        *token.TokenId
		)

		BeforeEach(func() {
			input1 := &token.Token{
				Owner:    &token.TokenOwner{Raw: []byte("Alice")},
				Type:     "TOK1",
				Quantity: ToHex(99),
			}
			input2 := &token.Token{
				Owner:    &token.TokenOwner{Raw: []byte("Alice")},
				Type:     "TOK2",
				Quantity: ToHex(99),
			}
			var err error
			inputBytes1, err = proto.Marshal(input1)
			Expect(err).NotTo(HaveOccurred())
			inputBytes2, err = proto.Marshal(input2)
			Expect(err).NotTo(HaveOccurred())
			fakeLedger = &mock.LedgerWriter{}
			fakeLedger.SetStateReturns(nil)
			fakeLedger.GetStateReturnsOnCall(0, inputBytes1, nil)
			fakeLedger.GetStateReturnsOnCall(1, inputBytes2, nil)
			transactor.Ledger = fakeLedger
			tokenID1 = &token.TokenId{TxId: "george", Index: 0}
			tokenID2 = &token.TokenId{TxId: "george", Index: 1}
		})

		It("returns an invalid transaction error", func() {
			transferRequest = &token.TransferRequest{
				Credential: []byte("credential"),
				TokenIds:   []*token.TokenId{tokenID1, tokenID2},
				Shares:     recipientTransferShares,
			}
			_, err := transactor.RequestTransfer(transferRequest)
			Expect(err.Error()).To(Equal("two or more token types specified in input: 'TOK1', 'TOK2'"))
		})
	})

	Describe("RequestRedeem", func() {
		var (
			fakeLedger     *mock.LedgerWriter
			redeemRequest  *token.RedeemRequest
			inputBytes     []byte
			inputQuantity  uint64
			redeemQuantity uint64
		)

		BeforeEach(func() {
			inputQuantity = 99
			input := &token.Token{
				Owner:    &token.TokenOwner{Raw: []byte("Alice")},
				Type:     "TOK1",
				Quantity: ToHex(inputQuantity),
			}
			var err error
			inputBytes, err = proto.Marshal(input)
			Expect(err).NotTo(HaveOccurred())
			fakeLedger = &mock.LedgerWriter{}
			fakeLedger.SetStateReturns(nil)
			fakeLedger.GetStateReturns(inputBytes, nil)
			transactor.Ledger = fakeLedger
		})

		It("creates a token transaction with 1 output if all tokens are redeemed", func() {
			redeemQuantity = inputQuantity
			redeemRequest = &token.RedeemRequest{
				Credential: []byte("credential"),
				TokenIds:   []*token.TokenId{{TxId: "robert", Index: uint32(0)}},
				Quantity:   ToHex(redeemQuantity),
			}
			tt, err := transactor.RequestRedeem(redeemRequest)
			Expect(err).NotTo(HaveOccurred())
			Expect(tt).To(Equal(&token.TokenTransaction{
				Action: &token.TokenTransaction_TokenAction{
					TokenAction: &token.TokenAction{
						Data: &token.TokenAction_Redeem{
							Redeem: &token.Transfer{
								Inputs: []*token.TokenId{
									{TxId: "robert", Index: uint32(0)},
								},
								Outputs: []*token.Token{
									{Type: "TOK1", Quantity: ToHex(redeemQuantity)},
								},
							},
						},
					},
				},
			}))
		})

		It("creates a token transaction with 2 outputs if some tokens are redeemed", func() {
			redeemQuantity = 50
			unredeemedQuantity := inputQuantity - 50
			redeemRequest = &token.RedeemRequest{
				Credential: []byte("credential"),
				TokenIds:   []*token.TokenId{{TxId: "robert", Index: uint32(0)}},
				Quantity:   ToHex(redeemQuantity),
			}
			tt, err := transactor.RequestRedeem(redeemRequest)
			Expect(err).NotTo(HaveOccurred())
			Expect(tt).To(Equal(&token.TokenTransaction{
				Action: &token.TokenTransaction_TokenAction{
					TokenAction: &token.TokenAction{
						Data: &token.TokenAction_Redeem{
							Redeem: &token.Transfer{
								Inputs: []*token.TokenId{
									{TxId: "robert", Index: uint32(0)},
								},
								Outputs: []*token.Token{
									{Type: "TOK1", Quantity: ToHex(redeemQuantity)},
									{Owner: &token.TokenOwner{Raw: []byte("Alice")}, Type: "TOK1", Quantity: ToHex(unredeemedQuantity)},
								},
							},
						},
					},
				},
			}))
		})

		Context("when quantity to redeem is greater than input quantity", func() {
			BeforeEach(func() {
				redeemQuantity = inputQuantity + 10
				redeemRequest = &token.RedeemRequest{
					Credential: []byte("credential"),
					TokenIds:   []*token.TokenId{{TxId: "robert", Index: uint32(0)}},
					Quantity:   ToHex(redeemQuantity),
				}
			})

			It("returns an error", func() {
				_, err := transactor.RequestRedeem(redeemRequest)
				Expect(err).To(MatchError(fmt.Sprintf("total quantity [%d] from TokenIds is less than quantity [%s] to be redeemed", inputQuantity, ToHex(redeemQuantity))))
			})
		})
	})

	Describe("RequestTokenOperation", func() {
		var (
			fakeLedger            *mock.LedgerWriter
			tokenOperationRequest *token.TokenOperationRequest
			inputQuantity         uint64
		)

		BeforeEach(func() {
			inputQuantity = 100
			input := &token.Token{
				Owner:    &token.TokenOwner{Raw: []byte("Alice")},
				Type:     "TOK1",
				Quantity: ToHex(inputQuantity),
			}
			inputBytes, err := proto.Marshal(input)
			Expect(err).NotTo(HaveOccurred())
			fakeLedger = &mock.LedgerWriter{}
			fakeLedger.GetStateReturns(inputBytes, nil)
			transactor.Ledger = fakeLedger

			tokenOperationRequest = &token.TokenOperationRequest{
				Credential: []byte("credential"),
				TokenIds:   []*token.TokenId{{TxId: "robert", Index: uint32(0)}},
				Operations: []*token.TokenOperation{{
					Operation: &token.TokenOperation_Action{
						Action: &token.TokenOperationAction{
							Payload: &token.TokenOperationAction_Transfer{
								Transfer: &token.TokenActionTerms{
									Sender: &token.TokenOwner{Raw: []byte("credential")},
									Outputs: []*token.Token{{
										Owner:    &token.TokenOwner{Raw: []byte("owner-1")},
										Type:     "TOK1",
										Quantity: ToHex(inputQuantity),
									}},
								},
							},
						},
					},
				},
				},
			}
		})

		It("creates a token transaction when input quantity is same as output quantity", func() {
			tt, count, err := transactor.RequestTokenOperation(tokenOperationRequest.TokenIds, tokenOperationRequest.Operations[0])
			Expect(err).NotTo(HaveOccurred())
			Expect(count).To(BeEquivalentTo(1))
			Expect(tt).To(Equal(&token.TokenTransaction{
				Action: &token.TokenTransaction_TokenAction{
					TokenAction: &token.TokenAction{
						Data: &token.TokenAction_Transfer{
							Transfer: &token.Transfer{
								Inputs: []*token.TokenId{
									{TxId: "robert", Index: uint32(0)},
								},
								Outputs: []*token.Token{{
									Owner:    &token.TokenOwner{Raw: []byte("owner-1")},
									Type:     "TOK1",
									Quantity: ToHex(inputQuantity),
								}},
							},
						},
					},
				},
			}))
		})

		It("creates a token transaction when input quantity is greater than output quantity", func() {
			// change quantity in operation output to be less than inputQuantity
			tokenOperationRequest.GetOperations()[0].GetAction().GetTransfer().Outputs[0].Quantity = ToHex(40)
			tt, count, err := transactor.RequestTokenOperation(tokenOperationRequest.TokenIds, tokenOperationRequest.Operations[0])
			Expect(count).To(BeEquivalentTo(1))
			Expect(err).NotTo(HaveOccurred())
			Expect(tt).To(Equal(&token.TokenTransaction{
				Action: &token.TokenTransaction_TokenAction{
					TokenAction: &token.TokenAction{
						Data: &token.TokenAction_Transfer{
							Transfer: &token.Transfer{
								Inputs: []*token.TokenId{
									{TxId: "robert", Index: uint32(0)},
								},
								Outputs: []*token.Token{
									{Owner: &token.TokenOwner{Raw: []byte("owner-1")}, Type: "TOK1", Quantity: ToHex(40)},
									{Owner: &token.TokenOwner{Raw: []byte("Alice")}, Type: "TOK1", Quantity: ToHex(inputQuantity - 40)},
								},
							},
						},
					},
				},
			}))
		})

		Context("when quantity in output is greater than input quantity", func() {
			BeforeEach(func() {
				// change quantity in operation output
				tokenOperationRequest.GetOperations()[0].GetAction().GetTransfer().Outputs[0].Quantity = ToHex(inputQuantity + 1)
			})

			It("returns an error", func() {
				_, count, err := transactor.RequestTokenOperation(tokenOperationRequest.TokenIds, tokenOperationRequest.Operations[0])
				Expect(count).To(BeEquivalentTo(0))
				Expect(err).To(MatchError(fmt.Sprintf("total quantity [%d] from TokenIds is less than total quantity [%d] in token operation", inputQuantity, inputQuantity+1)))
			})
		})

		Context("when quantity in output is greater than input quantity", func() {
			BeforeEach(func() {
				// change quantity in operation output
				tokenOperationRequest.GetOperations()[0].GetAction().GetTransfer().Outputs[0].Quantity = ToHex(inputQuantity + 1)
			})

			It("returns an error", func() {
				_, count, err := transactor.RequestTokenOperation(tokenOperationRequest.TokenIds, tokenOperationRequest.Operations[0])
				Expect(count).To(BeEquivalentTo(0))
				Expect(err).To(MatchError(fmt.Sprintf("total quantity [%d] from TokenIds is less than total quantity [%d] in token operation", inputQuantity, inputQuantity+1)))
			})
		})

		Context("when ExpectationRequest has nil PlainExpectation", func() {
			BeforeEach(func() {
				tokenOperationRequest.Operations = []*token.TokenOperation{{}}
			})

			It("returns the error", func() {
				_, count, err := transactor.RequestTokenOperation(tokenOperationRequest.TokenIds, tokenOperationRequest.Operations[0])
				Expect(count).To(BeEquivalentTo(0))
				Expect(err).To(MatchError("no action in request"))
			})
		})

		Context("when ExpectationRequest has nil TransferExpectation", func() {
			BeforeEach(func() {
				tokenOperationRequest.Operations =
					[]*token.TokenOperation{{
						Operation: &token.TokenOperation_Action{
							Action: &token.TokenOperationAction{},
						},
					},
					}
			})

			It("returns the error", func() {
				_, count, err := transactor.RequestTokenOperation(tokenOperationRequest.TokenIds, tokenOperationRequest.Operations[0])
				Expect(count).To(BeEquivalentTo(0))
				Expect(err).To(MatchError("no transfer in action"))
			})
		})
	})
})

func generateKey(owner, txID, index, namespace string) string {
	return "\x00" + namespace + "\x00" + owner + "\x00" + txID + "\x00" + index + "\x00"
}
