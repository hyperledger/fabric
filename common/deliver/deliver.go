/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package deliver

import (
	"context"
	"io"
	"math"
	"strconv"
	"time"

	"github.com/golang/protobuf/proto"
	cb "github.com/hyperledger/fabric-protos-go/common"
	ab "github.com/hyperledger/fabric-protos-go/orderer"
	"github.com/hyperledger/fabric/common/crypto"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/ledger/blockledger"
	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
)

var logger = flogging.MustGetLogger("common.deliver")

//go:generate counterfeiter -o mock/chain_manager.go -fake-name ChainManager . ChainManager

// ChainManager provides a way for the Handler to look up the Chain.
type ChainManager interface {
	GetChain(chainID string) Chain
}

//go:generate counterfeiter -o mock/chain.go -fake-name Chain . Chain

// Chain encapsulates chain operations and data.
type Chain interface {
	// Sequence returns the current config sequence number, can be used to detect config changes
	Sequence() uint64

	// PolicyManager returns the current policy manager as specified by the chain configuration
	PolicyManager() policies.Manager

	// Reader returns the chain Reader for the chain
	Reader() blockledger.Reader

	// Errored returns a channel which closes when the backing consenter has errored
	Errored() <-chan struct{}
}

//go:generate counterfeiter -o mock/policy_checker.go -fake-name PolicyChecker . PolicyChecker

// PolicyChecker checks the envelope against the policy logic supplied by the
// function.
type PolicyChecker interface {
	CheckPolicy(envelope *cb.Envelope, channelID string) error
}

// The PolicyCheckerFunc is an adapter that allows the use of an ordinary
// function as a PolicyChecker.
type PolicyCheckerFunc func(envelope *cb.Envelope, channelID string) error

// CheckPolicy calls pcf(envelope, channelID)
func (pcf PolicyCheckerFunc) CheckPolicy(envelope *cb.Envelope, channelID string) error {
	return pcf(envelope, channelID)
}

//go:generate counterfeiter -o mock/inspector.go -fake-name Inspector . Inspector

// Inspector verifies an appropriate binding between the message and the context.
type Inspector interface {
	Inspect(context.Context, proto.Message) error
}

// The InspectorFunc is an adapter that allows the use of an ordinary
// function as an Inspector.
type InspectorFunc func(context.Context, proto.Message) error

// Inspect calls inspector(ctx, p)
func (inspector InspectorFunc) Inspect(ctx context.Context, p proto.Message) error {
	return inspector(ctx, p)
}

// Handler handles server requests.
type Handler struct {
	ExpirationCheckFunc func(identityBytes []byte) time.Time
	ChainManager        ChainManager
	TimeWindow          time.Duration
	BindingInspector    Inspector
	Metrics             *Metrics
}

//go:generate counterfeiter -o mock/receiver.go -fake-name Receiver . Receiver

// Receiver is used to receive enveloped seek requests.
type Receiver interface {
	Recv() (*cb.Envelope, error)
}

//go:generate counterfeiter -o mock/response_sender.go -fake-name ResponseSender . ResponseSender

// ResponseSender defines the interface a handler must implement to send
// responses.
type ResponseSender interface {
	// SendStatusResponse sends completion status to the client.
	SendStatusResponse(status cb.Status) error
	// SendBlockResponse sends the block and optionally private data to the client.
	SendBlockResponse(data *cb.Block, channelID string, chain Chain, signedData *protoutil.SignedData) error
	// DataType returns the data type sent by the sender
	DataType() string
}

// Filtered is a marker interface that indicates a response sender
// is configured to send filtered blocks
// Note: this is replaced by "data_type" label. Keep it for now until we decide how to take care of compatibility issue.
type Filtered interface {
	IsFiltered() bool
}

// Server is a polymorphic structure to support generalization of this handler
// to be able to deliver different type of responses.
type Server struct {
	Receiver
	PolicyChecker
	ResponseSender
}

// ExtractChannelHeaderCertHash extracts the TLS cert hash from a channel header.
func ExtractChannelHeaderCertHash(msg proto.Message) []byte {
	chdr, isChannelHeader := msg.(*cb.ChannelHeader)
	if !isChannelHeader || chdr == nil {
		return nil
	}
	return chdr.TlsCertHash
}

// NewHandler creates an implementation of the Handler interface.
func NewHandler(cm ChainManager, timeWindow time.Duration, mutualTLS bool, metrics *Metrics, expirationCheckDisabled bool) *Handler {
	expirationCheck := crypto.ExpiresAt
	if expirationCheckDisabled {
		expirationCheck = noExpiration
	}
	return &Handler{
		ChainManager:        cm,
		TimeWindow:          timeWindow,
		BindingInspector:    InspectorFunc(NewBindingInspector(mutualTLS, ExtractChannelHeaderCertHash)),
		Metrics:             metrics,
		ExpirationCheckFunc: expirationCheck,
	}
}

