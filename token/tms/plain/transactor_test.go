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
		outputs[0], err = proto.Marshal(&token.PlainOutput{Owner: []byte("Alice"), Type: "TOK1", Quantity: 100})
		Expect(err).NotTo(HaveOccurred())
		outputs[1], err = proto.Marshal(&token.PlainOutput{Owner: []byte("Bob"), Type: "TOK2", Quantity: 200})
		Expect(err).NotTo(HaveOccurred())
		outputs[2], err = proto.Marshal(&token.PlainOutput{Owner: []byte("Alice"), Type: "TOK3", Quantity: 300})
		Expect(err).NotTo(HaveOccurred())
		outputs[3], err = proto.Marshal(&token.PlainOutput{Owner: []byte("Alice"), Type: "TOK4", Quantity: 400})
		Expect(err).NotTo(HaveOccurred())

		keys[0] = generateKey("1", "0", "tokenOutput")
		keys[1] = generateKey("1", "1", "tokenOutput")
		keys[2] = generateKey("2", "0", "tokenOutput")
		keys[3] = generateKey("3", "0", "tokenOutput")

		results[0] = &queryresult.KV{Key: keys[0], Value: outputs[0]}
		results[1] = &queryresult.KV{Key: keys[1], Value: outputs[1]}
		results[2] = &queryresult.KV{Key: keys[2], Value: outputs[2]}
		results[3] = &queryresult.KV{Key: keys[3], Value: outputs[3]}
		results[4] = &queryresult.KV{Key: "123", Value: []byte("not an output")}

		unspentTokens = &token.UnspentTokens{
			Tokens: []*token.TokenOutput{
				{Id: &token.TokenId{TxId: "1", Index: uint32(0)}, Type: "TOK1", Quantity: 100},
				{Id: &token.TokenId{TxId: "3", Index: uint32(0)}, Type: "TOK4", Quantity: 400},
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
				fakeIterator.NextReturnsOnCall(1, results[1], nil)
				fakeIterator.NextReturnsOnCall(2, results[2], nil)
				fakeIterator.NextReturnsOnCall(3, results[3], nil)
				fakeIterator.NextReturnsOnCall(4, results[4], nil)
				fakeIterator.NextReturnsOnCall(4, nil, nil)

				fakeLedger.GetStateReturnsOnCall(1, []byte("token is spent"), nil)
				tokens, err := transactor.ListTokens()
				Expect(err).NotTo(HaveOccurred())
				Expect(tokens).To(Equal(unspentTokens))
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
		recipientTransferShares []*token.RecipientTransferShare
	)

	BeforeEach(func() {
		recipientTransferShares = []*token.RecipientTransferShare{
			{Recipient: []byte("R1"), Quantity: 1001},
			{Recipient: []byte("R2"), Quantity: 1002},
			{Recipient: []byte("R3"), Quantity: 1003},
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
				Shares:     []*token.RecipientTransferShare{},
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

		BeforeEach(func() {
			input := &token.PlainOutput{
				Owner:    []byte("Alice"),
				Type:     "TOK1",
				Quantity: 99,
			}
			var err error
			inputBytes, err = proto.Marshal(input)
			Expect(err).ToNot(HaveOccurred())
			fakeLedger = &mock.LedgerWriter{}
			fakeLedger.SetStateReturns(nil)
			fakeLedger.GetStateReturnsOnCall(0, inputBytes, nil)
			transactor.Ledger = fakeLedger
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
				Action: &token.TokenTransaction_PlainAction{
					PlainAction: &token.PlainTokenAction{
						Data: &token.PlainTokenAction_PlainTransfer{
							PlainTransfer: &token.PlainTransfer{
								Inputs: []*token.TokenId{
									{TxId: "george", Index: uint32(0)},
								},
								Outputs: []*token.PlainOutput{
									{Owner: []byte("R1"), Type: "TOK1", Quantity: 1001},
									{Owner: []byte("R2"), Type: "TOK1", Quantity: 1002},
									{Owner: []byte("R3"), Type: "TOK1", Quantity: 1003},
								},
							},
						},
					},
				},
			}))
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
			Expect(err.Error()).To(Equal(fmt.Sprintf("input '%s' does not exist", string("\x00")+"tokenOutput"+string("\x00")+"george"+string("\x00")+"0"+string("\x00"))))
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
			input1 := &token.PlainOutput{
				Owner:    []byte("Alice"),
				Type:     "TOK1",
				Quantity: 99,
			}
			input2 := &token.PlainOutput{
				Owner:    []byte("Alice"),
				Type:     "TOK2",
				Quantity: 99,
			}
			var err error
			inputBytes1, err = proto.Marshal(input1)
			Expect(err).ToNot(HaveOccurred())
			inputBytes2, err = proto.Marshal(input2)
			Expect(err).ToNot(HaveOccurred())
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
			input := &token.PlainOutput{
				Owner:    []byte("Alice"),
				Type:     "TOK1",
				Quantity: inputQuantity,
			}
			var err error
			inputBytes, err = proto.Marshal(input)
			Expect(err).ToNot(HaveOccurred())
			fakeLedger = &mock.LedgerWriter{}
			fakeLedger.SetStateReturns(nil)
			fakeLedger.GetStateReturns(inputBytes, nil)
			transactor.Ledger = fakeLedger
		})

		It("creates a token transaction with 1 output if all tokens are redeemed", func() {
			redeemQuantity = inputQuantity
			redeemRequest = &token.RedeemRequest{
				Credential:       []byte("credential"),
				TokenIds:         []*token.TokenId{{TxId: "robert", Index: uint32(0)}},
				QuantityToRedeem: redeemQuantity,
			}
			tt, err := transactor.RequestRedeem(redeemRequest)
			Expect(err).NotTo(HaveOccurred())
			Expect(tt).To(Equal(&token.TokenTransaction{
				Action: &token.TokenTransaction_PlainAction{
					PlainAction: &token.PlainTokenAction{
						Data: &token.PlainTokenAction_PlainRedeem{
							PlainRedeem: &token.PlainTransfer{
								Inputs: []*token.TokenId{
									{TxId: "robert", Index: uint32(0)},
								},
								Outputs: []*token.PlainOutput{
									{Type: "TOK1", Quantity: redeemQuantity},
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
				Credential:       []byte("credential"),
				TokenIds:         []*token.TokenId{{TxId: "robert", Index: uint32(0)}},
				QuantityToRedeem: redeemQuantity,
			}
			tt, err := transactor.RequestRedeem(redeemRequest)
			Expect(err).NotTo(HaveOccurred())
			Expect(tt).To(Equal(&token.TokenTransaction{
				Action: &token.TokenTransaction_PlainAction{
					PlainAction: &token.PlainTokenAction{
						Data: &token.PlainTokenAction_PlainRedeem{
							PlainRedeem: &token.PlainTransfer{
								Inputs: []*token.TokenId{
									{TxId: "robert", Index: uint32(0)},
								},
								Outputs: []*token.PlainOutput{
									{Type: "TOK1", Quantity: redeemQuantity},
									{Owner: []byte("Alice"), Type: "TOK1", Quantity: unredeemedQuantity},
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
					Credential:       []byte("credential"),
					TokenIds:         []*token.TokenId{{TxId: "robert", Index: uint32(0)}},
					QuantityToRedeem: redeemQuantity,
				}
			})

			It("returns an error", func() {
				_, err := transactor.RequestRedeem(redeemRequest)
				Expect(err).To(MatchError(fmt.Sprintf("total quantity [%d] from TokenIds is less than quantity [%d] to be redeemed", inputQuantity, redeemQuantity)))
			})
		})
	})

	Describe("RequestExpectation", func() {
		var (
			fakeLedger         *mock.LedgerWriter
			expectationRequest *token.ExpectationRequest
			inputQuantity      uint64
		)

		BeforeEach(func() {
			inputQuantity = 100
			input := &token.PlainOutput{
				Owner:    []byte("Alice"),
				Type:     "TOK1",
				Quantity: inputQuantity,
			}
			inputBytes, err := proto.Marshal(input)
			Expect(err).ToNot(HaveOccurred())
			fakeLedger = &mock.LedgerWriter{}
			fakeLedger.GetStateReturns(inputBytes, nil)
			transactor.Ledger = fakeLedger

			expectationRequest = &token.ExpectationRequest{
				Credential: []byte("credential"),
				TokenIds:   []*token.TokenId{{TxId: "robert", Index: uint32(0)}},

				Expectation: &token.TokenExpectation{
					Expectation: &token.TokenExpectation_PlainExpectation{
						PlainExpectation: &token.PlainExpectation{
							Payload: &token.PlainExpectation_TransferExpectation{
								TransferExpectation: &token.PlainTokenExpectation{
									Outputs: []*token.PlainOutput{{
										Owner:    []byte("owner-1"),
										Type:     "TOK1",
										Quantity: inputQuantity,
									}},
								},
							},
						},
					},
				},
			}
		})

		It("creates a token transaction when input quantity is same as output quantity", func() {
			tt, err := transactor.RequestExpectation(expectationRequest)
			Expect(err).NotTo(HaveOccurred())
			Expect(tt).To(Equal(&token.TokenTransaction{
				Action: &token.TokenTransaction_PlainAction{
					PlainAction: &token.PlainTokenAction{
						Data: &token.PlainTokenAction_PlainTransfer{
							PlainTransfer: &token.PlainTransfer{
								Inputs: []*token.TokenId{
									{TxId: "robert", Index: uint32(0)},
								},
								Outputs: []*token.PlainOutput{{
									Owner:    []byte("owner-1"),
									Type:     "TOK1",
									Quantity: inputQuantity,
								}},
							},
						},
					},
				},
			}))
		})

		It("creates a token transaction when input quantity is greater than output quantity", func() {
			// change quantity in expectation output to be less than inputQuantity
			expectationRequest.GetExpectation().GetPlainExpectation().GetTransferExpectation().Outputs[0].Quantity = 40
			tt, err := transactor.RequestExpectation(expectationRequest)
			Expect(err).NotTo(HaveOccurred())
			Expect(tt).To(Equal(&token.TokenTransaction{
				Action: &token.TokenTransaction_PlainAction{
					PlainAction: &token.PlainTokenAction{
						Data: &token.PlainTokenAction_PlainTransfer{
							PlainTransfer: &token.PlainTransfer{
								Inputs: []*token.TokenId{
									{TxId: "robert", Index: uint32(0)},
								},
								Outputs: []*token.PlainOutput{
									{Owner: []byte("owner-1"), Type: "TOK1", Quantity: 40},
									{Owner: []byte("Alice"), Type: "TOK1", Quantity: inputQuantity - 40},
								},
							},
						},
					},
				},
			}))
		})

		Context("when quantity in output is greater than input quantity", func() {
			BeforeEach(func() {
				// change quantity in expectation output
				expectationRequest.GetExpectation().GetPlainExpectation().GetTransferExpectation().Outputs[0].Quantity = inputQuantity + 1
			})

			It("returns an error", func() {
				_, err := transactor.RequestExpectation(expectationRequest)
				Expect(err).To(MatchError(fmt.Sprintf("total quantity [%d] from TokenIds is less than total quantity [%d] in expectation", inputQuantity, inputQuantity+1)))
			})
		})

		Context("when quantity in output is greater than input quantity", func() {
			BeforeEach(func() {
				// change quantity in expectation output
				expectationRequest.GetExpectation().GetPlainExpectation().GetTransferExpectation().Outputs[0].Quantity = inputQuantity + 1
			})

			It("returns an error", func() {
				_, err := transactor.RequestExpectation(expectationRequest)
				Expect(err).To(MatchError(fmt.Sprintf("total quantity [%d] from TokenIds is less than total quantity [%d] in expectation", inputQuantity, inputQuantity+1)))
			})
		})

		Context("when ExpectationRequest has nil Expectation", func() {
			BeforeEach(func() {
				expectationRequest.Expectation = nil
			})

			It("returns the error", func() {
				_, err := transactor.RequestExpectation(expectationRequest)
				Expect(err).To(MatchError("no token expectation in ExpectationRequest"))
			})
		})

		Context("when ExpectationRequest has nil PlainExpectation", func() {
			BeforeEach(func() {
				expectationRequest.Expectation = &token.TokenExpectation{}
			})

			It("returns the error", func() {
				_, err := transactor.RequestExpectation(expectationRequest)
				Expect(err).To(MatchError("no plain expectation in ExpectationRequest"))
			})
		})

		Context("when ExpectationRequest has nil TransferExpectation", func() {
			BeforeEach(func() {
				expectationRequest.Expectation = &token.TokenExpectation{
					Expectation: &token.TokenExpectation_PlainExpectation{
						PlainExpectation: &token.PlainExpectation{},
					},
				}
			})

			It("returns the error", func() {
				_, err := transactor.RequestExpectation(expectationRequest)
				Expect(err).To(MatchError("no transfer expectation in ExpectationRequest"))
			})
		})
	})
})

var _ = Describe("Transactor Approve", func() {
	var (
		transactor      *plain.Transactor
		allowanceShares []*token.AllowanceRecipientShare
	)

	BeforeEach(func() {
		allowanceShares = []*token.AllowanceRecipientShare{
			{Recipient: []byte("Alice"), Quantity: 100},
			{Recipient: []byte("Bob"), Quantity: 200},
		}
		transactor = &plain.Transactor{}
	})

	Describe("converts an approve request into a token transaction", func() {
		var (
			fakeLedger     *mock.LedgerReader
			approveRequest *token.ApproveRequest
			inputBytes     []byte
		)

		BeforeEach(func() {
			input := &token.PlainOutput{
				Owner:    []byte("credential"),
				Type:     "XYZ",
				Quantity: 350,
			}
			var err error
			inputBytes, err = proto.Marshal(input)
			Expect(err).NotTo(HaveOccurred())
			fakeLedger = &mock.LedgerReader{}
			transactor.Ledger = fakeLedger
			transactor.PublicCredential = []byte("credential")

			fakeLedger.GetStateReturnsOnCall(0, inputBytes, nil)
		})

		It("creates a valid approve request", func() {
			approveRequest = &token.ApproveRequest{
				Credential: []byte("credential"),
				TokenIds:   []*token.TokenId{{TxId: "lalaland", Index: uint32(0)}},

				//[][]byte{[]byte(string("\x00") + "tokenOutput" + string("\x00") + "lalaland" + string("\x00") + "0" + string("\x00"))},
				AllowanceShares: allowanceShares,
			}
			tt, err := transactor.RequestApprove(approveRequest)
			Expect(err).NotTo(HaveOccurred())
			Expect(tt).To(Equal(&token.TokenTransaction{
				Action: &token.TokenTransaction_PlainAction{
					PlainAction: &token.PlainTokenAction{
						Data: &token.PlainTokenAction_PlainApprove{
							PlainApprove: &token.PlainApprove{
								Inputs: []*token.TokenId{
									{TxId: "lalaland", Index: uint32(0)},
								},
								DelegatedOutputs: []*token.PlainDelegatedOutput{
									{Owner: []byte("credential"), Delegatees: [][]byte{[]byte("Alice")}, Type: "XYZ", Quantity: 100},
									{Owner: []byte("credential"), Delegatees: [][]byte{[]byte("Bob")}, Type: "XYZ", Quantity: 200},
								},
								Output: &token.PlainOutput{Owner: []byte("credential"), Type: "XYZ", Quantity: 50},
							},
						},
					},
				},
			}))
		})

		It("creates a valid approve request without outputs", func() {
			input := &token.PlainOutput{
				Owner:    []byte("credential"),
				Type:     "XYZ",
				Quantity: 300,
			}
			var err error
			inputBytes, err = proto.Marshal(input)
			Expect(err).NotTo(HaveOccurred())

			fakeLedger.GetStateReturnsOnCall(0, inputBytes, nil)
			approveRequest = &token.ApproveRequest{
				Credential:      []byte("credential"),
				TokenIds:        []*token.TokenId{{TxId: "lalaland", Index: uint32(0)}},
				AllowanceShares: allowanceShares,
			}
			tt, err := transactor.RequestApprove(approveRequest)
			Expect(err).NotTo(HaveOccurred())
			Expect(tt).To(Equal(&token.TokenTransaction{
				Action: &token.TokenTransaction_PlainAction{
					PlainAction: &token.PlainTokenAction{
						Data: &token.PlainTokenAction_PlainApprove{
							PlainApprove: &token.PlainApprove{
								Inputs: []*token.TokenId{
									{TxId: "lalaland", Index: uint32(0)},
								},
								DelegatedOutputs: []*token.PlainDelegatedOutput{
									{Owner: []byte("credential"), Delegatees: [][]byte{[]byte("Alice")}, Type: "XYZ", Quantity: 100},
									{Owner: []byte("credential"), Delegatees: [][]byte{[]byte("Bob")}, Type: "XYZ", Quantity: 200},
								},
							},
						},
					},
				},
			}))
		})

		When("no inputs are provided", func() {
			It("returns an error", func() {
				approveRequest = &token.ApproveRequest{
					TokenIds:        []*token.TokenId{},
					AllowanceShares: allowanceShares,
				}

				tt, err := transactor.RequestApprove(approveRequest)
				Expect(err).To(HaveOccurred())
				Expect(tt).To(BeNil())
				Expect(err.Error()).To(Equal("no token ids in ApproveAllowanceRequest"))
			})
		})

		When("no recipient shares are provided", func() {
			It("returns an error", func() {
				approveRequest = &token.ApproveRequest{
					TokenIds:        []*token.TokenId{{TxId: "1", Index: 0}},
					AllowanceShares: []*token.AllowanceRecipientShare{},
				}

				tt, err := transactor.RequestApprove(approveRequest)
				Expect(err).To(HaveOccurred())
				Expect(tt).To(BeNil())
				Expect(err.Error()).To(Equal("no recipient shares in ApproveAllowanceRequest"))
			})
		})

		When("a quantity in a share <= 0", func() {
			It("returns an error", func() {
				approveRequest = &token.ApproveRequest{
					TokenIds:        []*token.TokenId{{TxId: "1", Index: 0}},
					AllowanceShares: []*token.AllowanceRecipientShare{{Recipient: []byte("Bob"), Quantity: 0}},
				}

				tt, err := transactor.RequestApprove(approveRequest)
				Expect(err).To(HaveOccurred())
				Expect(tt).To(BeNil())
				Expect(err.Error()).To(Equal("the quantity to approve [0] must be greater than 0"))
			})
		})

		When("a recipient is not specified", func() {
			It("returns an error", func() {
				approveRequest = &token.ApproveRequest{
					TokenIds:        []*token.TokenId{{TxId: "1", Index: 0}},
					AllowanceShares: []*token.AllowanceRecipientShare{{Quantity: 10}},
				}

				tt, err := transactor.RequestApprove(approveRequest)
				Expect(err).To(HaveOccurred())
				Expect(tt).To(BeNil())
				Expect(err.Error()).To(Equal("the recipient in approve must be specified"))
			})
		})

		When("inputs are not of the same type", func() {
			input := &token.PlainOutput{
				Owner:    []byte("credential"),
				Type:     "ABC",
				Quantity: 100,
			}
			It("returns an error", func() {
				var err error
				inputBytes, err = proto.Marshal(input)
				Expect(err).NotTo(HaveOccurred())

				fakeLedger.GetStateReturnsOnCall(1, inputBytes, nil)
				approveRequest = &token.ApproveRequest{
					Credential:      []byte("credential"),
					TokenIds:        []*token.TokenId{{TxId: "lalaland", Index: 0}, {TxId: "lalaland", Index: 1}},
					AllowanceShares: allowanceShares,
				}

				tt, err := transactor.RequestApprove(approveRequest)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(Equal("two or more token types specified in input: 'XYZ', 'ABC'"))
				Expect(tt).To(BeNil())
			})
		})

		When("inputs are not sufficient", func() {
			input := &token.PlainOutput{
				Owner:    []byte("credential"),
				Type:     "XYZ",
				Quantity: 100,
			}
			It("returns an error", func() {
				var err error
				inputBytes, err = proto.Marshal(input)
				Expect(err).NotTo(HaveOccurred())

				fakeLedger.GetStateReturnsOnCall(0, inputBytes, nil)
				approveRequest = &token.ApproveRequest{
					Credential:      []byte("credential"),
					TokenIds:        []*token.TokenId{{TxId: "lalaland", Index: 0}},
					AllowanceShares: allowanceShares,
				}

				tt, err := transactor.RequestApprove(approveRequest)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(Equal("insufficient funds: 100 < 300"))
				Expect(tt).To(BeNil())
			})
		})

		When("transactor fails to get inputs from ledger", func() {
			It("returns an error", func() {
				fakeLedger.GetStateReturnsOnCall(0, nil, errors.New("banana"))
				approveRequest = &token.ApproveRequest{
					Credential:      []byte("credential"),
					TokenIds:        []*token.TokenId{{TxId: "lalaland", Index: 0}},
					AllowanceShares: allowanceShares,
				}
				tt, err := transactor.RequestApprove(approveRequest)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(Equal("banana"))
				Expect(tt).To(BeNil())

			})
		})

	})
})

var _ = Describe("Transactor TransferFrom", func() {
	var (
		transactor *plain.Transactor
		shares     []*token.RecipientTransferShare
	)

	BeforeEach(func() {
		shares = []*token.RecipientTransferShare{
			{Recipient: []byte("Alice"), Quantity: 100},
			{Recipient: []byte("Bob"), Quantity: 200},
		}
		transactor = &plain.Transactor{PublicCredential: []byte("Charlie")}
	})

	Describe("converts a transferFrom request into a token transaction", func() {
		var (
			fakeLedger      *mock.LedgerReader
			transferRequest *token.TransferRequest
			inputBytes      []byte
		)

		BeforeEach(func() {
			input := &token.PlainDelegatedOutput{
				Owner:      []byte("Owner"),
				Delegatees: [][]byte{[]byte("Charlie")},
				Type:       "XYZ",
				Quantity:   350,
			}
			var err error
			inputBytes, err = proto.Marshal(input)
			Expect(err).NotTo(HaveOccurred())
			fakeLedger = &mock.LedgerReader{}
			transactor.Ledger = fakeLedger
			fakeLedger.GetStateReturnsOnCall(0, inputBytes, nil)
		})

		It("creates a valid transferFrom request", func() {
			transferRequest = &token.TransferRequest{
				Credential: []byte("Charlie"),
				TokenIds:   []*token.TokenId{{TxId: "pot pourri", Index: 0}},
				Shares:     shares,
			}
			tt, err := transactor.RequestTransferFrom(transferRequest)
			Expect(err).NotTo(HaveOccurred())
			Expect(tt).To(Equal(&token.TokenTransaction{
				Action: &token.TokenTransaction_PlainAction{
					PlainAction: &token.PlainTokenAction{
						Data: &token.PlainTokenAction_PlainTransfer_From{
							PlainTransfer_From: &token.PlainTransferFrom{
								Inputs: []*token.TokenId{
									{TxId: "pot pourri", Index: uint32(0)},
								},
								Outputs: []*token.PlainOutput{
									{Owner: []byte("Alice"), Type: "XYZ", Quantity: 100},
									{Owner: []byte("Bob"), Type: "XYZ", Quantity: 200},
								},
								DelegatedOutput: &token.PlainDelegatedOutput{Owner: []byte("Owner"), Delegatees: [][]byte{[]byte("Charlie")}, Type: "XYZ", Quantity: 50},
							},
						},
					},
				},
			}))
		})

		It("creates a valid TransferFrom request without outputs", func() {
			input := &token.PlainDelegatedOutput{
				Owner:      []byte("Owner"),
				Delegatees: [][]byte{[]byte("Charlie")},
				Type:       "XYZ",
				Quantity:   300,
			}
			var err error
			inputBytes, err = proto.Marshal(input)
			Expect(err).NotTo(HaveOccurred())

			fakeLedger.GetStateReturnsOnCall(0, inputBytes, nil)
			transferRequest = &token.TransferRequest{
				Credential: []byte("Charlie"),
				TokenIds:   []*token.TokenId{{TxId: "pot pourri", Index: 0}},
				Shares:     shares,
			}
			tt, err := transactor.RequestTransferFrom(transferRequest)
			Expect(err).NotTo(HaveOccurred())
			Expect(tt).To(Equal(&token.TokenTransaction{
				Action: &token.TokenTransaction_PlainAction{
					PlainAction: &token.PlainTokenAction{
						Data: &token.PlainTokenAction_PlainTransfer_From{
							PlainTransfer_From: &token.PlainTransferFrom{
								Inputs: []*token.TokenId{
									{TxId: "pot pourri", Index: uint32(0)},
								},
								Outputs: []*token.PlainOutput{
									{Owner: []byte("Alice"), Type: "XYZ", Quantity: 100},
									{Owner: []byte("Bob"), Type: "XYZ", Quantity: 200},
								},
							},
						},
					},
				},
			}))
		})

		It("when inputs are not of the same type", func() {
			input := &token.PlainDelegatedOutput{
				Owner:      []byte("Owner"),
				Delegatees: [][]byte{[]byte("Charlie")},
				Type:       "ABC",
				Quantity:   100,
			}
			var err error
			inputBytes, err = proto.Marshal(input)
			Expect(err).NotTo(HaveOccurred())

			fakeLedger.GetStateReturnsOnCall(1, inputBytes, nil)
			transferRequest = &token.TransferRequest{
				Credential: []byte("Charlie"),
				TokenIds:   []*token.TokenId{{TxId: "pot pourri", Index: 0}, {TxId: "pot pourri", Index: 1}},
				Shares:     shares,
			}

			tt, err := transactor.RequestTransferFrom(transferRequest)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal("two or more token types specified in input: 'XYZ', 'ABC'"))
			Expect(tt).To(BeNil())
		})

		It("when inputs do not belong to the same owner", func() {
			input := &token.PlainDelegatedOutput{
				Owner:      []byte("Owner*"),
				Delegatees: [][]byte{[]byte("Charlie")},
				Type:       "XYZ",
				Quantity:   100,
			}
			var err error
			inputBytes, err = proto.Marshal(input)
			Expect(err).NotTo(HaveOccurred())

			fakeLedger.GetStateReturnsOnCall(1, inputBytes, nil)
			transferRequest = &token.TransferRequest{
				Credential: []byte("Charlie"),
				TokenIds:   []*token.TokenId{{TxId: "pot pourri", Index: 0}, {TxId: "pot pourri", Index: 1}},
				Shares:     shares,
			}

			tt, err := transactor.RequestTransferFrom(transferRequest)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal("two or more token owners specified in input: 'Owner', 'Owner*'"))
			Expect(tt).To(BeNil())
		})

		It("when inputs are not sufficient", func() {
			input := &token.PlainDelegatedOutput{
				Owner:      []byte("Owner"),
				Delegatees: [][]byte{[]byte("Charlie")},
				Type:       "XYZ",
				Quantity:   100,
			}
			var err error
			inputBytes, err = proto.Marshal(input)
			Expect(err).NotTo(HaveOccurred())

			fakeLedger.GetStateReturnsOnCall(0, inputBytes, nil)
			transferRequest = &token.TransferRequest{
				Credential: []byte("Charlie"),
				TokenIds:   []*token.TokenId{{TxId: "pot pourri", Index: 0}},
				Shares:     shares,
			}
			tt, err := transactor.RequestTransferFrom(transferRequest)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal("insufficient funds: 100 < 300"))
			Expect(tt).To(BeNil())
		})

		It("when TransferFrom requestor does not own inputs", func() {
			transactor.PublicCredential = []byte("Dave")
			transferRequest = &token.TransferRequest{
				Credential: []byte("Dave"),
				TokenIds:   []*token.TokenId{{TxId: "pot pourri", Index: 0}},
				Shares:     shares,
			}
			tt, err := transactor.RequestTransferFrom(transferRequest)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal("requestor is not allowed to transfer inputs"))
			Expect(tt).To(BeNil())
		})

		It("when transactor fails to get inputs from ledger", func() {
			fakeLedger.GetStateReturnsOnCall(0, nil, errors.New("banana"))
			transferRequest = &token.TransferRequest{
				Credential: []byte("Charlie"),
				TokenIds:   []*token.TokenId{{TxId: "pot pourri", Index: 0}},
				Shares:     shares,
			}
			tt, err := transactor.RequestTransferFrom(transferRequest)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal("banana"))
			Expect(tt).To(BeNil())
		})

	})

})

func generateKey(txID, index, namespace string) string {
	return "\x00" + namespace + "\x00" + txID + "\x00" + index + "\x00"
}
