/*
Copyright IBM Corp. 2016 All Rights Reserved.

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

package deliver

import (
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
	"github.com/op/go-logging"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
)

const pkgLogID = "common/deliver"

var logger *logging.Logger

func init() {
	logger = flogging.MustGetLogger(pkgLogID)
}

// Handler defines an interface which handles Deliver requests
type Handler interface {
	Handle(srv *DeliverServer) error
}

// SupportManager provides a way for the Handler to look up the Support for a chain
type SupportManager interface {
	GetChain(chainID string) (Support, bool)
}

// Support provides the backing resources needed to support deliver on a chain
type Support interface {
	// Sequence returns the current config sequence number, can be used to detect config changes
	Sequence() uint64

	// PolicyManager returns the current policy manager as specified by the chain configuration
	PolicyManager() policies.Manager

	// Reader returns the chain Reader for the chain
	Reader() blockledger.Reader

	// Errored returns a channel which closes when the backing consenter has errored
	Errored() <-chan struct{}
}

// PolicyChecker checks the envelope against the policy logic supplied by the
// function
type PolicyChecker func(envelope *cb.Envelope, channelID string) error

type deliverHandler struct {
	sm               SupportManager
	timeWindow       time.Duration
	bindingInspector comm.BindingInspector
}

//DeliverSupport defines the interface a handler
// must implement for delivery services
type DeliverSupport interface {
	Recv() (*cb.Envelope, error)
	Context() context.Context
	CreateStatusReply(status cb.Status) proto.Message
	CreateBlockReply(block *cb.Block) proto.Message
}

// DeliverServer a polymorphic structure to support
// generalization of this handler to be able to deliver
// different type of responses
type DeliverServer struct {
	DeliverSupport
	PolicyChecker
	Send func(msg proto.Message) error
}

// NewDeliverServer constructing deliver
func NewDeliverServer(support DeliverSupport, policyChecker PolicyChecker, send func(msg proto.Message) error) *DeliverServer {
	return &DeliverServer{
		DeliverSupport: support,
		PolicyChecker:  policyChecker,
		Send:           send,
	}
}

// NewHandlerImpl creates an implementation of the Handler interface
func NewHandlerImpl(sm SupportManager, timeWindow time.Duration, mutualTLS bool) Handler {
	// function to extract the TLS cert hash from a channel header
	extract := func(msg proto.Message) []byte {
		chdr, isChannelHeader := msg.(*cb.ChannelHeader)
		if !isChannelHeader || chdr == nil {
			return nil
		}
		return chdr.TlsCertHash
	}
	bindingInspector := comm.NewBindingInspector(mutualTLS, extract)

	return &deliverHandler{
		sm:               sm,
		timeWindow:       timeWindow,
		bindingInspector: bindingInspector,
	}
}

// Handle used to handle incoming deliver requests
func (ds *deliverHandler) Handle(srv *DeliverServer) error {
	addr := util.ExtractRemoteAddress(srv.Context())
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

		if err := ds.deliverBlocks(srv, envelope); err != nil {
			return err
		}

		logger.Debugf("Waiting for new SeekInfo from %s", addr)
	}
}

func (ds *deliverHandler) deliverBlocks(srv *DeliverServer, envelope *cb.Envelope) error {
	addr := util.ExtractRemoteAddress(srv.Context())
	payload, err := utils.UnmarshalPayload(envelope.Payload)
	if err != nil {
		logger.Warningf("Received an envelope from %s with no payload: %s", addr, err)
		return sendStatusReply(srv, cb.Status_BAD_REQUEST)
	}

	if payload.Header == nil {
		logger.Warningf("Malformed envelope received from %s with bad header", addr)
		return sendStatusReply(srv, cb.Status_BAD_REQUEST)
	}

	chdr, err := utils.UnmarshalChannelHeader(payload.Header.ChannelHeader)
	if err != nil {
		logger.Warningf("Failed to unmarshal channel header from %s: %s", addr, err)
		return sendStatusReply(srv, cb.Status_BAD_REQUEST)
	}

	err = ds.validateChannelHeader(srv, chdr)
	if err != nil {
		logger.Warningf("Rejecting deliver for %s due to envelope validation error: %s", addr, err)
		return sendStatusReply(srv, cb.Status_BAD_REQUEST)
	}

	chain, ok := ds.sm.GetChain(chdr.ChannelId)
	if !ok {
		// Note, we log this at DEBUG because SDKs will poll waiting for channels to be created
		// So we would expect our log to be somewhat flooded with these
		logger.Debugf("Rejecting deliver for %s because channel %s not found", addr, chdr.ChannelId)
		return sendStatusReply(srv, cb.Status_NOT_FOUND)
	}

	erroredChan := chain.Errored()
	select {
	case <-erroredChan:
		logger.Warningf("[channel: %s] Rejecting deliver request for %s because of consenter error", chdr.ChannelId, addr)
		return sendStatusReply(srv, cb.Status_SERVICE_UNAVAILABLE)
	default:

	}

	accessControl, err := newSessionAC(chain, envelope, srv.PolicyChecker, chdr.ChannelId, crypto.ExpiresAt)
	if err != nil {
		logger.Warningf("[channel: %s] failed to create access control object due to %s", chdr.ChannelId, err)
		return sendStatusReply(srv, cb.Status_BAD_REQUEST)
	}

	if err := accessControl.evaluate(); err != nil {
		logger.Warningf("[channel: %s] Client authorization revoked for deliver request from %s: %s", chdr.ChannelId, addr, err)
		return sendStatusReply(srv, cb.Status_FORBIDDEN)
	}

	seekInfo := &ab.SeekInfo{}
	if err = proto.Unmarshal(payload.Data, seekInfo); err != nil {
		logger.Warningf("[channel: %s] Received a signed deliver request from %s with malformed seekInfo payload: %s", chdr.ChannelId, addr, err)
		return sendStatusReply(srv, cb.Status_BAD_REQUEST)
	}

	if seekInfo.Start == nil || seekInfo.Stop == nil {
		logger.Warningf("[channel: %s] Received seekInfo message from %s with missing start or stop %v, %v", chdr.ChannelId, addr, seekInfo.Start, seekInfo.Stop)
		return sendStatusReply(srv, cb.Status_BAD_REQUEST)
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
			return sendStatusReply(srv, cb.Status_BAD_REQUEST)
		}
	}

	for {
		if seekInfo.Behavior == ab.SeekInfo_FAIL_IF_NOT_READY {
			if number > chain.Reader().Height()-1 {
				return sendStatusReply(srv, cb.Status_NOT_FOUND)
			}
		}

		block, status := nextBlock(cursor, erroredChan)
		if status != cb.Status_SUCCESS {
			cursor.Close()
			logger.Errorf("[channel: %s] Error reading from channel, cause was: %v", chdr.ChannelId, status)
			return sendStatusReply(srv, status)
		}

		// increment block number to support FAIL_IF_NOT_READY deliver behavior
		number++

		if err := accessControl.evaluate(); err != nil {
			logger.Warningf("[channel: %s] Client authorization revoked for deliver request from %s: %s", chdr.ChannelId, addr, err)
			return sendStatusReply(srv, cb.Status_FORBIDDEN)
		}

		logger.Debugf("[channel: %s] Delivering block for (%p) for %s", chdr.ChannelId, seekInfo, addr)

		if err := sendBlockReply(srv, block); err != nil {
			logger.Warningf("[channel: %s] Error sending to %s: %s", chdr.ChannelId, addr, err)
			return err
		}

		if stopNum == block.Header.Number {
			break
		}
	}

	if err := sendStatusReply(srv, cb.Status_SUCCESS); err != nil {
		logger.Warningf("[channel: %s] Error sending to %s: %s", chdr.ChannelId, addr, err)
		return err
	}

	logger.Debugf("[channel: %s] Done delivering to %s for (%p)", chdr.ChannelId, addr, seekInfo)

	return nil

}

func (ds *deliverHandler) validateChannelHeader(srv *DeliverServer, chdr *cb.ChannelHeader) error {
	if chdr.GetTimestamp() == nil {
		err := errors.New("channel header in envelope must contain timestamp")
		return err
	}

	envTime := time.Unix(chdr.GetTimestamp().Seconds, int64(chdr.GetTimestamp().Nanos)).UTC()
	serverTime := time.Now()

	if math.Abs(float64(serverTime.UnixNano()-envTime.UnixNano())) > float64(ds.timeWindow.Nanoseconds()) {
		err := errors.Errorf("timestamp %s is more than the %s time window difference above/below server time %s. either the server and client clocks are out of sync or a relay attack has been attempted", envTime, ds.timeWindow, serverTime)
		return err
	}

	err := ds.bindingInspector(srv.Context(), chdr)
	if err != nil {
		return err
	}

	return nil
}

func nextBlock(cursor blockledger.Iterator, cancel <-chan struct{}) (block *cb.Block, status cb.Status) {
	done := make(chan struct{})
	go func() {
		defer close(done)
		block, status = cursor.Next()
	}()

	select {
	case <-done:
		return
	case <-cancel:
		logger.Warningf("Aborting deliver for request because of background error")
		return nil, cb.Status_SERVICE_UNAVAILABLE
	}
}

func sendStatusReply(srv *DeliverServer, status cb.Status) error {
	return srv.Send(srv.CreateStatusReply(status))

}

func sendBlockReply(srv *DeliverServer, block *cb.Block) error {
	return srv.Send(srv.CreateBlockReply(block))
}
