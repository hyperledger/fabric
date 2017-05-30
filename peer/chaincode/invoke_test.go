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
	"errors"
	"testing"

	"github.com/hyperledger/fabric/msp"
	"github.com/hyperledger/fabric/peer/common"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/stretchr/testify/assert"
)

func TestInvokeCmd(t *testing.T) {
	InitMSP()
	mockCF, err := getMockChaincodeCmdFactory()
	assert.NoError(t, err, "Error getting mock chaincode command factory")

	cmd := invokeCmd(mockCF)
	AddFlags(cmd)
	args := []string{"-n", "example02", "-p", "github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02",
		"-c", "{\"Args\": [\"invoke\",\"a\",\"b\",\"10\"]}"}
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
	AddFlags(cmd)
	args = []string{"-n", "example02", "-p", "github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02",
		"-c", "{\"Args\": [\"invoke\",\"a\",\"b\",\"10\"]}"}
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

func TestInvokeCmdEndorseFail(t *testing.T) {
	InitMSP()
	mockCF, err := getMockChaincodeCmdFactoryWithErr()
	assert.NoError(t, err, "Error getting mock chaincode command factory")

	cmd := invokeCmd(mockCF)
	AddFlags(cmd)
	args := []string{"-n", "example02", "-p", "github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02",
		"-c", "{\"Args\": [\"invoke\",\"a\",\"b\",\"10\"]}"}
	cmd.SetArgs(args)
	err = cmd.Execute()
	assert.Error(t, err, "Expected error executing invoke command")
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
