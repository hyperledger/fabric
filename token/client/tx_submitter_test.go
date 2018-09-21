/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package client_test

import (
	"io"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/protos/common"
	ab "github.com/hyperledger/fabric/protos/orderer"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/hyperledger/fabric/protos/token"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/hyperledger/fabric/token/client"
	"github.com/hyperledger/fabric/token/client/mock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
)

var _ = Describe("TxSubmitter", func() {
	var (
		channelId     string
		config        *client.ClientConfig
		broadcastResp *ab.BroadcastResponse
		deliverResp   *pb.DeliverResponse

		txBytes               []byte
		tokenTx               *token.TokenTransaction
		txEnvelope            *common.Envelope
		expectedTxid          string
		expectedChannelHeader *common.ChannelHeader

		fakeSigner          *mock.SignerIdentity
		fakeBroadcast       *mock.Broadcast
		fakeDeliverFiltered *mock.DeliverFiltered
		fakeOrdererClient   *mock.OrdererClient
		fakeDeliverClient   *mock.DeliverClient

		txSubmitter *client.TxSubmitter
	)

	BeforeEach(func() {
		channelId = "test-channel"

		ordererCfg := client.ConnectionConfig{
			Address: "fake_address",
		}

		commitPeerCfg := client.ConnectionConfig{
			Address: "fake_address",
		}

		config = &client.ClientConfig{
			ChannelId:     channelId,
			TlsEnabled:    false,
			OrdererCfg:    ordererCfg,
			CommitPeerCfg: commitPeerCfg,
		}

		fakeSigner = &mock.SignerIdentity{}
		fakeSigner.SerializeReturns([]byte("creator"), nil)
		fakeSigner.SignReturns([]byte("envelop-signature"), nil)

		broadcastResp = &ab.BroadcastResponse{Status: common.Status_SUCCESS}
		fakeBroadcast = &mock.Broadcast{}
		fakeBroadcast.SendReturns(nil)
		fakeBroadcast.CloseSendReturns(nil)
		fakeBroadcast.RecvReturnsOnCall(0, broadcastResp, nil)
		fakeBroadcast.RecvReturnsOnCall(1, nil, io.EOF)

		fakeOrdererClient = &mock.OrdererClient{}
		fakeOrdererClient.NewBroadcastReturns(fakeBroadcast, nil)
		fakeOrdererClient.CertificateReturns(nil)

		fakeDeliverFiltered = &mock.DeliverFiltered{}
		fakeDeliverFiltered.SendReturns(nil)
		fakeDeliverFiltered.CloseSendReturns(nil)

		fakeDeliverClient = &mock.DeliverClient{}
		fakeDeliverClient.NewDeliverFilteredReturns(fakeDeliverFiltered, nil)
		fakeDeliverClient.CertificateReturns(nil)

		txSubmitter = &client.TxSubmitter{
			Config:        config,
			Signer:        fakeSigner,
			Creator:       []byte("creator"),
			OrdererClient: fakeOrdererClient,
			DeliverClient: fakeDeliverClient,
		}

		tokenTx = &token.TokenTransaction{
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
		txBytes, _ = proto.Marshal(tokenTx)
		expectedTxid, txEnvelope, _ = txSubmitter.CreateTxEnvelope(txBytes)

		// expected fields for channel header - exclude dynamically generated fields
		expectedChannelHeader = &common.ChannelHeader{
			Type:      int32(common.HeaderType_TOKEN_TRANSACTION),
			ChannelId: channelId,
			Epoch:     uint64(0),
			TxId:      "dynamically generated",
		}

		deliverResp = &pb.DeliverResponse{
			Type: &pb.DeliverResponse_FilteredBlock{
				FilteredBlock: createFilteredBlock(channelId, expectedTxid),
			},
		}
		fakeDeliverFiltered.RecvReturns(deliverResp, nil)
	})

	Describe("SubmitTransaction", func() {
		It("receives transaction commit event from event channel", func() {
			eventCh := make(chan client.TxEvent, 1)
			_, txid, err := txSubmitter.SubmitTransactionWithChan(txEnvelope, eventCh)
			Expect(err).NotTo(HaveOccurred())
			Expect(txid).To(Equal(expectedTxid))

			// read from eventCh and verify tx is committed
			select {
			case event, _ := <-eventCh:
				Expect(event.Committed).To(Equal(true))
				Expect(event.Txid).To(Equal(txid))
				Expect(event.Err).NotTo(HaveOccurred())
			}

			// Sign method should be called twice, 1st one for tx envelope, 2nd one for deliverfiltered envelope
			Expect(fakeSigner.SignCallCount()).To(Equal(2))
			raw := fakeSigner.SignArgsForCall(0)
			payload := &common.Payload{}
			err = proto.Unmarshal(raw, payload)
			Expect(err).NotTo(HaveOccurred())
			Expect(payload.Data).To(Equal(txBytes))
		})

		Context("when eventCh buffer size is 0", func() {
			It("returns an error", func() {
				eventCh := make(chan client.TxEvent, 0)
				_, _, err := txSubmitter.SubmitTransactionWithChan(txEnvelope, eventCh)
				Expect(err).To(MatchError("eventCh buffer size must be greater than 0"))
			})
		})

		Context("when eventCh buffer is full", func() {
			It("returns an error", func() {
				eventCh := make(chan client.TxEvent, 1)
				eventCh <- client.TxEvent{}
				_, _, err := txSubmitter.SubmitTransactionWithChan(txEnvelope, eventCh)
				Expect(err).To(MatchError("eventCh buffer is full. Read events and try again"))
			})
		})

		Context("when OrdererClient fails to create broadcast", func() {
			BeforeEach(func() {
				fakeOrdererClient.NewBroadcastReturns(nil, errors.New("wild-banana"))
			})

			It("returns an error", func() {
				_, _, err := txSubmitter.SubmitTransaction(txEnvelope, 0)
				Expect(err).To(MatchError("wild-banana"))

				Expect(fakeBroadcast.Invocations()).To(BeEmpty())
				Expect(fakeDeliverFiltered.Invocations()).To(BeEmpty())
			})
		})

		Context("when DeliverClient fails to create deliverfiltered", func() {
			BeforeEach(func() {
				fakeDeliverClient.NewDeliverFilteredReturns(nil, errors.New("wild-banana"))
			})

			It("returns an error", func() {
				// set waitTimeInSeconds>0 so that it will call DeliverClient
				_, _, err := txSubmitter.SubmitTransaction(txEnvelope, 1)
				Expect(err).To(MatchError("wild-banana"))

				Expect(fakeBroadcast.Invocations()).To(BeEmpty())
				Expect(fakeDeliverFiltered.Invocations()).To(BeEmpty())
			})
		})

		Context("when Broadcast.Recv returns error", func() {
			BeforeEach(func() {
				fakeBroadcast.RecvReturnsOnCall(1, nil, errors.New("flying-banana"))
			})

			It("returns an error", func() {
				committed, _, err := txSubmitter.SubmitTransaction(txEnvelope, 0)
				Expect(committed).To(Equal(false))
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("flying-banana"))
			})
		})

		Context("when DeliverFiltered.Recv returns error", func() {
			BeforeEach(func() {
				fakeDeliverFiltered.RecvReturns(nil, errors.New("flying-pineapple"))
			})

			It("returns an error", func() {
				// set waitTimeInSeconds>0 so that it will call DeliverFiltered
				committed, _, err := txSubmitter.SubmitTransaction(txEnvelope, 1)
				Expect(committed).To(Equal(false))
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("flying-pineapple"))
			})
		})

		Context("when DeliverFiltered.Recv returns DeliverResponse_Status", func() {
			BeforeEach(func() {
				deliverResp = &pb.DeliverResponse{
					Type: &pb.DeliverResponse_Status{
						Status: common.Status_BAD_REQUEST,
					},
				}
				fakeDeliverFiltered.RecvReturns(deliverResp, nil)
			})

			It("returns an error", func() {
				// pass eventCh and verify an event with error is received
				eventCh := make(chan client.TxEvent, 1)
				committed, txid, err := txSubmitter.SubmitTransactionWithChan(txEnvelope, eventCh)
				Expect(committed).To(Equal(false))
				Expect(err).NotTo(HaveOccurred())

				// read from eventCh and verify tx is not committed
				select {
				case event, _ := <-eventCh:
					Expect(event.Committed).To(Equal(false))
					Expect(event.Txid).To(Equal(txid))
					Expect(event.Err.Error()).To(ContainSubstring("deliver completed with status (BAD_REQUEST)"))
				}
			})
		})
	})

	Describe("CreateTxEnvelope", func() {
		It("returns expected envelope", func() {
			txid, envelope, err := txSubmitter.CreateTxEnvelope(txBytes)
			Expect(err).NotTo(HaveOccurred())

			payload := common.Payload{}
			err = proto.Unmarshal(envelope.Payload, &payload)
			Expect(err).NotTo(HaveOccurred())

			// verify payload data
			Expect(payload.Data).To(Equal(txBytes))

			// verify channel header
			channelHeader := common.ChannelHeader{}
			err = proto.Unmarshal(payload.Header.ChannelHeader, &channelHeader)
			Expect(err).NotTo(HaveOccurred())
			Expect(channelHeader.ChannelId).To(Equal(expectedChannelHeader.ChannelId))
			Expect(channelHeader.Type).To(Equal(expectedChannelHeader.Type))
			Expect(channelHeader.Epoch).To(Equal(expectedChannelHeader.Epoch))
			Expect(channelHeader.TxId).To(Equal(txid))

			// verify signature header
			signatureHeader := common.SignatureHeader{}
			err = proto.Unmarshal(payload.Header.SignatureHeader, &signatureHeader)
			Expect(err).NotTo(HaveOccurred())
			Expect(signatureHeader.Creator).To(Equal(txSubmitter.Creator))

			// verify txid
			expectedTxid, err := utils.ComputeTxID(signatureHeader.Nonce, txSubmitter.Creator)
			Expect(err).NotTo(HaveOccurred())
			Expect(channelHeader.TxId).To(Equal(expectedTxid))

			// 1st call is by CreateTxEnvelope in BeforeEach
			Expect(fakeSigner.SignCallCount()).To(Equal(2))
			raw := fakeSigner.SignArgsForCall(1)
			Expect(raw).To(Equal(envelope.Payload))
		})

		Context("when SignerIdentity returns error", func() {
			BeforeEach(func() {
				fakeSigner.SignReturns(nil, errors.New("flying-pineapple"))
			})

			It("returns an error", func() {
				_, _, err := txSubmitter.CreateTxEnvelope(txBytes)
				Expect(err).To(MatchError("flying-pineapple"))
			})
		})
	})
})

