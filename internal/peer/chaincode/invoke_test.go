/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	cb "github.com/hyperledger/fabric-protos-go/common"
	pb "github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/bccsp"
	"github.com/hyperledger/fabric/bccsp/sw"
	"github.com/hyperledger/fabric/common/flogging/floggingtest"
	"github.com/hyperledger/fabric/internal/peer/chaincode/mock"
	"github.com/hyperledger/fabric/internal/peer/common"
	"github.com/hyperledger/fabric/msp"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

func TestInvokeCmd(t *testing.T) {
	defer viper.Reset()
	defer resetFlags()

	resetFlags()
	mockCF, err := getMockChaincodeCmdFactory()
	require.NoError(t, err, "Error getting mock chaincode command factory")

	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	require.NoError(t, err)

	// Error case 0: no channelID specified
	cmd := invokeCmd(mockCF, cryptoProvider)
	addFlags(cmd)
	args := []string{"-n", "example02", "-c", "{\"Args\": [\"invoke\",\"a\",\"b\",\"10\"]}"}
	cmd.SetArgs(args)
	err = cmd.Execute()
	require.Error(t, err, "'peer chaincode invoke' command should have returned error when called without -C flag")

	// Success case
	cmd = invokeCmd(mockCF, cryptoProvider)
	addFlags(cmd)
	args = []string{"-n", "example02", "-c", "{\"Args\": [\"invoke\",\"a\",\"b\",\"10\"]}", "-C", "mychannel"}
	cmd.SetArgs(args)
	err = cmd.Execute()
	require.NoError(t, err, "Run chaincode invoke cmd error")

	// set timeout for error cases
	viper.Set("peer.client.connTimeout", 10*time.Millisecond)

	// Error case 1: no orderer endpoints
	t.Logf("Start error case 1: no orderer endpoints")
	getEndorserClient := common.GetEndorserClientFnc
	getOrdererEndpointOfChain := common.GetOrdererEndpointOfChainFnc
	getBroadcastClient := common.GetBroadcastClientFnc
	getDefaultSigner := common.GetDefaultSignerFnc
	getDeliverClient := common.GetDeliverClientFnc
	getPeerDeliverClient := common.GetPeerDeliverClientFnc
	defer func() {
		common.GetEndorserClientFnc = getEndorserClient
		common.GetOrdererEndpointOfChainFnc = getOrdererEndpointOfChain
		common.GetBroadcastClientFnc = getBroadcastClient
		common.GetDefaultSignerFnc = getDefaultSigner
		common.GetDeliverClientFnc = getDeliverClient
		common.GetPeerDeliverClientFnc = getPeerDeliverClient
	}()
	common.GetEndorserClientFnc = func(string, string) (pb.EndorserClient, error) {
		return mockCF.EndorserClients[0], nil
	}
	common.GetOrdererEndpointOfChainFnc = func(chainID string, signer common.Signer, endorserClient pb.EndorserClient, cryptoProvider bccsp.BCCSP) ([]string, error) {
		return []string{}, nil
	}
	cmd = invokeCmd(nil, cryptoProvider)
	addFlags(cmd)
	args = []string{"-n", "example02", "-c", "{\"Args\": [\"invoke\",\"a\",\"b\",\"10\"]}", "-C", "mychannel"}
	cmd.SetArgs(args)
	err = cmd.Execute()
	require.Error(t, err)

	// Error case 2: getEndorserClient returns error
	t.Logf("Start error case 2: getEndorserClient returns error")
	common.GetEndorserClientFnc = func(string, string) (pb.EndorserClient, error) {
		return nil, errors.New("error")
	}
	err = cmd.Execute()
	require.Error(t, err)

	// Error case 3: getDeliverClient returns error
	t.Logf("Start error case 3: getDeliverClient returns error")
	common.GetDeliverClientFnc = func(string, string) (pb.Deliver_DeliverClient, error) {
		return nil, errors.New("error")
	}
	err = cmd.Execute()
	require.Error(t, err)

	// Error case 4 : getPeerDeliverClient returns error
	t.Logf("Start error case 4: getPeerDeliverClient returns error")
	common.GetPeerDeliverClientFnc = func(string, string) (pb.DeliverClient, error) {
		return nil, errors.New("error")
	}
	err = cmd.Execute()
	require.Error(t, err)

	// Error case 5: getDefaultSignerFnc returns error
	t.Logf("Start error case 5: getDefaultSignerFnc returns error")
	common.GetEndorserClientFnc = func(string, string) (pb.EndorserClient, error) {
		return mockCF.EndorserClients[0], nil
	}
	common.GetPeerDeliverClientFnc = func(string, string) (pb.DeliverClient, error) {
		return mockCF.DeliverClients[0], nil
	}
	common.GetDefaultSignerFnc = func() (msp.SigningIdentity, error) {
		return nil, errors.New("error")
	}
	err = cmd.Execute()
	require.Error(t, err)
	common.GetDefaultSignerFnc = common.GetDefaultSigner

	// Error case 6: getOrdererEndpointOfChainFnc returns error
	t.Logf("Start error case 6: getOrdererEndpointOfChainFnc returns error")
	common.GetEndorserClientFnc = func(string, string) (pb.EndorserClient, error) {
		return mockCF.EndorserClients[0], nil
	}
	common.GetOrdererEndpointOfChainFnc = func(chainID string, signer common.Signer, endorserClient pb.EndorserClient, cryptoProvider bccsp.BCCSP) ([]string, error) {
		return nil, errors.New("error")
	}
	err = cmd.Execute()
	require.Error(t, err)

	// Error case 7: getBroadcastClient returns error
	t.Logf("Start error case 7: getBroadcastClient returns error")
	common.GetOrdererEndpointOfChainFnc = func(chainID string, signer common.Signer, endorserClient pb.EndorserClient, cryptoProvider bccsp.BCCSP) ([]string, error) {
		return []string{"localhost:9999"}, nil
	}
	common.GetBroadcastClientFnc = func() (common.BroadcastClient, error) {
		return nil, errors.New("error")
	}
	err = cmd.Execute()
	require.Error(t, err)

	// Success case
	t.Logf("Start success case")
	common.GetBroadcastClientFnc = func() (common.BroadcastClient, error) {
		return mockCF.BroadcastClient, nil
	}
	err = cmd.Execute()
	require.NoError(t, err)
}

