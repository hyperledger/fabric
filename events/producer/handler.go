/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package producer

import (
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/golang/protobuf/proto"

	"github.com/hyperledger/fabric/common/crypto"
	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/msp/mgmt"
	pb "github.com/hyperledger/fabric/protos/peer"
)

type handler struct {
	ChatStream       pb.Events_ChatServer
	interestedEvents map[string]*pb.Interest
	sessionEndTime   time.Time
	RemoteAddr       string
}

func newEventHandler(stream pb.Events_ChatServer) *handler {
	h := &handler{
		ChatStream:       stream,
		interestedEvents: make(map[string]*pb.Interest),
		RemoteAddr:       util.ExtractRemoteAddress(stream.Context()),
	}
	logger.Debug("event handler created for", h.RemoteAddr)
	return h
}

// Stop stops this handler
func (h *handler) Stop() error {
	h.deregisterAll()
	h.interestedEvents = nil
	logger.Debug("handler stopped for", h.RemoteAddr)
	return nil
}

func getInterestKey(interest pb.Interest) string {
	var key string
	switch interest.EventType {
	case pb.EventType_BLOCK:
		key = "/" + strconv.Itoa(int(pb.EventType_BLOCK))
	case pb.EventType_FILTEREDBLOCK:
		key = "/" + strconv.Itoa(int(pb.EventType_FILTEREDBLOCK))
	case pb.EventType_REJECTION:
		key = "/" + strconv.Itoa(int(pb.EventType_REJECTION))
	case pb.EventType_CHAINCODE:
		key = "/" + strconv.Itoa(int(pb.EventType_CHAINCODE)) + "/" + interest.GetChaincodeRegInfo().ChaincodeId + "/" + interest.GetChaincodeRegInfo().EventName
	default:
		logger.Errorf("unsupported interest type: %s", interest.EventType)
	}

	return key
}

func (h *handler) register(iMsg []*pb.Interest) error {
	// Could consider passing interest array to registerHandler
	// and only lock once for entire array here
	for _, v := range iMsg {
		if err := registerHandler(v, h); err != nil {
			logger.Errorf("could not register %s for %s: %s", v, h.RemoteAddr, err)
			continue
		}
		h.interestedEvents[getInterestKey(*v)] = v
	}

	return nil
}

func (h *handler) deregister(iMsg []*pb.Interest) error {
	for _, v := range iMsg {
		if err := deRegisterHandler(v, h); err != nil {
			logger.Errorf("could not deregister %s for %s: %s", v, h.RemoteAddr, err)
			continue
		}
		delete(h.interestedEvents, getInterestKey(*v))
	}
	return nil
}

func (h *handler) deregisterAll() {
	for k, v := range h.interestedEvents {
		if err := deRegisterHandler(v, h); err != nil {
			logger.Errorf("could not deregister %s", v)
			continue
		}
		delete(h.interestedEvents, k)
	}
}

// HandleMessage handles the Openchain messages for the Peer.
func (h *handler) HandleMessage(msg *pb.SignedEvent) error {
	evt, err := h.validateEventMessage(msg)
	if err != nil {
		return fmt.Errorf("event message validation failed for %s: %s", h.RemoteAddr, err)
	}

	switch evt.Event.(type) {
	case *pb.Event_Register:
		eventsObj := evt.GetRegister()
		if err := h.register(eventsObj.Events); err != nil {
			return fmt.Errorf("could not register events for %s: %s", h.RemoteAddr, err)
		}
	case *pb.Event_Unregister:
		eventsObj := evt.GetUnregister()
		if err := h.deregister(eventsObj.Events); err != nil {
			return fmt.Errorf("could not deregister events for %s: %s", h.RemoteAddr, err)
		}
	case nil:
	default:
		return fmt.Errorf("invalid event type received from %s: %T", h.RemoteAddr, evt.Event)
	}
	//TODO return supported events.. for now just return the received msg
	if err := h.ChatStream.Send(evt); err != nil {
		return fmt.Errorf("error sending response to %s: %s", h.RemoteAddr, err)
	}

	return nil
}

// SendMessage sends a message to the remote PEER through the stream
func (h *handler) SendMessage(msg *pb.Event) error {
	logger.Debug("sending event to", h.RemoteAddr)
	err := h.ChatStream.Send(msg)
	if err != nil {
		logger.Debugf("sending event failed for %s: %s", h.RemoteAddr, err)
		return fmt.Errorf("error sending message through ChatStream: %s", err)
	}
	logger.Debug("event sent successfully to", h.RemoteAddr)
	return nil
}

// Validates event messages by validating the Creator and verifying
// the signature. Returns the unmarshaled Event object
// Validation of the creator identity's validity is done by checking with local MSP to ensure the
// submitter is a member in the same organization as the peer
//
// TODO: ideally this should also check each channel's "Readers" policy to ensure the identity satisfies
// each channel's access control policy. This step is necessary because the registered listener is going
// to get read access to all channels by receiving Block events from all channels.
// However, this is not being done for v1.0 due to complexity concerns and the need to complex a stable,
// minimally viable release. Eventually events will be made channel-specific, at which point this method
// should be revisited
func (h *handler) validateEventMessage(signedEvt *pb.SignedEvent) (*pb.Event, error) {
	logger.Debugf("validating for signed event %p", signedEvt)

	// messages from the client for registering and unregistering must be signed
	// and accompanied by the signing certificate in the "Creator" field
	evt := &pb.Event{}
	err := proto.Unmarshal(signedEvt.EventBytes, evt)
	if err != nil {
		return nil, fmt.Errorf("error unmarshaling the event bytes in the SignedEvent: %s", err)
	}

	expirationTime := crypto.ExpiresAt(evt.Creator)
	if !expirationTime.IsZero() && time.Now().After(expirationTime) {
		return nil, fmt.Errorf("identity expired")
	}
	h.sessionEndTime = expirationTime

	if evt.GetTimestamp() != nil {
		evtTime := time.Unix(evt.GetTimestamp().Seconds, int64(evt.GetTimestamp().Nanos)).UTC()
		peerTime := time.Now()

		if math.Abs(float64(peerTime.UnixNano()-evtTime.UnixNano())) > float64(gEventProcessor.TimeWindow.Nanoseconds()) {
			logger.Warningf("Message timestamp %s more than %s apart from current server time %s", evtTime, gEventProcessor.TimeWindow, peerTime)
			return nil, fmt.Errorf("message timestamp out of acceptable range. must be within %s of current server time", gEventProcessor.TimeWindow)
		}
	}

	err = gEventProcessor.BindingInspector(h.ChatStream.Context(), evt)
	if err != nil {
		return nil, err
	}

	localMSP := mgmt.GetLocalMSP()
	principalGetter := mgmt.NewLocalMSPPrincipalGetter()

	// Load MSPPrincipal for policy
	principal, err := principalGetter.Get(mgmt.Members)
	if err != nil {
		return nil, fmt.Errorf("failed getting local MSP principal [member]: %s", err)
	}

	id, err := localMSP.DeserializeIdentity(evt.Creator)
	if err != nil {
		return nil, fmt.Errorf("failed deserializing event creator: %s", err)
	}

	// Verify that event's creator satisfies the principal
	err = id.SatisfiesPrincipal(principal)
	if err != nil {
		return nil, fmt.Errorf("failed verifying the creator satisfies local MSP's [member] principal: %s", err)
	}

	// Verify the signature
	err = id.Verify(signedEvt.EventBytes, signedEvt.Signature)
	if err != nil {
		return nil, fmt.Errorf("failed verifying the event signature: %s", err)
	}

	return evt, nil
}