var _ = Describe("Create an envelope", func() {
	var (
		fakeSigner *mock.SignerIdentity

		// CreateEnvelope(data []byte, header *common.Header, signer SignerIdentity)
		data             []byte
		header           *common.Header
		expectedPayload  []byte
		expectedEnvelope *common.Envelope
	)

	BeforeEach(func() {
		fakeSigner = &mock.SignerIdentity{}
		fakeSigner.SignReturns([]byte("envelop-signature"), nil)

		data = []byte("tx-data")
		header = &common.Header{}
		expectedPayload = ProtoMarshal(&common.Payload{
			Header: header,
			Data:   data,
		})
		expectedEnvelope = &common.Envelope{
			Payload:   expectedPayload,
			Signature: []byte("envelop-signature"),
		}
	})

	Describe("CreateEnvelope", func() {
		It("returns expected envelope", func() {
			envelope, err := client.CreateEnvelope(data, header, fakeSigner)
			Expect(err).NotTo(HaveOccurred())
			Expect(envelope).To(Equal(expectedEnvelope))

			Expect(fakeSigner.SignCallCount()).To(Equal(1))
			raw := fakeSigner.SignArgsForCall(0)
			Expect(raw).To(Equal(expectedPayload))
		})

		Context("when SignerIdentity returns error", func() {
			BeforeEach(func() {
				fakeSigner.SignReturns(nil, errors.New("flying-pineapple"))
			})

			It("returns an error", func() {
				_, err := client.CreateEnvelope(data, header, fakeSigner)
				Expect(err).To(MatchError("flying-pineapple"))
			})
		})
	})
})

