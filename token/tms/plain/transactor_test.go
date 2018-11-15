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
	"github.com/hyperledger/fabric/core/ledger/customtx"
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
				Owner:    []byte("owner-1"),
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
			Expect(err).To(Equal(&customtx.InvalidTxError{Msg: fmt.Sprintf("input '%s' does not exist", inputID)}))
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
				Owner:    []byte("owner-1"),
				Type:     "TOK1",
				Quantity: 99,
			}
			input2 := &token.PlainOutput{
				Owner:    []byte("owner-1"),
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
			Expect(err).To(Equal(&customtx.InvalidTxError{Msg: "two or more token types specified in input: 'TOK1', 'TOK2'"}))
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
			Expect(err).To(Equal(&customtx.InvalidTxError{Msg: "not enough components in output ID composite key; expected 2, received '[george 0 1]'"}))
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
			Expect(err).To(Equal(&customtx.InvalidTxError{Msg: "error splitting input composite key: 'invalid composite key - no components found'"}))
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
			Expect(err).To(Equal(&customtx.InvalidTxError{Msg: "namespace not 'tokenOutput': 'badNamespace'"}))
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
			Expect(err).To(Equal(&customtx.InvalidTxError{Msg: "error parsing output index 'bear': 'strconv.Atoi: parsing \"bear\": invalid syntax'"}))
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
				Owner:    []byte("owner-1"),
				Type:     "TOK1",
				Quantity: inputQuantity,
			}
			var err error
			inputBytes, err = proto.Marshal(input)
			Expect(err).ToNot(HaveOccurred())
			fakeLedger = &mock.LedgerWriter{}
			fakeLedger.SetStateReturns(nil)
			//fakeLedger.GetStateReturnsOnCall(0, inputBytes, nil)
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
})
