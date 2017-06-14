/*
 Copyright IBM Corp. 2017 All Rights Reserved.

 Licensed under the Apache License, Version 2.0 (the "License");
 you may not use this file except in compliance with the License.
 You may obtain a copy of the License at

      http://www.apache.org/licenses/LICENSE-2.0

 Unless required by applicable law or agreed to in writing, software
 distributed under the License is distributed on an "AS IS" BASIS,
 WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 See the License for the specific language governing permissions and
 limitations under the License.
*/

package chaincode

import (
	"bytes"
	"errors"
	"fmt"
	"testing"

	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/msp"
	"github.com/hyperledger/fabric/peer/common"
	cb "github.com/hyperledger/fabric/protos/common"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/hyperledger/fabric/protos/utils"
	logging "github.com/op/go-logging"
	"github.com/stretchr/testify/assert"
)

func TestInvokeCmd(t *testing.T) {
	InitMSP()
	mockCF, err := getMockChaincodeCmdFactory()
	assert.NoError(t, err, "Error getting mock chaincode command factory")

	cmd := invokeCmd(mockCF)
	addFlags(cmd)
	args := []string{"-n", "example02", "-c", "{\"Args\": [\"invoke\",\"a\",\"b\",\"10\"]}"}
	cmd.SetArgs(args)
	err = cmd.Execute()
	assert.NoError(t, err, "Run chaincode invoke cmd error")

	// Error case 1: no orderer endpoints
	t.Logf("Start error case 1: no orderer endpoints")
	getEndorserClient := common.GetEndorserClientFnc
	getOrdererEndpointOfChain := common.GetOrdererEndpointOfChainFnc
	getBroadcastClient := common.GetBroadcastClientFnc
	getDefaultSigner := common.GetDefaultSignerFnc
	defer func() {
		common.GetEndorserClientFnc = getEndorserClient
		common.GetOrdererEndpointOfChainFnc = getOrdererEndpointOfChain
		common.GetBroadcastClientFnc = getBroadcastClient
		common.GetDefaultSignerFnc = getDefaultSigner
	}()
	common.GetEndorserClientFnc = func() (pb.EndorserClient, error) {
		return mockCF.EndorserClient, nil
	}
	common.GetOrdererEndpointOfChainFnc = func(chainID string, signer msp.SigningIdentity, endorserClient pb.EndorserClient) ([]string, error) {
		return []string{}, nil
	}
	cmd = invokeCmd(nil)
	addFlags(cmd)
	args = []string{"-n", "example02", "-c", "{\"Args\": [\"invoke\",\"a\",\"b\",\"10\"]}"}
	cmd.SetArgs(args)
	err = cmd.Execute()
	assert.Error(t, err)

	// Error case 2: getEndorserClient returns error
	t.Logf("Start error case 2: getEndorserClient returns error")
	common.GetEndorserClientFnc = func() (pb.EndorserClient, error) {
		return nil, errors.New("error")
	}
	err = cmd.Execute()
	assert.Error(t, err)

	// Error case 3: getDefaultSignerFnc returns error
	t.Logf("Start error case 3: getDefaultSignerFnc returns error")
	common.GetEndorserClientFnc = func() (pb.EndorserClient, error) {
		return mockCF.EndorserClient, nil
	}
	common.GetDefaultSignerFnc = func() (msp.SigningIdentity, error) {
		return nil, errors.New("error")
	}
	err = cmd.Execute()
	assert.Error(t, err)
	common.GetDefaultSignerFnc = common.GetDefaultSigner

	// Error case 4: getOrdererEndpointOfChainFnc returns error
	t.Logf("Start error case 4: getOrdererEndpointOfChainFnc returns error")
	common.GetEndorserClientFnc = func() (pb.EndorserClient, error) {
		return mockCF.EndorserClient, nil
	}
	common.GetOrdererEndpointOfChainFnc = func(chainID string, signer msp.SigningIdentity, endorserClient pb.EndorserClient) ([]string, error) {
		return nil, errors.New("error")
	}
	err = cmd.Execute()
	assert.Error(t, err)

	// Error case 5: getBroadcastClient returns error
	t.Logf("Start error case 5: getBroadcastClient returns error")
	common.GetOrdererEndpointOfChainFnc = func(chainID string, signer msp.SigningIdentity, endorserClient pb.EndorserClient) ([]string, error) {
		return []string{"localhost:9999"}, nil
	}
	common.GetBroadcastClientFnc = func(orderingEndpoint string, tlsEnabled bool, caFile string) (common.BroadcastClient, error) {
		return nil, errors.New("error")
	}
	err = cmd.Execute()
	assert.Error(t, err)

	// Success case
	t.Logf("Start success case")
	common.GetBroadcastClientFnc = func(orderingEndpoint string, tlsEnabled bool, caFile string) (common.BroadcastClient, error) {
		return mockCF.BroadcastClient, nil
	}
	err = cmd.Execute()
	assert.NoError(t, err)
}

