/*
Copyright IBM Corp. 2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package cluster_test

import (
	"context"
	"io"
	"testing"

	"github.com/hyperledger/fabric/core/comm"
	"github.com/hyperledger/fabric/orderer/common/cluster"
	"github.com/hyperledger/fabric/orderer/common/cluster/mocks"
	"github.com/hyperledger/fabric/protos/orderer"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

var (
	submitRequest1  = &orderer.SubmitRequest{}
	submitRequest2  = &orderer.SubmitRequest{}
	submitResponse1 = &orderer.SubmitResponse{}
	submitResponse2 = &orderer.SubmitResponse{}
	stepRequest     = &orderer.StepRequest{}
	stepResponse    = &orderer.StepResponse{}
)

func TestStep(t *testing.T) {
	t.Parallel()
	dispatcher := &mocks.Dispatcher{}

	svc := &cluster.Service{
		Dispatcher: dispatcher,
	}

	t.Run("Success", func(t *testing.T) {
		dispatcher.On("DispatchStep", mock.Anything, stepRequest).Return(stepResponse, nil).Once()
		res, err := svc.Step(context.Background(), stepRequest)
		assert.NoError(t, err)
		assert.Equal(t, stepResponse, res)
	})

	t.Run("Failure", func(t *testing.T) {
		dispatcher.On("DispatchStep", mock.Anything, stepRequest).Return(nil, errors.New("oops")).Once()
		_, err := svc.Step(context.Background(), stepRequest)
		assert.EqualError(t, err, "oops")
	})
}

func TestSubmitSuccess(t *testing.T) {
	t.Parallel()
	dispatcher := &mocks.Dispatcher{}

	stream := &mocks.SubmitStream{}
	stream.On("Context").Return(context.Background())
	// Send to the stream 2 messages, and afterwards close the stream
	stream.On("Recv").Return(submitRequest1, nil).Once()
	stream.On("Recv").Return(submitRequest2, nil).Once()
	stream.On("Recv").Return(nil, io.EOF).Once()
	// Send should be called for each corresponding receive
	stream.On("Send", submitResponse1).Return(nil).Twice()

	responses := make(chan *orderer.SubmitRequest, 2)
	responses <- submitRequest1
	responses <- submitRequest2

	dispatcher.On("DispatchSubmit", mock.Anything, mock.Anything).Return(submitResponse1, nil).Once()
	dispatcher.On("DispatchSubmit", mock.Anything, mock.Anything).Return(submitResponse2, nil).Once()
	// Ensure we pass requests to DispatchSubmit in-order
	dispatcher.On("DispatchSubmit", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		expectedRequest := <-responses
		actualRequest := args.Get(1).(*orderer.SubmitRequest)
		assert.True(t, expectedRequest == actualRequest)
	})

	svc := &cluster.Service{
		Dispatcher: dispatcher,
	}

	err := svc.Submit(stream)
	assert.NoError(t, err)
	dispatcher.AssertNumberOfCalls(t, "DispatchSubmit", 2)
}

type tuple struct {
	msg interface{}
	err error
}

func (t tuple) asArray() []interface{} {
	return []interface{}{t.msg, t.err}
}

func TestSubmitFailure(t *testing.T) {
	t.Parallel()
	oops := errors.New("oops")
	testCases := []struct {
		name               string
		receiveReturns     []tuple
		sendReturns        []error
		dispatchReturns    []interface{}
		expectedDispatches int
	}{
		{
			name: "Recv() fails",
			receiveReturns: []tuple{
				{msg: nil, err: oops},
			},
		},
		{
			name: "Send() fails",
			receiveReturns: []tuple{
				{msg: submitRequest1},
			},
			expectedDispatches: 1,
			dispatchReturns:    []interface{}{submitResponse1, nil},
			sendReturns:        []error{oops},
		},
		{
			name: "DispatchSubmit() fails",
			receiveReturns: []tuple{
				{msg: submitRequest1},
			},
			expectedDispatches: 1,
			dispatchReturns:    []interface{}{nil, oops},
		},
		{
			name: "Recv() and Send() succeed, and then Recv() fails",
			receiveReturns: []tuple{
				{msg: submitRequest1},
				{msg: nil, err: oops},
			},
			expectedDispatches: 1,
			dispatchReturns:    []interface{}{submitResponse1, nil},
			sendReturns:        []error{nil, nil},
		},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			dispatcher := &mocks.Dispatcher{}
			stream := &mocks.SubmitStream{}
			stream.On("Context").Return(context.Background())
			for _, recv := range testCase.receiveReturns {
				stream.On("Recv").Return(recv.asArray()...).Once()
			}
			for _, send := range testCase.sendReturns {
				stream.On("Send", mock.Anything).Return(send).Once()
			}
			defer dispatcher.AssertNumberOfCalls(t, "DispatchSubmit", testCase.expectedDispatches)
			dispatcher.On("DispatchSubmit", mock.Anything, mock.Anything).Return(testCase.dispatchReturns...)
			svc := &cluster.Service{
				Dispatcher: dispatcher,
			}
			err := svc.Submit(stream)
			assert.EqualError(t, err, oops.Error())
		})
	}
}

func TestServiceGRPC(t *testing.T) {
	t.Parallel()
	// Check that Service correctly implements the gRPC interface
	srv, err := comm.NewGRPCServer("127.0.0.1:0", comm.ServerConfig{})
	assert.NoError(t, err)
	orderer.RegisterClusterServer(srv.Server(), &cluster.Service{})
}
