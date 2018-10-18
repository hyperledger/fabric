/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package plain_test

import (
	"errors"
	"fmt"
	"strings"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/core/ledger/customtx"
	"github.com/hyperledger/fabric/protos/token"
	"github.com/hyperledger/fabric/token/tms/plain"
	"github.com/hyperledger/fabric/token/transaction/mock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Verifier", func() {
	var (
		fakeCreatorInfo     *mock.CreatorInfo
		fakePolicyValidator *mock.PolicyValidator
		fakeLedger          *mock.LedgerWriter
		memoryLedger        *plain.MemoryLedger

		transaction *token.TokenTransaction
		txID        string

		verifier *plain.Verifier
	)

	BeforeEach(func() {
		fakeCreatorInfo = &mock.CreatorInfo{}
		fakePolicyValidator = &mock.PolicyValidator{}
		fakeLedger = &mock.LedgerWriter{}
		fakeLedger.SetStateReturns(nil)

		txID = "0"
		transaction = &token.TokenTransaction{
			Action: &token.TokenTransaction_PlainAction{
				PlainAction: &token.PlainTokenAction{
					Data: &token.PlainTokenAction_PlainImport{
						PlainImport: &token.PlainImport{
							Outputs: []*token.PlainOutput{
								{Owner: []byte("owner-1"), Type: "TOK1", Quantity: 111},
								{Owner: []byte("owner-2"), Type: "TOK2", Quantity: 222},
							},
						},
					},
				},
			},
		}

		verifier = &plain.Verifier{
			PolicyValidator: fakePolicyValidator,
		}
	})

	Describe("ProcessTx PlainImport", func() {
		It("evaluates policy for each output", func() {
			err := verifier.ProcessTx(txID, fakeCreatorInfo, transaction, fakeLedger)
			Expect(err).NotTo(HaveOccurred())

			Expect(fakePolicyValidator.IsIssuerCallCount()).To(Equal(2))
			creator, tt := fakePolicyValidator.IsIssuerArgsForCall(0)
			Expect(creator).To(Equal(fakeCreatorInfo))
			Expect(tt).To(Equal("TOK1"))
			creator, tt = fakePolicyValidator.IsIssuerArgsForCall(1)
			Expect(creator).To(Equal(fakeCreatorInfo))
			Expect(tt).To(Equal("TOK2"))
		})

		It("checks the fake ledger", func() {
			err := verifier.ProcessTx(txID, fakeCreatorInfo, transaction, fakeLedger)
			Expect(err).NotTo(HaveOccurred())

			Expect(fakeLedger.SetStateCallCount()).To(Equal(3))

			outputBytes, err := proto.Marshal(&token.PlainOutput{Owner: []byte("owner-1"), Type: "TOK1", Quantity: 111})
			Expect(err).NotTo(HaveOccurred())
			ns, k, td := fakeLedger.SetStateArgsForCall(0)
			Expect(ns).To(Equal("tms"))
			expectedOutput := strings.Join([]string{"", "tokenOutput", "0", "0", ""}, "\x00")
			Expect(k).To(Equal(expectedOutput))
			Expect(td).To(Equal(outputBytes))

			outputBytes, err = proto.Marshal(&token.PlainOutput{Owner: []byte("owner-2"), Type: "TOK2", Quantity: 222})
			Expect(err).NotTo(HaveOccurred())
			ns, k, td = fakeLedger.SetStateArgsForCall(1)
			Expect(ns).To(Equal("tms"))
			expectedOutput = strings.Join([]string{"", "tokenOutput", "0", "1", ""}, "\x00")
			Expect(k).To(Equal(expectedOutput))
			Expect(td).To(Equal(outputBytes))

			ttxBytes, err := proto.Marshal(transaction)
			Expect(err).NotTo(HaveOccurred())
			ns, k, td = fakeLedger.SetStateArgsForCall(2)
			Expect(ns).To(Equal("tms"))
			expectedOutput = strings.Join([]string{"", "tokenTx", "0", ""}, "\x00")
			Expect(k).To(Equal(expectedOutput))
			Expect(td).To(Equal(ttxBytes))
		})

		Context("when policy validation fails", func() {
			BeforeEach(func() {
				fakePolicyValidator.IsIssuerReturns(errors.New("no-way-man"))
			})

			It("returns an error and does not write to the ledger", func() {
				err := verifier.ProcessTx(txID, fakeCreatorInfo, transaction, fakeLedger)
				Expect(err).To(HaveOccurred())
				Expect(err).To(Equal(&customtx.InvalidTxError{Msg: "import policy check failed: no-way-man"}))
				Expect(fakeLedger.SetStateCallCount()).To(Equal(0))
			})
		})

		Context("when the ledger write check fails", func() {
			BeforeEach(func() {
				fakeLedger.SetStateReturns(errors.New("no-can-do"))
			})

			It("returns an error", func() {
				err := verifier.ProcessTx(txID, fakeCreatorInfo, transaction, fakeLedger)
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError("no-can-do"))

				Expect(fakeLedger.SetStateCallCount()).To(Equal(1))
			})
		})

		Context("when transaction does not contain any outputs", func() {
			BeforeEach(func() {
				transaction = &token.TokenTransaction{
					Action: &token.TokenTransaction_PlainAction{
						PlainAction: &token.PlainTokenAction{
							Data: &token.PlainTokenAction_PlainImport{
								PlainImport: &token.PlainImport{
									Outputs: []*token.PlainOutput{},
								},
							},
						},
					},
				}
			})
			It("returns an error", func() {
				err := verifier.ProcessTx(txID, fakeCreatorInfo, transaction, fakeLedger)
				Expect(err).To(HaveOccurred())
				Expect(err).To(Equal(&customtx.InvalidTxError{Msg: "no outputs in transaction: 0"}))
			})
		})

		Context("when the output of a transaction has quantity of 0", func() {
			BeforeEach(func() {
				transaction = &token.TokenTransaction{
					Action: &token.TokenTransaction_PlainAction{
						PlainAction: &token.PlainTokenAction{
							Data: &token.PlainTokenAction_PlainImport{
								PlainImport: &token.PlainImport{
									Outputs: []*token.PlainOutput{
										{Owner: []byte("owner-1"), Type: "TOK1", Quantity: 0},
									},
								},
							},
						},
					},
				}
			})
			It("returns an error", func() {
				err := verifier.ProcessTx(txID, fakeCreatorInfo, transaction, fakeLedger)
				Expect(err).To(HaveOccurred())
				Expect(err).To(Equal(&customtx.InvalidTxError{Msg: "output 0 quantity is 0 in transaction: 0"}))
			})
		})

		Context("when an output already exists", func() {
			BeforeEach(func() {
				memoryLedger = plain.NewMemoryLedger()
				err := verifier.ProcessTx(txID, fakeCreatorInfo, transaction, memoryLedger)
				Expect(err).NotTo(HaveOccurred())
			})
			It("returns an error", func() {
				err := verifier.ProcessTx(txID, fakeCreatorInfo, transaction, memoryLedger)
				Expect(err).To(HaveOccurred())
				existingOutputId := strings.Join([]string{"", "tokenOutput", "0", "0", ""}, "\x00")
				Expect(err).To(Equal(&customtx.InvalidTxError{Msg: fmt.Sprintf("output already exists: %s", existingOutputId)}))
			})
		})

	})

	Describe("Output GetState", func() {
		BeforeEach(func() {
			memoryLedger = plain.NewMemoryLedger()
			err := verifier.ProcessTx(txID, fakeCreatorInfo, transaction, memoryLedger)
			Expect(err).NotTo(HaveOccurred())
		})

		It("retrieves the PlainOutput associated with the entry ID", func() {
			po, err := memoryLedger.GetState("tms", strings.Join([]string{"", "tokenOutput", "0", "0", ""}, "\x00"))
			Expect(err).NotTo(HaveOccurred())

			output := &token.PlainOutput{}
			err = proto.Unmarshal(po, output)
			Expect(err).NotTo(HaveOccurred())

			Expect(output).To(Equal(&token.PlainOutput{
				Owner:    []byte("owner-1"),
				Type:     "TOK1",
				Quantity: 111,
			}))

			po, err = memoryLedger.GetState("tms", strings.Join([]string{"", "tokenOutput", "0", "1", ""}, "\x00"))
			Expect(err).NotTo(HaveOccurred())

			err = proto.Unmarshal(po, output)
			Expect(err).NotTo(HaveOccurred())

			Expect(output).To(Equal(&token.PlainOutput{
				Owner:    []byte("owner-2"),
				Type:     "TOK2",
				Quantity: 222,
			}))
		})

		Context("when the output does not exist", func() {
			It("returns a nil and no error", func() {
				val, err := memoryLedger.GetState("tms", strings.Join([]string{"", "tokenOutput", "george", "0", ""}, "\x00"))
				Expect(err).NotTo(HaveOccurred())
				Expect(val).To(BeNil())
			})
		})
	})

	Describe("ProcessTx", func() {
		Context("when a plain action is not provided", func() {
			BeforeEach(func() {
				txID = "255"
				transaction = &token.TokenTransaction{}
			})

			It("returns an error", func() {
				err := verifier.ProcessTx(txID, fakeCreatorInfo, transaction, fakeLedger)
				Expect(err).To(MatchError("check process failed for transaction '255': missing token action"))
			})
		})

		Context("when an unknown plain token action is provided", func() {
			BeforeEach(func() {
				txID = "254"
				transaction = &token.TokenTransaction{
					Action: &token.TokenTransaction_PlainAction{
						PlainAction: &token.PlainTokenAction{},
					},
				}
			})

			It("returns an error", func() {
				err := verifier.ProcessTx(txID, fakeCreatorInfo, transaction, fakeLedger)
				Expect(err).To(Equal(&customtx.InvalidTxError{Msg: "unknown plain token action: <nil>"}))
			})
		})

		Context("when a transaction has invalid characters in key", func() {
			BeforeEach(func() {
				txID = string(0)
			})

			It("fails when creating the ledger key for the output", func() {
				By("returning an error")
				err := verifier.ProcessTx(txID, fakeCreatorInfo, transaction, fakeLedger)
				Expect(err).To(Equal(&customtx.InvalidTxError{Msg: "error creating output ID: input contain unicode U+0000 starting at position [0]. U+0000 and U+10FFFF are not allowed in the input attribute of a composite key"}))
			})
		})

		Context("when a transaction has invalid characters in key", func() {
			BeforeEach(func() {
				txID = string(0)
			})

			It("fails when creating the ledger key for the first output", func() {
				By("returning an error")
				err := verifier.ProcessTx(txID, fakeCreatorInfo, transaction, fakeLedger)
				Expect(err).To(Equal(&customtx.InvalidTxError{Msg: "error creating output ID: input contain unicode U+0000 starting at position [0]. U+0000 and U+10FFFF are not allowed in the input attribute of a composite key"}))
			})
		})

		Context("when a transaction key is an invalid utf8 string", func() {
			BeforeEach(func() {
				txID = string([]byte{0xE0, 0x80, 0x80})
			})

			It("fails when creating the ledger key for the output", func() {
				By("returning an error")
				err := verifier.ProcessTx(txID, fakeCreatorInfo, transaction, fakeLedger)
				Expect(err).To(Equal(&customtx.InvalidTxError{Msg: "error creating output ID: not a valid utf8 string: [e08080]"}))
			})
		})

		Context("when the ledger read of an output fails", func() {
			BeforeEach(func() {
				fakeLedger.GetStateReturnsOnCall(0, nil, errors.New("error reading output"))
			})

			It("returns an error", func() {
				err := verifier.ProcessTx(txID, fakeCreatorInfo, transaction, fakeLedger)
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError("error reading output"))

				Expect(fakeLedger.GetStateCallCount()).To(Equal(1))
				Expect(fakeLedger.SetStateCallCount()).To(Equal(0))
				ns, k := fakeLedger.GetStateArgsForCall(0)
				expectedOutput := strings.Join([]string{"", "tokenOutput", "0", "0", ""}, "\x00")
				Expect(k).To(Equal(expectedOutput))
				Expect(ns).To(Equal("tms"))
			})
		})

		Context("when the ledger read of a transaction fails", func() {
			BeforeEach(func() {
				fakeLedger.GetStateReturnsOnCall(0, nil, nil)
				fakeLedger.GetStateReturnsOnCall(1, nil, nil)
				fakeLedger.GetStateReturnsOnCall(2, nil, errors.New("error reading transaction"))
			})

			It("returns an error", func() {
				err := verifier.ProcessTx(txID, fakeCreatorInfo, transaction, fakeLedger)
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError("error reading transaction"))

				Expect(fakeLedger.GetStateCallCount()).To(Equal(3))
				Expect(fakeLedger.SetStateCallCount()).To(Equal(0))
				ns, k := fakeLedger.GetStateArgsForCall(2)
				expectedTx := strings.Join([]string{"", "tokenTx", "0", ""}, "\x00")
				Expect(k).To(Equal(expectedTx))
				Expect(ns).To(Equal("tms"))
			})
		})

		Context("when a tx with the same txID already exists", func() {
			BeforeEach(func() {
				fakeLedger.GetStateReturnsOnCall(2, []byte("fake-tx"), nil)
			})

			It("returns an error", func() {
				err := verifier.ProcessTx(txID, fakeCreatorInfo, transaction, fakeLedger)
				Expect(err).To(Equal(&customtx.InvalidTxError{Msg: "transaction already exists: 0"}))
			})
		})
	})
})
