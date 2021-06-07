/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package peer

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric-protos-go/orderer"
	"github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/common/deliver"
	"github.com/hyperledger/fabric/common/ledger/blockledger"
	"github.com/hyperledger/fabric/common/metrics/disabled"
	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/rwsetutil"
	fake "github.com/hyperledger/fabric/core/peer/mock"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/metadata"
	peer2 "google.golang.org/grpc/peer"
)

// defaultPolicyCheckerProvider policy checker provider used by default,
// generates policy checker which always accepts regardless of arguments
// passed in
var defaultPolicyCheckerProvider = func(_ string) deliver.PolicyCheckerFunc {
	return func(_ *common.Envelope, _ string) error {
		return nil
	}
}

//go:generate counterfeiter -o mock/peer_ledger.go -fake-name PeerLedger . peerLedger

type peerLedger interface {
	ledger.PeerLedger
}

//go:generate counterfeiter -o mock/identity_deserializer_manager.go -fake-name IdentityDeserializerManager . identityDeserializerManager

type identityDeserializerManager interface {
	IdentityDeserializerManager
}

//go:generate counterfeiter -o mock/collection_policy_checker.go -fake-name CollectionPolicyChecker . collectionPolicyChecker

type collectionPolicyChecker interface {
	CollectionPolicyChecker
}

// mockIterator mock structure implementing
// the blockledger.Iterator interface
type mockIterator struct {
	mock.Mock
}

func (m *mockIterator) Next() (*common.Block, common.Status) {
	args := m.Called()
	return args.Get(0).(*common.Block), args.Get(1).(common.Status)
}

func (m *mockIterator) ReadyChan() <-chan struct{} {
	panic("implement me")
}

func (m *mockIterator) Close() {
}

// mockReader mock structure implementing
// the blockledger.Reader interface
type mockReader struct {
	mock.Mock
}

func (m *mockReader) Iterator(startType *orderer.SeekPosition) (blockledger.Iterator, uint64) {
	args := m.Called(startType)
	return args.Get(0).(blockledger.Iterator), args.Get(1).(uint64)
}

func (m *mockReader) Height() uint64 {
	args := m.Called()
	return args.Get(0).(uint64)
}

func (m *mockReader) RetrieveBlockByNumber(blockNum uint64) (*common.Block, error) {
	args := m.Called()
	return args.Get(0).(*common.Block), args.Error(1)
}

// mockChainSupport
type mockChainSupport struct {
	mock.Mock
}

func (m *mockChainSupport) Sequence() uint64 {
	return m.Called().Get(0).(uint64)
}

func (m *mockChainSupport) Ledger() ledger.PeerLedger {
	return m.Called().Get(0).(ledger.PeerLedger)
}

func (m *mockChainSupport) PolicyManager() policies.Manager {
	panic("implement me")
}

func (m *mockChainSupport) Reader() blockledger.Reader {
	return m.Called().Get(0).(blockledger.Reader)
}

func (*mockChainSupport) Errored() <-chan struct{} {
	return make(chan struct{})
}

// mockChainManager mock implementation of the ChainManager interface
type mockChainManager struct {
	mock.Mock
}

func (m *mockChainManager) GetChain(channelID string) deliver.Chain {
	args := m.Called(channelID)
	return args.Get(0).(deliver.Chain)
}

// mockDeliverServer mock implementation of the Deliver_DeliverServer
type mockDeliverServer struct {
	mock.Mock
}

func (m *mockDeliverServer) Context() context.Context {
	return m.Called().Get(0).(context.Context)
}

func (m *mockDeliverServer) Recv() (*common.Envelope, error) {
	args := m.Called()
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*common.Envelope), args.Error(1)
}

func (m *mockDeliverServer) Send(response *peer.DeliverResponse) error {
	args := m.Called(response)
	return args.Error(0)
}

func (*mockDeliverServer) RecvMsg(m interface{}) error {
	panic("implement me")
}

func (*mockDeliverServer) SendHeader(metadata.MD) error {
	panic("implement me")
}