func TestInvokeCmdSimulateESCCPluginResponse(t *testing.T) {
	defer resetFlags()
	mockCF, err := getMockChaincodeCmdFactory()
	require.NoError(t, err, "Error getting mock chaincode command factory")
	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	require.NoError(t, err)

	// success case - simulate an ESCC plugin that endorses a chaincode response
	// with status greater than shim.ERRORTHRESHOLD or even shim.ERROR
	mockResponse := &pb.ProposalResponse{
		Response:    &pb.Response{Status: 504},
		Endorsement: &pb.Endorsement{},
	}
	mockCF.EndorserClients = []pb.EndorserClient{
		common.GetMockEndorserClient(mockResponse, nil),
		common.GetMockEndorserClient(mockResponse, nil),
	}

	// set logger to logger with a backend that writes to a byte buffer
	oldLogger := logger
	defer func() { logger = oldLogger }()
	l, recorder := floggingtest.NewTestLogger(t)
	logger = l

	cmd := invokeCmd(mockCF, cryptoProvider)
	addFlags(cmd)
	args := []string{"-n", "example02", "-c", "{\"Args\": [\"invoke\",\"a\",\"b\",\"10\"]}", "-C", "mychannel"}
	cmd.SetArgs(args)

	err = cmd.Execute()
	require.NoError(t, err, "Run chaincode invoke cmd error")

	require.NotEmpty(t, recorder.MessagesContaining("Chaincode invoke successful"), "missing invoke success log record")
	require.NotEmpty(t, recorder.MessagesContaining("result: <nil>"), "missing result log record")
}

func TestInvokeCmdEndorsementError(t *testing.T) {
	defer resetFlags()
	mockCF, err := getMockChaincodeCmdFactoryWithErr()
	require.NoError(t, err, "Error getting mock chaincode command factory")
	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	require.NoError(t, err)

	cmd := invokeCmd(mockCF, cryptoProvider)
	addFlags(cmd)
	args := []string{"-n", "example02", "-C", "mychannel", "-c", "{\"Args\": [\"invoke\",\"a\",\"b\",\"10\"]}"}
	cmd.SetArgs(args)
	err = cmd.Execute()
	require.Error(t, err, "Expected error executing invoke command")
}

