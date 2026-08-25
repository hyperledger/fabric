/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package broadcast

import (
	"context"
	"io"
	"time"

	"github.com/hyperledger/fabric-lib-go/common/flogging"
	cb "github.com/hyperledger/fabric-protos-go-apiv2/common"
	ab "github.com/hyperledger/fabric-protos-go-apiv2/orderer"
	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/internal/pkg/telemetry"
	"github.com/hyperledger/fabric/orderer/common/msgprocessor"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

var logger = flogging.MustGetLogger("orderer.common.broadcast")

//go:generate counterfeiter -o mock/channel_support_registrar.go --fake-name ChannelSupportRegistrar . ChannelSupportRegistrar

// ChannelSupportRegistrar provides a way for the Handler to look up the Support for a channel
type ChannelSupportRegistrar interface {
	// BroadcastChannelSupport returns the message channel header, whether the message is a config update
	// and the channel resources for a message or an error if the message is not a message which can
	// be processed directly (like CONFIG and ORDERER_TRANSACTION messages)
	BroadcastChannelSupport(msg *cb.Envelope) (*cb.ChannelHeader, bool, ChannelSupport, error)
}

//go:generate counterfeiter -o mock/channel_support.go --fake-name ChannelSupport . ChannelSupport

// ChannelSupport provides the backing resources needed to support broadcast on a channel
type ChannelSupport interface {
	msgprocessor.Processor
	Consenter
}

// Consenter provides methods to send messages through consensus
type Consenter interface {
	// Order accepts a message or returns an error indicating the cause of failure
	// It ultimately passes through to the consensus.Chain interface
	Order(env *cb.Envelope, configSeq uint64) error

	// Configure accepts a reconfiguration or returns an error indicating the cause of failure
	// It ultimately passes through to the consensus.Chain interface
	Configure(config *cb.Envelope, configSeq uint64) error

	// WaitReady blocks waiting for consenter to be ready for accepting new messages.
	// This is useful when consenter needs to temporarily block ingress messages so
	// that in-flight messages can be consumed. It could return error if consenter is
	// in erroneous states. If this blocking behavior is not desired, consenter could
	// simply return nil.
	WaitReady() error
}

// Handler is designed to handle connections from Broadcast AB gRPC service
type Handler struct {
	SupportRegistrar ChannelSupportRegistrar
	Metrics          *Metrics
}

// Handle reads requests from a Broadcast stream, processes them, and returns the responses to the stream
func (bh *Handler) Handle(srv ab.AtomicBroadcast_BroadcastServer) error {
	addr := util.ExtractRemoteAddress(srv.Context())
	logger.Debugf("Starting new broadcast loop for %s", addr)
	for {
		msg, err := srv.Recv()
		if errors.Is(err, io.EOF) {
			logger.Debugf("Received EOF from %s, hangup", addr)
			return nil
		}
		if err != nil {
			logger.Warningf("Error reading from %s: %s", addr, err)
			return err
		}

		resp := bh.ProcessMessage(srv.Context(), msg, addr)
		err = srv.Send(resp)
		if resp.Status != cb.Status_SUCCESS {
			return err
		}

		if err != nil {
			logger.Warningf("Error sending to %s: %s", addr, err)
			return err
		}
	}
}

type MetricsTracker struct {
	ValidateStartTime time.Time
	EnqueueStartTime  time.Time
	ValidateDuration  time.Duration
	ChannelID         string
	TxType            string
	Metrics           *Metrics
}

func (mt *MetricsTracker) Record(resp *ab.BroadcastResponse) {
	labels := []string{
		"status", resp.Status.String(),
		"channel", mt.ChannelID,
		"type", mt.TxType,
	}

	if mt.ValidateDuration == 0 {
		mt.EndValidate()
	}
	mt.Metrics.ValidateDuration.With(labels...).Observe(mt.ValidateDuration.Seconds())

	if mt.EnqueueStartTime != (time.Time{}) {
		enqueueDuration := time.Since(mt.EnqueueStartTime)
		mt.Metrics.EnqueueDuration.With(labels...).Observe(enqueueDuration.Seconds())
	}

	mt.Metrics.ProcessedCount.With(labels...).Add(1)
}

func (mt *MetricsTracker) BeginValidate() {
	mt.ValidateStartTime = time.Now()
}

func (mt *MetricsTracker) EndValidate() {
	mt.ValidateDuration = time.Since(mt.ValidateStartTime)
}

func (mt *MetricsTracker) BeginEnqueue() {
	mt.EnqueueStartTime = time.Now()
}

// envelopeTraceContext recovers the trace context a client wrote into a
// transaction before signing it.
//
// The Broadcast stream is long-lived and often carries transactions from many
// different traces, so the trace context on the stream itself identifies at best
// the connection and not the transaction. When the client has embedded its trace
// context in the envelope, that is the accurate parent for this message.
//
// This re-parses the envelope header, which BroadcastChannelSupport will parse
// again a moment later. That duplicate unmarshal is why the whole thing is
// skipped unless tracing is actually exporting.
func envelopeTraceContext(msg *cb.Envelope) trace.SpanContext {
	if !telemetry.Enabled() {
		return trace.SpanContext{}
	}

	payload, err := protoutil.UnmarshalPayload(msg.Payload)
	if err != nil || payload.Header == nil {
		return trace.SpanContext{}
	}
	chdr, err := protoutil.UnmarshalChannelHeader(payload.Header.ChannelHeader)
	if err != nil {
		return trace.SpanContext{}
	}
	return telemetry.TraceContextFromHeaderExtension(chdr.Extension)
}

// ProcessMessage validates and enqueues a single message
func (bh *Handler) ProcessMessage(ctx context.Context, msg *cb.Envelope, addr string) (resp *ab.BroadcastResponse) {
	// Prefer the trace context carried inside the signed transaction; fall back
	// to the stream's, which is correct for clients that open a stream per
	// transaction.
	if sc := envelopeTraceContext(msg); sc.IsValid() {
		ctx = trace.ContextWithRemoteSpanContext(ctx, sc)
	}

	tracer := telemetry.Tracer(telemetry.TracerBroadcast)
	ctx, span := tracer.Start(ctx, "Broadcast.ProcessMessage", trace.WithSpanKind(trace.SpanKindServer))
	defer span.End()

	tracker := &MetricsTracker{
		ChannelID: "unknown",
		TxType:    "unknown",
		Metrics:   bh.Metrics,
	}
	defer func() {
		// This looks a little unnecessary, but if done directly as
		// a defer, resp gets the (always nil) current state of resp
		// and not the return value
		tracker.Record(resp)
		if resp != nil {
			span.SetAttributes(telemetry.AttrResponseStatus.String(resp.Status.String()))
			if resp.Status != cb.Status_SUCCESS {
				span.SetStatus(codes.Error, resp.Status.String())
			}
		}
	}()
	tracker.BeginValidate()

	chdr, isConfig, processor, err := bh.SupportRegistrar.BroadcastChannelSupport(msg)
	if chdr != nil {
		tracker.ChannelID = chdr.ChannelId
		tracker.TxType = cb.HeaderType(chdr.Type).String()
		span.SetAttributes(
			telemetry.AttrChannelID.String(chdr.ChannelId),
			telemetry.AttrTxID.String(chdr.TxId),
			telemetry.AttrTxType.String(cb.HeaderType(chdr.Type).String()),
		)
	}
	if err != nil {
		logger.Warningf("[channel: %s] Could not get message processor for serving %s: %s", tracker.ChannelID, addr, err)
		return &ab.BroadcastResponse{Status: cb.Status_BAD_REQUEST, Info: err.Error()}
	}

	if !isConfig {
		logger.Debugf("[channel: %s] Broadcast is processing normal message from %s with txid '%s' of type %s", chdr.ChannelId, addr, chdr.TxId, cb.HeaderType_name[chdr.Type])

		var configSeq uint64
		err = inSpan(ctx, tracer, "Broadcast.ProcessNormalMsg", func() error {
			var err error
			configSeq, err = processor.ProcessNormalMsg(msg)
			return err
		})
		if err != nil {
			logger.Warningf("[channel: %s] Rejecting broadcast of normal message from %s because of error: %s", chdr.ChannelId, addr, err)
			return &ab.BroadcastResponse{Status: ClassifyError(err), Info: err.Error()}
		}
		tracker.EndValidate()

		tracker.BeginEnqueue()
		// WaitReady blocks while the consenter is applying backpressure, so a
		// long span here means the ordering service is the bottleneck rather
		// than validation or the client.
		if err = inSpan(ctx, tracer, "Broadcast.WaitReady", processor.WaitReady); err != nil {
			logger.Warningf("[channel: %s] Rejecting broadcast of message from %s with SERVICE_UNAVAILABLE: rejected by Consenter: %s", chdr.ChannelId, addr, err)
			return &ab.BroadcastResponse{Status: cb.Status_SERVICE_UNAVAILABLE, Info: err.Error()}
		}

		// Order only enqueues the message for the consenter. The block it ends
		// up in is cut asynchronously, so this span deliberately does not cover
		// consensus; that is what the block commit spans on the peers show.
		err = inSpan(ctx, tracer, "Broadcast.Order", func() error {
			return processor.Order(msg, configSeq)
		})
		if err != nil {
			logger.Warningf("[channel: %s] Rejecting broadcast of normal message from %s with SERVICE_UNAVAILABLE: rejected by Order: %s", chdr.ChannelId, addr, err)
			return &ab.BroadcastResponse{Status: cb.Status_SERVICE_UNAVAILABLE, Info: err.Error()}
		}
	} else { // isConfig
		logger.Debugf("[channel: %s] Broadcast is processing config update message from %s", chdr.ChannelId, addr)

		var config *cb.Envelope
		var configSeq uint64
		err = inSpan(ctx, tracer, "Broadcast.ProcessConfigUpdateMsg", func() error {
			var err error
			config, configSeq, err = processor.ProcessConfigUpdateMsg(msg)
			return err
		})
		if err != nil {
			logger.Warningf("[channel: %s] Rejecting broadcast of config message from %s because of error: %s", chdr.ChannelId, addr, err)
			return &ab.BroadcastResponse{Status: ClassifyError(err), Info: err.Error()}
		}
		tracker.EndValidate()

		tracker.BeginEnqueue()
		if err = inSpan(ctx, tracer, "Broadcast.WaitReady", processor.WaitReady); err != nil {
			logger.Warningf("[channel: %s] Rejecting broadcast of message from %s with SERVICE_UNAVAILABLE: rejected by Consenter: %s", chdr.ChannelId, addr, err)
			return &ab.BroadcastResponse{Status: cb.Status_SERVICE_UNAVAILABLE, Info: err.Error()}
		}

		err = inSpan(ctx, tracer, "Broadcast.Configure", func() error {
			return processor.Configure(config, configSeq)
		})
		if err != nil {
			logger.Warningf("[channel: %s] Rejecting broadcast of config message from %s with SERVICE_UNAVAILABLE: rejected by Configure: %s", chdr.ChannelId, addr, err)
			return &ab.BroadcastResponse{Status: cb.Status_SERVICE_UNAVAILABLE, Info: err.Error()}
		}
	}

	logger.Debugf("[channel: %s] Broadcast has successfully enqueued message of type %s from %s", chdr.ChannelId, cb.HeaderType_name[chdr.Type], addr)

	return &ab.BroadcastResponse{Status: cb.Status_SUCCESS}
}

// inSpan runs fn inside a child span, recording any error it returns. The
// broadcast path is a sequence of discrete steps whose individual durations are
// the whole point of tracing it, and this keeps that from burying the logic in
// span bookkeeping.
func inSpan(ctx context.Context, tracer trace.Tracer, name string, fn func() error) error {
	_, span := tracer.Start(ctx, name)
	defer span.End()

	err := fn()
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	return err
}

// ClassifyError converts an error type into a status code.
func ClassifyError(err error) cb.Status {
	switch errors.Cause(err) {
	case msgprocessor.ErrChannelDoesNotExist:
		return cb.Status_NOT_FOUND
	case msgprocessor.ErrPermissionDenied:
		return cb.Status_FORBIDDEN
	case msgprocessor.ErrMaintenanceMode:
		return cb.Status_SERVICE_UNAVAILABLE
	default:
		return cb.Status_BAD_REQUEST
	}
}