func (*mockDeliverServer) SendMsg(m interface{}) error {
	panic("implement me")
}

func (*mockDeliverServer) SetHeader(metadata.MD) error {
	panic("implement me")
}

func (*mockDeliverServer) SetTrailer(metadata.MD) {
	panic("implement me")
}

type testConfig struct {
	channelID     string
	eventName     string
	chaincodeName string
	txID          string
	payload       *common.Payload
	*require.Assertions
}

type testCase struct {
	name    string
	prepare func(wg *sync.WaitGroup) (deliver.ChainManager, peer.Deliver_DeliverServer)
}

func TestFilteredBlockResponseSenderIsFiltered(t *testing.T) {
	var fbrs interface{} = &filteredBlockResponseSender{}
	filtered, ok := fbrs.(deliver.Filtered)
	require.True(t, ok, "should be filtered")
	require.True(t, filtered.IsFiltered(), "should return true from IsFiltered")
}

func TestEventsServer_DeliverFiltered(t *testing.T) {
	tests := []testCase{
		{
			name: "Testing deliver of the filtered block events",
			prepare: func(config testConfig) func(wg *sync.WaitGroup) (deliver.ChainManager, peer.Deliver_DeliverServer) {
				return func(wg *sync.WaitGroup) (deliver.ChainManager, peer.Deliver_DeliverServer) {
					wg.Add(2)
					p := &peer2.Peer{}
					chaincodeActionPayload, err := createChaincodeAction(config.chaincodeName, config.eventName, config.txID)
					config.NoError(err)

					chainManager := createDefaultSupportMamangerMock(config, chaincodeActionPayload, nil)
					// setup mock deliver server
					deliverServer := &mockDeliverServer{}
					deliverServer.On("Context").Return(peer2.NewContext(context.TODO(), p))

					deliverServer.On("Recv").Return(&common.Envelope{
						Payload: protoutil.MarshalOrPanic(config.payload),
					}, nil).Run(func(_ mock.Arguments) {
						// once we are getting new message we need to mock
						// Recv call to get io.EOF to stop the looping for
						// next message and we can assert for the DeliverResponse
						// value we are getting from the deliver server
						deliverServer.Mock = mock.Mock{}
						deliverServer.On("Context").Return(peer2.NewContext(context.TODO(), p))
						deliverServer.On("Recv").Return(&common.Envelope{}, io.EOF)
						deliverServer.On("Send", mock.Anything).Run(func(args mock.Arguments) {
							defer wg.Done()
							response := args.Get(0).(*peer.DeliverResponse)
							switch response.Type.(type) {
							case *peer.DeliverResponse_Status:
								config.Equal(common.Status_SUCCESS, response.GetStatus())
							case *peer.DeliverResponse_FilteredBlock:
								block := response.GetFilteredBlock()
								config.Equal(uint64(0), block.Number)
								config.Equal(config.channelID, block.ChannelId)
								config.Equal(1, len(block.FilteredTransactions))
								tx := block.FilteredTransactions[0]
								config.Equal(config.txID, tx.Txid)
								config.Equal(peer.TxValidationCode_VALID, tx.TxValidationCode)
								config.Equal(common.HeaderType_ENDORSER_TRANSACTION, tx.Type)
								transactionActions := tx.GetTransactionActions()
								config.NotNil(transactionActions)
								chaincodeActions := transactionActions.ChaincodeActions
								config.Equal(1, len(chaincodeActions))
								config.Equal(config.eventName, chaincodeActions[0].ChaincodeEvent.EventName)
								config.Equal(config.txID, chaincodeActions[0].ChaincodeEvent.TxId)
								config.Equal(config.chaincodeName, chaincodeActions[0].ChaincodeEvent.ChaincodeId)
							default:
								config.FailNow("Unexpected response type")
							}
						}).Return(nil)
					})

					return chainManager, deliverServer
				}
			}(testConfig{
				channelID:     "testChannelID",
				eventName:     "testEvent",
				chaincodeName: "mycc",
				txID:          "testID",
				payload: &common.Payload{
					Header: &common.Header{
						ChannelHeader: protoutil.MarshalOrPanic(&common.ChannelHeader{
							ChannelId: "testChannelID",
							Timestamp: util.CreateUtcTimestamp(),
						}),
						SignatureHeader: protoutil.MarshalOrPanic(&common.SignatureHeader{}),
					},
					Data: protoutil.MarshalOrPanic(&orderer.SeekInfo{
						Start:    &orderer.SeekPosition{Type: &orderer.SeekPosition_Specified{Specified: &orderer.SeekSpecified{Number: 0}}},
						Stop:     &orderer.SeekPosition{Type: &orderer.SeekPosition_Newest{Newest: &orderer.SeekNewest{}}},
						Behavior: orderer.SeekInfo_BLOCK_UNTIL_READY,
					}),
				},
				Assertions: require.New(t),
			}),
		},
		{
			name: "Testing deliver of the filtered block events with nil chaincode action payload",
			prepare: func(config testConfig) func(wg *sync.WaitGroup) (deliver.ChainManager, peer.Deliver_DeliverServer) {
				return func(wg *sync.WaitGroup) (deliver.ChainManager, peer.Deliver_DeliverServer) {
					wg.Add(2)
					p := &peer2.Peer{}
					chainManager := createDefaultSupportMamangerMock(config, nil, nil)

					// setup mock deliver server
					deliverServer := &mockDeliverServer{}
					deliverServer.On("Context").Return(peer2.NewContext(context.TODO(), p))

					deliverServer.On("Recv").Return(&common.Envelope{
						Payload: protoutil.MarshalOrPanic(config.payload),
					}, nil).Run(func(_ mock.Arguments) {
						// once we are getting new message we need to mock
						// Recv call to get io.EOF to stop the looping for
						// next message and we can assert for the DeliverResponse
						// value we are getting from the deliver server
						deliverServer.Mock = mock.Mock{}
						deliverServer.On("Context").Return(peer2.NewContext(context.TODO(), p))
						deliverServer.On("Recv").Return(&common.Envelope{}, io.EOF)
						deliverServer.On("Send", mock.Anything).Run(func(args mock.Arguments) {
							defer wg.Done()
							response := args.Get(0).(*peer.DeliverResponse)
							switch response.Type.(type) {
							case *peer.DeliverResponse_Status:
								config.Equal(common.Status_SUCCESS, response.GetStatus())
							case *peer.DeliverResponse_FilteredBlock:
								block := response.GetFilteredBlock()
								config.Equal(uint64(0), block.Number)
								config.Equal(config.channelID, block.ChannelId)
								config.Equal(1, len(block.FilteredTransactions))
								tx := block.FilteredTransactions[0]
								config.Equal(config.txID, tx.Txid)
								config.Equal(peer.TxValidationCode_VALID, tx.TxValidationCode)
								config.Equal(common.HeaderType_ENDORSER_TRANSACTION, tx.Type)
								transactionActions := tx.GetTransactionActions()
								config.NotNil(transactionActions)
								chaincodeActions := transactionActions.ChaincodeActions
								// we expecting to get zero chaincode action,
								// since provided nil payload
								config.Equal(0, len(chaincodeActions))
							default:
								config.FailNow("Unexpected response type")
							}
						}).Return(nil)
					})

					return chainManager, deliverServer
				}
			}(testConfig{
				channelID:     "testChannelID",
				eventName:     "testEvent",
				chaincodeName: "mycc",
				txID:          "testID",
				payload: &common.Payload{
					Header: &common.Header{
						ChannelHeader: protoutil.MarshalOrPanic(&common.ChannelHeader{
							ChannelId: "testChannelID",
							Timestamp: util.CreateUtcTimestamp(),
						}),
						SignatureHeader: protoutil.MarshalOrPanic(&common.SignatureHeader{}),
					},
					Data: protoutil.MarshalOrPanic(&orderer.SeekInfo{
						Start:    &orderer.SeekPosition{Type: &orderer.SeekPosition_Specified{Specified: &orderer.SeekSpecified{Number: 0}}},
						Stop:     &orderer.SeekPosition{Type: &orderer.SeekPosition_Newest{Newest: &orderer.SeekNewest{}}},
						Behavior: orderer.SeekInfo_BLOCK_UNTIL_READY,
					}),
				},
				Assertions: require.New(t),
			}),
		},
		{
			name: "Testing deliver of the filtered block events with nil payload header",
			prepare: func(config testConfig) func(wg *sync.WaitGroup) (deliver.ChainManager, peer.Deliver_DeliverServer) {
				return func(wg *sync.WaitGroup) (deliver.ChainManager, peer.Deliver_DeliverServer) {
					wg.Add(1)
					p := &peer2.Peer{}
					chainManager := createDefaultSupportMamangerMock(config, nil, nil)

					// setup mock deliver server
					deliverServer := &mockDeliverServer{}
					deliverServer.On("Context").Return(peer2.NewContext(context.TODO(), p))

					deliverServer.On("Recv").Return(&common.Envelope{
						Payload: protoutil.MarshalOrPanic(config.payload),
					}, nil).Run(func(_ mock.Arguments) {
						// once we are getting new message we need to mock
						// Recv call to get io.EOF to stop the looping for
						// next message and we can assert for the DeliverResponse
						// value we are getting from the deliver server
						deliverServer.Mock = mock.Mock{}
						deliverServer.On("Context").Return(peer2.NewContext(context.TODO(), p))
						deliverServer.On("Recv").Return(&common.Envelope{}, io.EOF)
						deliverServer.On("Send", mock.Anything).Run(func(args mock.Arguments) {
							defer wg.Done()
							response := args.Get(0).(*peer.DeliverResponse)
							switch response.Type.(type) {
							case *peer.DeliverResponse_Status:
								config.Equal(common.Status_BAD_REQUEST, response.GetStatus())
							case *peer.DeliverResponse_FilteredBlock:
								config.FailNow("Unexpected response type")
							default:
								config.FailNow("Unexpected response type")
							}
						}).Return(nil)
					})

					return chainManager, deliverServer
				}
			}(testConfig{
				channelID:     "testChannelID",
				eventName:     "testEvent",
				chaincodeName: "mycc",
				txID:          "testID",
				payload: &common.Payload{
					Header: nil,
					Data: protoutil.MarshalOrPanic(&orderer.SeekInfo{
						Start:    &orderer.SeekPosition{Type: &orderer.SeekPosition_Specified{Specified: &orderer.SeekSpecified{Number: 0}}},
						Stop:     &orderer.SeekPosition{Type: &orderer.SeekPosition_Newest{Newest: &orderer.SeekNewest{}}},
						Behavior: orderer.SeekInfo_BLOCK_UNTIL_READY,
					}),
				},
				Assertions: require.New(t),
			}),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			wg := &sync.WaitGroup{}
			chainManager, deliverServer := test.prepare(wg)

			metrics := deliver.NewMetrics(&disabled.Provider{})
			server := &DeliverServer{
				DeliverHandler:        deliver.NewHandler(chainManager, time.Second, false, metrics, false),
				PolicyCheckerProvider: defaultPolicyCheckerProvider,
			}

			err := server.DeliverFiltered(deliverServer)
			wg.Wait()
			// no error expected
			require.NoError(t, err)
		})
	}
}