func TestInvokeCmdEndorsementFailure(t *testing.T) {
	defer resetFlags()
	ccRespStatus := [2]int32{502, 400}
	ccRespPayload := [][]byte{[]byte("Invalid function name"), []byte("Incorrect parameters")}
	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	require.NoError(t, err)

	for i := 0; i < 2; i++ {
		mockCF, err := getMockChaincodeCmdFactoryEndorsementFailure(ccRespStatus[i], ccRespPayload[i])
		require.NoError(t, err, "Error getting mock chaincode command factory")

		cmd := invokeCmd(mockCF, cryptoProvider)
		addFlags(cmd)
		args := []string{"-C", "mychannel", "-n", "example02", "-c", "{\"Args\": [\"invokeinvalid\",\"a\",\"b\",\"10\"]}"}
		cmd.SetArgs(args)

		err = cmd.Execute()
		require.Error(t, err)
		require.Contains(t, err.Error(), "endorsement failure during invoke")
		require.Contains(t, err.Error(), fmt.Sprintf("response: status:%d payload:\"%s\"", ccRespStatus[i], ccRespPayload[i]))
	}
}

// Returns mock chaincode command factory with multiple endorser and deliver clients
func getMockChaincodeCmdFactory() (*ChaincodeCmdFactory, error) {
	signer, err := common.GetDefaultSigner()
	if err != nil {
		return nil, err
	}
	mockResponse := &pb.ProposalResponse{
		Response:    &pb.Response{Status: 200},
		Endorsement: &pb.Endorsement{},
	}
	mockEndorserClients := []pb.EndorserClient{common.GetMockEndorserClient(mockResponse, nil), common.GetMockEndorserClient(mockResponse, nil)}
	mockBroadcastClient := common.GetMockBroadcastClient(nil)
	mockDC := getMockDeliverClientResponseWithTxStatusAndID(pb.TxValidationCode_VALID, "txid0")
	mockDeliverClients := []pb.DeliverClient{mockDC, mockDC}
	mockCF := &ChaincodeCmdFactory{
		EndorserClients: mockEndorserClients,
		Signer:          signer,
		BroadcastClient: mockBroadcastClient,
		DeliverClients:  mockDeliverClients,
	}
	return mockCF, nil
}

// Returns mock chaincode command factory that is constructed with an endorser
// client that returns an error for proposal request and a deliver client
func getMockChaincodeCmdFactoryWithErr() (*ChaincodeCmdFactory, error) {
	signer, err := common.GetDefaultSigner()
	if err != nil {
		return nil, err
	}

	errMsg := "invoke error"
	mockEndorserClients := []pb.EndorserClient{common.GetMockEndorserClient(nil, errors.New(errMsg))}
	mockBroadcastClient := common.GetMockBroadcastClient(nil)
	mockDeliverClients := []pb.DeliverClient{getMockDeliverClientResponseWithTxStatusAndID(pb.TxValidationCode_INVALID_OTHER_REASON, "txid0")}
	mockCF := &ChaincodeCmdFactory{
		EndorserClients: mockEndorserClients,
		Signer:          signer,
		BroadcastClient: mockBroadcastClient,
		DeliverClients:  mockDeliverClients,
	}
	return mockCF, nil
}

// Returns mock chaincode command factory with an endorser client (that fails) and
// a deliver client
func getMockChaincodeCmdFactoryEndorsementFailure(ccRespStatus int32, ccRespPayload []byte) (*ChaincodeCmdFactory, error) {
	signer, err := common.GetDefaultSigner()
	if err != nil {
		return nil, err
	}

	// create a proposal from a ChaincodeInvocationSpec
	prop, _, err := protoutil.CreateChaincodeProposal(cb.HeaderType_ENDORSER_TRANSACTION, "testchannelid", createCIS(), nil)
	if err != nil {
		return nil, fmt.Errorf("Could not create chaincode proposal, err %s\n", err)
	}

	response := &pb.Response{Status: ccRespStatus, Payload: ccRespPayload}
	result := []byte("res")

	mockRespFailure, err := protoutil.CreateProposalResponseFailure(prop.Header, prop.Payload, response, result, nil, "foo")
	if err != nil {
		return nil, fmt.Errorf("Could not create proposal response failure, err %s\n", err)
	}

	mockEndorserClients := []pb.EndorserClient{common.GetMockEndorserClient(mockRespFailure, nil)}
	mockBroadcastClient := common.GetMockBroadcastClient(nil)
	mockDeliverClients := []pb.DeliverClient{getMockDeliverClientResponseWithTxStatusAndID(pb.TxValidationCode(mockRespFailure.Response.Status), "txid0")}
	mockCF := &ChaincodeCmdFactory{
		EndorserClients: mockEndorserClients,
		Signer:          signer,
		BroadcastClient: mockBroadcastClient,
		DeliverClients:  mockDeliverClients,
	}
	return mockCF, nil
}

