/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package producer

import (
	"fmt"
	"sync"
	"time"

	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/pkg/errors"
)

type handlerList struct {
	sync.Mutex
	handlers Set
}

// Set is a set of handlers
type Set map[*handler]struct{}

func (hl *handlerList) add(h *handler) (bool, error) {
	if h == nil {
		return false, fmt.Errorf("cannot add nil handler")
	}
	hl.Lock()
	defer hl.Unlock()
	handlers := hl.copyHandlers()
	if _, ok := handlers[h]; ok {
		logger.Warningf("handler already exists for event type")
		return true, nil
	}
	handlers[h] = struct{}{}
	hl.handlers = handlers
	return true, nil
}

func (hl *handlerList) remove(h *handler) (bool, error) {
	hl.Lock()
	defer hl.Unlock()
	handlers := hl.copyHandlers()
	if _, ok := handlers[h]; !ok {
		logger.Warningf("handler does not exist for event type")
		return true, nil
	}
	delete(handlers, h)
	hl.handlers = handlers
	return true, nil
}

func (hl *handlerList) copyHandlers() Set {
	handlerCopy := Set{}
	for k, v := range hl.handlers {
		handlerCopy[k] = v
	}
	return handlerCopy
}

func (hl *handlerList) getHandlers() Set {
	hl.Lock()
	defer hl.Unlock()
	return hl.handlers
}

// eventProcessor has a map of event type to handlers interested in that
// event type. start() kicks off the event processor where it waits for Events
// from producers. We could easily generalize the one event handling loop to one
// per handlerMap if necessary.
//
type eventProcessor struct {
	sync.RWMutex
	eventConsumers map[pb.EventType]*handlerList

	// we could generalize this with mutiple channels each with its own size
	eventChannel chan *pb.Event

	*EventsServerConfig
}

// global eventProcessor singleton created by initializeEvents. Openchain producers
// send events simply over a reentrant static method
var gEventProcessor *eventProcessor

func (ep *eventProcessor) start() {
	defer ep.cleanup()
	logger.Info("Event processor started")
	for e := range ep.eventChannel {
		hl, err := ep.getHandlerList(e)
		if err != nil {
			logger.Error(err.Error())
			continue
		}
		failedHandlers := []*handler{}
		for h := range hl.getHandlers() {
			if h.hasSessionExpired() {
				failedHandlers = append(failedHandlers, h)
				continue
			}
			if e.Event != nil {
				err := h.SendMessageWithTimeout(e, ep.SendTimeout)
				if err != nil {
					failedHandlers = append(failedHandlers, h)
				}
			}
		}

		for _, h := range failedHandlers {
			ep.cleanupHandler(h)
		}
	}
}

func (ep *eventProcessor) getHandlerList(e *pb.Event) (*handlerList, error) {
	eType := getMessageType(e)
	ep.Lock()
	defer ep.Unlock()
	hl := ep.eventConsumers[eType]
	if hl == nil {
		return nil, errors.Errorf("event type %T not supported", e.Event)
	}
	return hl, nil
}

func (ep *eventProcessor) cleanupHandler(h *handler) {
	// deregister handler from all handler lists
	ep.deregisterAll(h)
	logger.Debug("handler cleanup complete for", h.RemoteAddr)
}

func getMessageType(e *pb.Event) pb.EventType {
	switch e.Event.(type) {
	case *pb.Event_Block:
		return pb.EventType_BLOCK
	case *pb.Event_FilteredBlock:
		return pb.EventType_FILTEREDBLOCK
	default:
		return -1
	}
}

// initialize and start
func initializeEvents(config *EventsServerConfig) *eventProcessor {
	if gEventProcessor != nil {
		panic("should not be called twice")
	}

	gEventProcessor = &eventProcessor{
		eventConsumers:     map[pb.EventType]*handlerList{},
		eventChannel:       make(chan *pb.Event, config.BufferSize),
		EventsServerConfig: config,
	}

	gEventProcessor.addSupportedEventTypes()

	// start the event processor
	go gEventProcessor.start()

	return gEventProcessor
}

func (ep *eventProcessor) cleanup() {
	close(ep.eventChannel)
}

func (ep *eventProcessor) addSupportedEventTypes() {
	gEventProcessor.addEventType(pb.EventType_BLOCK)
	gEventProcessor.addEventType(pb.EventType_FILTEREDBLOCK)
}

// addEventType supported event
func (ep *eventProcessor) addEventType(eventType pb.EventType) error {
	ep.Lock()
	defer ep.Unlock()
	logger.Debugf("Registering %s", pb.EventType_name[int32(eventType)])
	if _, ok := ep.eventConsumers[eventType]; ok {
		return fmt.Errorf("event type %s already exists", pb.EventType_name[int32(eventType)])
	}

	switch eventType {
	case pb.EventType_BLOCK, pb.EventType_FILTEREDBLOCK:
		ep.eventConsumers[eventType] = &handlerList{handlers: Set{}}
	default:
		return fmt.Errorf("event type %T not supported", eventType)
	}

	return nil
}

func (ep *eventProcessor) registerHandler(ie *pb.Interest, h *handler) error {
	logger.Debugf("Registering event type: %s", ie.EventType)
	ep.Lock()
	defer ep.Unlock()
	if hl, ok := ep.eventConsumers[ie.EventType]; !ok {
		return fmt.Errorf("event type %s does not exist", ie.EventType)
	} else if _, err := hl.add(h); err != nil {
		return fmt.Errorf("error registering handler for  %s: %s", ie.EventType, err)
	}

	return nil
}

func (ep *eventProcessor) deregisterHandler(ie *pb.Interest, h *handler) error {
	logger.Debugf("Deregistering event type %s", ie.EventType)

	ep.Lock()
	defer ep.Unlock()
	if hl, ok := ep.eventConsumers[ie.EventType]; !ok {
		return fmt.Errorf("event type %s does not exist", ie.EventType)
	} else if _, err := hl.remove(h); err != nil {
		return fmt.Errorf("error deregistering handler for %s: %s", ie.EventType, err)
	}

	return nil
}

func (ep *eventProcessor) deregisterAll(h *handler) {
	for k, v := range h.interestedEvents {
		if err := ep.deregisterHandler(v, h); err != nil {
			logger.Errorf("failed deregistering event type %s for %s", v, h.RemoteAddr)
			continue
		}
		delete(h.interestedEvents, k)
	}
}

// ------------- producer API's -------------------------------

// Send sends the event to the global event processor's buffered channel
func Send(e *pb.Event) error {
	if e.Event == nil {
		logger.Error("Event not set")
		return fmt.Errorf("event not set")
	}

	if gEventProcessor == nil {
		logger.Debugf("Event processor is nil")
		return nil
	}

	switch {
	case gEventProcessor.Timeout < 0:
		select {
		case gEventProcessor.eventChannel <- e:
		default:
			return fmt.Errorf("could not add block event to event processor queue")
		}
	case gEventProcessor.Timeout == 0:
		gEventProcessor.eventChannel <- e
	default:
		select {
		case gEventProcessor.eventChannel <- e:
		case <-time.After(gEventProcessor.Timeout):
			return fmt.Errorf("could not add block event to event processor queue")
		}
	}

	logger.Debugf("Event added to event processor queue")
	return nil
}