func TestEventsServer_DeliverWithPrivateData(t *testing.T) {
	fakeDeserializerMgr := &fake.IdentityDeserializerManager{}
	fakeDeserializerMgr.DeserializerReturns(nil, nil)
	fakeCollPolicyChecker := &fake.CollectionPolicyChecker{}
	fakeCollPolicyChecker.CheckCollectionPolicyReturns(true, nil)
	tests := []testCase{
		{
			name: "Testing deliver block with private data events",
			prepare: func(config testConfig) func(wg *sync.WaitGroup) (deliver.ChainManager, peer.Deliver_DeliverServer) {
				return func(wg *sync.WaitGroup) (deliver.ChainManager, peer.Deliver_DeliverServer) {
					wg.Add(2)
					p := &peer2.Peer{}
					chaincodeActionPayload, err := createChaincodeAction(config.chaincodeName, config.eventName, config.txID)
					config.NoError(err)

					pvtData := []*ledger.TxPvtData{
						produceSamplePvtdataOrPanic(0, []string{"ns-0:coll-0", "ns-2:coll-20", "ns-2:coll-21"}),
					}
					chainManager := createDefaultSupportMamangerMock(config, chaincodeActionPayload, pvtData)

					deliverServer := &mockDeliverServer{}
					deliverServer.On("Context").Return(peer2.NewContext(context.TODO(), p))
					deliverServer.On("Recv").Return(&common.Envelope{
						Payload: protoutil.MarshalOrPanic(config.payload),
					}, nil).Run(func(_ mock.Arguments) {
						// mock Recv calls to get io.EOF to stop the looping for next message
						deliverServer.Mock = mock.Mock{}
						deliverServer.On("Context").Return(peer2.NewContext(context.TODO(), p))
						deliverServer.On("Recv").Return(&common.Envelope{}, io.EOF)
						deliverServer.On("Send", mock.Anything).Run(func(args mock.Arguments) {
							defer wg.Done()
							response := args.Get(0).(*peer.DeliverResponse)
							switch response.Type.(type) {
							case *peer.DeliverResponse_Status:
								config.Equal(common.Status_SUCCESS, response.GetStatus())
							case *peer.DeliverResponse_BlockAndPrivateData:
								blockAndPvtData := response.GetBlockAndPrivateData()
								block := blockAndPvtData.Block
								config.Equal(uint64(0), block.Header.Number)
								config.Equal(1, len(blockAndPvtData.PrivateDataMap))
								config.NotNil(blockAndPvtData.PrivateDataMap[uint64(0)])
								txPvtRwset := blockAndPvtData.PrivateDataMap[uint64(0)]
								// expect to have 2 NsPvtRwset (i.e., 2 namespaces)
								config.Equal(2, len(txPvtRwset.NsPvtRwset))
								// check namespace because the index may be out of order
								for _, nsPvtRwset := range txPvtRwset.NsPvtRwset {
									switch nsPvtRwset.Namespace {
									case "ns-0":
										config.Equal(1, len(nsPvtRwset.CollectionPvtRwset))
										config.Equal("coll-0", nsPvtRwset.CollectionPvtRwset[0].CollectionName)
									case "ns-2":
										config.Equal(2, len(nsPvtRwset.CollectionPvtRwset))
										config.Equal("coll-20", nsPvtRwset.CollectionPvtRwset[0].CollectionName)
										config.Equal("coll-21", nsPvtRwset.CollectionPvtRwset[1].CollectionName)
									default:
										config.FailNow("Wrong namespace " + nsPvtRwset.Namespace)
									}
								}
							default:
								config.FailNow("Unexpected response type")
							}
						}).Return(nil)
					})

					return chainManager, deliverServer
				}
			}(testConfig{
				channelID:     "testChannelID",
				eventName:     "testEvent",
				chaincodeName: "mycc",
				txID:          "testID",
				payload: &common.Payload{
					Header: &common.Header{
						ChannelHeader: protoutil.MarshalOrPanic(&common.ChannelHeader{
							ChannelId: "testChannelID",
							Timestamp: util.CreateUtcTimestamp(),
						}),
						SignatureHeader: protoutil.MarshalOrPanic(&common.SignatureHeader{}),
					},
					Data: protoutil.MarshalOrPanic(&orderer.SeekInfo{
						Start:    &orderer.SeekPosition{Type: &orderer.SeekPosition_Specified{Specified: &orderer.SeekSpecified{Number: 0}}},
						Stop:     &orderer.SeekPosition{Type: &orderer.SeekPosition_Newest{Newest: &orderer.SeekNewest{}}},
						Behavior: orderer.SeekInfo_BLOCK_UNTIL_READY,
					}),
				},
				Assertions: require.New(t),
			}),
		},
		{
			name: "Testing deliver of block with private data events with nil header",
			prepare: func(config testConfig) func(wg *sync.WaitGroup) (deliver.ChainManager, peer.Deliver_DeliverServer) {
				return func(wg *sync.WaitGroup) (deliver.ChainManager, peer.Deliver_DeliverServer) {
					wg.Add(1)
					p := &peer2.Peer{}
					chainManager := createDefaultSupportMamangerMock(config, nil, nil)

					deliverServer := &mockDeliverServer{}
					deliverServer.On("Context").Return(peer2.NewContext(context.TODO(), p))
					deliverServer.On("Recv").Return(&common.Envelope{
						Payload: protoutil.MarshalOrPanic(config.payload),
					}, nil).Run(func(_ mock.Arguments) {
						// mock Recv calls to get io.EOF to stop the looping for next message
						deliverServer.Mock = mock.Mock{}
						deliverServer.On("Context").Return(peer2.NewContext(context.TODO(), p))
						deliverServer.On("Recv").Return(&common.Envelope{}, io.EOF)
						deliverServer.On("Send", mock.Anything).Run(func(args mock.Arguments) {
							defer wg.Done()
							response := args.Get(0).(*peer.DeliverResponse)
							switch response.Type.(type) {
							case *peer.DeliverResponse_Status:
								config.Equal(common.Status_BAD_REQUEST, response.GetStatus())
							case *peer.DeliverResponse_BlockAndPrivateData:
								config.FailNow("Unexpected response type")
							default:
								config.FailNow("Unexpected response type")
							}
						}).Return(nil)
					})

					return chainManager, deliverServer
				}
			}(testConfig{
				channelID:     "testChannelID",
				eventName:     "testEvent",
				chaincodeName: "mycc",
				txID:          "testID",
				payload: &common.Payload{
					Header: nil,
					Data: protoutil.MarshalOrPanic(&orderer.SeekInfo{
						Start:    &orderer.SeekPosition{Type: &orderer.SeekPosition_Specified{Specified: &orderer.SeekSpecified{Number: 0}}},
						Stop:     &orderer.SeekPosition{Type: &orderer.SeekPosition_Newest{Newest: &orderer.SeekNewest{}}},
						Behavior: orderer.SeekInfo_BLOCK_UNTIL_READY,
					}),
				},
				Assertions: require.New(t),
			}),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			wg := &sync.WaitGroup{}
			chainManager, deliverServer := test.prepare(wg)

			metrics := deliver.NewMetrics(&disabled.Provider{})
			handler := deliver.NewHandler(chainManager, time.Second, false, metrics, false)
			server := &DeliverServer{
				DeliverHandler:          handler,
				PolicyCheckerProvider:   defaultPolicyCheckerProvider,
				CollectionPolicyChecker: fakeCollPolicyChecker,
				IdentityDeserializerMgr: fakeDeserializerMgr,
			}

			err := server.DeliverWithPrivateData(deliverServer)
			wg.Wait()
			// no error expected
			require.NoError(t, err)
		})
	}
}