func createCIS() *pb.ChaincodeInvocationSpec {
	return &pb.ChaincodeInvocationSpec{
		ChaincodeSpec: &pb.ChaincodeSpec{
			Type:        pb.ChaincodeSpec_GOLANG,
			ChaincodeId: &pb.ChaincodeID{Name: "chaincode_name"},
			Input:       &pb.ChaincodeInput{Args: [][]byte{[]byte("arg1"), []byte("arg2")}},
		},
	}
}

func getMockDeliverClientResponseWithTxStatusAndID(txStatus pb.TxValidationCode, txID string) *mock.PeerDeliverClient {
	mockDC := &mock.PeerDeliverClient{}
	mockDC.DeliverFilteredStub = func(ctx context.Context, opts ...grpc.CallOption) (pb.Deliver_DeliverFilteredClient, error) {
		return getMockDeliverConnectionResponseWithTxStatusAndID(txStatus, txID), nil
	}
	return mockDC
}

func getMockDeliverConnectionResponseWithTxStatusAndID(txStatus pb.TxValidationCode, txID string) *mock.Deliver {
	mockDF := &mock.Deliver{}
	resp := &pb.DeliverResponse{
		Type: &pb.DeliverResponse_FilteredBlock{
			FilteredBlock: createFilteredBlock(txStatus, txID),
		},
	}
	mockDF.RecvReturns(resp, nil)
	return mockDF
}

func getMockDeliverClientRespondsWithFilteredBlocks(fb []*pb.FilteredBlock) *mock.PeerDeliverClient {
	mockDC := &mock.PeerDeliverClient{}
	mockDC.DeliverFilteredStub = func(ctx context.Context, opts ...grpc.CallOption) (pb.Deliver_DeliverFilteredClient, error) {
		mockDF := &mock.Deliver{}
		for i, f := range fb {
			resp := &pb.DeliverResponse{
				Type: &pb.DeliverResponse_FilteredBlock{
					FilteredBlock: f,
				},
			}
			mockDF.RecvReturnsOnCall(i, resp, nil)
		}
		return mockDF, nil
	}
	return mockDC
}

func getMockDeliverClientRegisterAfterDelay(delayChan chan struct{}) *mock.PeerDeliverClient {
	mockDC := &mock.PeerDeliverClient{}
	mockDC.DeliverFilteredStub = func(ctx context.Context, opts ...grpc.CallOption) (pb.Deliver_DeliverFilteredClient, error) {
		mockDF := &mock.Deliver{}
		mockDF.SendStub = func(*cb.Envelope) error {
			<-delayChan
			return nil
		}
		return mockDF, nil
	}
	return mockDC
}

func getMockDeliverClientRespondAfterDelay(delayChan chan struct{}, txStatus pb.TxValidationCode, txID string) *mock.PeerDeliverClient {
	mockDC := &mock.PeerDeliverClient{}
	mockDC.DeliverFilteredStub = func(ctx context.Context, opts ...grpc.CallOption) (pb.Deliver_DeliverFilteredClient, error) {
		mockDF := &mock.Deliver{}
		mockDF.RecvStub = func() (*pb.DeliverResponse, error) {
			<-delayChan
			resp := &pb.DeliverResponse{
				Type: &pb.DeliverResponse_FilteredBlock{
					FilteredBlock: createFilteredBlock(txStatus, txID),
				},
			}
			return resp, nil
		}
		return mockDF, nil
	}
	return mockDC
}

func getMockDeliverClientWithErr(errMsg string) *mock.PeerDeliverClient {
	mockDC := &mock.PeerDeliverClient{}
	mockDC.DeliverFilteredStub = func(ctx context.Context, opts ...grpc.CallOption) (pb.Deliver_DeliverFilteredClient, error) {
		return nil, fmt.Errorf(errMsg)
	}
	return mockDC
}

func createFilteredBlock(txStatus pb.TxValidationCode, txIDs ...string) *pb.FilteredBlock {
	var filteredTransactions []*pb.FilteredTransaction
	for _, txID := range txIDs {
		ft := &pb.FilteredTransaction{
			Txid:             txID,
			TxValidationCode: txStatus,
		}
		filteredTransactions = append(filteredTransactions, ft)
	}
	fb := &pb.FilteredBlock{
		Number:               0,
		ChannelId:            "testchannel",
		FilteredTransactions: filteredTransactions,
	}
	return fb
}
