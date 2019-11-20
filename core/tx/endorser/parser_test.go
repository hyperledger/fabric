/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package endorsertx_test

import (
	"encoding/hex"

	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/common/configtx"
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
		chHeader    *common.ChannelHeader
		sigHeader   *common.SignatureHeader
	)

	BeforeEach(func() {
		prp, hdrExt, prpExt, payloadData = nil, nil, nil, nil

		chHeader = &common.ChannelHeader{
			ChannelId: "my-channel",
			Epoch:     0,
		}

		sigHeader = &common.SignatureHeader{
			Nonce:   []byte("1234"),
			Creator: []byte("creator"),
		}
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

		chHeader.Extension = hdrExtBytes

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

		When("there is a bad epoch", func() {
			BeforeEach(func() {
				chHeader = &common.ChannelHeader{
					ChannelId: "my-channel",
					Epoch:     35,
				}
			})

			It("returns an error", func() {
				pe, err := endorsertx.NewEndorserTx(txenv)
				Expect(err).To(MatchError("invalid Epoch in ChannelHeader. Expected 0, got [35]"))
				Expect(pe).To(BeNil())
			})
		})

		When("there is a bad version", func() {
			BeforeEach(func() {
				chHeader = &common.ChannelHeader{
					ChannelId: "my-channel",
					Version:   35,
				}
			})

			It("returns an error", func() {
				pe, err := endorsertx.NewEndorserTx(txenv)
				Expect(err).To(MatchError("invalid version in ChannelHeader. Expected 0, got [35]"))
				Expect(pe).To(BeNil())
			})
		})

		When("there is an empty channel name", func() {
			BeforeEach(func() {
				chHeader = &common.ChannelHeader{
					ChannelId: "",
				}
			})

			It("returns an error", func() {
				pe, err := endorsertx.NewEndorserTx(txenv)
				Expect(err).To(MatchError("channel ID illegal, cannot be empty"))
				Expect(pe).To(BeNil())
			})
		})

		When("there is an invalid channel name", func() {
			BeforeEach(func() {
				chHeader = &common.ChannelHeader{
					ChannelId: ".foo",
				}
			})

			It("returns an error", func() {
				pe, err := endorsertx.NewEndorserTx(txenv)
				Expect(err).To(MatchError("'.foo' contains illegal characters"))
				Expect(pe).To(BeNil())
			})
		})

		When("there is an empty nonce", func() {
			BeforeEach(func() {
				sigHeader = &common.SignatureHeader{
					Creator: []byte("creator"),
				}
			})

			It("returns an error", func() {
				pe, err := endorsertx.NewEndorserTx(txenv)
				Expect(err).To(MatchError("empty nonce"))
				Expect(pe).To(BeNil())
			})
		})

		When("there is an empty creator", func() {
			BeforeEach(func() {
				sigHeader = &common.SignatureHeader{
					Nonce: []byte("1234"),
				}
			})

			It("returns an error", func() {
				pe, err := endorsertx.NewEndorserTx(txenv)
				Expect(err).To(MatchError("empty creator"))
				Expect(pe).To(BeNil())
			})
		})

		When("there is no chaincode ID", func() {
			BeforeEach(func() {
				// annoyingly, it's not easy to get a nonzero length
				// marshalling of a proto message with zero values
				// everywhere. We simulate this condition by adding
				// extra bytes for a non-existent second field that
				// our unmarshaler will skip. Still, the presence of
				// an extraneous field will get the unmarshaler to
				// return a non-nil struct
				bytes, err := hex.DecodeString("1a046369616f")
				Expect(err).To(BeNil())
				hdrExt = &protomsg{
					msg: bytes,
				}
			})

			It("returns an error", func() {
				pe, err := endorsertx.NewEndorserTx(txenv)
				Expect(err).To(MatchError("nil ChaincodeId"))
				Expect(pe).To(BeNil())
			})
		})

		When("there is an empty chaincode name", func() {
			BeforeEach(func() {
				hdrExt = &protomsg{
					msg: protoutil.MarshalOrPanic(
						&peer.ChaincodeHeaderExtension{
							ChaincodeId: &peer.ChaincodeID{},
						},
					),
				}
			})

			It("returns an error", func() {
				pe, err := endorsertx.NewEndorserTx(txenv)
				Expect(err).To(MatchError("empty chaincode name in chaincode id"))
				Expect(pe).To(BeNil())
			})
		})
	})

	Describe("Validation of the channel ID", func() {
		Context("the used constants", func() {
			It("ensures that are kept in sync", func() {
				Expect(endorsertx.ChannelAllowedChars).To(Equal(configtx.ChannelAllowedChars))
				Expect(endorsertx.MaxLength).To(Equal(configtx.MaxLength))
			})
		})

		Context("the validation function", func() {
			It("behaves as the one in the configtx package", func() {
				err1 := endorsertx.ValidateChannelID(randomLowerAlphaString(endorsertx.MaxLength + 1))
				err2 := configtx.ValidateChannelID(randomLowerAlphaString(endorsertx.MaxLength + 1))
				Expect(err1).To(HaveOccurred())
				Expect(err2).To(HaveOccurred())
				Expect(err1.Error()).To(Equal(err2.Error()))

				err1 = endorsertx.ValidateChannelID("foo_bar")
				err2 = configtx.ValidateChannelID("foo_bar")
				Expect(err1).To(HaveOccurred())
				Expect(err2).To(HaveOccurred())
				Expect(err1.Error()).To(Equal(err2.Error()))

				err1 = endorsertx.ValidateChannelID("8foo")
				err2 = configtx.ValidateChannelID("8foo")
				Expect(err1).To(HaveOccurred())
				Expect(err2).To(HaveOccurred())
				Expect(err1.Error()).To(Equal(err2.Error()))

				err1 = endorsertx.ValidateChannelID(".foo")
				err2 = configtx.ValidateChannelID(".foo")
				Expect(err1).To(HaveOccurred())
				Expect(err2).To(HaveOccurred())
				Expect(err1.Error()).To(Equal(err2.Error()))

				err1 = endorsertx.ValidateChannelID("")
				err2 = configtx.ValidateChannelID("")
				Expect(err1).To(HaveOccurred())
				Expect(err2).To(HaveOccurred())
				Expect(err1.Error()).To(Equal(err2.Error()))

				err1 = endorsertx.ValidateChannelID("f-oo.bar")
				err2 = configtx.ValidateChannelID("f-oo.bar")
				Expect(err1).NotTo(HaveOccurred())
				Expect(err2).NotTo(HaveOccurred())
			})
		})
	})
})