func createDefaultSupportMamangerMock(config testConfig, chaincodeActionPayload *peer.ChaincodeActionPayload, pvtData []*ledger.TxPvtData) *mockChainManager {
	chainManager := &mockChainManager{}
	iter := &mockIterator{}
	reader := &mockReader{}
	chain := &mockChainSupport{}
	ldgr := &fake.PeerLedger{}

	payload, err := createEndorsement(config.channelID, config.txID, chaincodeActionPayload)
	config.NoError(err)

	payloadBytes, err := proto.Marshal(payload)
	config.NoError(err)

	block, err := createTestBlock([]*common.Envelope{{
		Payload:   payloadBytes,
		Signature: []byte{},
	}})
	config.NoError(err)

	iter.On("Next").Return(block, common.Status_SUCCESS)
	reader.On("Iterator", mock.Anything).Return(iter, uint64(1))
	reader.On("Height").Return(uint64(1))
	chain.On("Sequence").Return(uint64(0))
	chain.On("Reader").Return(reader)
	chain.On("Ledger").Return(ldgr)
	chainManager.On("GetChain", config.channelID).Return(chain, true)

	ldgr.GetPvtDataByNumReturns(pvtData, nil)

	return chainManager
}

func createEndorsement(channelID string, txID string, chaincodeActionPayload *peer.ChaincodeActionPayload) (*common.Payload, error) {
	var chActionBytes []byte
	var err error
	if chaincodeActionPayload != nil {
		chActionBytes, err = proto.Marshal(chaincodeActionPayload)
		if err != nil {
			return nil, err
		}
	}

	// the transaction
	txBytes, err := proto.Marshal(&peer.Transaction{
		Actions: []*peer.TransactionAction{
			{
				Payload: chActionBytes,
			},
		},
	})
	if err != nil {
		return nil, err
	}
	// channel header
	chdrBytes, err := proto.Marshal(&common.ChannelHeader{
		ChannelId: channelID,
		TxId:      txID,
		Type:      int32(common.HeaderType_ENDORSER_TRANSACTION),
	})
	if err != nil {
		return nil, err
	}

	// the payload
	payload := &common.Payload{
		Header: &common.Header{
			ChannelHeader: chdrBytes,
		},
		Data: txBytes,
	}
	return payload, nil
}

