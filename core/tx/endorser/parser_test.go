/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package endorsertx_test

import (
	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric-protos-go/peer"
	endorsertx "github.com/hyperledger/fabric/core/tx/endorser"
	"github.com/hyperledger/fabric/pkg/tx"
	txpkg "github.com/hyperledger/fabric/pkg/tx"
	"github.com/hyperledger/fabric/protoutil"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Parser", func() {
	var (
		txenv       *tx.Envelope
		hdrExt      *protomsg
		payloadData *protomsg
		prpExt      *protomsg
		prp         *protomsg
	)

	BeforeEach(func() {
		prp, hdrExt, prpExt, payloadData = nil, nil, nil, nil
	})

	JustBeforeEach(func() {
		var hdrExtBytes []byte
		if hdrExt != nil {
			hdrExtBytes = hdrExt.msg
		} else {
			hdrExtBytes = protoutil.MarshalOrPanic(
				&peer.ChaincodeHeaderExtension{
					ChaincodeId: &peer.ChaincodeID{
						Name: "my-called-cc",
					},
				},
			)
		}

		chHeader := &common.ChannelHeader{
			ChannelId: "my-channel",
			Epoch:     35,
			Extension: hdrExtBytes,
		}

		sigHeader := &common.SignatureHeader{
			Nonce:   []byte("1234"),
			Creator: []byte("creator"),
		}

		var extBytes []byte
		if prpExt != nil {
			extBytes = prpExt.msg
		} else {
			extBytes = protoutil.MarshalOrPanic(&peer.ChaincodeAction{
				Results: []byte("results"),
				Response: &peer.Response{
					Status: 200,
				},
				Events: []byte("events"),
			})
		}

		var prpBytes []byte
		if prp != nil {
			prpBytes = prp.msg
		} else {
			prpBytes = protoutil.MarshalOrPanic(&peer.ProposalResponsePayload{
				Extension:    extBytes,
				ProposalHash: []byte("phash"),
			})
		}

		ccEndAct := &peer.ChaincodeEndorsedAction{
			ProposalResponsePayload: prpBytes,
			Endorsements: []*peer.Endorsement{
				{
					Endorser:  []byte("endorser"),
					Signature: []byte("signature"),
				},
			},
		}

		ccActP := &peer.ChaincodeActionPayload{
			Action: ccEndAct,
		}

		tx := &peer.Transaction{
			Actions: []*peer.TransactionAction{
				{
					Payload: protoutil.MarshalOrPanic(ccActP),
				},
			},
		}

		var txenvPayloadDataBytes []byte
		if payloadData != nil {
			txenvPayloadDataBytes = payloadData.msg
		} else {
			txenvPayloadDataBytes = protoutil.MarshalOrPanic(tx)
		}

		txenv = &txpkg.Envelope{
			ChannelHeader:   chHeader,
			SignatureHeader: sigHeader,
			Data:            txenvPayloadDataBytes,
		}
	})

	It("returns an instance of EndorserTx", func() {
		pe, err := endorsertx.NewEndorserTx(txenv)
		Expect(err).NotTo(HaveOccurred())
		Expect(pe).To(Equal(&endorsertx.EndorserTx{
			Response: &peer.Response{
				Status: 200,
			},
			Results:      []byte("results"),
			Events:       []byte("events"),
			ComputedTxID: "0befbaa99e45fb676a54d6df7e44a52a0594d524d696d9f77e8ee21bbfb554f0",
			Endorsements: []*peer.Endorsement{
				{
					Endorser:  []byte("endorser"),
					Signature: []byte("signature"),
				},
			},
			ChID:    "my-channel",
			Creator: []byte("creator"),
			CcID:    "my-called-cc",
		}))
	})

	When("there is no payload data", func() {
		BeforeEach(func() {
			payloadData = &protomsg{}
		})

		It("returns an error", func() {
			pe, err := endorsertx.NewEndorserTx(txenv)
			Expect(err).To(MatchError("nil payload data"))
			Expect(pe).To(BeNil())
		})
	})

	When("there is bad payload data", func() {
		BeforeEach(func() {
			payloadData = &protomsg{msg: []byte("barf")}
		})

		It("returns an error", func() {
			pe, err := endorsertx.NewEndorserTx(txenv)
			Expect(err).To(MatchError("error unmarshaling Transaction: unexpected EOF"))
			Expect(pe).To(BeNil())
		})
	})

	When("there is bad payload data", func() {
		BeforeEach(func() {
			payloadData = &protomsg{msg: protoutil.MarshalOrPanic(&peer.Transaction{
				Actions: []*peer.TransactionAction{{}, {}},
			})}
		})

		It("returns an error", func() {
			pe, err := endorsertx.NewEndorserTx(txenv)
			Expect(err).To(MatchError("only one transaction action is supported, 2 were present"))
			Expect(pe).To(BeNil())
		})
	})

	When("the transaction action has no payload", func() {
		BeforeEach(func() {
			payloadData = &protomsg{msg: protoutil.MarshalOrPanic(&peer.Transaction{
				Actions: []*peer.TransactionAction{{}},
			})}
		})

		It("returns an error", func() {
			pe, err := endorsertx.NewEndorserTx(txenv)
			Expect(err).To(MatchError("empty ChaincodeActionPayload"))
			Expect(pe).To(BeNil())
		})
	})

	When("the transaction action has a bad payload", func() {
		BeforeEach(func() {
			payloadData = &protomsg{msg: protoutil.MarshalOrPanic(&peer.Transaction{
				Actions: []*peer.TransactionAction{{Payload: []byte("barf")}},
			})}
		})

		It("returns an error", func() {
			pe, err := endorsertx.NewEndorserTx(txenv)
			Expect(err).To(MatchError("error unmarshaling ChaincodeActionPayload: unexpected EOF"))
			Expect(pe).To(BeNil())
		})
	})

	When("the transaction action has a bad payload", func() {
		BeforeEach(func() {
			payloadData = &protomsg{msg: protoutil.MarshalOrPanic(&peer.Transaction{
				Actions: []*peer.TransactionAction{
					{
						Payload: protoutil.MarshalOrPanic(
							&peer.ChaincodeActionPayload{
								ChaincodeProposalPayload: []byte("some proposal payload"),
								Action:                   nil,
							},
						),
					},
				},
			})}
		})

		It("returns an error", func() {
			pe, err := endorsertx.NewEndorserTx(txenv)
			Expect(err).To(MatchError("nil ChaincodeEndorsedAction"))
			Expect(pe).To(BeNil())
		})
	})

	When("there is no header extension", func() {
		BeforeEach(func() {
			hdrExt = &protomsg{}
		})

		It("returns an error", func() {
			pe, err := endorsertx.NewEndorserTx(txenv)
			Expect(err).To(MatchError("empty header extension"))
			Expect(pe).To(BeNil())
		})
	})

	When("there is a bad header extension", func() {
		BeforeEach(func() {
			hdrExt = &protomsg{msg: []byte("barf")}
		})

		It("returns an error", func() {
			pe, err := endorsertx.NewEndorserTx(txenv)
			Expect(err).To(MatchError("error unmarshaling ChaincodeHeaderExtension: unexpected EOF"))
			Expect(pe).To(BeNil())
		})
	})

	When("there is no ProposalResponsePayload", func() {
		BeforeEach(func() {
			prp = &protomsg{}
		})

		It("returns an error", func() {
			pe, err := endorsertx.NewEndorserTx(txenv)
			Expect(err).To(MatchError("empty ProposalResponsePayload"))
			Expect(pe).To(BeNil())
		})
	})

	When("there is a bad ProposalResponsePayload", func() {
		BeforeEach(func() {
			prp = &protomsg{msg: []byte("barf")}
		})

		It("returns an error", func() {
			pe, err := endorsertx.NewEndorserTx(txenv)
			Expect(err).To(MatchError("error unmarshaling ProposalResponsePayload: unexpected EOF"))
			Expect(pe).To(BeNil())
		})
	})

	When("there is no ProposalResponsePayload", func() {
		BeforeEach(func() {
			prpExt = &protomsg{}
		})

		It("returns an error", func() {
			pe, err := endorsertx.NewEndorserTx(txenv)
			Expect(err).To(MatchError("nil Extension"))
			Expect(pe).To(BeNil())
		})
	})

	When("there is a bad ProposalResponsePayload", func() {
		BeforeEach(func() {
			prpExt = &protomsg{msg: []byte("barf")}
		})

		It("returns an error", func() {
			pe, err := endorsertx.NewEndorserTx(txenv)
			Expect(err).To(MatchError("error unmarshaling ChaincodeAction: unexpected EOF"))
			Expect(pe).To(BeNil())
		})
	})
})
