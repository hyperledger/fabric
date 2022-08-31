/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package cluster

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hyperledger/fabric-protos-go/orderer"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	"google.golang.org/grpc"
)

// RemoteContext interacts with remote cluster
// nodes. Every call can be aborted via call to Abort()
type RemoteContext struct {
	expiresAt                        time.Time
	minimumExpirationWarningInterval time.Duration
	certExpWarningThreshold          time.Duration
	Metrics                          *Metrics
	Channel                          string
	SendBuffSize                     int
	shutdownSignal                   chan struct{}
	Logger                           *flogging.FabricLogger
	endpoint                         string
	GetStreamFunc                    func(context.Context) (StepClientStream, error) // interface{}
	ProbeConn                        func(conn *grpc.ClientConn) error
	conn                             *grpc.ClientConn
	nextStreamID                     uint64
	streamsByID                      streamsMapperReporter
	workerCountReporter              workerCountReporter
}

// NewStream creates a new stream.
// It is not thread safe, and Send() or Recv() block only until the timeout expires.
func (rc *RemoteContext) NewStream(timeout time.Duration) (*Stream, error) {
	if err := rc.ProbeConn(rc.conn); err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.TODO())
	stream, err := rc.GetStreamFunc(ctx)
	if err != nil {
		cancel()
		return nil, errors.WithStack(err)
	}

	streamID := atomic.AddUint64(&rc.nextStreamID, 1)
	nodeName := commonNameFromContext(stream.Context())

	var canceled uint32

	abortChan := make(chan struct{})
	abortReason := &atomic.Value{}

	once := &sync.Once{}

	cancelWithReason := func(err error) {
		once.Do(func() {
			abortReason.Store(err.Error())
			cancel()
			rc.streamsByID.Delete(streamID)
			rc.Metrics.reportEgressStreamCount(rc.Channel, atomic.LoadUint32(&rc.streamsByID.size))
			rc.Logger.Debugf("Stream %d to %s(%s) is aborted", streamID, nodeName, rc.endpoint)
			atomic.StoreUint32(&canceled, 1)
			close(abortChan)
		})
	}

	logger := flogging.MustGetLogger("orderer.common.cluster.step")
	stepLogger := logger.WithOptions(zap.AddCallerSkip(1))

	s := &Stream{
		Channel:     rc.Channel,
		metrics:     rc.Metrics,
		abortReason: abortReason,
		abortChan:   abortChan,
		sendBuff: make(chan struct {
			request *orderer.StepRequest
			report  func(error)
		}, rc.SendBuffSize),
		commShutdown: rc.shutdownSignal,
		NodeName:     nodeName,
		Logger:       stepLogger,
		ID:           streamID,
		Endpoint:     rc.endpoint,
		Timeout:      timeout,
		StepClient:   stream,
		Cancel:       cancelWithReason,
		canceled:     &canceled,
	}

	s.expCheck = &certificateExpirationCheck{
		minimumExpirationWarningInterval: rc.minimumExpirationWarningInterval,
		expirationWarningThreshold:       rc.certExpWarningThreshold,
		expiresAt:                        rc.expiresAt,
		endpoint:                         s.Endpoint,
		nodeName:                         s.NodeName,
		alert: func(template string, args ...interface{}) {
			s.Logger.Warningf(template, args...)
		},
	}

	err = stream.Auth()
	if err != nil {
		return nil, errors.Wrap(err, "failed to create new stream")
	}

	rc.Logger.Debugf("Created new stream to %s with ID of %d and buffer size of %d",
		rc.endpoint, streamID, cap(s.sendBuff))

	rc.streamsByID.Store(streamID, s)
	rc.Metrics.reportEgressStreamCount(rc.Channel, atomic.LoadUint32(&rc.streamsByID.size))

	go func() {
		rc.workerCountReporter.increment(s.metrics)
		s.serviceStream()
		rc.workerCountReporter.decrement(s.metrics)
	}()

	return s, nil
}

// Abort aborts the contexts the RemoteContext uses, thus effectively
// causes all operations that use this RemoteContext to terminate.
func (rc *RemoteContext) Abort() {
	rc.streamsByID.Range(func(_, value interface{}) bool {
		value.(*Stream).Cancel(errAborted)
		return false
	})
}
