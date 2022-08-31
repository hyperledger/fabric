/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package cluster

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hyperledger/fabric-protos-go/orderer"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/pkg/errors"
	"go.uber.org/zap"
)

// Stream is used to send/receive messages to/from the remote cluster member.
type Stream struct {
	abortChan <-chan struct{}
	sendBuff  chan struct {
		request *orderer.StepRequest
		report  func(error)
	}
	commShutdown chan struct{}
	abortReason  *atomic.Value
	metrics      *Metrics
	ID           uint64
	Channel      string
	NodeName     string
	Endpoint     string
	Logger       *flogging.FabricLogger
	Timeout      time.Duration
	StepClient   StepClientStream
	Cancel       func(error)
	canceled     *uint32
	expCheck     *certificateExpirationCheck
}

// StreamOperation denotes an operation done by a stream, such a Send or Receive.
type StreamOperation func() (*orderer.StepResponse, error)

// Canceled returns whether the stream was canceled.
func (stream *Stream) Canceled() bool {
	return atomic.LoadUint32(stream.canceled) == uint32(1)
}

// Send sends the given request to the remote cluster member.
func (stream *Stream) Send(request *orderer.StepRequest) error {
	return stream.SendWithReport(request, func(_ error) {})
}

// SendWithReport sends the given request to the remote cluster member and invokes report on the send result.
func (stream *Stream) SendWithReport(request *orderer.StepRequest, report func(error)) error {
	if stream.Canceled() {
		return errors.New(stream.abortReason.Load().(string))
	}
	var allowDrop bool
	// We want to drop consensus transactions if the remote node cannot keep up with us,
	// otherwise we'll slow down the entire FSM.
	if request.GetConsensusRequest() != nil {
		allowDrop = true
	}

	return stream.sendOrDrop(request, allowDrop, report)
}

// sendOrDrop sends the given request to the remote cluster member, or drops it
// if it is a consensus request and the queue is full.
func (stream *Stream) sendOrDrop(request *orderer.StepRequest, allowDrop bool, report func(error)) error {
	msgType := "transaction"
	if allowDrop {
		msgType = "consensus"
	}

	stream.metrics.reportQueueOccupancy(stream.Endpoint, msgType, stream.Channel, len(stream.sendBuff), cap(stream.sendBuff))

	if allowDrop && len(stream.sendBuff) == cap(stream.sendBuff) {
		stream.Cancel(errOverflow)
		stream.metrics.reportMessagesDropped(stream.Endpoint, stream.Channel)
		return errOverflow
	}

	select {
	case <-stream.abortChan:
		return errors.Errorf("stream %d aborted", stream.ID)
	case stream.sendBuff <- struct {
		request *orderer.StepRequest
		report  func(error)
	}{request: request, report: report}:
		return nil
	case <-stream.commShutdown:
		return nil
	}
}

// sendMessage sends the request down the stream
func (stream *Stream) sendMessage(request *orderer.StepRequest, report func(error)) {
	start := time.Now()
	var err error
	defer func() {
		message := fmt.Sprintf("Send of %s to %s(%s) took %v",
			requestAsString(request), stream.NodeName, stream.Endpoint, time.Since(start))
		if err != nil {
			stream.Logger.Warnf("%s but failed due to %s", message, err.Error())
		} else {
			stream.Logger.Debug(message)
		}
	}()

	f := func() (*orderer.StepResponse, error) {
		startSend := time.Now()
		stream.expCheck.checkExpiration(startSend, stream.Channel)
		err := stream.StepClient.Send(request)
		stream.metrics.reportMsgSendTime(stream.Endpoint, stream.Channel, time.Since(startSend))
		return nil, err
	}

	_, err = stream.operateWithTimeout(f, report)
}

func (stream *Stream) serviceStream() {
	streamStartTime := time.Now()
	defer func() {
		stream.Cancel(errAborted)
		stream.Logger.Debugf("Stream %d to (%s) terminated with total lifetime of %s",
			stream.ID, stream.Endpoint, time.Since(streamStartTime))
	}()

	for {
		select {
		case reqReport := <-stream.sendBuff:
			stream.sendMessage(reqReport.request, reqReport.report)
		case <-stream.abortChan:
			return
		case <-stream.commShutdown:
			return
		}
	}
}

// Recv receives a message from a remote cluster member.
func (stream *Stream) Recv() (*orderer.StepResponse, error) {
	start := time.Now()
	defer func() {
		if !stream.Logger.IsEnabledFor(zap.DebugLevel) {
			return
		}
		stream.Logger.Debugf("Receive from %s(%s) took %v", stream.NodeName, stream.Endpoint, time.Since(start))
	}()

	f := func() (*orderer.StepResponse, error) {
		return stream.StepClient.Recv()
	}

	return stream.operateWithTimeout(f, func(_ error) {})
}

// operateWithTimeout performs the given operation on the stream, and blocks until the timeout expires.
func (stream *Stream) operateWithTimeout(invoke StreamOperation, report func(error)) (*orderer.StepResponse, error) {
	timer := time.NewTimer(stream.Timeout)
	defer timer.Stop()

	var operationEnded sync.WaitGroup
	operationEnded.Add(1)

	responseChan := make(chan struct {
		res *orderer.StepResponse
		err error
	}, 1)

	go func() {
		defer operationEnded.Done()
		res, err := invoke()
		responseChan <- struct {
			res *orderer.StepResponse
			err error
		}{res: res, err: err}
	}()

	select {
	case r := <-responseChan:
		report(r.err)
		if r.err != nil {
			stream.Cancel(r.err)
		}
		return r.res, r.err
	case <-timer.C:
		report(errTimeout)
		stream.Logger.Warningf("Stream %d to %s(%s) was forcibly terminated because timeout (%v) expired",
			stream.ID, stream.NodeName, stream.Endpoint, stream.Timeout)
		stream.Cancel(errTimeout)
		// Wait for the operation goroutine to end
		operationEnded.Wait()
		return nil, errTimeout
	}
}