// Handle receives incoming deliver requests.
func (h *Handler) Handle(ctx context.Context, srv *Server) error {
	addr := util.ExtractRemoteAddress(ctx)
	logger.Debugf("Starting new deliver loop for %s", addr)
	h.Metrics.StreamsOpened.Add(1)
	defer h.Metrics.StreamsClosed.Add(1)
	for {
		logger.Debugf("Attempting to read seek info message from %s", addr)
		envelope, err := srv.Recv()
		if err == io.EOF {
			logger.Debugf("Received EOF from %s, hangup", addr)
			return nil
		}
		if err != nil {
			logger.Warningf("Error reading from %s: %s", addr, err)
			return err
		}

		status, err := h.deliverBlocks(ctx, srv, envelope)
		if err != nil {
			return err
		}

		err = srv.SendStatusResponse(status)
		if status != cb.Status_SUCCESS {
			return err
		}
		if err != nil {
			logger.Warningf("Error sending to %s: %s", addr, err)
			return err
		}

		logger.Debugf("Waiting for new SeekInfo from %s", addr)
	}
}

func isFiltered(srv *Server) bool {
	if filtered, ok := srv.ResponseSender.(Filtered); ok {
		return filtered.IsFiltered()
	}
	return false
}

func (h *Handler) deliverBlocks(ctx context.Context, srv *Server, envelope *cb.Envelope) (status cb.Status, err error) {
	addr := util.ExtractRemoteAddress(ctx)
	payload, chdr, shdr, err := h.parseEnvelope(ctx, envelope)
	if err != nil {
		logger.Warningf("error parsing envelope from %s: %s", addr, err)
		return cb.Status_BAD_REQUEST, nil
	}

	chain := h.ChainManager.GetChain(chdr.ChannelId)
	if chain == nil {
		// Note, we log this at DEBUG because SDKs will poll waiting for channels to be created
		// So we would expect our log to be somewhat flooded with these
		logger.Debugf("Rejecting deliver for %s because channel %s not found", addr, chdr.ChannelId)
		return cb.Status_NOT_FOUND, nil
	}

	labels := []string{
		"channel", chdr.ChannelId,
		"filtered", strconv.FormatBool(isFiltered(srv)),
		"data_type", srv.DataType(),
	}
	h.Metrics.RequestsReceived.With(labels...).Add(1)
	defer func() {
		labels := append(labels, "success", strconv.FormatBool(status == cb.Status_SUCCESS))
		h.Metrics.RequestsCompleted.With(labels...).Add(1)
	}()

	seekInfo := &ab.SeekInfo{}
	if err = proto.Unmarshal(payload.Data, seekInfo); err != nil {
		logger.Warningf("[channel: %s] Received a signed deliver request from %s with malformed seekInfo payload: %s", chdr.ChannelId, addr, err)
		return cb.Status_BAD_REQUEST, nil
	}

	erroredChan := chain.Errored()
	if seekInfo.ErrorResponse == ab.SeekInfo_BEST_EFFORT {
		// In a 'best effort' delivery of blocks, we should ignore consenter errors
		// and continue to deliver blocks according to the client's request.
		erroredChan = nil
	}
	select {
	case <-erroredChan:
		logger.Warningf("[channel: %s] Rejecting deliver request for %s because of consenter error", chdr.ChannelId, addr)
		return cb.Status_SERVICE_UNAVAILABLE, nil
	default:
	}

	accessControl, err := NewSessionAC(chain, envelope, srv.PolicyChecker, chdr.ChannelId, h.ExpirationCheckFunc)
	if err != nil {
		logger.Warningf("[channel: %s] failed to create access control object due to %s", chdr.ChannelId, err)
		return cb.Status_BAD_REQUEST, nil
	}

	if err := accessControl.Evaluate(); err != nil {
		logger.Warningf("[channel: %s] Client %s is not authorized: %s", chdr.ChannelId, addr, err)
		return cb.Status_FORBIDDEN, nil
	}

	if seekInfo.Start == nil || seekInfo.Stop == nil {
		logger.Warningf("[channel: %s] Received seekInfo message from %s with missing start or stop %v, %v", chdr.ChannelId, addr, seekInfo.Start, seekInfo.Stop)
		return cb.Status_BAD_REQUEST, nil
	}

	logger.Debugf("[channel: %s] Received seekInfo (%p) %v from %s", chdr.ChannelId, seekInfo, seekInfo, addr)

	cursor, number := chain.Reader().Iterator(seekInfo.Start)
	defer cursor.Close()
	var stopNum uint64
	switch stop := seekInfo.Stop.Type.(type) {
	case *ab.SeekPosition_Oldest:
		stopNum = number
	case *ab.SeekPosition_Newest:
		// when seeking only the newest block (i.e. starting
		// and stopping at newest), don't reevaluate the ledger
		// height as this can lead to multiple blocks being
		// sent when only one is expected
		if proto.Equal(seekInfo.Start, seekInfo.Stop) {
			stopNum = number
			break
		}
		stopNum = chain.Reader().Height() - 1
	case *ab.SeekPosition_Specified:
		stopNum = stop.Specified.Number
		if stopNum < number {
			logger.Warningf("[channel: %s] Received invalid seekInfo message from %s: start number %d greater than stop number %d", chdr.ChannelId, addr, number, stopNum)
			return cb.Status_BAD_REQUEST, nil
		}
	}

	for {
		if seekInfo.Behavior == ab.SeekInfo_FAIL_IF_NOT_READY {
			if number > chain.Reader().Height()-1 {
				logger.Warningf("[channel: %s] Block %d not found, block number greater than chain length bounds", chdr.ChannelId, number)
				return cb.Status_NOT_FOUND, nil
			}
		}

		var block *cb.Block
		var status cb.Status

		iterCh := make(chan struct{})
		go func() {
			block, status = cursor.Next()
			close(iterCh)
		}()

		select {
		case <-ctx.Done():
			logger.Debugf("Context canceled, aborting wait for next block")
			return cb.Status_INTERNAL_SERVER_ERROR, errors.Wrapf(ctx.Err(), "context finished before block retrieved")
		case <-erroredChan:
			// TODO, today, the only user of the errorChan is the orderer consensus implementations.  If the peer ever reports
			// this error, we will need to update this error message, possibly finding a way to signal what error text to return.
			logger.Warningf("Aborting deliver for request because the backing consensus implementation indicates an error")
			return cb.Status_SERVICE_UNAVAILABLE, nil
		case <-iterCh:
			// Iterator has set the block and status vars
		}

		if status != cb.Status_SUCCESS {
			logger.Warningf("[channel: %s] Error reading from channel, cause was: %v", chdr.ChannelId, status)
			return status, nil
		}

		// increment block number to support FAIL_IF_NOT_READY deliver behavior
		number++

		if err := accessControl.Evaluate(); err != nil {
			logger.Warningf("[channel: %s] Client authorization revoked for deliver request from %s: %s", chdr.ChannelId, addr, err)
			return cb.Status_FORBIDDEN, nil
		}

		logger.Debugf("[channel: %s] Delivering block [%d] for (%p) for %s", chdr.ChannelId, block.Header.Number, seekInfo, addr)

		signedData := &protoutil.SignedData{Data: envelope.Payload, Identity: shdr.Creator, Signature: envelope.Signature}
		if err := srv.SendBlockResponse(block, chdr.ChannelId, chain, signedData); err != nil {
			logger.Warningf("[channel: %s] Error sending to %s: %s", chdr.ChannelId, addr, err)
			return cb.Status_INTERNAL_SERVER_ERROR, err
		}

		h.Metrics.BlocksSent.With(labels...).Add(1)

		if stopNum == block.Header.Number {
			break
		}
	}

	logger.Debugf("[channel: %s] Done delivering to %s for (%p)", chdr.ChannelId, addr, seekInfo)

	return cb.Status_SUCCESS, nil
}