func createChaincodeAction(chaincodeName string, eventName string, txID string) (*peer.ChaincodeActionPayload, error) {
	// chaincode events
	eventsBytes, err := proto.Marshal(&peer.ChaincodeEvent{
		ChaincodeId: chaincodeName,
		EventName:   eventName,
		TxId:        txID,
	})
	if err != nil {
		return nil, err
	}

	// chaincode action
	actionBytes, err := proto.Marshal(&peer.ChaincodeAction{
		ChaincodeId: &peer.ChaincodeID{
			Name: chaincodeName,
		},
		Events: eventsBytes,
	})
	if err != nil {
		return nil, err
	}

	// proposal response
	proposalResBytes, err := proto.Marshal(&peer.ProposalResponsePayload{
		Extension: actionBytes,
	})
	if err != nil {
		return nil, err
	}

	// chaincode action
	chaincodeActionPayload := &peer.ChaincodeActionPayload{
		Action: &peer.ChaincodeEndorsedAction{
			ProposalResponsePayload: proposalResBytes,
			Endorsements:            []*peer.Endorsement{},
		},
	}
	return chaincodeActionPayload, err
}

func createTestBlock(data []*common.Envelope) (*common.Block, error) {
	// block
	block := &common.Block{
		Header: &common.BlockHeader{
			Number:   0,
			DataHash: []byte{},
		},
		Data:     &common.BlockData{},
		Metadata: &common.BlockMetadata{},
	}
	for _, d := range data {
		envBytes, err := proto.Marshal(d)
		if err != nil {
			return nil, err
		}

		block.Data.Data = append(block.Data.Data, envBytes)
	}
	// making up metadata
	block.Metadata.Metadata = make([][]byte, 4)
	block.Metadata.Metadata[common.BlockMetadataIndex_TRANSACTIONS_FILTER] = make([]byte, len(data))
	return block, nil
}

func produceSamplePvtdataOrPanic(txNum uint64, nsColls []string) *ledger.TxPvtData {
	builder := rwsetutil.NewRWSetBuilder()
	for _, nsColl := range nsColls {
		nsCollSplit := strings.Split(nsColl, ":")
		ns := nsCollSplit[0]
		coll := nsCollSplit[1]
		builder.AddToPvtAndHashedWriteSet(ns, coll, fmt.Sprintf("key-%s-%s", ns, coll), []byte(fmt.Sprintf("value-%s-%s", ns, coll)))
	}
	simRes, err := builder.GetTxSimulationResults()
	if err != nil {
		panic(err)
	}
	return &ledger.TxPvtData{SeqInBlock: txNum, WriteSet: simRes.PvtSimulationResults}
}