var _ = Describe("Create a header", func() {
	var (
		channelId             string
		txType                common.HeaderType
		creator               []byte
		expectedChannelHeader *common.ChannelHeader
	)

	BeforeEach(func() {
		channelId = "test-channel"
		txType = common.HeaderType_TOKEN_TRANSACTION
		creator = []byte("creator")

		// expected fields for channel header
		expectedChannelHeader = &common.ChannelHeader{
			Type:      int32(txType),
			ChannelId: channelId,
			Epoch:     uint64(0),
			TxId:      "dynamically generated",
		}
	})

	Describe("CreateHeader", func() {
		It("returns expected header", func() {
			txid, header, err := client.CreateHeader(txType, channelId, creator, nil)

			channelHeader := common.ChannelHeader{}
			err = proto.Unmarshal(header.ChannelHeader, &channelHeader)
			Expect(err).NotTo(HaveOccurred())

			signatureHeader := common.SignatureHeader{}
			err = proto.Unmarshal(header.SignatureHeader, &signatureHeader)
			Expect(err).NotTo(HaveOccurred())

			expectedTxid, err := utils.ComputeTxID(signatureHeader.Nonce, creator)
			Expect(txid).To(Equal(expectedTxid))
			Expect(err).NotTo(HaveOccurred())

			// validate each field except for nonce and timestamp because they are dynamically generated
			Expect(channelHeader.ChannelId).To(Equal(expectedChannelHeader.ChannelId))
			Expect(channelHeader.Type).To(Equal(expectedChannelHeader.Type))
			Expect(channelHeader.Epoch).To(Equal(expectedChannelHeader.Epoch))
			Expect(channelHeader.TxId).To(Equal(txid))
			Expect(signatureHeader.Creator).To(Equal(creator))
		})
	})
})

func createFilteredBlock(channelId string, txIDs ...string) *pb.FilteredBlock {
	var filteredTransactions []*pb.FilteredTransaction
	for _, txID := range txIDs {
		ft := &pb.FilteredTransaction{
			Txid:             txID,
			TxValidationCode: pb.TxValidationCode_VALID,
		}
		filteredTransactions = append(filteredTransactions, ft)
	}
	fb := &pb.FilteredBlock{
		Number:               0,
		ChannelId:            channelId,
		FilteredTransactions: filteredTransactions,
	}
	return fb
}

func getTxid() {

}
