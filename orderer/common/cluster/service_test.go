/*
Copyright IBM Corp. 2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package cluster_test

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/hyperledger/fabric/common/crypto/tlsgen"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/metrics/disabled"
	"github.com/hyperledger/fabric/core/comm"
	"github.com/hyperledger/fabric/orderer/common/cluster"
	"github.com/hyperledger/fabric/orderer/common/cluster/mocks"
	"github.com/hyperledger/fabric/protos/orderer"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var (
	submitRequest1 = &orderer.StepRequest{
		Payload: &orderer.StepRequest_SubmitRequest{
			SubmitRequest: &orderer.SubmitRequest{},
		},
	}
	submitRequest2 = &orderer.StepRequest{
		Payload: &orderer.StepRequest_SubmitRequest{
			SubmitRequest: &orderer.SubmitRequest{},
		},
	}
	submitResponse1 = &orderer.StepResponse{
		Payload: &orderer.StepResponse_SubmitRes{
			SubmitRes: &orderer.SubmitResponse{},
		},
	}
	submitResponse2 = &orderer.StepResponse{
		Payload: &orderer.StepResponse_SubmitRes{
			SubmitRes: &orderer.SubmitResponse{},
		},
	}
	consensusRequest = &orderer.StepRequest{
		Payload: &orderer.StepRequest_ConsensusRequest{
			ConsensusRequest: &orderer.ConsensusRequest{
				Payload: []byte{1, 2, 3},
				Channel: "mychannel",
			},
		},
	}
)

func TestStep(t *testing.T) {
	t.Parallel()
	dispatcher := &mocks.Dispatcher{}

	svc := &cluster.Service{
		StreamCountReporter: &cluster.StreamCountReporter{
			Metrics: cluster.NewMetrics(&disabled.Provider{}),
		},
		Logger:     flogging.MustGetLogger("test"),
		StepLogger: flogging.MustGetLogger("test"),
		Dispatcher: dispatcher,
	}

	t.Run("Success", func(t *testing.T) {
		stream := &mocks.StepStream{}
		stream.On("Context").Return(context.Background())
		stream.On("Recv").Return(consensusRequest, nil).Once()
		stream.On("Recv").Return(consensusRequest, nil).Once()
		dispatcher.On("DispatchConsensus", mock.Anything, consensusRequest.GetConsensusRequest()).Return(nil).Once()
		dispatcher.On("DispatchConsensus", mock.Anything, consensusRequest.GetConsensusRequest()).Return(io.EOF).Once()
		err := svc.Step(stream)
		assert.NoError(t, err)
	})

	t.Run("Failure", func(t *testing.T) {
		stream := &mocks.StepStream{}
		stream.On("Context").Return(context.Background())
		stream.On("Recv").Return(consensusRequest, nil).Once()
		dispatcher.On("DispatchConsensus", mock.Anything, consensusRequest.GetConsensusRequest()).Return(errors.New("oops")).Once()
		err := svc.Step(stream)
		assert.EqualError(t, err, "oops")
	})
}

func TestSubmitSuccess(t *testing.T) {
	t.Parallel()
	dispatcher := &mocks.Dispatcher{}

	stream := &mocks.StepStream{}
	stream.On("Context").Return(context.Background())
	// Send to the stream 2 messages, and afterwards close the stream
	stream.On("Recv").Return(submitRequest1, nil).Once()
	stream.On("Recv").Return(submitRequest2, nil).Once()
	stream.On("Recv").Return(nil, io.EOF).Once()
	// Send should be called for each corresponding receive
	stream.On("Send", submitResponse1).Return(nil).Twice()

	responses := make(chan *orderer.StepRequest, 2)
	responses <- submitRequest1
	responses <- submitRequest2

	dispatcher.On("DispatchSubmit", mock.Anything, mock.Anything).Return(nil).Once()
	dispatcher.On("DispatchSubmit", mock.Anything, mock.Anything).Return(nil).Once()
	// Ensure we pass requests to DispatchSubmit in-order
	dispatcher.On("DispatchSubmit", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		expectedRequest := <-responses
		actualRequest := args.Get(1).(*orderer.StepRequest)
		assert.True(t, expectedRequest == actualRequest)
	})

	svc := &cluster.Service{
		StreamCountReporter: &cluster.StreamCountReporter{
			Metrics: cluster.NewMetrics(&disabled.Provider{}),
		},
		Logger:     flogging.MustGetLogger("test"),
		StepLogger: flogging.MustGetLogger("test"),
		Dispatcher: dispatcher,
	}

	err := svc.Step(stream)
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
		dispatchReturns    error
		expectedDispatches int
	}{
		{
			name: "Recv() fails",
			receiveReturns: []tuple{
				{msg: nil, err: oops},
			},
		},
		{
			name: "DispatchSubmit() fails",
			receiveReturns: []tuple{
				{msg: submitRequest1},
			},
			expectedDispatches: 1,
			dispatchReturns:    oops,
		},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			dispatcher := &mocks.Dispatcher{}
			stream := &mocks.StepStream{}
			stream.On("Context").Return(context.Background())
			for _, recv := range testCase.receiveReturns {
				stream.On("Recv").Return(recv.asArray()...).Once()
			}
			for _, send := range testCase.sendReturns {
				stream.On("Send", mock.Anything).Return(send).Once()
			}
			defer dispatcher.AssertNumberOfCalls(t, "DispatchSubmit", testCase.expectedDispatches)
			dispatcher.On("DispatchSubmit", mock.Anything, mock.Anything).Return(testCase.dispatchReturns)
			svc := &cluster.Service{
				StreamCountReporter: &cluster.StreamCountReporter{
					Metrics: cluster.NewMetrics(&disabled.Provider{}),
				},
				Logger:     flogging.MustGetLogger("test"),
				StepLogger: flogging.MustGetLogger("test"),
				Dispatcher: dispatcher,
			}
			err := svc.Step(stream)
			assert.EqualError(t, err, oops.Error())
		})
	}
}

func TestIngresStreamsMetrics(t *testing.T) {
	t.Parallel()

	dispatcher := &mocks.Dispatcher{}
	dispatcher.On("DispatchConsensus", mock.Anything, mock.Anything).Return(nil)

	fakeProvider := &mocks.MetricsProvider{}
	testMetrics := &testMetrics{
		fakeProvider: fakeProvider,
	}
	testMetrics.initialize()

	metrics := cluster.NewMetrics(fakeProvider)

	svc := &cluster.Service{
		Logger:     flogging.MustGetLogger("test"),
		StepLogger: flogging.MustGetLogger("test"),
		Dispatcher: dispatcher,
		StreamCountReporter: &cluster.StreamCountReporter{
			Metrics: metrics,
		},
	}

	stream := &mocks.StepStream{}
	stream.On("Context").Return(context.Background())
	// Upon first receive, return nil to proceed to the next receive.
	stream.On("Recv").Return(nil, nil).Once()
	// Upon the second receive, return EOF to trigger the stream to end
	stream.On("Recv").Return(nil, io.EOF).Once()

	svc.Step(stream)
	// The stream started so stream count incremented from 0 to 1
	assert.Equal(t, float64(1), testMetrics.ingressStreamsCount.SetArgsForCall(0))
	// The stream ended so stream count is decremented from 1 to 0
	assert.Equal(t, float64(0), testMetrics.ingressStreamsCount.SetArgsForCall(1))
}

func TestServiceGRPC(t *testing.T) {
	t.Parallel()
	// Check that Service correctly implements the gRPC interface
	srv, err := comm.NewGRPCServer("127.0.0.1:0", comm.ServerConfig{})
	assert.NoError(t, err)
	orderer.RegisterClusterServer(srv.Server(), &cluster.Service{
		Logger:     flogging.MustGetLogger("test"),
		StepLogger: flogging.MustGetLogger("test"),
	})
}

func TestExpirationWarningIngress(t *testing.T) {
	t.Parallel()

	ca, err := tlsgen.NewCA()
	assert.NoError(t, err)

	serverCert, err := ca.NewServerCertKeyPair("127.0.0.1")
	assert.NoError(t, err)

	clientCert, err := ca.NewClientCertKeyPair()
	assert.NoError(t, err)

	dispatcher := &mocks.Dispatcher{}
	dispatcher.On("DispatchConsensus", mock.Anything, mock.Anything).Return(nil)

	svc := &cluster.Service{
		CertExpWarningThreshold:          time.Until(clientCert.TLSCert.NotAfter),
		MinimumExpirationWarningInterval: time.Second * 2,
		StreamCountReporter: &cluster.StreamCountReporter{
			Metrics: cluster.NewMetrics(&disabled.Provider{}),
		},
		Logger:     flogging.MustGetLogger("test"),
		StepLogger: flogging.MustGetLogger("test"),
		Dispatcher: dispatcher,
	}

	alerts := make(chan struct{}, 10)
	svc.Logger = svc.Logger.WithOptions(zap.Hooks(func(entry zapcore.Entry) error {
		if strings.Contains(entry.Message, "expires in less than 23h59m") {
			alerts <- struct{}{}
		}
		return nil
	}))

	srvConf := comm.ServerConfig{
		SecOpts: &comm.SecureOptions{
			Certificate:       serverCert.Cert,
			Key:               serverCert.Key,
			UseTLS:            true,
			ClientRootCAs:     [][]byte{ca.CertBytes()},
			RequireClientCert: true,
		},
	}

	srv, err := comm.NewGRPCServer("127.0.0.1:0", srvConf)
	assert.NoError(t, err)
	orderer.RegisterClusterServer(srv.Server(), svc)
	go srv.Start()
	defer srv.Stop()

	clientConf := comm.ClientConfig{
		Timeout: time.Second * 3,
		SecOpts: &comm.SecureOptions{
			ServerRootCAs:     [][]byte{ca.CertBytes()},
			UseTLS:            true,
			Key:               clientCert.Key,
			Certificate:       clientCert.Cert,
			RequireClientCert: true,
		},
	}

	client, err := comm.NewGRPCClient(clientConf)
	assert.NoError(t, err)

	conn, err := client.NewConnection(srv.Address(), "")
	assert.NoError(t, err)

	cl := orderer.NewClusterClient(conn)
	stream, err := cl.Step(context.Background())
	assert.NoError(t, err)

	err = stream.Send(consensusRequest)
	assert.NoError(t, err)

	// An alert is logged at the first time.
	select {
	case <-alerts:
	case <-time.After(time.Second * 5):
		t.Fatal("Should have received an alert")
	}

	err = stream.Send(consensusRequest)
	assert.NoError(t, err)

	// No alerts in a consecutive time.
	select {
	case <-alerts:
		t.Fatal("Should have not received an alert")
	case <-time.After(time.Millisecond * 500):
	}

	// Wait for alert expiration interval to expire.
	time.Sleep(svc.MinimumExpirationWarningInterval + time.Second)

	err = stream.Send(consensusRequest)
	assert.NoError(t, err)

	// An alert should be logged now after the timeout expired.
	select {
	case <-alerts:
	case <-time.After(time.Second * 5):
		t.Fatal("Should have received an alert")
	}
}
