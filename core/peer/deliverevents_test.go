/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package peer

import (
	"io"
	"sync"
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/deliver"
	"github.com/hyperledger/fabric/common/ledger/blockledger"
	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/orderer"
	"github.com/hyperledger/fabric/protos/peer"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"golang.org/x/net/context"
	"google.golang.org/grpc/metadata"
	peer2 "google.golang.org/grpc/peer"
)

// defaultPolicyCheckerProvider policy checker provider used by default,
// generates policy checker which always accepts regardless of arguments
// passed in
var defaultPolicyCheckerProvider = func(_ string) deliver.PolicyChecker {
	return func(_ *common.Envelope, _ string) error {
		return nil
	}
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

// mockChainSupport
type mockChainSupport struct {
	mock.Mock
}

func (m *mockChainSupport) Sequence() uint64 {
	return m.Called().Get(0).(uint64)
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

// mockSupportManager mock implementation of the SupportManager interface
type mockSupportManager struct {
	mock.Mock
}

func (m *mockSupportManager) GetChain(chainID string) (deliver.Support, bool) {
	args := m.Called(chainID)
	return args.Get(0).(deliver.Support), args.Get(1).(bool)
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

func TestEventsServer_DeliverFiltered(t *testing.T) {
	viper.Set("peer.authentication.timewindow", "1s")

	tests := []struct {
		name          string
		channelID     string
		eventName     string
		chaincodeName string
		txID          string
		*assert.Assertions
		supportManager *mockSupportManager
		iter           *mockIterator
		reader         *mockReader
		chain          *mockChainSupport
	}{
		{
			name:           "Testing deliver of the filtered block events",
			channelID:      "testChainID",
			eventName:      "testEvent",
			chaincodeName:  "mycc",
			txID:           "testID",
			supportManager: &mockSupportManager{},
			iter:           &mockIterator{},
			reader:         &mockReader{},
			chain:          &mockChainSupport{},
			Assertions:     assert.New(t),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			// setup the testing mocks
			server := NewDeliverEventsServer(false, defaultPolicyCheckerProvider, test.supportManager)
			// setup mock deliver server
			deliverServer := &mockDeliverServer{}
			p := &peer2.Peer{}

			wg := sync.WaitGroup{}
			// we need 2 since we expecting to get the block and the status
			// response once deliver API is called
			wg.Add(2)

			chaincodeActionPayload, err := createChaincodeAction(test.chaincodeName, test.eventName, test.txID)
			test.NoError(err)

			payload, err := createEndorsement(chaincodeActionPayload, test.channelID, test.txID)
			test.NoError(err)

			payloadBytes, err := proto.Marshal(payload)
			test.NoError(err)

			block, err := createTestBlock([]*common.Envelope{{
				Payload:   payloadBytes,
				Signature: []byte{}}})
			test.NoError(err)

			test.iter.On("Next").Return(block, common.Status_SUCCESS)

			test.reader.On("Iterator", mock.Anything).Return(test.iter, uint64(1))
			test.reader.On("Height").Return(uint64(1))

			test.chain.On("Sequence").Return(uint64(0))
			test.chain.On("Reader").Return(test.reader)

			test.supportManager.On("GetChain", test.channelID).Return(test.chain, true)

			deliverServer.On("Context").Return(peer2.NewContext(context.TODO(), p))

			deliverServer.On("Recv").Return(&common.Envelope{
				Payload: utils.MarshalOrPanic(&common.Payload{
					Header: &common.Header{
						ChannelHeader: utils.MarshalOrPanic(&common.ChannelHeader{
							ChannelId: test.channelID,
							Timestamp: util.CreateUtcTimestamp(),
						}),
						SignatureHeader: utils.MarshalOrPanic(&common.SignatureHeader{}),
					},
					Data: utils.MarshalOrPanic(&orderer.SeekInfo{
						Start:    &orderer.SeekPosition{Type: &orderer.SeekPosition_Specified{Specified: &orderer.SeekSpecified{Number: 0}}},
						Stop:     &orderer.SeekPosition{Type: &orderer.SeekPosition_Newest{Newest: &orderer.SeekNewest{}}},
						Behavior: orderer.SeekInfo_BLOCK_UNTIL_READY,
					}),
				}),
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
						test.Equal(common.Status_SUCCESS, response.GetStatus())
					case *peer.DeliverResponse_FilteredBlock:
						block := response.GetFilteredBlock()
						test.Equal(uint64(0), block.Number)
						test.Equal(test.channelID, block.ChannelId)
						test.Equal(1, len(block.FilteredTx))
						tx := block.FilteredTx[0]
						test.Equal(test.txID, tx.Txid)
						test.Equal(peer.TxValidationCode_VALID, tx.TxValidationCode)
						test.Equal(common.HeaderType_ENDORSER_TRANSACTION, tx.Type)
						proposalResponse := tx.GetProposalResponse()
						test.NotNil(proposalResponse)
						chaincodeActions := proposalResponse.ChaincodeActions
						test.Equal(1, len(chaincodeActions))
						test.Equal(test.eventName, chaincodeActions[0].CcEvent.EventName)
						test.Equal(test.txID, chaincodeActions[0].CcEvent.TxId)
						test.Equal(test.chaincodeName, chaincodeActions[0].CcEvent.ChaincodeId)
					default:
						test.FailNow("Unexpected response type")
					}
				}).Return(nil)

			})

			err = server.DeliverFiltered(deliverServer)
			wg.Wait()
			// no error expected
			test.NoError(err)
		})
	}
}

func createEndorsement(chaincodeActionPayload *peer.ChaincodeActionPayload, channelID string, txID string) (*common.Payload, error) {
	chActionBytes, err := proto.Marshal(chaincodeActionPayload)
	if err != nil {
		return nil, err
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
