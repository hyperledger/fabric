/*
Copyright IBM Corp. 2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package cluster_test

import (
	"testing"

	"github.com/hyperledger/fabric/orderer/common/cluster"
	"github.com/hyperledger/fabric/orderer/common/cluster/mocks"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/orderer"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestRPCStep(t *testing.T) {
	t.Parallel()
	expectedResponse := &orderer.StepResponse{}
	for _, testcase := range []struct {
		name             string
		remoteErr        error
		stepReturns      []interface{}
		expectedErr      string
		expectedResponse *orderer.StepResponse
	}{
		{
			name:             "Success",
			stepReturns:      []interface{}{expectedResponse, nil},
			expectedResponse: expectedResponse,
		},
		{
			name:        "Failure on Step()",
			stepReturns: []interface{}{nil, errors.New("oops")},
			expectedErr: "oops",
		},
		{
			name:             "Failure on Remote()",
			stepReturns:      []interface{}{expectedResponse, nil},
			expectedResponse: expectedResponse,
			remoteErr:        errors.New("timed out"),
			expectedErr:      "timed out",
		},
	} {
		testcase := testcase
		t.Run(testcase.name, func(t *testing.T) {
			comm := &mocks.Communicator{}
			client := &mocks.ClusterClient{}
			client.On("Step", mock.Anything, mock.Anything).Return(testcase.stepReturns...)
			comm.On("Remote", "mychannel", uint64(1)).Return(&cluster.RemoteContext{
				Client: client,
			}, testcase.remoteErr)

			rpc := &cluster.RPC{
				DestinationToStream: make(map[uint64]orderer.Cluster_SubmitClient),
				Channel:             "mychannel",
				Comm:                comm,
			}

			response, err := rpc.Step(1, &orderer.StepRequest{
				Channel: "mychannel",
			})

			if testcase.expectedErr == "" {
				assert.NoError(t, err)
				assert.True(t, expectedResponse == response)
			} else {
				assert.EqualError(t, err, testcase.expectedErr)
			}
		})
	}
}

func TestRPCChangeDestination(t *testing.T) {
	t.Parallel()
	// We send a Submit() to 2 different nodes - 1 and 2.
	// The first invocation of Submit() establishes a stream with node 1
	// and the second establishes a stream with node 2.
	// We define a mock behavior for only a single invocation of Send() on each
	// of the streams (to node 1 and to node 2), therefore we test that invocation
	// of rpc.SendSubmit to node 2 doesn't send the message to node 1.
	comm := &mocks.Communicator{}

	client1 := &mocks.ClusterClient{}
	client2 := &mocks.ClusterClient{}

	comm.On("Remote", "mychannel", uint64(1)).Return(&cluster.RemoteContext{Client: client1}, nil)
	comm.On("Remote", "mychannel", uint64(2)).Return(&cluster.RemoteContext{Client: client2}, nil)

	streamToNode1 := &mocks.SubmitClient{}
	streamToNode2 := &mocks.SubmitClient{}

	client1.On("Submit", mock.Anything).Return(streamToNode1, nil).Once()
	client2.On("Submit", mock.Anything).Return(streamToNode2, nil).Once()

	rpc := &cluster.RPC{
		DestinationToStream: make(map[uint64]orderer.Cluster_SubmitClient),
		Channel:             "mychannel",
		Comm:                comm,
	}

	streamToNode1.On("Send", mock.Anything).Return(nil).Once()
	streamToNode2.On("Send", mock.Anything).Return(nil).Once()

	rpc.SendSubmit(1, &orderer.SubmitRequest{Channel: "mychannel"})
	rpc.SendSubmit(2, &orderer.SubmitRequest{Channel: "mychannel"})

	streamToNode1.AssertNumberOfCalls(t, "Send", 1)
	streamToNode2.AssertNumberOfCalls(t, "Send", 1)
}

func TestRPCSubmitSend(t *testing.T) {
	t.Parallel()
	submitRequest := &orderer.SubmitRequest{Channel: "mychannel"}
	submitResponse := &orderer.SubmitResponse{Status: common.Status_SUCCESS}

	comm := &mocks.Communicator{}
	stream := &mocks.SubmitClient{}
	client := &mocks.ClusterClient{}

	resetMocks := func() {
		stream.Mock = mock.Mock{}
		client.Mock = mock.Mock{}
		comm.Mock = mock.Mock{}
	}

	for _, testCase := range []struct {
		name           string
		sendReturns    interface{}
		receiveReturns []interface{}
		submitReturns  []interface{}
		remoteError    error
		expectedErr    string
	}{
		{
			name:          "Send() succeeds",
			sendReturns:   nil,
			submitReturns: []interface{}{stream, nil},
		},
		{
			name:           "Recv() succeeds",
			receiveReturns: []interface{}{submitResponse, nil},
			submitReturns:  []interface{}{stream, nil},
		},
		{
			name:          "Send() fails",
			sendReturns:   errors.New("oops"),
			submitReturns: []interface{}{stream, nil},
			expectedErr:   "oops",
		},
		{
			name:           "Recv() fails",
			receiveReturns: []interface{}{nil, errors.New("oops")},
			submitReturns:  []interface{}{stream, nil},
			expectedErr:    "oops",
		},
		{
			name:          "Remote() fails",
			remoteError:   errors.New("timed out"),
			submitReturns: []interface{}{stream, nil},
			expectedErr:   "timed out",
		},
		{
			name:          "Submit() fails with Send",
			submitReturns: []interface{}{nil, errors.New("deadline exceeded")},
			expectedErr:   "deadline exceeded",
		},
		{
			name:           "Submit() fails with Recv",
			submitReturns:  []interface{}{nil, errors.New("deadline exceeded")},
			expectedErr:    "deadline exceeded",
			receiveReturns: []interface{}{submitResponse, nil},
		},
	} {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			isSend := testCase.receiveReturns == nil
			defer resetMocks()
			stream.On("Send", mock.Anything).Return(testCase.sendReturns)
			stream.On("Recv").Return(testCase.receiveReturns...)
			client.On("Submit", mock.Anything).Return(testCase.submitReturns...)
			comm.On("Remote", "mychannel", uint64(1)).Return(&cluster.RemoteContext{
				Client: client,
			}, testCase.remoteError)

			rpc := &cluster.RPC{
				DestinationToStream: make(map[uint64]orderer.Cluster_SubmitClient),
				Channel:             "mychannel",
				Comm:                comm,
			}

			var msg *orderer.SubmitResponse
			var err error

			if isSend {
				err = rpc.SendSubmit(1, submitRequest)
			} else {
				msg, err = rpc.ReceiveSubmitResponse(1)
				if err == nil {
					assert.Equal(t, submitResponse, msg)
				}
			}

			if testCase.expectedErr == "" {
				assert.NoError(t, err)
			} else {
				assert.EqualError(t, err, testCase.expectedErr)
			}
			if testCase.remoteError == nil && testCase.expectedErr == "" && isSend {
				stream.AssertCalled(t, "Send", submitRequest)
				// Ensure that if we succeeded - only 1 stream was created despite 2 calls
				// to Send() were made
				err := rpc.SendSubmit(1, submitRequest)
				assert.NoError(t, err)
				stream.AssertNumberOfCalls(t, "Send", 2)
				client.AssertNumberOfCalls(t, "Submit", 1)
			}
		})
	}
}