func (h *Handler) parseEnvelope(ctx context.Context, envelope *cb.Envelope) (*cb.Payload, *cb.ChannelHeader, *cb.SignatureHeader, error) {
	payload, err := protoutil.UnmarshalPayload(envelope.Payload)
	if err != nil {
		return nil, nil, nil, err
	}

	if payload.Header == nil {
		return nil, nil, nil, errors.New("envelope has no header")
	}

	chdr, err := protoutil.UnmarshalChannelHeader(payload.Header.ChannelHeader)
	if err != nil {
		return nil, nil, nil, err
	}

	shdr, err := protoutil.UnmarshalSignatureHeader(payload.Header.SignatureHeader)
	if err != nil {
		return nil, nil, nil, err
	}

	err = h.validateChannelHeader(ctx, chdr)
	if err != nil {
		return nil, nil, nil, err
	}

	return payload, chdr, shdr, nil
}

func (h *Handler) validateChannelHeader(ctx context.Context, chdr *cb.ChannelHeader) error {
	if chdr.GetTimestamp() == nil {
		err := errors.New("channel header in envelope must contain timestamp")
		return err
	}

	envTime := time.Unix(chdr.GetTimestamp().Seconds, int64(chdr.GetTimestamp().Nanos)).UTC()
	serverTime := time.Now()

	if math.Abs(float64(serverTime.UnixNano()-envTime.UnixNano())) > float64(h.TimeWindow.Nanoseconds()) {
		err := errors.Errorf("envelope timestamp %s is more than %s apart from current server time %s", envTime, h.TimeWindow, serverTime)
		return err
	}

	err := h.BindingInspector.Inspect(ctx, chdr)
	if err != nil {
		return err
	}

	return nil
}

func noExpiration(_ []byte) time.Time {
	return time.Time{}
}

func (h *Handler) HandleAttestation(ctx context.Context, srv *Server, env *cb.Envelope) error {
	status, err := h.deliverBlocks(ctx, srv, env)
	if err != nil {
		return err
	}

	err = srv.SendStatusResponse(status)
	if status != cb.Status_SUCCESS {
		return err
	}
	if err != nil {
		addr := util.ExtractRemoteAddress(ctx)
		logger.Warningf("Error sending to %s: %s", addr, err)
		return err
	}
	return nil
}
