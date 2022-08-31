/*
Copyright IBM Corp. 2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package cluster_test

import (
	"context"
	"io"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric-protos-go/orderer"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/metrics/disabled"
	"github.com/hyperledger/fabric/orderer/common/cluster"
	"github.com/hyperledger/fabric/orderer/common/cluster/mocks"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

func noopReport(_ error) {
}

func TestSendSubmitWithReport(t *testing.T) {
	t.Parallel()
	node1 := newTestNode(t)
	node2 := newTestNode(t)

	var receptionWaitGroup sync.WaitGroup
	receptionWaitGroup.Add(1)
	node2.handler.On("OnSubmit", testChannel, mock.Anything, mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		receptionWaitGroup.Done()
	})

	defer node1.stop()
	defer node2.stop()

	config := []cluster.RemoteNode{node1.nodeInfo, node2.nodeInfo}
	node1.c.Configure(testChannel, config)
	node2.c.Configure(testChannel, config)

	node1RPC := &cluster.RPC{
		Logger:        flogging.MustGetLogger("test"),
		Timeout:       time.Hour,
		StreamsByType: cluster.NewStreamsByType(),
		Channel:       testChannel,
		Comm:          node1.c,
	}

	// Wait for connections to be established
	time.Sleep(time.Second * 5)

	err := node1RPC.SendSubmit(node2.nodeInfo.ID, &orderer.SubmitRequest{Channel: testChannel, Payload: &common.Envelope{Payload: []byte("1")}}, noopReport)
	require.NoError(t, err)
	receptionWaitGroup.Wait() // Wait for message to be received

	// Restart the node
	node2.stop()
	node2.resurrect()

	/*
	 * allow the node2 to restart completely
	 * if restart not complete, the existing stream able to successfully send
	 * the next SubmitRequest which makes the testcase fails. Hence this delay
	 * required
	 */
	time.Sleep(time.Second * 5)

	var wg2 sync.WaitGroup
	wg2.Add(1)

	reportSubmitFailed := func(err error) {
		defer wg2.Done()
		require.EqualError(t, err, io.EOF.Error())
	}

	err = node1RPC.SendSubmit(node2.nodeInfo.ID, &orderer.SubmitRequest{Channel: testChannel, Payload: &common.Envelope{Payload: []byte("2")}}, reportSubmitFailed)
	require.NoError(t, err)

	wg2.Wait()

	// Ensure stale stream is cleaned up and removed from the mapping
	require.Len(t, node1RPC.StreamsByType[cluster.SubmitOperation], 0)

	// Wait for connection to be re-established
	time.Sleep(time.Second * 5)

	// Send again, this time it should be received
	receptionWaitGroup.Add(1)
	err = node1RPC.SendSubmit(node2.nodeInfo.ID, &orderer.SubmitRequest{Channel: testChannel, Payload: &common.Envelope{Payload: []byte("3")}}, noopReport)
	require.NoError(t, err)
	receptionWaitGroup.Wait()
}

