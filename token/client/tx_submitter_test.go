/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package client_test

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/protos/common"
	ab "github.com/hyperledger/fabric/protos/orderer"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/hyperledger/fabric/token/client"
	"github.com/hyperledger/fabric/token/client/mock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

var _ = Describe("TxSubmitter", func() {
	var (
		channelID     string
		config        *client.ClientConfig
		broadcastResp *ab.BroadcastResponse
		deliverResp   *pb.DeliverResponse

		txEnvelope   *common.Envelope
		expectedTxid string

		fakeSigningIdentity *mock.SigningIdentity
		fakeBroadcast       *mock.Broadcast
		fakeDeliverFiltered *mock.DeliverFiltered
		fakeOrdererClient   *mock.OrdererClient
		fakeDeliverClient   *mock.DeliverClient

		txSubmitter *client.TxSubmitter
	)

	BeforeEach(func() {
		channelID = "test-channel"

		orderer := client.ConnectionConfig{
			Address: "fake_address",
		}
		committerPeer := client.ConnectionConfig{
			Address: "fake_address",
		}
		proverPeer := client.ConnectionConfig{
			Address: "fake_address",
		}
		config = &client.ClientConfig{
			ChannelID:     channelID,
			Orderer:       orderer,
			CommitterPeer: committerPeer,
			ProverPeer:    proverPeer,
		}

		fakeSigningIdentity = &mock.SigningIdentity{}
		fakeSigningIdentity.SerializeReturns([]byte("creator"), nil)
		fakeSigningIdentity.SignReturns([]byte("envelop-signature"), nil)

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
			Config:          config,
			SigningIdentity: fakeSigningIdentity,
			Creator:         []byte("creator"),
			OrdererClient:   fakeOrdererClient,
			DeliverClient:   fakeDeliverClient,
		}

		// prepare txEnvelope
		expectedTxid = "txid-12345"
		channelHeader := &common.ChannelHeader{TxId: expectedTxid}
		payload := &common.Payload{
			Header: &common.Header{
				ChannelHeader: ProtoMarshal(channelHeader),
			},
			Data: []byte("tx-data"),
		}
		txEnvelope = &common.Envelope{
			Payload:   ProtoMarshal(payload),
			Signature: []byte("envelop-signature"),
		}

		deliverResp = &pb.DeliverResponse{
			Type: &pb.DeliverResponse_FilteredBlock{
				FilteredBlock: createFilteredBlock(channelID, pb.TxValidationCode_VALID, expectedTxid),
			},
		}
		fakeDeliverFiltered.RecvReturns(deliverResp, nil)
	})

	Describe("Submit", func() {
		It("submits transaction when waitTimeout is 0", func() {
			ordererStatus, committed, err := txSubmitter.Submit(txEnvelope, 0)
			Expect(err).NotTo(HaveOccurred())
			Expect(*ordererStatus).To(Equal(common.Status_SUCCESS))
			Expect(committed).To(Equal(false))

			Expect(fakeBroadcast.SendCallCount()).To(Equal(1))
			Expect(fakeBroadcast.CloseSendCallCount()).To(Equal(1))
			Expect(fakeBroadcast.RecvCallCount()).To(Equal(2))
			envelope := fakeBroadcast.SendArgsForCall(0)
			Expect(envelope).To(Equal(txEnvelope))
			Expect(fakeDeliverFiltered.Invocations()).To(BeEmpty())
		})

		It("submits transaction when waitTimeout is greater than 0", func() {
			ordererStatus, committed, err := txSubmitter.Submit(txEnvelope, 10*time.Second)
			Expect(err).NotTo(HaveOccurred())
			Expect(*ordererStatus).To(Equal(common.Status_SUCCESS))
			Expect(committed).To(Equal(true))

			Expect(fakeBroadcast.SendCallCount()).To(Equal(1))
			Expect(fakeBroadcast.CloseSendCallCount()).To(Equal(1))
			Expect(fakeBroadcast.RecvCallCount()).To(Equal(2))
			envelope := fakeBroadcast.SendArgsForCall(0)
			Expect(envelope).To(Equal(txEnvelope))

			Expect(fakeDeliverFiltered.SendCallCount()).To(Equal(1))
			Expect(fakeDeliverFiltered.CloseSendCallCount()).To(Equal(1))
			Expect(fakeDeliverFiltered.RecvCallCount()).To(Equal(1))
		})

		Context("when OrdererClient fails to create broadcast", func() {
			BeforeEach(func() {
				fakeOrdererClient.NewBroadcastReturns(nil, errors.New("wild-banana"))
			})

			It("returns an error", func() {
				_, _, err := txSubmitter.Submit(txEnvelope, 0)
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
				_, _, err := txSubmitter.Submit(txEnvelope, 1)
				Expect(err).To(MatchError("wild-banana"))

				Expect(fakeBroadcast.Invocations()).To(BeEmpty())
				Expect(fakeDeliverFiltered.Invocations()).To(BeEmpty())
			})
		})

		Context("when Broadcast.Send returns error", func() {
			BeforeEach(func() {
				fakeBroadcast.SendReturns(errors.New("flying-banana"))
			})

			It("returns an error", func() {
				ordererStatus, committed, err := txSubmitter.Submit(txEnvelope, 0)
				Expect(err.Error()).To(ContainSubstring("flying-banana"))
				Expect(ordererStatus).To(BeNil())
				Expect(committed).To(Equal(false))
			})
		})

		Context("when DeliverFiltered.Send returns error", func() {
			BeforeEach(func() {
				fakeDeliverFiltered.SendReturns(errors.New("flying-banana"))
			})

			It("returns an error", func() {
				ordererStatus, committed, err := txSubmitter.Submit(txEnvelope, time.Second)
				Expect(err.Error()).To(ContainSubstring("flying-banana"))
				Expect(ordererStatus).To(BeNil())
				Expect(committed).To(Equal(false))
			})
		})

		Context("when Broadcast.Recv returns error", func() {
			BeforeEach(func() {
				fakeBroadcast.RecvReturnsOnCall(0, nil, errors.New("flying-banana"))
			})

			It("returns an error", func() {
				ordererStatus, committed, err := txSubmitter.Submit(txEnvelope, 0)
				Expect(err.Error()).To(ContainSubstring("flying-banana"))
				Expect(*ordererStatus).To(Equal(common.Status_UNKNOWN))
				Expect(committed).To(Equal(false))
			})
		})

		Context("when Broadcast.Recv returns a bad status", func() {
			BeforeEach(func() {
				resp := &ab.BroadcastResponse{Status: common.Status_UNKNOWN}
				fakeBroadcast.RecvReturnsOnCall(0, resp, nil)
			})

			It("returns an error", func() {
				ordererStatus, committed, err := txSubmitter.Submit(txEnvelope, 0)
				expectedErr := fmt.Sprintf("broadcast response error %d from orderer %s",
					int32(common.Status_UNKNOWN), config.Orderer.Address)
				Expect(err).To(MatchError(expectedErr))
				Expect(*ordererStatus).To(Equal(common.Status_UNKNOWN))
				Expect(committed).To(Equal(false))
			})
		})

		Context("when DeliverFiltered.Recv returns error", func() {
			BeforeEach(func() {
				fakeDeliverFiltered.RecvReturns(nil, errors.New("flying-pineapple"))
			})

			It("returns an error", func() {
				// set waitTimeout>0 so that it will call DeliverFiltered
				ordererStatus, committed, err := txSubmitter.Submit(txEnvelope, 10*time.Second)
				Expect(*ordererStatus).To(Equal(common.Status_SUCCESS))
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
				ordererStatus, committed, err := txSubmitter.Submit(txEnvelope, 10*time.Second)
				Expect(*ordererStatus).To(Equal(common.Status_SUCCESS))
				Expect(committed).To(Equal(false))
				Expect(err).To(MatchError(fmt.Sprintf("deliver completed with status (%s) before txid %s received from peer %s",
					common.Status_BAD_REQUEST, expectedTxid, config.CommitterPeer.Address)))
			})
		})

		Context("when DeliverFiltered.Recv returns invalid code", func() {
			BeforeEach(func() {
				deliverResp = &pb.DeliverResponse{
					Type: &pb.DeliverResponse_FilteredBlock{
						FilteredBlock: createFilteredBlock(channelID, pb.TxValidationCode_NOT_VALIDATED, expectedTxid),
					},
				}
				fakeDeliverFiltered.RecvReturns(deliverResp, nil)
			})

			It("returns an error", func() {
				ordererStatus, committed, err := txSubmitter.Submit(txEnvelope, 10*time.Second)
				Expect(*ordererStatus).To(Equal(common.Status_SUCCESS))
				Expect(committed).To(Equal(false))
				Expect(err).To(MatchError(fmt.Sprintf("transaction [%s] status is not valid: NOT_VALIDATED", expectedTxid)))
			})
		})

		Context("when SigningIdentity.Sign fails", func() {
			BeforeEach(func() {
				fakeSigningIdentity.SignReturns(nil, errors.New("banana-seesaw"))
			})

			It("returns an error", func() {
				ordererStatus, committed, err := txSubmitter.Submit(txEnvelope, time.Second)
				Expect(ordererStatus).To(BeNil())
				Expect(committed).To(Equal(false))
				Expect(err).To(MatchError("banana-seesaw"))
			})
		})

		Context("when envelope is nil", func() {
			It("returns an error", func() {
				ordererStatus, committed, err := txSubmitter.Submit(nil, time.Second)
				Expect(ordererStatus).To(BeNil())
				Expect(committed).To(Equal(false))
				Expect(err).To(MatchError("envelope is nil"))
			})
		})

		Context("when envelope has invalid payload", func() {
			BeforeEach(func() {
				txEnvelope = &common.Envelope{
					Payload:   []byte("invalid-payload"),
					Signature: []byte("envelop-signature"),
				}
			})

			It("returns an error", func() {
				ordererStatus, committed, err := txSubmitter.Submit(txEnvelope, 10*time.Second)
				Expect(ordererStatus).To(BeNil())
				Expect(committed).To(Equal(false))
				Expect(err.Error()).To(ContainSubstring("failed to unmarshal envelope payload"))
			})
		})

		Context("when envelope has invalid header", func() {
			BeforeEach(func() {
				payload := &common.Payload{
					Header: &common.Header{
						ChannelHeader: []byte("invalid-channel-header"),
					},
					Data: []byte("tx-data"),
				}
				txEnvelope = &common.Envelope{
					Payload:   ProtoMarshal(payload),
					Signature: []byte("envelop-signature"),
				}
			})

			It("returns an error", func() {
				ordererStatus, committed, err := txSubmitter.Submit(txEnvelope, 10*time.Second)
				Expect(ordererStatus).To(BeNil())
				Expect(committed).To(Equal(false))
				Expect(err.Error()).To(ContainSubstring("failed to unmarshal channel header"))
			})
		})
	})

	Describe("CreateTxEnvelope", func() {
		var (
			expectedChannelHeader *common.ChannelHeader
			txBytes               []byte
		)

		BeforeEach(func() {
			expectedChannelHeader = &common.ChannelHeader{
				Type:      int32(common.HeaderType_TOKEN_TRANSACTION),
				ChannelId: channelID,
				Epoch:     0,
				TxId:      "dynamically generated",
			}
			txBytes = []byte("serialized-token-transaction")
		})

		It("returns expected envelope", func() {
			txBytes := []byte("serialized-token-transaction")
			envelope, txid, err := txSubmitter.CreateTxEnvelope(txBytes)
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
			expectedTxid, err := protoutil.ComputeTxID(signatureHeader.Nonce, txSubmitter.Creator)
			Expect(err).NotTo(HaveOccurred())
			Expect(channelHeader.TxId).To(Equal(expectedTxid))

			Expect(fakeSigningIdentity.SignCallCount()).To(Equal(1))
			raw := fakeSigningIdentity.SignArgsForCall(0)
			Expect(raw).To(Equal(envelope.Payload))
		})

		Context("when SigningIdentity returns error", func() {
			BeforeEach(func() {
				fakeSigningIdentity.SignReturns(nil, errors.New("flying-pineapple"))
			})

			It("returns an error", func() {
				_, _, err := txSubmitter.CreateTxEnvelope(txBytes)
				Expect(err).To(MatchError("flying-pineapple"))
			})
		})
	})

	Describe("NewTxSubmitter", func() {
		var (
			config          *client.ClientConfig
			ordererListener net.Listener
			deliverListener net.Listener
			ordererServer   *grpc.Server
			deliverServer   *grpc.Server
		)

		BeforeEach(func() {
			// create listeners to get endpoints
			// grpc servers will be started in each test case with or without TLS
			var err error
			ordererListener, err = net.Listen("tcp", "127.0.0.1:")
			Expect(err).To(BeNil())

			deliverListener, err = net.Listen("tcp", "127.0.0.1:")
			Expect(err).To(BeNil())

			ordererEndpoint := ordererListener.Addr().String()
			deliverEndpoint := deliverListener.Addr().String()
			config = getClientConfig(true, channelID, ordererEndpoint, deliverEndpoint, "dummy_endpoint")
		})

		AfterEach(func() {
			if ordererListener != nil {
				ordererListener.Close()
			}
			if deliverListener != nil {
				deliverListener.Close()
			}
			if ordererServer != nil {
				ordererServer.Stop()
			}
			if deliverServer != nil {
				deliverServer.Stop()
			}
		})

		It("creates a TxSubmitter when TLS is enabled", func() {
			// start grpc servers with TLS
			ordererServerCert, err := tls.LoadX509KeyPair(
				"./testdata/crypto/ordererOrganizations/example.com/orderers/orderer.example.com/tls/server.crt",
				"./testdata/crypto/ordererOrganizations/example.com/orderers/orderer.example.com/tls/server.key",
			)
			Expect(err).NotTo(HaveOccurred())
			ordererServer = grpc.NewServer(grpc.Creds(credentials.NewTLS(&tls.Config{
				Certificates: []tls.Certificate{ordererServerCert},
			})))
			ab.RegisterAtomicBroadcastServer(ordererServer, &mock.AtomicBroadcastServer{})
			go ordererServer.Serve(ordererListener)

			deliverServerCert, err := tls.LoadX509KeyPair(
				"./testdata/crypto/peerOrganizations/org1.example.com/peers/peer0.org1.example.com/tls/server.crt",
				"./testdata/crypto/peerOrganizations/org1.example.com/peers/peer0.org1.example.com/tls/server.key",
			)
			Expect(err).NotTo(HaveOccurred())
			deliverServer = grpc.NewServer(grpc.Creds(credentials.NewTLS(&tls.Config{
				Certificates: []tls.Certificate{deliverServerCert},
			})))
			pb.RegisterDeliverServer(deliverServer, &mock.DeliverServer{})
			go deliverServer.Serve(deliverListener)

			submitter, err := client.NewTxSubmitter(config, fakeSigningIdentity)
			Expect(err).NotTo(HaveOccurred())
			Expect(submitter.Config).To(Equal(config))
			Expect(submitter.Creator).To(Equal([]byte("creator")))
			Expect(submitter.SigningIdentity).To(Equal(fakeSigningIdentity))

			// verify OrdererClient can create a broadcast client
			broadcastClient, err := submitter.OrdererClient.NewBroadcast(context.Background())
			Expect(err).NotTo(HaveOccurred())
			Expect(broadcastClient).NotTo(BeNil())

			// verify DeliverClient can create a deliverfiltered client
			dfClient, err := submitter.DeliverClient.NewDeliverFiltered(context.Background())
			Expect(err).NotTo(HaveOccurred())
			Expect(dfClient).NotTo(BeNil())
		})

		It("creates a TxSubmitter when orderer TLS is disabled", func() {
			config.Orderer.TLSEnabled = false
			config.CommitterPeer.TLSEnabled = false

			// start grpc servers without TLS
			ordererServer = grpc.NewServer()
			ab.RegisterAtomicBroadcastServer(ordererServer, &mock.AtomicBroadcastServer{})
			go ordererServer.Serve(ordererListener)

			deliverServer = grpc.NewServer()
			pb.RegisterDeliverServer(deliverServer, &mock.DeliverServer{})
			go deliverServer.Serve(deliverListener)

			submitter, err := client.NewTxSubmitter(config, fakeSigningIdentity)
			Expect(err).NotTo(HaveOccurred())
			Expect(submitter.Config).To(Equal(config))
			Expect(submitter.Creator).To(Equal([]byte("creator")))
			Expect(submitter.SigningIdentity).To(Equal(fakeSigningIdentity))

			// verify OrdererClient can create a broadcast client
			broadcastClient, err := submitter.OrdererClient.NewBroadcast(context.Background())
			Expect(err).NotTo(HaveOccurred())
			Expect(broadcastClient).NotTo(BeNil())

			// verify DeliverClient can create a deliver filtered client
			dfClient, err := submitter.DeliverClient.NewDeliverFiltered(context.Background())
			Expect(err).NotTo(HaveOccurred())
			Expect(dfClient).NotTo(BeNil())
		})

		Context("when it fails to connect to orderer", func() {
			BeforeEach(func() {
				// do not start orderer server so that it cannot connect to orderer
			})

			It("returns an error", func() {
				_, err := client.NewTxSubmitter(config, fakeSigningIdentity)
				Expect(err.Error()).To(ContainSubstring("failed to connect to orderer"))
			})
		})

		Context("when it fails to connect to committer peer", func() {
			BeforeEach(func() {
				// start orderer server but not deliver server so that it cannot connect to committer peer
				ordererServer = grpc.NewServer()
				go ordererServer.Serve(ordererListener)
				config.Orderer.TLSEnabled = false
			})

			It("returns an error", func() {
				_, err := client.NewTxSubmitter(config, fakeSigningIdentity)
				Expect(err.Error()).To(ContainSubstring("failed to connect to commit peer"))
			})
		})

		Context("when it failed to load root cert file", func() {
			BeforeEach(func() {
				config.Orderer.TLSRootCertFile = "./testdata/crypto/non-file"
			})

			It("returns an error", func() {
				_, err := client.NewTxSubmitter(config, fakeSigningIdentity)
				Expect(err.Error()).To(ContainSubstring("unable to load TLS cert from " + config.Orderer.TLSRootCertFile))
			})
		})

		Context("when SigningIdentity.Serialize fails", func() {
			BeforeEach(func() {
				fakeSigningIdentity.SerializeReturns(nil, errors.New("banana-seesaw"))
			})

			It("returns an error", func() {
				_, err := client.NewTxSubmitter(config, fakeSigningIdentity)
				Expect(err).To(MatchError("banana-seesaw"))
			})
		})
	})
})

