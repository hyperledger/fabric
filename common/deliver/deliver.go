/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package deliver

import (
	"context"
	"io"
	"math"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/crypto"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/ledger/blockledger"
	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/core/comm"
	cb "github.com/hyperledger/fabric/protos/common"
	ab "github.com/hyperledger/fabric/protos/orderer"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/pkg/errors"
)

var logger = flogging.MustGetLogger("common/deliver")

//go:generate counterfeiter -o mock/chain_manager.go -fake-name ChainManager . ChainManager

// ChainManager provides a way for the Handler to look up the Chain.
type ChainManager interface {
	GetChain(chainID string) (Chain, bool)
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
	ChainManager     ChainManager
	TimeWindow       time.Duration
	BindingInspector Inspector
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
	SendStatusResponse(status cb.Status) error
	SendBlockResponse(block *cb.Block) error
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
func NewHandler(cm ChainManager, timeWindow time.Duration, mutualTLS bool) *Handler {
	return &Handler{
		ChainManager:     cm,
		TimeWindow:       timeWindow,
		BindingInspector: InspectorFunc(comm.NewBindingInspector(mutualTLS, ExtractChannelHeaderCertHash)),
	}
}

// Handle receives incoming deliver requests.
func (h *Handler) Handle(ctx context.Context, srv *Server) error {
	addr := util.ExtractRemoteAddress(ctx)
	logger.Debugf("Starting new deliver loop for %s", addr)
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

		if err := h.deliverBlocks(ctx, srv, envelope); err != nil {
			return err
		}

		logger.Debugf("Waiting for new SeekInfo from %s", addr)
	}
}

func (h *Handler) deliverBlocks(ctx context.Context, srv *Server, envelope *cb.Envelope) error {
	addr := util.ExtractRemoteAddress(ctx)
	payload, err := utils.UnmarshalPayload(envelope.Payload)
	if err != nil {
		logger.Warningf("Received an envelope from %s with no payload: %s", addr, err)
		return srv.SendStatusResponse(cb.Status_BAD_REQUEST)
	}

	if payload.Header == nil {
		logger.Warningf("Malformed envelope received from %s with bad header", addr)
		return srv.SendStatusResponse(cb.Status_BAD_REQUEST)
	}

	chdr, err := utils.UnmarshalChannelHeader(payload.Header.ChannelHeader)
	if err != nil {
		logger.Warningf("Failed to unmarshal channel header from %s: %s", addr, err)
		return srv.SendStatusResponse(cb.Status_BAD_REQUEST)
	}

	err = h.validateChannelHeader(ctx, chdr)
	if err != nil {
		logger.Warningf("Rejecting deliver for %s due to envelope validation error: %s", addr, err)
		return srv.SendStatusResponse(cb.Status_BAD_REQUEST)
	}

	chain, ok := h.ChainManager.GetChain(chdr.ChannelId)
	if !ok {
		// Note, we log this at DEBUG because SDKs will poll waiting for channels to be created
		// So we would expect our log to be somewhat flooded with these
		logger.Debugf("Rejecting deliver for %s because channel %s not found", addr, chdr.ChannelId)
		return srv.SendStatusResponse(cb.Status_NOT_FOUND)
	}

	erroredChan := chain.Errored()
	select {
	case <-erroredChan:
		logger.Warningf("[channel: %s] Rejecting deliver request for %s because of consenter error", chdr.ChannelId, addr)
		return srv.SendStatusResponse(cb.Status_SERVICE_UNAVAILABLE)
	default:

	}

	accessControl, err := NewSessionAC(chain, envelope, srv.PolicyChecker, chdr.ChannelId, crypto.ExpiresAt)
	if err != nil {
		logger.Warningf("[channel: %s] failed to create access control object due to %s", chdr.ChannelId, err)
		return srv.SendStatusResponse(cb.Status_BAD_REQUEST)
	}

	if err := accessControl.Evaluate(); err != nil {
		logger.Warningf("[channel: %s] Client authorization revoked for deliver request from %s: %s", chdr.ChannelId, addr, err)
		return srv.SendStatusResponse(cb.Status_FORBIDDEN)
	}

	seekInfo := &ab.SeekInfo{}
	if err = proto.Unmarshal(payload.Data, seekInfo); err != nil {
		logger.Warningf("[channel: %s] Received a signed deliver request from %s with malformed seekInfo payload: %s", chdr.ChannelId, addr, err)
		return srv.SendStatusResponse(cb.Status_BAD_REQUEST)
	}

	if seekInfo.Start == nil || seekInfo.Stop == nil {
		logger.Warningf("[channel: %s] Received seekInfo message from %s with missing start or stop %v, %v", chdr.ChannelId, addr, seekInfo.Start, seekInfo.Stop)
		return srv.SendStatusResponse(cb.Status_BAD_REQUEST)
	}

	logger.Debugf("[channel: %s] Received seekInfo (%p) %v from %s", chdr.ChannelId, seekInfo, seekInfo, addr)

	cursor, number := chain.Reader().Iterator(seekInfo.Start)
	defer cursor.Close()
	var stopNum uint64
	switch stop := seekInfo.Stop.Type.(type) {
	case *ab.SeekPosition_Oldest:
		stopNum = number
	case *ab.SeekPosition_Newest:
		stopNum = chain.Reader().Height() - 1
	case *ab.SeekPosition_Specified:
		stopNum = stop.Specified.Number
		if stopNum < number {
			logger.Warningf("[channel: %s] Received invalid seekInfo message from %s: start number %d greater than stop number %d", chdr.ChannelId, addr, number, stopNum)
			return srv.SendStatusResponse(cb.Status_BAD_REQUEST)
		}
	}

	for {
		if seekInfo.Behavior == ab.SeekInfo_FAIL_IF_NOT_READY {
			if number > chain.Reader().Height()-1 {
				return srv.SendStatusResponse(cb.Status_NOT_FOUND)
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
			return errors.Wrapf(ctx.Err(), "context finished before block retrieved")
		case <-erroredChan:
			logger.Warningf("Aborting deliver for request because of background error")
			return srv.SendStatusResponse(cb.Status_SERVICE_UNAVAILABLE)
		case <-iterCh:
			// Iterator has set the block and status vars
		}

		if status != cb.Status_SUCCESS {
			logger.Errorf("[channel: %s] Error reading from channel, cause was: %v", chdr.ChannelId, status)
			return srv.SendStatusResponse(status)
		}

		// increment block number to support FAIL_IF_NOT_READY deliver behavior
		number++

		if err := accessControl.Evaluate(); err != nil {
			logger.Warningf("[channel: %s] Client authorization revoked for deliver request from %s: %s", chdr.ChannelId, addr, err)
			return srv.SendStatusResponse(cb.Status_FORBIDDEN)
		}

		logger.Debugf("[channel: %s] Delivering block for (%p) for %s", chdr.ChannelId, seekInfo, addr)

		if err := srv.SendBlockResponse(block); err != nil {
			logger.Warningf("[channel: %s] Error sending to %s: %s", chdr.ChannelId, addr, err)
			return err
		}

		if stopNum == block.Header.Number {
			break
		}
	}

	if err := srv.SendStatusResponse(cb.Status_SUCCESS); err != nil {
		logger.Warningf("[channel: %s] Error sending to %s: %s", chdr.ChannelId, addr, err)
		return err
	}

	logger.Debugf("[channel: %s] Done delivering to %s for (%p)", chdr.ChannelId, addr, seekInfo)

	return nil
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