func TestRPCChangeDestination(t *testing.T) {
	// We send a Submit() to 2 different nodes - 1 and 2.
	// The first invocation of Submit() establishes a stream with node 1
	// and the second establishes a stream with node 2.
	// We define a mock behavior for only a single invocation of Send() on each
	// of the streams (to node 1 and to node 2), therefore we test that invocation
	// of rpc.SendSubmit to node 2 doesn't send the message to node 1.
	comm := &mocks.Communicator{}

	metrics := cluster.NewMetrics(&disabled.Provider{})

	fakeStream1 := &mocks.StepClientStream{}
	fakeStream2 := &mocks.StepClientStream{}

	comm.On("Remote", "mychannel", uint64(1)).Return(&cluster.RemoteContext{
		SendBuffSize: 10,
		Metrics:      metrics,
		Logger:       flogging.MustGetLogger("test"),
		ProbeConn:    func(_ *grpc.ClientConn) error { return nil },
		GetStreamFunc: func(ctx context.Context) (cluster.StepClientStream, error) {
			return fakeStream1, nil
		},
		Channel: "mychannel",
	}, nil)

	comm.On("Remote", "mychannel", uint64(2)).Return(&cluster.RemoteContext{
		SendBuffSize: 10,
		Metrics:      metrics,
		Logger:       flogging.MustGetLogger("test"),
		ProbeConn:    func(_ *grpc.ClientConn) error { return nil },
		GetStreamFunc: func(ctx context.Context) (cluster.StepClientStream, error) {
			return fakeStream2, nil
		},
		Channel: "mychannel",
	}, nil)

	rpc := &cluster.RPC{
		Logger:        flogging.MustGetLogger("test"),
		Timeout:       time.Hour,
		StreamsByType: cluster.NewStreamsByType(),
		Channel:       "mychannel",
		Comm:          comm,
	}

	var sent sync.WaitGroup
	sent.Add(2)

	signalSent := func(_ mock.Arguments) {
		sent.Done()
	}

	fakeStream1.On("Context", mock.Anything).Return(context.Background())
	fakeStream1.On("Auth").Return(nil).Once()
	fakeStream1.On("Send", mock.Anything).Return(nil).Run(signalSent).Once()
	fakeStream1.On("Recv").Return(nil, io.EOF)

	fakeStream2.On("Context", mock.Anything).Return(context.Background())
	fakeStream2.On("Auth").Return(nil).Once()
	fakeStream2.On("Send", mock.Anything).Return(nil).Run(signalSent).Once()
	fakeStream2.On("Recv").Return(nil, io.EOF)

	rpc.SendSubmit(1, &orderer.SubmitRequest{Channel: "mychannel"}, noopReport)
	rpc.SendSubmit(2, &orderer.SubmitRequest{Channel: "mychannel"}, noopReport)

	sent.Wait()

	fakeStream1.AssertNumberOfCalls(t, "Send", 1)
	fakeStream2.AssertNumberOfCalls(t, "Send", 1)
}

func TestSend(t *testing.T) {
	submitRequest := &orderer.SubmitRequest{Channel: "mychannel"}
	submitResponse := &orderer.StepResponse{
		Payload: &orderer.StepResponse_SubmitRes{
			SubmitRes: &orderer.SubmitResponse{Status: common.Status_SUCCESS},
		},
	}

	consensusRequest := &orderer.ConsensusRequest{
		Channel: "mychannel",
	}

	submitReq := wrapSubmitReq(submitRequest)

	consensusReq := &orderer.StepRequest{
		Payload: &orderer.StepRequest_ConsensusRequest{
			ConsensusRequest: consensusRequest,
		},
	}

	submit := func(rpc *cluster.RPC) error {
		err := rpc.SendSubmit(1, submitRequest, noopReport)
		return err
	}

	step := func(rpc *cluster.RPC) error {
		return rpc.SendConsensus(1, consensusRequest)
	}

	type testCase struct {
		name           string
		method         func(rpc *cluster.RPC) error
		sendReturns    error
		sendCalledWith *orderer.StepRequest
		receiveReturns []interface{}
		remoteError    error
		expectedErr    string
	}

	l := &sync.Mutex{}
	var tst testCase

	sent := make(chan struct{})

	var sendCalls uint32

	fakeStream1 := &mocks.StepClientStream{}
	fakeStream1.On("Context", mock.Anything).Return(context.Background())
	fakeStream1.On("Auth").Return(nil)
	fakeStream1.On("Send", mock.Anything).Return(func(*orderer.StepRequest) error {
		l.Lock()
		defer l.Unlock()
		atomic.AddUint32(&sendCalls, 1)
		sent <- struct{}{}
		return tst.sendReturns
	})

	for _, tst := range []testCase{
		{
			name:           "Send and Receive submit succeed",
			method:         submit,
			sendReturns:    nil,
			receiveReturns: []interface{}{submitResponse, nil},
			sendCalledWith: submitReq,
		},
		{
			name:           "Send step succeed",
			method:         step,
			sendReturns:    nil,
			sendCalledWith: consensusReq,
		},
		{
			name:           "Send submit fails",
			method:         submit,
			sendReturns:    errors.New("oops"),
			sendCalledWith: submitReq,
			expectedErr:    "stream is aborted",
		},
		{
			name:           "Send step fails",
			method:         step,
			sendReturns:    errors.New("oops"),
			sendCalledWith: consensusReq,
			expectedErr:    "stream is aborted",
		},
		{
			name:        "Remote() fails",
			method:      submit,
			remoteError: errors.New("timed out"),
			expectedErr: "timed out",
		},
		{
			name:        "Submit fails with Send",
			method:      submit,
			expectedErr: "deadline exceeded",
		},
	} {
		l.Lock()
		testCase := tst
		l.Unlock()

		t.Run(testCase.name, func(t *testing.T) {
			atomic.StoreUint32(&sendCalls, 0)
			isSend := testCase.receiveReturns == nil
			comm := &mocks.Communicator{}

			rm := &cluster.RemoteContext{
				Metrics:      cluster.NewMetrics(&disabled.Provider{}),
				SendBuffSize: 1,
				Logger:       flogging.MustGetLogger("test"),
				ProbeConn:    func(_ *grpc.ClientConn) error { return nil },
				GetStreamFunc: func(ctx context.Context) (cluster.StepClientStream, error) {
					return fakeStream1, nil
				},
				Channel: "mychannel",
			}
			defer rm.Abort()
			comm.On("Remote", "mychannel", uint64(1)).Return(rm, testCase.remoteError)

			rpc := &cluster.RPC{
				Logger:        flogging.MustGetLogger("test"),
				Timeout:       time.Hour,
				StreamsByType: cluster.NewStreamsByType(),
				Channel:       "mychannel",
				Comm:          comm,
			}

			_ = testCase.method(rpc)
			if testCase.remoteError == nil {
				<-sent
			}

			if testCase.remoteError == nil && testCase.expectedErr == "" && isSend {
				fakeStream1.AssertCalled(t, "Send", testCase.sendCalledWith)
				// Ensure that if we succeeded - only 1 stream was created despite 2 calls
				// to Send() were made
				err := testCase.method(rpc)
				<-sent

				require.NoError(t, err)
				require.Equal(t, 2, int(atomic.LoadUint32(&sendCalls)))
			}
		})
	}
}

