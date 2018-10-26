/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package plain_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/ledger"
	"github.com/hyperledger/fabric/protos/ledger/queryresult"
	"github.com/hyperledger/fabric/protos/token"
	"github.com/hyperledger/fabric/token/ledger/mock"
	"github.com/hyperledger/fabric/token/tms/plain"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/assert"
)

type nextReturns struct {
	result interface{}
	err    error
}

type getStateRangeScanIteratorReturns struct {
	iterator ledger.ResultsIterator
	err      error
}

type getStateReturns struct {
	value []byte
	err   error
}

func TestTransactor_ListTokens(t *testing.T) {
	t.Parallel()

	var err error

	ledgerReader := &mock.LedgerReader{}
	iterator := &mock.ResultsIterator{}

	transactor := &plain.Transactor{PublicCredential: []byte("Alice"), Ledger: ledgerReader}

	outputs := make([][]byte, 3)
	keys := make([]string, 3)
	results := make([]*queryresult.KV, 4)

	outputs[0], err = proto.Marshal(&token.PlainOutput{Owner: []byte("Alice"), Type: "TOK1", Quantity: 100})
	assert.NoError(t, err)
	outputs[1], err = proto.Marshal(&token.PlainOutput{Owner: []byte("Bob"), Type: "TOK2", Quantity: 200})
	assert.NoError(t, err)
	outputs[2], err = proto.Marshal(&token.PlainOutput{Owner: []byte("Alice"), Type: "TOK3", Quantity: 300})
	assert.NoError(t, err)

	keys[0], err = plain.GenerateKeyForTest("1", 0)
	assert.NoError(t, err)
	keys[1], err = plain.GenerateKeyForTest("1", 1)
	assert.NoError(t, err)
	keys[2], err = plain.GenerateKeyForTest("2", 0)
	assert.NoError(t, err)

	results[0] = &queryresult.KV{Key: keys[0], Value: outputs[0]}
	results[1] = &queryresult.KV{Key: keys[1], Value: outputs[1]}
	results[2] = &queryresult.KV{Key: keys[2], Value: outputs[2]}
	results[3] = &queryresult.KV{Key: "123", Value: []byte("not an output")}

	for _, testCase := range []struct {
		name                             string
		getStateRangeScanIteratorReturns getStateRangeScanIteratorReturns
		nextReturns                      []nextReturns
		getStateReturns                  []getStateReturns
		expectedErr                      string
	}{
		{
			name:                             "getStateRangeScanIterator() fails",
			getStateRangeScanIteratorReturns: getStateRangeScanIteratorReturns{nil, errors.New("wild potato")},
			expectedErr:                      "wild potato",
		},
		{
			name:                             "next() fails",
			getStateRangeScanIteratorReturns: getStateRangeScanIteratorReturns{iterator, nil},
			nextReturns:                      []nextReturns{{queryresult.KV{}, errors.New("wild banana")}},
			expectedErr:                      "wild banana",
		},
		{
			name:                             "getStateReturns() fails",
			getStateRangeScanIteratorReturns: getStateRangeScanIteratorReturns{iterator, nil},
			nextReturns:                      []nextReturns{{results[0], nil}},
			getStateReturns:                  []getStateReturns{{nil, errors.New("wild apple")}},
			expectedErr:                      "wild apple",
		},
		{
			name:                             "Success",
			getStateRangeScanIteratorReturns: getStateRangeScanIteratorReturns{iterator, nil},
			getStateReturns: []getStateReturns{
				{nil, nil},
				{[]byte("value"), nil},
			},
			nextReturns: []nextReturns{
				{results[0], nil},
				{results[1], nil},
				{results[2], nil},
				{results[3], nil},
				{nil, nil},
			},
		},
	} {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {

			ledgerReader.GetStateRangeScanIteratorReturns(testCase.getStateRangeScanIteratorReturns.iterator, testCase.getStateRangeScanIteratorReturns.err)
			if testCase.getStateRangeScanIteratorReturns.iterator != nil {
				if len(testCase.nextReturns) == 1 {
					iterator.NextReturns(testCase.nextReturns[0].result, testCase.nextReturns[0].err)
					if testCase.nextReturns[0].err == nil {
						ledgerReader.GetStateReturns(testCase.getStateReturns[0].value, testCase.getStateReturns[0].err)
					}
				} else {
					iterator.NextReturnsOnCall(2, testCase.nextReturns[0].result, testCase.nextReturns[0].err)
					iterator.NextReturnsOnCall(3, testCase.nextReturns[1].result, testCase.nextReturns[1].err)
					iterator.NextReturnsOnCall(4, testCase.nextReturns[2].result, testCase.nextReturns[2].err)
					iterator.NextReturnsOnCall(5, testCase.nextReturns[3].result, testCase.nextReturns[3].err)
					iterator.NextReturnsOnCall(6, testCase.nextReturns[4].result, testCase.nextReturns[4].err)

					ledgerReader.GetStateReturnsOnCall(1, testCase.getStateReturns[0].value, testCase.getStateReturns[0].err)
					ledgerReader.GetStateReturnsOnCall(2, testCase.getStateReturns[1].value, testCase.getStateReturns[1].err)
				}

			}
			expectedTokens := &token.UnspentTokens{Tokens: []*token.TokenOutput{{Type: "TOK1", Quantity: 100, Id: []byte(keys[0])}}}
			tokens, err := transactor.ListTokens()

			if testCase.expectedErr == "" {
				assert.NoError(t, err)
				assert.NotNil(t, tokens)
				assert.Equal(t, expectedTokens, tokens)
			} else {
				assert.Error(t, err)
				assert.Nil(t, tokens)
				assert.EqualError(t, err, testCase.expectedErr)
			}
			if testCase.getStateRangeScanIteratorReturns.err != nil {
				assert.Equal(t, 1, ledgerReader.GetStateRangeScanIteratorCallCount())
				assert.Equal(t, 0, ledgerReader.GetStateCallCount())
				assert.Equal(t, 0, iterator.NextCallCount())
			} else {
				if testCase.nextReturns[0].err != nil {
					assert.Equal(t, 2, ledgerReader.GetStateRangeScanIteratorCallCount())
					assert.Equal(t, 0, ledgerReader.GetStateCallCount())
					assert.Equal(t, 1, iterator.NextCallCount())
				} else {
					if testCase.getStateReturns[0].err != nil {
						assert.Equal(t, 3, ledgerReader.GetStateRangeScanIteratorCallCount())
						assert.Equal(t, 1, ledgerReader.GetStateCallCount())
						assert.Equal(t, 2, iterator.NextCallCount())
					} else {
						assert.Equal(t, 4, ledgerReader.GetStateRangeScanIteratorCallCount())
						assert.Equal(t, 3, ledgerReader.GetStateCallCount())
						assert.Equal(t, 7, iterator.NextCallCount())
					}

				}
			}

		})

	}
}

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
			TokenIds:   [][]byte{},
			Shares:     recipientTransferShares,
		}

		tt, err := transactor.RequestTransfer(transferRequest)
		Expect(err).NotTo(HaveOccurred())
		Expect(tt).To(Equal(&token.TokenTransaction{
			Action: &token.TokenTransaction_PlainAction{
				PlainAction: &token.PlainTokenAction{
					Data: &token.PlainTokenAction_PlainTransfer{
						PlainTransfer: &token.PlainTransfer{
							Inputs: nil,
							Outputs: []*token.PlainOutput{
								{Owner: []byte("R1"), Type: "", Quantity: 1001},
								{Owner: []byte("R2"), Type: "", Quantity: 1002},
								{Owner: []byte("R3"), Type: "", Quantity: 1003},
							},
						},
					},
				},
			},
		}))
	})

	Describe("when no inputs or tokens to issue are provided", func() {
		It("creates a token transaction with no outputs", func() {
			transferRequest := &token.TransferRequest{
				Credential: []byte("credential"),
				TokenIds:   [][]byte{},
				Shares:     []*token.RecipientTransferShare{},
			}

			tt, err := transactor.RequestTransfer(transferRequest)
			Expect(err).NotTo(HaveOccurred())

			Expect(tt).To(Equal(&token.TokenTransaction{
				Action: &token.TokenTransaction_PlainAction{
					PlainAction: &token.PlainTokenAction{
						Data: &token.PlainTokenAction_PlainTransfer{PlainTransfer: &token.PlainTransfer{}},
					},
				},
			}))
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
				TokenIds:   [][]byte{[]byte(string("\x00") + "tokenOutput" + string("\x00") + "george" + string("\x00") + "0" + string("\x00"))},
				Shares:     recipientTransferShares,
			}
			tt, err := transactor.RequestTransfer(transferRequest)
			Expect(err).NotTo(HaveOccurred())
			Expect(tt).To(Equal(&token.TokenTransaction{
				Action: &token.TokenTransaction_PlainAction{
					PlainAction: &token.PlainTokenAction{
						Data: &token.PlainTokenAction_PlainTransfer{
							PlainTransfer: &token.PlainTransfer{
								Inputs: []*token.InputId{
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
			inputID         string
		)

		BeforeEach(func() {
			fakeLedger = &mock.LedgerWriter{}
			fakeLedger.SetStateReturns(nil)
			fakeLedger.GetStateReturnsOnCall(0, nil, nil)
			transactor.Ledger = fakeLedger
			inputID = "\x00" + strings.Join([]string{"tokenOutput", "george", "0"}, "\x00") + "\x00"
		})

		It("returns an invalid transaction error", func() {
			transferRequest = &token.TransferRequest{
				Credential: []byte("credential"),
				TokenIds:   [][]byte{[]byte(inputID)},
				Shares:     recipientTransferShares,
			}
			_, err := transactor.RequestTransfer(transferRequest)
			Expect(err.Error()).To(Equal(fmt.Sprintf("input '%s' does not exist", inputID)))
		})
	})

	Describe("when a transfer request with two different input token types is provided", func() {
		var (
			fakeLedger      *mock.LedgerWriter
			transferRequest *token.TransferRequest
			inputBytes1     []byte
			inputBytes2     []byte
			inputID1        string
			inputID2        string
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
			inputID1 = "\x00" + strings.Join([]string{"tokenOutput", "george", "0"}, "\x00") + "\x00"
			inputID2 = "\x00" + strings.Join([]string{"tokenOutput", "george", "1"}, "\x00") + "\x00"
		})

		It("returns an invalid transaction error", func() {
			transferRequest = &token.TransferRequest{
				Credential: []byte("credential"),
				TokenIds:   [][]byte{[]byte(inputID1), []byte(inputID2)},
				Shares:     recipientTransferShares,
			}
			_, err := transactor.RequestTransfer(transferRequest)
			Expect(err.Error()).To(Equal("two or more token types specified in input: 'TOK1', 'TOK2'"))
		})
	})

	Describe("when a transfer request where the input is a composite key with an invalid number of components is provided", func() {
		var (
			fakeLedger      *mock.LedgerWriter
			transferRequest *token.TransferRequest
			inputID         string
		)

		BeforeEach(func() {
			fakeLedger = &mock.LedgerWriter{}
			transactor.Ledger = fakeLedger
			inputID = "\x00" + strings.Join([]string{"tokenOutput", "george", "0", "1"}, "\x00") + "\x00"
		})

		It("returns an invalid transaction error", func() {
			transferRequest = &token.TransferRequest{
				Credential: []byte("credential"),
				TokenIds:   [][]byte{[]byte(inputID)},
				Shares:     recipientTransferShares,
			}
			_, err := transactor.RequestTransfer(transferRequest)
			Expect(err.Error()).To(Equal("not enough components in output ID composite key; expected 2, received '[george 0 1]'"))
		})
	})

	Describe("when a transfer request where the input is not a composite key is provided", func() {
		var (
			fakeLedger      *mock.LedgerWriter
			transferRequest *token.TransferRequest
			inputID         string
		)

		BeforeEach(func() {
			fakeLedger = &mock.LedgerWriter{}
			transactor.Ledger = fakeLedger
			inputID = "not a composite key"
		})

		It("returns an invalid transaction error", func() {
			transferRequest = &token.TransferRequest{
				Credential: []byte("credential"),
				TokenIds:   [][]byte{[]byte(inputID)},
				Shares:     recipientTransferShares,
			}
			_, err := transactor.RequestTransfer(transferRequest)
			Expect(err.Error()).To(Equal("error splitting input composite key: 'invalid composite key - not enough components found in key 'not a composite key''"))
		})
	})

	Describe("when a transfer request where the input is a composite key with an invalid namespace is provided", func() {
		var (
			fakeLedger      *mock.LedgerWriter
			transferRequest *token.TransferRequest
			inputID         string
		)

		BeforeEach(func() {
			fakeLedger = &mock.LedgerWriter{}
			transactor.Ledger = fakeLedger
			inputID = "\x00" + strings.Join([]string{"badNamespace", "george", "0"}, "\x00") + "\x00"
		})

		It("returns an invalid transaction error", func() {
			transferRequest = &token.TransferRequest{
				Credential: []byte("credential"),
				TokenIds:   [][]byte{[]byte(inputID)},
				Shares:     recipientTransferShares,
			}
			_, err := transactor.RequestTransfer(transferRequest)
			Expect(err.Error()).To(Equal("namespace not 'tokenOutput': 'badNamespace'"))
		})
	})

	Describe("when a transfer request where the input is a composite key with an output index that is not an integer is provided", func() {
		var (
			fakeLedger      *mock.LedgerWriter
			transferRequest *token.TransferRequest
			inputID         string
		)

		BeforeEach(func() {
			fakeLedger = &mock.LedgerWriter{}
			transactor.Ledger = fakeLedger
			inputID = "\x00" + strings.Join([]string{"tokenOutput", "george", "bear"}, "\x00") + "\x00"
		})

		It("returns an invalid transaction error", func() {
			transferRequest = &token.TransferRequest{
				Credential: []byte("credential"),
				TokenIds:   [][]byte{[]byte(inputID)},
				Shares:     recipientTransferShares,
			}
			_, err := transactor.RequestTransfer(transferRequest)
			Expect(err.Error()).To(Equal("error parsing output index 'bear': 'strconv.Atoi: parsing \"bear\": invalid syntax'"))
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
				TokenIds:         [][]byte{[]byte(string("\x00") + "tokenOutput" + string("\x00") + "robert" + string("\x00") + "0" + string("\x00"))},
				QuantityToRedeem: redeemQuantity,
			}
			tt, err := transactor.RequestRedeem(redeemRequest)
			Expect(err).NotTo(HaveOccurred())
			Expect(tt).To(Equal(&token.TokenTransaction{
				Action: &token.TokenTransaction_PlainAction{
					PlainAction: &token.PlainTokenAction{
						Data: &token.PlainTokenAction_PlainRedeem{
							PlainRedeem: &token.PlainTransfer{
								Inputs: []*token.InputId{
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
				TokenIds:         [][]byte{[]byte(string("\x00") + "tokenOutput" + string("\x00") + "robert" + string("\x00") + "0" + string("\x00"))},
				QuantityToRedeem: redeemQuantity,
			}
			tt, err := transactor.RequestRedeem(redeemRequest)
			Expect(err).NotTo(HaveOccurred())
			Expect(tt).To(Equal(&token.TokenTransaction{
				Action: &token.TokenTransaction_PlainAction{
					PlainAction: &token.PlainTokenAction{
						Data: &token.PlainTokenAction_PlainRedeem{
							PlainRedeem: &token.PlainTransfer{
								Inputs: []*token.InputId{
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
					TokenIds:         [][]byte{[]byte(string("\x00") + "tokenOutput" + string("\x00") + "robert" + string("\x00") + "0" + string("\x00"))},
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
				TokenIds:   [][]byte{[]byte(string("\x00") + "tokenOutput" + string("\x00") + "robert" + string("\x00") + "0" + string("\x00"))},
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
								Inputs: []*token.InputId{
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
								Inputs: []*token.InputId{
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
				Credential:      []byte("credential"),
				TokenIds:        [][]byte{[]byte(string("\x00") + "tokenOutput" + string("\x00") + "lalaland" + string("\x00") + "0" + string("\x00"))},
				AllowanceShares: allowanceShares,
			}
			tt, err := transactor.RequestApprove(approveRequest)
			Expect(err).NotTo(HaveOccurred())
			Expect(tt).To(Equal(&token.TokenTransaction{
				Action: &token.TokenTransaction_PlainAction{
					PlainAction: &token.PlainTokenAction{
						Data: &token.PlainTokenAction_PlainApprove{
							PlainApprove: &token.PlainApprove{
								Inputs: []*token.InputId{
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
				TokenIds:        [][]byte{[]byte(string("\x00") + "tokenOutput" + string("\x00") + "lalaland" + string("\x00") + "0" + string("\x00"))},
				AllowanceShares: allowanceShares,
			}
			tt, err := transactor.RequestApprove(approveRequest)
			Expect(err).NotTo(HaveOccurred())
			Expect(tt).To(Equal(&token.TokenTransaction{
				Action: &token.TokenTransaction_PlainAction{
					PlainAction: &token.PlainTokenAction{
						Data: &token.PlainTokenAction_PlainApprove{
							PlainApprove: &token.PlainApprove{
								Inputs: []*token.InputId{
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
					TokenIds:        [][]byte{},
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
				key, err := plain.GenerateKeyForTest("1", 0)
				Expect(err).NotTo(HaveOccurred())
				approveRequest = &token.ApproveRequest{
					TokenIds:        [][]byte{[]byte(key)},
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
				key, err := plain.GenerateKeyForTest("1", 0)
				Expect(err).NotTo(HaveOccurred())
				approveRequest = &token.ApproveRequest{
					TokenIds:        [][]byte{[]byte(key)},
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
				key, err := plain.GenerateKeyForTest("1", 0)
				Expect(err).NotTo(HaveOccurred())
				approveRequest = &token.ApproveRequest{
					TokenIds:        [][]byte{[]byte(key)},
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
					Credential: []byte("credential"),
					TokenIds: [][]byte{
						[]byte(string("\x00") + "tokenOutput" + string("\x00") + "lalaland" + string("\x00") + "0" + string("\x00")),
						[]byte(string("\x00") + "tokenOutput" + string("\x00") + "lalaland" + string("\x00") + "1" + string("\x00"))},
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
					TokenIds:        [][]byte{[]byte(string("\x00") + "tokenOutput" + string("\x00") + "lalaland" + string("\x00") + "0" + string("\x00"))},
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
					TokenIds:        [][]byte{[]byte(string("\x00") + "tokenOutput" + string("\x00") + "lalaland" + string("\x00") + "0" + string("\x00"))},
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
				TokenIds:   [][]byte{[]byte(string("\x00") + "tokenDelegatedOutput" + string("\x00") + "pot pourri" + string("\x00") + "0" + string("\x00"))},
				Shares:     shares,
			}
			tt, err := transactor.RequestTransferFrom(transferRequest)
			Expect(err).NotTo(HaveOccurred())
			Expect(tt).To(Equal(&token.TokenTransaction{
				Action: &token.TokenTransaction_PlainAction{
					PlainAction: &token.PlainTokenAction{
						Data: &token.PlainTokenAction_PlainTransfer_From{
							PlainTransfer_From: &token.PlainTransferFrom{
								Inputs: []*token.InputId{
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
				TokenIds:   [][]byte{[]byte(string("\x00") + "tokenDelegatedOutput" + string("\x00") + "pot pourri" + string("\x00") + "0" + string("\x00"))},
				Shares:     shares,
			}
			tt, err := transactor.RequestTransferFrom(transferRequest)
			Expect(err).NotTo(HaveOccurred())
			Expect(tt).To(Equal(&token.TokenTransaction{
				Action: &token.TokenTransaction_PlainAction{
					PlainAction: &token.PlainTokenAction{
						Data: &token.PlainTokenAction_PlainTransfer_From{
							PlainTransfer_From: &token.PlainTransferFrom{
								Inputs: []*token.InputId{
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
				TokenIds: [][]byte{
					[]byte(string("\x00") + "tokenDelegatedOutput" + string("\x00") + "pot pourri" + string("\x00") + "0" + string("\x00")),
					[]byte(string("\x00") + "tokenDelegatedOutput" + string("\x00") + "pot pourri" + string("\x00") + "1" + string("\x00"))},
				Shares: shares,
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
				TokenIds: [][]byte{
					[]byte(string("\x00") + "tokenDelegatedOutput" + string("\x00") + "pot pourri" + string("\x00") + "0" + string("\x00")),
					[]byte(string("\x00") + "tokenDelegatedOutput" + string("\x00") + "pot pourri" + string("\x00") + "1" + string("\x00"))},
				Shares: shares,
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
				TokenIds:   [][]byte{[]byte(string("\x00") + "tokenDelegatedOutput" + string("\x00") + "pot pourri" + string("\x00") + "0" + string("\x00"))},
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
				TokenIds:   [][]byte{[]byte(string("\x00") + "tokenDelegatedOutput" + string("\x00") + "pot pourri" + string("\x00") + "0" + string("\x00"))},
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
				TokenIds:   [][]byte{[]byte(string("\x00") + "tokenDelegatedOutput" + string("\x00") + "pot pourri" + string("\x00") + "0" + string("\x00"))},
				Shares:     shares,
			}
			tt, err := transactor.RequestTransferFrom(transferRequest)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal("banana"))
			Expect(tt).To(BeNil())
		})

	})

})