func TestInvokeCmdEndorsementError(t *testing.T) {
	InitMSP()
	mockCF, err := getMockChaincodeCmdFactoryWithErr()
	assert.NoError(t, err, "Error getting mock chaincode command factory")

	cmd := invokeCmd(mockCF)
	addFlags(cmd)
	args := []string{"-n", "example02", "-c", "{\"Args\": [\"invoke\",\"a\",\"b\",\"10\"]}"}
	cmd.SetArgs(args)
	err = cmd.Execute()
	assert.Error(t, err, "Expected error executing invoke command")
}

func TestInvokeCmdEndorsementFailure(t *testing.T) {
	InitMSP()
	ccRespStatus := [2]int32{502, 400}
	ccRespPayload := [][]byte{[]byte("Invalid function name"), []byte("Incorrect parameters")}

	for i := 0; i < 2; i++ {
		mockCF, err := getMockChaincodeCmdFactoryEndorsementFailure(ccRespStatus[i], ccRespPayload[i])
		assert.NoError(t, err, "Error getting mock chaincode command factory")

		cmd := invokeCmd(mockCF)
		addFlags(cmd)
		args := []string{"-n", "example02", "-c", "{\"Args\": [\"invokeinvalid\",\"a\",\"b\",\"10\"]}"}
		cmd.SetArgs(args)

		// set logger to logger with a backend that writes to a byte buffer
		var buffer bytes.Buffer
		logger.SetBackend(logging.AddModuleLevel(logging.NewLogBackend(&buffer, "", 0)))
		// reset the logger after test
		defer func() {
			flogging.Reset()
		}()
		// make sure buffer is "clean" before running the invoke
		buffer.Reset()

		err = cmd.Execute()
		assert.NoError(t, err)
		assert.Regexp(t, "Endorsement failure during invoke", buffer.String())
		assert.Regexp(t, fmt.Sprintf("chaincode result: status:%d payload:\"%s\"", ccRespStatus[i], ccRespPayload[i]), buffer.String())
	}

}

// Returns mock chaincode command factory
func getMockChaincodeCmdFactory() (*ChaincodeCmdFactory, error) {
	signer, err := common.GetDefaultSigner()
	if err != nil {
		return nil, err
	}
	mockResponse := &pb.ProposalResponse{
		Response:    &pb.Response{Status: 200},
		Endorsement: &pb.Endorsement{},
	}
	mockEndorserClient := common.GetMockEndorserClient(mockResponse, nil)
	mockBroadcastClient := common.GetMockBroadcastClient(nil)
	mockCF := &ChaincodeCmdFactory{
		EndorserClient:  mockEndorserClient,
		Signer:          signer,
		BroadcastClient: mockBroadcastClient,
	}
	return mockCF, nil
}

// Returns mock chaincode command factory that is constructed with an endorser
// client that returns an error for proposal request
func getMockChaincodeCmdFactoryWithErr() (*ChaincodeCmdFactory, error) {
	signer, err := common.GetDefaultSigner()
	if err != nil {
		return nil, err
	}

	errMsg := "invoke error"
	mockEndorerClient := common.GetMockEndorserClient(nil, errors.New(errMsg))
	mockBroadcastClient := common.GetMockBroadcastClient(nil)

	mockCF := &ChaincodeCmdFactory{
		EndorserClient:  mockEndorerClient,
		Signer:          signer,
		BroadcastClient: mockBroadcastClient,
	}
	return mockCF, nil
}

// Returns mock chaincode command factory
func getMockChaincodeCmdFactoryEndorsementFailure(ccRespStatus int32, ccRespPayload []byte) (*ChaincodeCmdFactory, error) {
	signer, err := common.GetDefaultSigner()
	if err != nil {
		return nil, err
	}

	// create a proposal from a ChaincodeInvocationSpec
	prop, _, err := utils.CreateChaincodeProposal(cb.HeaderType_ENDORSER_TRANSACTION, util.GetTestChainID(), createCIS(), nil)
	if err != nil {
		return nil, fmt.Errorf("Could not create chaincode proposal, err %s\n", err)
	}

	response := &pb.Response{Status: ccRespStatus, Payload: ccRespPayload}
	result := []byte("res")
	ccid := &pb.ChaincodeID{Name: "foo", Version: "v1"}

	mockRespFailure, err := utils.CreateProposalResponseFailure(prop.Header, prop.Payload, response, result, nil, ccid, nil)
	if err != nil {

		return nil, fmt.Errorf("Could not create proposal response failure, err %s\n", err)
	}

	mockEndorserClient := common.GetMockEndorserClient(mockRespFailure, nil)
	mockBroadcastClient := common.GetMockBroadcastClient(nil)
	mockCF := &ChaincodeCmdFactory{
		EndorserClient:  mockEndorserClient,
		Signer:          signer,
		BroadcastClient: mockBroadcastClient,
	}
	return mockCF, nil
}

func createCIS() *pb.ChaincodeInvocationSpec {
	return &pb.ChaincodeInvocationSpec{
		ChaincodeSpec: &pb.ChaincodeSpec{
			Type:        pb.ChaincodeSpec_GOLANG,
			ChaincodeId: &pb.ChaincodeID{Name: "chaincode_name"},
			Input:       &pb.ChaincodeInput{Args: [][]byte{[]byte("arg1"), []byte("arg2")}}}}
}