func TestRPCGarbageCollection(t *testing.T) {
	// Scenario: Send a message to a remote node, and establish a stream
	// while doing it.
	// Afterwards - make that stream be aborted, and send a message to a different
	// remote node.
	// The first stream should be cleaned from the mapping.

	comm := &mocks.Communicator{}

	fakeStream1 := &mocks.StepClientStream{}

	remote := &cluster.RemoteContext{
		SendBuffSize: 10,
		Metrics:      cluster.NewMetrics(&disabled.Provider{}),
		Logger:       flogging.MustGetLogger("test"),
		ProbeConn:    func(_ *grpc.ClientConn) error { return nil },
		GetStreamFunc: func(ctx context.Context) (cluster.StepClientStream, error) {
			return fakeStream1, nil
		},
		Channel: "mychannel",
	}

	var sent sync.WaitGroup

	defineMocks := func(destination uint64) {
		sent.Add(1)
		comm.On("Remote", "mychannel", destination).Return(remote, nil)
		fakeStream1.On("Context", mock.Anything).Return(context.Background())
		fakeStream1.On("Auth").Return(nil)
		fakeStream1.On("Send", mock.Anything).Return(nil).Run(func(_ mock.Arguments) {
			sent.Done()
		}).Once()
		fakeStream1.On("Recv").Return(nil, nil)
	}

	mapping := cluster.NewStreamsByType()

	rpc := &cluster.RPC{
		Logger:        flogging.MustGetLogger("test"),
		Timeout:       time.Hour,
		StreamsByType: mapping,
		Channel:       "mychannel",
		Comm:          comm,
	}

	defineMocks(1)

	rpc.SendSubmit(1, &orderer.SubmitRequest{Channel: "mychannel"}, noopReport)
	// Wait for the message to arrive
	sent.Wait()
	// Ensure the stream is initialized in the mapping
	require.Len(t, mapping[cluster.SubmitOperation], 1)
	require.Equal(t, uint64(1), mapping[cluster.SubmitOperation][1].ID)
	// And the underlying gRPC stream indeed had Send invoked on it.
	fakeStream1.AssertNumberOfCalls(t, "Send", 1)

	// Abort all streams we currently have that are associated to the remote.
	remote.Abort()

	// The stream still exists, as it is not cleaned yet.
	require.Len(t, mapping[cluster.SubmitOperation], 1)
	require.Equal(t, uint64(1), mapping[cluster.SubmitOperation][1].ID)

	// Prepare for the next transmission.
	defineMocks(2)

	// Send a message to a different node.
	rpc.SendSubmit(2, &orderer.SubmitRequest{Channel: "mychannel"}, noopReport)
	// The mapping should be now cleaned from the previous stream.
	require.Len(t, mapping[cluster.SubmitOperation], 1)
	require.Equal(t, uint64(2), mapping[cluster.SubmitOperation][2].ID)
}