var _ = Describe("Create an envelope", func() {
	var (
		fakeSigningIdentity *mock.SigningIdentity

		data             []byte
		header           *common.Header
		expectedPayload  []byte
		expectedEnvelope *common.Envelope
	)

	BeforeEach(func() {
		fakeSigningIdentity = &mock.SigningIdentity{}
		fakeSigningIdentity.SignReturns([]byte("envelop-signature"), nil)

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
			envelope, err := client.CreateEnvelope(data, header, fakeSigningIdentity)
			Expect(err).NotTo(HaveOccurred())
			Expect(envelope).To(Equal(expectedEnvelope))

			Expect(fakeSigningIdentity.SignCallCount()).To(Equal(1))
			raw := fakeSigningIdentity.SignArgsForCall(0)
			Expect(raw).To(Equal(expectedPayload))
		})

		Context("when SignIdentity returns error", func() {
			BeforeEach(func() {
				fakeSigningIdentity.SignReturns(nil, errors.New("flying-pineapple"))
			})

			It("returns an error", func() {
				_, err := client.CreateEnvelope(data, header, fakeSigningIdentity)
				Expect(err).To(MatchError("flying-pineapple"))
			})
		})
	})
})

var _ = Describe("Create a header", func() {
	var (
		channelID             string
		txType                common.HeaderType
		creator               []byte
		expectedChannelHeader *common.ChannelHeader
	)

	BeforeEach(func() {
		channelID = "test-channel"
		txType = common.HeaderType_TOKEN_TRANSACTION
		creator = []byte("creator")

		// expected fields for channel header
		expectedChannelHeader = &common.ChannelHeader{
			Type:      int32(txType),
			ChannelId: channelID,
			Epoch:     uint64(0),
			TxId:      "dynamically generated",
		}
	})

	Describe("CreateHeader", func() {
		It("returns expected header", func() {
			txid, header, err := client.CreateHeader(txType, channelID, creator, nil)
			Expect(err).NotTo(HaveOccurred())

			channelHeader := common.ChannelHeader{}
			err = proto.Unmarshal(header.ChannelHeader, &channelHeader)
			Expect(err).NotTo(HaveOccurred())

			signatureHeader := common.SignatureHeader{}
			err = proto.Unmarshal(header.SignatureHeader, &signatureHeader)
			Expect(err).NotTo(HaveOccurred())

			expectedTxid, err := protoutil.ComputeTxID(signatureHeader.Nonce, creator)
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
