/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package plain_test

import (
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/core/ledger/customtx"
	"github.com/hyperledger/fabric/protos/token"
	"github.com/hyperledger/fabric/token/identity"
	mockid "github.com/hyperledger/fabric/token/identity/mock"
	mockledger "github.com/hyperledger/fabric/token/ledger/mock"
	"github.com/hyperledger/fabric/token/tms/plain"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

const (
	tokenNamespace = "_fabtoken"
	tokenKeyPrefix = "token"
)

var _ = Describe("Verifier", func() {
	var (
		fakePublicInfo          *mockid.PublicInfo
		fakeIssuingValidator    *mockid.IssuingValidator
		fakeTokenOwnerValidator identity.TokenOwnerValidator
		fakeLedger              *mockledger.LedgerWriter
		memoryLedger            *plain.MemoryLedger

		issueTransaction *token.TokenTransaction
		issueTxID        string

		verifier *plain.Verifier
	)

	BeforeEach(func() {
		fakePublicInfo = &mockid.PublicInfo{}
		fakeIssuingValidator = &mockid.IssuingValidator{}
		fakeTokenOwnerValidator = &TestTokenOwnerValidator{}
		fakeLedger = &mockledger.LedgerWriter{}
		fakeLedger.SetStateReturns(nil)

		issueTxID = "0"
		issueTransaction = &token.TokenTransaction{
			Action: &token.TokenTransaction_TokenAction{
				TokenAction: &token.TokenAction{
					Data: &token.TokenAction_Issue{
						Issue: &token.Issue{
							Outputs: []*token.Token{
								{Owner: &token.TokenOwner{Raw: []byte("owner-1")}, Type: "TOK1", Quantity: ToHex(111)},
								{Owner: &token.TokenOwner{Raw: []byte("owner-2")}, Type: "TOK2", Quantity: ToHex(222)},
							},
						},
					},
				},
			},
		}

		verifier = &plain.Verifier{
			IssuingValidator:    fakeIssuingValidator,
			TokenOwnerValidator: fakeTokenOwnerValidator,
		}
	})

	Describe("ProcessTx Issue", func() {
		It("evaluates policy for each output", func() {
			err := verifier.ProcessTx(issueTxID, fakePublicInfo, issueTransaction, fakeLedger)
			Expect(err).NotTo(HaveOccurred())

			Expect(fakeIssuingValidator.ValidateCallCount()).To(Equal(2))
			creator, tt := fakeIssuingValidator.ValidateArgsForCall(0)
			Expect(creator).To(Equal(fakePublicInfo))
			Expect(tt).To(Equal("TOK1"))
			creator, tt = fakeIssuingValidator.ValidateArgsForCall(1)
			Expect(creator).To(Equal(fakePublicInfo))
			Expect(tt).To(Equal("TOK2"))
		})

		It("checks the fake ledger", func() {
			err := verifier.ProcessTx(issueTxID, fakePublicInfo, issueTransaction, fakeLedger)
			Expect(err).NotTo(HaveOccurred())

			Expect(fakeLedger.SetStateCallCount()).To(Equal(2))

			outputBytes, err := proto.Marshal(&token.Token{Owner: &token.TokenOwner{Raw: []byte("owner-1")}, Type: "TOK1", Quantity: ToHex(111)})
			Expect(err).NotTo(HaveOccurred())
			ns, k, td := fakeLedger.SetStateArgsForCall(0)
			Expect(ns).To(Equal(tokenNamespace))
			ownerString := buildTokenOwnerString([]byte("owner-1"))
			expectedOutput := strings.Join([]string{"", tokenKeyPrefix, ownerString, "0", "0", ""}, "\x00")
			Expect(k).To(Equal(expectedOutput))
			Expect(td).To(Equal(outputBytes))

			outputBytes, err = proto.Marshal(&token.Token{Owner: &token.TokenOwner{Raw: []byte("owner-2")}, Type: "TOK2", Quantity: ToHex(222)})
			Expect(err).NotTo(HaveOccurred())
			ns, k, td = fakeLedger.SetStateArgsForCall(1)
			Expect(ns).To(Equal(tokenNamespace))
			ownerString = buildTokenOwnerString([]byte("owner-2"))
			expectedOutput = strings.Join([]string{"", tokenKeyPrefix, ownerString, "0", "1", ""}, "\x00")
			Expect(k).To(Equal(expectedOutput))
			Expect(td).To(Equal(outputBytes))
		})

		Context("when policy validation fails", func() {
			BeforeEach(func() {
				fakeIssuingValidator.ValidateReturns(errors.New("no-way-man"))
			})

			It("returns an error and does not write to the ledger", func() {
				err := verifier.ProcessTx(issueTxID, fakePublicInfo, issueTransaction, fakeLedger)
				Expect(err).To(HaveOccurred())
				Expect(err).To(Equal(&customtx.InvalidTxError{Msg: "issue policy check failed: no-way-man"}))
				Expect(fakeLedger.SetStateCallCount()).To(Equal(0))
			})
		})

		Context("when the ledger write check fails", func() {
			BeforeEach(func() {
				fakeLedger.SetStateReturns(errors.New("no-can-do"))
			})

			It("returns an error", func() {
				err := verifier.ProcessTx(issueTxID, fakePublicInfo, issueTransaction, fakeLedger)
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError("no-can-do"))

				Expect(fakeLedger.SetStateCallCount()).To(Equal(1))
			})
		})

		Context("when transaction does not contain any outputs", func() {
			BeforeEach(func() {
				issueTransaction = &token.TokenTransaction{
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
			})
			It("returns an error", func() {
				err := verifier.ProcessTx(issueTxID, fakePublicInfo, issueTransaction, fakeLedger)
				Expect(err).To(HaveOccurred())
				Expect(err).To(Equal(&customtx.InvalidTxError{Msg: "no outputs in transaction: 0"}))
			})
		})

		Context("when the output of a transaction has quantity of 0", func() {
			BeforeEach(func() {
				issueTransaction = &token.TokenTransaction{
					Action: &token.TokenTransaction_TokenAction{
						TokenAction: &token.TokenAction{
							Data: &token.TokenAction_Issue{
								Issue: &token.Issue{
									Outputs: []*token.Token{
										{Owner: &token.TokenOwner{Raw: []byte("owner-1")}, Type: "TOK1", Quantity: ToHex(0)},
									},
								},
							},
						},
					},
				}
			})
			It("returns an error", func() {
				err := verifier.ProcessTx(issueTxID, fakePublicInfo, issueTransaction, fakeLedger)
				Expect(err).To(HaveOccurred())
				Expect(err).To(Equal(&customtx.InvalidTxError{Msg: "output 0 quantity is invalid in transaction: 0"}))
			})
		})

		Context("when an output already exists", func() {
			BeforeEach(func() {
				memoryLedger = plain.NewMemoryLedger()
				err := verifier.ProcessTx(issueTxID, fakePublicInfo, issueTransaction, memoryLedger)
				Expect(err).NotTo(HaveOccurred())
			})
			It("returns an error", func() {
				err := verifier.ProcessTx(issueTxID, fakePublicInfo, issueTransaction, memoryLedger)
				Expect(err).To(HaveOccurred())
				ownerString := buildTokenOwnerString([]byte("owner-1"))
				existingOutputId := strings.Join([]string{"", tokenKeyPrefix, ownerString, "0", "0", ""}, "\x00")
				Expect(err).To(Equal(&customtx.InvalidTxError{Msg: fmt.Sprintf("token already exists: %s", existingOutputId)}))
			})
		})

		Context("when an output has no owner", func() {
			BeforeEach(func() {
				issueTransaction.GetTokenAction().GetIssue().Outputs[0].Owner = nil
				issueTxID = "no-owner-id"
			})

			It("returns an InvalidTxError", func() {
				err := verifier.ProcessTx(issueTxID, fakePublicInfo, issueTransaction, memoryLedger)
				Expect(err).To(Equal(&customtx.InvalidTxError{Msg: fmt.Sprintf("invalid owner in output for txID '%s', err 'owner is nil'", issueTxID)}))
			})
		})

		Context("when an output has empty owner", func() {
			BeforeEach(func() {
				issueTransaction.GetTokenAction().GetIssue().Outputs[0].Owner = &token.TokenOwner{}
				issueTxID = "no-owner-id"
			})

			It("returns an InvalidTxError", func() {
				err := verifier.ProcessTx(issueTxID, fakePublicInfo, issueTransaction, memoryLedger)
				Expect(err).To(Equal(&customtx.InvalidTxError{Msg: fmt.Sprintf("invalid owner in output for txID '%s', err 'raw is empty'", issueTxID)}))
			})
		})

	})

	Describe("Output GetState error scenarios", func() {
		BeforeEach(func() {
			memoryLedger = plain.NewMemoryLedger()
			err := verifier.ProcessTx(issueTxID, fakePublicInfo, issueTransaction, memoryLedger)
			Expect(err).NotTo(HaveOccurred())
		})

		It("retrieves the Token associated with the entry ID", func() {
			ownerString := buildTokenOwnerString([]byte("owner-1"))
			po, err := memoryLedger.GetState(tokenNamespace, strings.Join([]string{"", tokenKeyPrefix, ownerString, "0", "0", ""}, "\x00"))
			Expect(err).NotTo(HaveOccurred())

			output := &token.Token{}
			err = proto.Unmarshal(po, output)
			Expect(err).NotTo(HaveOccurred())

			Expect(output).To(Equal(&token.Token{
				Owner:    &token.TokenOwner{Raw: []byte("owner-1")},
				Type:     "TOK1",
				Quantity: ToHex(111),
			}))

			ownerString = buildTokenOwnerString([]byte("owner-2"))
			po, err = memoryLedger.GetState(tokenNamespace, strings.Join([]string{"", tokenKeyPrefix, ownerString, "0", "1", ""}, "\x00"))
			Expect(err).NotTo(HaveOccurred())

			err = proto.Unmarshal(po, output)
			Expect(err).NotTo(HaveOccurred())

			Expect(output).To(Equal(&token.Token{
				Owner:    &token.TokenOwner{Raw: []byte("owner-2")},
				Type:     "TOK2",
				Quantity: ToHex(222),
			}))
		})

		Context("when the output does not exist", func() {
			It("returns a nil and no error", func() {
				val, err := memoryLedger.GetState(tokenNamespace, strings.Join([]string{"", tokenKeyPrefix, "george", "0", ""}, "\x00"))
				Expect(err).NotTo(HaveOccurred())
				Expect(val).To(BeNil())
			})
		})
	})

	Describe("ProcessTx empty or invalid input", func() {
		Context("when a plain action is not provided", func() {
			BeforeEach(func() {
				issueTxID = "255"
				issueTransaction = &token.TokenTransaction{}
			})

			It("returns an error", func() {
				err := verifier.ProcessTx(issueTxID, fakePublicInfo, issueTransaction, fakeLedger)
				Expect(err).To(MatchError("check process failed for transaction '255': missing token action"))
			})
		})

		Context("when an unknown plain token action is provided", func() {
			BeforeEach(func() {
				issueTxID = "254"
				issueTransaction = &token.TokenTransaction{
					Action: &token.TokenTransaction_TokenAction{
						TokenAction: &token.TokenAction{},
					},
				}
			})

			It("returns an error", func() {
				err := verifier.ProcessTx(issueTxID, fakePublicInfo, issueTransaction, fakeLedger)
				Expect(err).To(Equal(&customtx.InvalidTxError{Msg: "unknown plain token action: <nil>"}))
			})
		})

		Context("when a transaction has invalid characters in key", func() {
			BeforeEach(func() {
				issueTxID = string(0)
			})

			It("fails when creating the ledger key for the output", func() {
				By("returning an error")
				err := verifier.ProcessTx(issueTxID, fakePublicInfo, issueTransaction, fakeLedger)
				Expect(err).To(Equal(&customtx.InvalidTxError{Msg: "error creating output ID: input contain unicode U+0000 starting at position [0]. U+0000 and U+10FFFF are not allowed in the input attribute of a composite key"}))
			})
		})

		Context("when a transaction has invalid characters in key", func() {
			BeforeEach(func() {
				issueTxID = string(0)
			})

			It("fails when creating the ledger key for the first output", func() {
				By("returning an error")
				err := verifier.ProcessTx(issueTxID, fakePublicInfo, issueTransaction, fakeLedger)
				Expect(err).To(Equal(&customtx.InvalidTxError{Msg: "error creating output ID: input contain unicode U+0000 starting at position [0]. U+0000 and U+10FFFF are not allowed in the input attribute of a composite key"}))
			})
		})

		Context("when a transaction key is an invalid utf8 string", func() {
			BeforeEach(func() {
				issueTxID = string([]byte{0xE0, 0x80, 0x80})
			})

			It("fails when creating the ledger key for the output", func() {
				By("returning an error")
				err := verifier.ProcessTx(issueTxID, fakePublicInfo, issueTransaction, fakeLedger)
				Expect(err).To(Equal(&customtx.InvalidTxError{Msg: "error creating output ID: not a valid utf8 string: [e08080]"}))
			})
		})

		Context("when the ledger read of an output fails", func() {
			BeforeEach(func() {
				fakeLedger.GetStateReturnsOnCall(0, nil, errors.New("error reading output"))
			})

			It("returns an error", func() {
				err := verifier.ProcessTx(issueTxID, fakePublicInfo, issueTransaction, fakeLedger)
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError("error reading output"))

				Expect(fakeLedger.GetStateCallCount()).To(Equal(1))
				Expect(fakeLedger.SetStateCallCount()).To(Equal(0))
				ns, k := fakeLedger.GetStateArgsForCall(0)
				ownerString := buildTokenOwnerString([]byte("owner-1"))
				expectedOutput := strings.Join([]string{"", tokenKeyPrefix, ownerString, "0", "0", ""}, "\x00")
				Expect(k).To(Equal(expectedOutput))
				Expect(ns).To(Equal(tokenNamespace))
			})
		})
	})

	Describe("Test ProcessTx Transfer with memory ledger", func() {
		var (
			transferTransaction *token.TokenTransaction
			transferTxID        string
		)

		BeforeEach(func() {
			transferTxID = "1"
			transferTransaction = &token.TokenTransaction{
				Action: &token.TokenTransaction_TokenAction{
					TokenAction: &token.TokenAction{
						Data: &token.TokenAction_Transfer{
							Transfer: &token.Transfer{
								Inputs: []*token.TokenId{
									{TxId: "0", Index: 0},
								},
								Outputs: []*token.Token{
									{Owner: &token.TokenOwner{Raw: []byte("owner-1")}, Type: "TOK1", Quantity: ToHex(99)},
									{Owner: &token.TokenOwner{Raw: []byte("owner-2")}, Type: "TOK1", Quantity: ToHex(12)},
								},
							},
						},
					},
				},
			}
			fakePublicInfo.PublicReturns([]byte("owner-1"))
			memoryLedger = plain.NewMemoryLedger()
			err := verifier.ProcessTx(issueTxID, fakePublicInfo, issueTransaction, memoryLedger)
			Expect(err).NotTo(HaveOccurred())
		})

		Context("when a valid transfer is provided", func() {
			BeforeEach(func() {
				err := verifier.ProcessTx(transferTxID, fakePublicInfo, transferTransaction, memoryLedger)
				Expect(err).NotTo(HaveOccurred())
			})

			It("is processed successfully", func() {
				ownerString := buildTokenOwnerString([]byte("owner-1"))
				po, err := memoryLedger.GetState(tokenNamespace, strings.Join([]string{"", tokenKeyPrefix, ownerString, transferTxID, "0", ""}, "\x00"))
				Expect(err).NotTo(HaveOccurred())

				output := &token.Token{}
				err = proto.Unmarshal(po, output)
				Expect(err).NotTo(HaveOccurred())

				Expect(output).To(Equal(&token.Token{
					Owner:    &token.TokenOwner{Raw: []byte("owner-1")},
					Type:     "TOK1",
					Quantity: ToHex(99),
				}))

				ownerString = buildTokenOwnerString([]byte("owner-2"))
				po, err = memoryLedger.GetState(tokenNamespace, strings.Join([]string{"", tokenKeyPrefix, ownerString, transferTxID, "1", ""}, "\x00"))
				Expect(err).NotTo(HaveOccurred())

				err = proto.Unmarshal(po, output)
				Expect(err).NotTo(HaveOccurred())

				Expect(output).To(Equal(&token.Token{
					Owner:    &token.TokenOwner{Raw: []byte("owner-2")},
					Type:     "TOK1",
					Quantity: ToHex(12),
				}))

				tokenOutput, err := memoryLedger.GetState(tokenNamespace, string("\x00")+tokenKeyPrefix+string("\x00")+"0"+string("\x00")+"0"+string("\x00"))
				Expect(err).NotTo(HaveOccurred())
				Expect(tokenOutput).To(BeNil())
			})
		})

		Context("when the input is empty", func() {
			BeforeEach(func() {
				transferTransaction = &token.TokenTransaction{
					Action: &token.TokenTransaction_TokenAction{
						TokenAction: &token.TokenAction{
							Data: &token.TokenAction_Transfer{
								Transfer: &token.Transfer{
									Inputs: nil,
									Outputs: []*token.Token{
										{Owner: &token.TokenOwner{Raw: []byte("owner-1")}, Type: "TOK1", Quantity: ToHex(99)},
										{Owner: &token.TokenOwner{Raw: []byte("owner-2")}, Type: "TOK1", Quantity: ToHex(12)},
									},
								},
							},
						},
					},
				}
			})

			It("returns an InvalidTxError", func() {
				err := verifier.ProcessTx(transferTxID, fakePublicInfo, transferTransaction, memoryLedger)
				Expect(err).To(Equal(&customtx.InvalidTxError{Msg: fmt.Sprintf("no tokenIds in transaction: %s", transferTxID)}))
			})
		})

		Context("when a non-existent input is referenced", func() {
			BeforeEach(func() {
				transferTransaction = &token.TokenTransaction{
					Action: &token.TokenTransaction_TokenAction{
						TokenAction: &token.TokenAction{
							Data: &token.TokenAction_Transfer{
								Transfer: &token.Transfer{
									Inputs: []*token.TokenId{
										{TxId: "wild_pineapple", Index: 0},
									},
									Outputs: []*token.Token{
										{Owner: &token.TokenOwner{Raw: []byte("owner-1")}, Type: "TOK1", Quantity: ToHex(99)},
										{Owner: &token.TokenOwner{Raw: []byte("owner-2")}, Type: "TOK1", Quantity: ToHex(12)},
									},
								},
							},
						},
					},
				}
			})

			It("returns an InvalidTxError", func() {
				err := verifier.ProcessTx(transferTxID, fakePublicInfo, transferTransaction, memoryLedger)
				ownerString := buildTokenOwnerString(fakePublicInfo.Public())
				Expect(err).To(Equal(&customtx.InvalidTxError{Msg: "token with ID \x00" + tokenKeyPrefix + "\x00" + ownerString + "\x00wild_pineapple\x000\x00 does not exist"}))
			})
		})

		Context("when the creator of the transfer transaction is not the owner of the input", func() {
			BeforeEach(func() {
				fakePublicInfo.PublicReturns([]byte("owner-pineapple"))
			})

			It("returns an InvalidTxError", func() {
				err := verifier.ProcessTx(transferTxID, fakePublicInfo, transferTransaction, memoryLedger)
				ownerString := buildTokenOwnerString(fakePublicInfo.Public())
				Expect(err).To(Equal(&customtx.InvalidTxError{Msg: "token with ID \x00token\x00" + ownerString + "\x000\x000\x00 does not exist"}))
			})
		})

		Context("when the same input is spent twice", func() {
			BeforeEach(func() {
				transferTransaction = &token.TokenTransaction{
					Action: &token.TokenTransaction_TokenAction{
						TokenAction: &token.TokenAction{
							Data: &token.TokenAction_Transfer{
								Transfer: &token.Transfer{
									Inputs: []*token.TokenId{
										{TxId: "0", Index: 0},
										{TxId: "0", Index: 0},
									},
									Outputs: []*token.Token{
										{Owner: &token.TokenOwner{Raw: []byte("owner-1")}, Type: "TOK1", Quantity: ToHex(221)},
										{Owner: &token.TokenOwner{Raw: []byte("owner-2")}, Type: "TOK1", Quantity: ToHex(1)},
									},
								},
							},
						},
					},
				}
			})

			It("returns an InvalidTxError", func() {
				err := verifier.ProcessTx(transferTxID, fakePublicInfo, transferTransaction, memoryLedger)
				Expect(err).To(Equal(&customtx.InvalidTxError{Msg: "Input duplicates found"}))
			})
		})

		Context("when the input type does not match the output type", func() {
			BeforeEach(func() {
				transferTransaction = &token.TokenTransaction{
					Action: &token.TokenTransaction_TokenAction{
						TokenAction: &token.TokenAction{
							Data: &token.TokenAction_Transfer{
								Transfer: &token.Transfer{
									Inputs: []*token.TokenId{
										{TxId: "0", Index: 0},
									},
									Outputs: []*token.Token{
										{Owner: &token.TokenOwner{Raw: []byte("owner-1")}, Type: "wild_pineapple", Quantity: ToHex(100)},
										{Owner: &token.TokenOwner{Raw: []byte("owner-2")}, Type: "wild_pineapple", Quantity: ToHex(11)},
									},
								},
							},
						},
					},
				}
			})

			It("returns an InvalidTxError", func() {
				err := verifier.ProcessTx(transferTxID, fakePublicInfo, transferTransaction, memoryLedger)
				Expect(err).To(Equal(&customtx.InvalidTxError{Msg: "token type mismatch in inputs and outputs for transaction ID 1 (wild_pineapple vs TOK1)"}))
			})
		})

		Context("when the input sum does not match the output sum", func() {
			BeforeEach(func() {
				transferTransaction = &token.TokenTransaction{
					Action: &token.TokenTransaction_TokenAction{
						TokenAction: &token.TokenAction{
							Data: &token.TokenAction_Transfer{
								Transfer: &token.Transfer{
									Inputs: []*token.TokenId{
										{TxId: "0", Index: 0},
									},
									Outputs: []*token.Token{
										{Owner: &token.TokenOwner{Raw: []byte("owner-1")}, Type: "TOK1", Quantity: ToHex(112)},
										{Owner: &token.TokenOwner{Raw: []byte("owner-2")}, Type: "TOK1", Quantity: ToHex(12)},
									},
								},
							},
						},
					},
				}
			})

			It("returns an InvalidTxError", func() {
				err := verifier.ProcessTx(transferTxID, fakePublicInfo, transferTransaction, memoryLedger)
				Expect(err).To(Equal(&customtx.InvalidTxError{Msg: "token sum mismatch in inputs and outputs for transaction ID 1 (124 vs 111)"}))
			})
		})

		Context("when the input contains multiple token types", func() {
			var (
				anotherIssueTransaction *token.TokenTransaction
				anotherIssueTxID        string
			)
			BeforeEach(func() {
				anotherIssueTxID = "2"
				anotherIssueTransaction = &token.TokenTransaction{
					Action: &token.TokenTransaction_TokenAction{
						TokenAction: &token.TokenAction{
							Data: &token.TokenAction_Issue{
								Issue: &token.Issue{
									Outputs: []*token.Token{
										{Owner: &token.TokenOwner{Raw: []byte("owner-1")}, Type: "TOK2", Quantity: ToHex(2121)},
									},
								},
							},
						},
					},
				}
				err := verifier.ProcessTx(anotherIssueTxID, fakePublicInfo, anotherIssueTransaction, memoryLedger)
				Expect(err).NotTo(HaveOccurred())
				transferTransaction = &token.TokenTransaction{
					Action: &token.TokenTransaction_TokenAction{
						TokenAction: &token.TokenAction{
							Data: &token.TokenAction_Transfer{
								Transfer: &token.Transfer{
									Inputs: []*token.TokenId{
										{TxId: "0", Index: 0},
										{TxId: "2", Index: 0},
									},
									Outputs: []*token.Token{
										{Owner: &token.TokenOwner{Raw: []byte("owner-1")}, Type: "TOK1", Quantity: ToHex(111)},
									},
								},
							},
						},
					},
				}
			})

			It("returns an InvalidTxError", func() {
				err := verifier.ProcessTx(transferTxID, fakePublicInfo, transferTransaction, memoryLedger)
				Expect(err).To(Equal(&customtx.InvalidTxError{Msg: "multiple token types in input for txID: 1 (TOK1, TOK2)"}))
			})
		})

		Context("when the output contains multiple token types", func() {
			BeforeEach(func() {
				transferTransaction = &token.TokenTransaction{
					Action: &token.TokenTransaction_TokenAction{
						TokenAction: &token.TokenAction{
							Data: &token.TokenAction_Transfer{
								Transfer: &token.Transfer{
									Inputs: []*token.TokenId{
										{TxId: "0", Index: 0},
									},
									Outputs: []*token.Token{
										{Owner: &token.TokenOwner{Raw: []byte("owner-1")}, Type: "TOK1", Quantity: ToHex(112)},
										{Owner: &token.TokenOwner{Raw: []byte("owner-2")}, Type: "TOK2", Quantity: ToHex(12)},
									},
								},
							},
						},
					},
				}
			})

			It("returns an InvalidTxError", func() {
				err := verifier.ProcessTx(transferTxID, fakePublicInfo, transferTransaction, memoryLedger)
				Expect(err).To(Equal(&customtx.InvalidTxError{Msg: "multiple token types ('TOK1', 'TOK2') in output for txID '1'"}))
			})
		})

		Context("when an input has already been spent", func() {
			BeforeEach(func() {
				err := verifier.ProcessTx(transferTxID, fakePublicInfo, transferTransaction, memoryLedger)
				Expect(err).NotTo(HaveOccurred())
			})

			It("returns an InvalidTxError", func() {
				err := verifier.ProcessTx("2", fakePublicInfo, transferTransaction, memoryLedger)
				ownerString := buildTokenOwnerString(fakePublicInfo.Public())
				Expect(err).To(Equal(&customtx.InvalidTxError{Msg: "token with ID \x00" + tokenKeyPrefix + "\x00" + ownerString + "\x000\x000\x00 does not exist"}))
			})
		})

		Context("when an output already exists", func() {
			BeforeEach(func() {
				memoryLedger = plain.NewMemoryLedger()
				err := verifier.ProcessTx(issueTxID, fakePublicInfo, issueTransaction, memoryLedger)
				Expect(err).NotTo(HaveOccurred())

				transferTransaction = &token.TokenTransaction{
					Action: &token.TokenTransaction_TokenAction{
						TokenAction: &token.TokenAction{
							Data: &token.TokenAction_Transfer{
								Transfer: &token.Transfer{
									Inputs: []*token.TokenId{
										{TxId: "0", Index: 0},
									},
									Outputs: []*token.Token{
										{Owner: &token.TokenOwner{Raw: []byte("owner-1")}, Type: "TOK1", Quantity: ToHex(111)},
									},
								},
							},
						},
					},
				}
			})
			It("returns an error", func() {
				err := verifier.ProcessTx(issueTxID, fakePublicInfo, transferTransaction, memoryLedger)
				Expect(err).To(HaveOccurred())
				ownerString := buildTokenOwnerString([]byte("owner-1"))
				existingOutputId := "\x00" + tokenKeyPrefix + "\x00" + ownerString + "\x00" + issueTxID + "\x00" + "0" + "\x00"
				Expect(err).To(Equal(&customtx.InvalidTxError{Msg: fmt.Sprintf("token already exists: %s", existingOutputId)}))
			})
		})

		Context("when an output has no owner", func() {
			BeforeEach(func() {
				transferTransaction.GetTokenAction().GetTransfer().Outputs[0].Owner = nil
			})

			It("returns an InvalidTxError", func() {
				err := verifier.ProcessTx(transferTxID, fakePublicInfo, transferTransaction, memoryLedger)
				Expect(err).To(Equal(&customtx.InvalidTxError{Msg: fmt.Sprintf("invalid owner in output for txID '%s', err 'owner is nil'", transferTxID)}))
			})
		})

		Context("when the sum of the inputs produces an overflow", func() {
			BeforeEach(func() {
				issueTxID = "1"
				issueTransaction = &token.TokenTransaction{
					Action: &token.TokenTransaction_TokenAction{
						TokenAction: &token.TokenAction{
							Data: &token.TokenAction_Issue{
								Issue: &token.Issue{
									Outputs: []*token.Token{
										{Owner: &token.TokenOwner{Raw: []byte("owner-1")}, Type: "TOK1", Quantity: ToHex(math.MaxUint64)},
										{Owner: &token.TokenOwner{Raw: []byte("owner-1")}, Type: "TOK1", Quantity: ToHex(10)},
									},
								},
							},
						},
					},
				}
				err := verifier.ProcessTx(issueTxID, fakePublicInfo, issueTransaction, memoryLedger)
				Expect(err).NotTo(HaveOccurred())

				transferTxID = "2"
				transferTransaction = &token.TokenTransaction{
					Action: &token.TokenTransaction_TokenAction{
						TokenAction: &token.TokenAction{
							Data: &token.TokenAction_Transfer{
								Transfer: &token.Transfer{
									Inputs: []*token.TokenId{
										{TxId: "1", Index: 0},
										{TxId: "1", Index: 1},
									},
									Outputs: []*token.Token{
										{Owner: &token.TokenOwner{Raw: []byte("owner-1")}, Type: "TOK1", Quantity: ToHex(10)},
										{Owner: &token.TokenOwner{Raw: []byte("owner-2")}, Type: "TOK1", Quantity: ToHex(10)},
									},
								},
							},
						},
					},
				}
			})

			It("it fails", func() {
				err := verifier.ProcessTx(transferTxID, fakePublicInfo, transferTransaction, memoryLedger)
				Expect(err).To(MatchError("failed adding up input quantities, err '18446744073709551615 + 10 = overflow'"))
			})
		})

		Context("when the sum of the outputs produces an overflow", func() {
			BeforeEach(func() {
				transferTxID = "1"
				transferTransaction = &token.TokenTransaction{
					Action: &token.TokenTransaction_TokenAction{
						TokenAction: &token.TokenAction{
							Data: &token.TokenAction_Transfer{
								Transfer: &token.Transfer{
									Inputs: []*token.TokenId{
										{TxId: "0", Index: 0},
									},
									Outputs: []*token.Token{
										{Owner: &token.TokenOwner{Raw: []byte("owner-1")}, Type: "TOK1", Quantity: ToHex(math.MaxUint64)},
										{Owner: &token.TokenOwner{Raw: []byte("owner-2")}, Type: "TOK1", Quantity: ToHex(math.MaxUint64)},
									},
								},
							},
						},
					},
				}
			})

			It("it fails", func() {
				err := verifier.ProcessTx(transferTxID, fakePublicInfo, transferTransaction, memoryLedger)
				Expect(err).To(MatchError("failed adding up output quantities, err '18446744073709551615 + 18446744073709551615 = overflow'"))
			})
		})

	})

	Describe("Test get/set/delete token with fake ledger", func() {
		var redeemTx *token.TokenTransaction
		var issuedTokenBytes []byte

		BeforeEach(func() {

			fakePublicInfo.PublicReturns([]byte("owner-1"))

			// issue tokens
			issuedToken := &token.Token{Owner: &token.TokenOwner{Raw: []byte("owner-1")}, Type: "TOK1", Quantity: ToHex(111)}
			issuedTokenBytes, _ = proto.Marshal(issuedToken)

			// redeem tokens
			redeemTx = &token.TokenTransaction{
				Action: &token.TokenTransaction_TokenAction{
					TokenAction: &token.TokenAction{
						Data: &token.TokenAction_Redeem{
							Redeem: &token.Transfer{
								Inputs: []*token.TokenId{
									{TxId: "0", Index: 0},
								},
								Outputs: []*token.Token{
									{Type: "TOK1", Quantity: ToHex(100)},
									{Owner: &token.TokenOwner{Raw: []byte("owner-1")}, Type: "TOK1", Quantity: ToHex(11)},
								},
							},
						},
					},
				},
			}
		})

		Context("when ledger returns error on delete token", func() {
			It("returns an error", func() {

				// first call checks the output does not exists. The redeem output without owner will be skipped for checking.
				fakeLedger.GetStateReturnsOnCall(0, nil, nil)
				// next call is check input exists
				fakeLedger.GetStateReturnsOnCall(1, issuedTokenBytes, nil)

				expectedErr := errors.New("some delete error")
				fakeLedger.DeleteStateReturns(expectedErr)

				err := verifier.ProcessTx("r2", fakePublicInfo, redeemTx, fakeLedger)
				Expect(err).To(HaveOccurred())
				Expect(err).To(Equal(expectedErr))
				Expect(fakeLedger.GetStateCallCount()).To(Equal(2))
				Expect(fakeLedger.DeleteStateCallCount()).To(Equal(1))

			})
		})

		Context("when ledger returns error on get token", func() {
			It("returns an error", func() {

				expectedErr := errors.New("some error on get token")

				// first call checks the output does not exists. The redeem output without owner will be skipped for checking.
				fakeLedger.GetStateReturnsOnCall(0, nil, nil)
				// second call is check input exists
				fakeLedger.GetStateReturnsOnCall(1, nil, expectedErr)

				err := verifier.ProcessTx("r2", fakePublicInfo, redeemTx, fakeLedger)
				Expect(err).To(HaveOccurred())
				Expect(err).To(Equal(expectedErr))
				Expect(fakeLedger.GetStateCallCount()).To(Equal(2))

			})
		})

		Context("when ledger returns invalid token bytes on get token", func() {
			It("returns an error", func() {

				expectedErr := &customtx.InvalidTxError{Msg: "unmarshaling error: unexpected EOF"}

				// first call checks the output does not exists. The redeem output without owner will be skipped for checking.
				fakeLedger.GetStateReturnsOnCall(0, nil, nil)
				// next call is check input exists
				fakeLedger.GetStateReturnsOnCall(1, []byte("some invalid proto bytes"), nil)

				err := verifier.ProcessTx("r2", fakePublicInfo, redeemTx, fakeLedger)
				Expect(err).To(HaveOccurred())
				Expect(err).To(Equal(expectedErr))
				Expect(fakeLedger.GetStateCallCount()).To(Equal(2))
			})
		})

		Context("when ledger return error on set state (commit issue)", func() {
			It("returns an error", func() {

				expectedErr := errors.New("some error")

				// first call is checkout output does not exists
				fakeLedger.GetStateReturnsOnCall(0, nil, nil)

				fakeLedger.SetStateReturnsOnCall(0, expectedErr)

				err := verifier.ProcessTx("0", fakePublicInfo, issueTransaction, fakeLedger)
				Expect(err).To(HaveOccurred())
				Expect(err).To(Equal(expectedErr))
				Expect(fakeLedger.GetStateCallCount()).To(Equal(2))
				Expect(fakeLedger.SetStateCallCount()).To(Equal(1))
			})
		})

		Context("when ledger return error on set state (commit transfer/redeem)", func() {
			It("returns an error", func() {
				expectedErr := errors.New("some error")

				// first call checks the output does not exists. The redeem output without owner will be skipped for checking.
				fakeLedger.GetStateReturnsOnCall(0, nil, nil)
				// next call is check input exists
				fakeLedger.GetStateReturnsOnCall(1, issuedTokenBytes, nil)

				fakeLedger.SetStateReturnsOnCall(0, expectedErr)

				err := verifier.ProcessTx("0", fakePublicInfo, redeemTx, fakeLedger)
				Expect(err).To(HaveOccurred())
				Expect(err).To(Equal(expectedErr))
				Expect(fakeLedger.GetStateCallCount()).To(Equal(2))
				Expect(fakeLedger.SetStateCallCount()).To(Equal(1))
			})
		})
	})

	Describe("Test ProcessTx Redeem with memory ledger", func() {
		var (
			tokenIds          []*token.TokenId
			redeemTxID        string
			redeemTransaction *token.TokenTransaction
		)

		BeforeEach(func() {
			redeemTxID = "r1"
			tokenIds = []*token.TokenId{
				{TxId: "0", Index: 0},
			}
			redeemTransaction = &token.TokenTransaction{
				Action: &token.TokenTransaction_TokenAction{
					TokenAction: &token.TokenAction{
						Data: &token.TokenAction_Redeem{
							Redeem: &token.Transfer{
								Inputs: tokenIds,
								Outputs: []*token.Token{
									{Type: "TOK1", Quantity: ToHex(111)},
								},
							},
						},
					},
				},
			}

			fakePublicInfo.PublicReturns([]byte("owner-1"))
			memoryLedger = plain.NewMemoryLedger()
			err := verifier.ProcessTx(issueTxID, fakePublicInfo, issueTransaction, memoryLedger)
			Expect(err).NotTo(HaveOccurred())
		})

		It("processes a redeem transaction with all tokens redeemed", func() {
			ownerString := buildTokenOwnerString([]byte("owner-1"))
			tokenId := strings.Join([]string{"", tokenKeyPrefix, ownerString, "0", "0", ""}, "\x00")

			// verify that token does exist
			po, err := memoryLedger.GetState(tokenNamespace, tokenId)
			Expect(err).NotTo(HaveOccurred())

			output := &token.Token{}
			err = proto.Unmarshal(po, output)
			Expect(err).NotTo(HaveOccurred())
			Expect(proto.Equal(output, &token.Token{Owner: &token.TokenOwner{Raw: []byte("owner-1")}, Type: "TOK1", Quantity: ToHex(111)})).To(BeTrue())

			err = verifier.ProcessTx(redeemTxID, fakePublicInfo, redeemTransaction, memoryLedger)
			Expect(err).NotTo(HaveOccurred())

			// verify that token does not exist anymore
			po, err = memoryLedger.GetState(tokenNamespace, tokenId)
			Expect(err).NotTo(HaveOccurred())
			Expect(po).To(Equal([]byte{}))
		})

		It("processes a redeem transaction with some tokens redeemed", func() {
			// prepare redeemTransaction with 2 outputs: one for redeemed tokens and another for remaining tokens
			redeemTransaction = &token.TokenTransaction{
				Action: &token.TokenTransaction_TokenAction{
					TokenAction: &token.TokenAction{
						Data: &token.TokenAction_Redeem{
							Redeem: &token.Transfer{
								Inputs: tokenIds,
								Outputs: []*token.Token{
									{Type: "TOK1", Quantity: ToHex(99)},
									{Owner: &token.TokenOwner{Raw: []byte("owner-1")}, Type: "TOK1", Quantity: ToHex(12)},
								},
							},
						},
					},
				},
			}

			// check that token (TOK1 111) does exist
			ownerString := buildTokenOwnerString([]byte("owner-1"))
			tokenId := strings.Join([]string{"", tokenKeyPrefix, ownerString, "0", "0", ""}, "\x00")
			po, err := memoryLedger.GetState(tokenNamespace, tokenId)
			Expect(err).NotTo(HaveOccurred())

			output := &token.Token{}
			err = proto.Unmarshal(po, output)
			Expect(err).NotTo(HaveOccurred())
			Expect(proto.Equal(output, &token.Token{Owner: &token.TokenOwner{Raw: []byte("owner-1")}, Type: "TOK1", Quantity: ToHex(111)})).To(BeTrue())

			err = verifier.ProcessTx(redeemTxID, fakePublicInfo, redeemTransaction, memoryLedger)
			Expect(err).NotTo(HaveOccurred())

			// check that token (TOK1 111) does not exist anymore
			po, err = memoryLedger.GetState(tokenNamespace, tokenId)
			Expect(err).NotTo(HaveOccurred())
			Expect(po).To(Equal([]byte{}))

			// check that token (TOK1 12) does exist
			ownerString = buildTokenOwnerString(fakePublicInfo.Public())
			newTokenId := strings.Join([]string{"", tokenKeyPrefix, ownerString, redeemTxID, "1", ""}, "\x00")
			po, err = memoryLedger.GetState(tokenNamespace, newTokenId)
			Expect(err).NotTo(HaveOccurred())

			output = &token.Token{}
			err = proto.Unmarshal(po, output)
			Expect(err).NotTo(HaveOccurred())
			Expect(proto.Equal(output, &token.Token{Owner: &token.TokenOwner{Raw: []byte("owner-1")}, Type: "TOK1", Quantity: ToHex(12)})).To(BeTrue())
		})

		Context("when an input has already been spent", func() {
			BeforeEach(func() {
				err := verifier.ProcessTx(redeemTxID, fakePublicInfo, redeemTransaction, memoryLedger)
				Expect(err).NotTo(HaveOccurred())
			})

			It("returns an InvalidTxError", func() {
				err := verifier.ProcessTx("r2", fakePublicInfo, redeemTransaction, memoryLedger)
				ownerString := buildTokenOwnerString(fakePublicInfo.Public())
				Expect(err).To(Equal(&customtx.InvalidTxError{Msg: "token with ID \x00" + tokenKeyPrefix + "\x00" + ownerString + "\x000\x000\x00 does not exist"}))
			})
		})

		Context("when token sum mismatches in inputs and outputs", func() {
			BeforeEach(func() {
				redeemTransaction = &token.TokenTransaction{
					Action: &token.TokenTransaction_TokenAction{
						TokenAction: &token.TokenAction{
							Data: &token.TokenAction_Redeem{
								Redeem: &token.Transfer{
									Inputs: tokenIds,
									Outputs: []*token.Token{
										{Type: "TOK1", Quantity: ToHex(100)},
									},
								},
							},
						},
					},
				}
			})

			It("returns an error", func() {
				err := verifier.ProcessTx(redeemTxID, fakePublicInfo, redeemTransaction, memoryLedger)
				Expect(err).To(Equal(&customtx.InvalidTxError{
					Msg: fmt.Sprintf("token sum mismatch in inputs and outputs for transaction ID %s (%d vs %d)", redeemTxID, 100, 111)}))
			})
		})

		Context("when redeem has more than two outputs", func() {
			BeforeEach(func() {
				redeemTransaction = &token.TokenTransaction{
					Action: &token.TokenTransaction_TokenAction{
						TokenAction: &token.TokenAction{
							Data: &token.TokenAction_Redeem{
								Redeem: &token.Transfer{
									Inputs: tokenIds,
									Outputs: []*token.Token{
										{Type: "TOK1", Quantity: ToHex(100)},
										{Owner: &token.TokenOwner{Raw: []byte("owner-1")}, Type: "TOK1", Quantity: ToHex(10)},
										{Owner: &token.TokenOwner{Raw: []byte("my-friend")}, Type: "TOK1", Quantity: ToHex(1)},
									},
								},
							},
						},
					},
				}
			})

			It("returns an error", func() {
				err := verifier.ProcessTx(redeemTxID, fakePublicInfo, redeemTransaction, memoryLedger)
				Expect(err).To(Equal(&customtx.InvalidTxError{
					Msg: fmt.Sprintf("too many outputs in a redeem transaction")}))
			})
		})

		Context("when inputs have more than one type", func() {
			var (
				anotherIssueTransaction *token.TokenTransaction
				anotherIssueTxID        string
			)
			BeforeEach(func() {
				anotherIssueTxID = "2"
				anotherIssueTransaction = &token.TokenTransaction{
					Action: &token.TokenTransaction_TokenAction{
						TokenAction: &token.TokenAction{
							Data: &token.TokenAction_Issue{
								Issue: &token.Issue{
									Outputs: []*token.Token{
										{Owner: &token.TokenOwner{Raw: []byte("owner-1")}, Type: "TOK2", Quantity: ToHex(222)},
									},
								},
							},
						},
					},
				}
				err := verifier.ProcessTx(anotherIssueTxID, fakePublicInfo, anotherIssueTransaction, memoryLedger)
				Expect(err).NotTo(HaveOccurred())

				redeemTransaction = &token.TokenTransaction{
					Action: &token.TokenTransaction_TokenAction{
						TokenAction: &token.TokenAction{
							Data: &token.TokenAction_Redeem{
								Redeem: &token.Transfer{
									Inputs: []*token.TokenId{
										{TxId: "0", Index: 0},
										{TxId: "2", Index: 0},
									},
									Outputs: []*token.Token{
										{Type: "TOK1", Quantity: ToHex(300)},
									},
								},
							},
						},
					},
				}
			})

			It("returns an error", func() {
				err := verifier.ProcessTx(redeemTxID, fakePublicInfo, redeemTransaction, memoryLedger)
				Expect(err).To(Equal(&customtx.InvalidTxError{
					Msg: fmt.Sprintf("multiple token types in input for txID: %s (TOK1, TOK2)", redeemTxID)}))
			})
		})

		Context("when redeem output has wrong type", func() {
			BeforeEach(func() {
				redeemTransaction.GetTokenAction().GetRedeem().Outputs[0].Type = "newtype"
			})

			It("returns an error", func() {
				err := verifier.ProcessTx(redeemTxID, fakePublicInfo, redeemTransaction, memoryLedger)
				Expect(err).To(MatchError(fmt.Sprintf(
					fmt.Sprintf("token type mismatch in inputs and outputs for transaction ID %s (%s vs %s)", redeemTxID, "newtype", "TOK1"))))
			})
		})

		Context("when output for remaining tokens has wrong owner", func() {
			BeforeEach(func() {
				// set wrong owner in the output for unredeemed tokens
				redeemTransaction = &token.TokenTransaction{
					Action: &token.TokenTransaction_TokenAction{
						TokenAction: &token.TokenAction{
							Data: &token.TokenAction_Redeem{
								Redeem: &token.Transfer{
									Inputs: tokenIds,
									Outputs: []*token.Token{
										{Type: "TOK1", Quantity: ToHex(99)},
										{Owner: &token.TokenOwner{Raw: []byte("owner-2")}, Type: "TOK1", Quantity: ToHex(12)},
									},
								},
							},
						},
					},
				}
			})

			It("returns an error", func() {
				err := verifier.ProcessTx(redeemTxID, fakePublicInfo, redeemTransaction, memoryLedger)
				Expect(err).To(MatchError(fmt.Sprintf(fmt.Sprintf("wrong owner for remaining tokens, should be original owner owner-1, but got owner-2"))))
			})
		})

		Context("when output for remaining tokens has no owner", func() {
			BeforeEach(func() {
				// do not set owner in the output for unredeemed tokens
				redeemTransaction = &token.TokenTransaction{
					Action: &token.TokenTransaction_TokenAction{
						TokenAction: &token.TokenAction{
							Data: &token.TokenAction_Redeem{
								Redeem: &token.Transfer{
									Inputs: tokenIds,
									Outputs: []*token.Token{
										{Type: "TOK1", Quantity: ToHex(99)},
										{Owner: &token.TokenOwner{Raw: []byte("wrong-owner")}, Type: "TOK1", Quantity: ToHex(12)},
									},
								},
							},
						},
					},
				}
			})

			It("returns an error", func() {
				err := verifier.ProcessTx(redeemTxID, fakePublicInfo, redeemTransaction, memoryLedger)
				Expect(err).To(MatchError(fmt.Sprintf(fmt.Sprintf("wrong owner for remaining tokens, should be original owner owner-1, but got wrong-owner"))))
			})
		})

		Context("when output for redeemed tokens has owner", func() {
			BeforeEach(func() {
				// set owner for the redeem output
				redeemTransaction.GetTokenAction().GetRedeem().Outputs[0].Owner = &token.TokenOwner{Raw: []byte("Owner-1")}
			})

			It("returns an error", func() {
				err := verifier.ProcessTx(redeemTxID, fakePublicInfo, redeemTransaction, memoryLedger)
				Expect(err).To(MatchError(fmt.Sprintf(fmt.Sprintf("owner should be nil in a redeem output"))))
			})
		})
	})
})

type TestTokenOwnerValidator struct {
}

func (TestTokenOwnerValidator) Validate(owner *token.TokenOwner) error {
	if owner == nil {
		return errors.New("owner is nil")
	}

	if len(owner.Raw) == 0 {
		return errors.New("raw is empty")
	}

	return nil
}
