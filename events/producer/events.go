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

package producer

import (
	"fmt"
	"sync"
	"time"

	pb "github.com/hyperledger/fabric/protos/peer"
)

//---- event hub framework ----

//handlerListi uses map to implement a set of handlers. use mutex to access
//the map. Note that we don't have lock/unlock wrapper methods as the lock
//of handler list has to be done under the eventProcessor lock. See
//registerHandler, deRegisterHandler. register/deRegister methods
//will be called only when a new consumer chat starts/ends respectively
//and the big lock should have no performance impact
//
type handlerList interface {
	add(ie *pb.Interest, h *handler) (bool, error)
	del(ie *pb.Interest, h *handler) (bool, error)
	foreach(ie *pb.Event, action func(h *handler))
}

type genericHandlerList struct {
	sync.RWMutex
	handlers map[*handler]bool
}

type chaincodeHandlerList struct {
	sync.RWMutex
	handlers map[string]map[string]map[*handler]bool
}

func (hl *chaincodeHandlerList) add(ie *pb.Interest, h *handler) (bool, error) {
	if h == nil {
		return false, fmt.Errorf("cannot add nil chaincode handler")
	}

	hl.Lock()
	defer hl.Unlock()

	//chaincode registration info must be non-nil
	if ie.GetChaincodeRegInfo() == nil {
		return false, fmt.Errorf("chaincode information not provided for registering")
	}
	//chaincode registration info must be for a non-empty chaincode ID (even if the chaincode does not exist)
	if ie.GetChaincodeRegInfo().ChaincodeId == "" {
		return false, fmt.Errorf("chaincode ID not provided for registering")
	}
	//is there a event type map for the chaincode
	emap, ok := hl.handlers[ie.GetChaincodeRegInfo().ChaincodeId]
	if !ok {
		emap = make(map[string]map[*handler]bool)
		hl.handlers[ie.GetChaincodeRegInfo().ChaincodeId] = emap
	}

	//create handler map if this is the first handler for the type
	var handlerMap map[*handler]bool
	if handlerMap, _ = emap[ie.GetChaincodeRegInfo().EventName]; handlerMap == nil {
		handlerMap = make(map[*handler]bool)
		emap[ie.GetChaincodeRegInfo().EventName] = handlerMap
	} else if _, ok = handlerMap[h]; ok {
		return false, fmt.Errorf("handler exists for event type")
	}

	//the handler is added to the map
	handlerMap[h] = true

	return true, nil
}
func (hl *chaincodeHandlerList) del(ie *pb.Interest, h *handler) (bool, error) {
	hl.Lock()
	defer hl.Unlock()

	//chaincode registration info must be non-nil
	if ie.GetChaincodeRegInfo() == nil {
		return false, fmt.Errorf("chaincode information not provided for de-registering")
	}

	//chaincode registration info must be for a non-empty chaincode ID (even if the chaincode does not exist)
	if ie.GetChaincodeRegInfo().ChaincodeId == "" {
		return false, fmt.Errorf("chaincode ID not provided for de-registering")
	}

	//if there's no event type map, nothing to do
	emap, ok := hl.handlers[ie.GetChaincodeRegInfo().ChaincodeId]
	if !ok {
		return false, fmt.Errorf("chaincode ID not registered")
	}

	//if there are no handlers for the event type, nothing to do
	var handlerMap map[*handler]bool
	if handlerMap, _ = emap[ie.GetChaincodeRegInfo().EventName]; handlerMap == nil {
		return false, fmt.Errorf("event name %s not registered for chaincode ID %s", ie.GetChaincodeRegInfo().EventName, ie.GetChaincodeRegInfo().ChaincodeId)
	} else if _, ok = handlerMap[h]; !ok {
		//the handler is not registered for the event type
		return false, fmt.Errorf("handler not registered for event name %s for chaincode ID %s", ie.GetChaincodeRegInfo().EventName, ie.GetChaincodeRegInfo().ChaincodeId)
	}
	//remove the handler from the map
	delete(handlerMap, h)

	//if the last handler has been removed from handler map for a chaincode's event,
	//remove the event map.
	//if the last map of events have been removed for the chaincode UUID
	//remove the chaincode UUID map
	if len(handlerMap) == 0 {
		delete(emap, ie.GetChaincodeRegInfo().EventName)
		if len(emap) == 0 {
			delete(hl.handlers, ie.GetChaincodeRegInfo().ChaincodeId)
		}
	}

	return true, nil
}

func (hl *chaincodeHandlerList) foreach(e *pb.Event, action func(h *handler)) {
	hl.Lock()
	defer hl.Unlock()

	//if there's no chaincode event in the event... nothing to do (why was this event sent ?)
	if e.GetChaincodeEvent() == nil || e.GetChaincodeEvent().ChaincodeId == "" {
		return
	}

	//get the event map for the chaincode
	if emap := hl.handlers[e.GetChaincodeEvent().ChaincodeId]; emap != nil {
		//get the handler map for the event
		if handlerMap := emap[e.GetChaincodeEvent().EventName]; handlerMap != nil {
			for h := range handlerMap {
				action(h)
			}
		}
		//send to handlers who want all events from the chaincode, but only if
		//EventName is not already "" (chaincode should NOT send nameless events though)
		if e.GetChaincodeEvent().EventName != "" {
			if handlerMap := emap[""]; handlerMap != nil {
				for h := range handlerMap {
					action(h)
				}
			}
		}
	}
}

func (hl *genericHandlerList) add(ie *pb.Interest, h *handler) (bool, error) {
	if h == nil {
		return false, fmt.Errorf("cannot add nil generic handler")
	}
	hl.Lock()
	if _, ok := hl.handlers[h]; ok {
		hl.Unlock()
		return false, fmt.Errorf("handler exists for event type")
	}
	hl.handlers[h] = true
	hl.Unlock()
	return true, nil
}

func (hl *genericHandlerList) del(ie *pb.Interest, h *handler) (bool, error) {
	hl.Lock()
	if _, ok := hl.handlers[h]; !ok {
		hl.Unlock()
		return false, fmt.Errorf("handler does not exist for event type")
	}
	delete(hl.handlers, h)
	hl.Unlock()
	return true, nil
}

func (hl *genericHandlerList) foreach(e *pb.Event, action func(h *handler)) {
	hl.Lock()
	for h := range hl.handlers {
		action(h)
	}
	hl.Unlock()
}

//eventProcessor has a map of event type to handlers interested in that
//event type. start() kicks of the event processor where it waits for Events
//from producers. We could easily generalize the one event handling loop to one
//per handlerMap if necessary.
//
type eventProcessor struct {
	sync.RWMutex
	eventConsumers map[pb.EventType]handlerList

	//we could generalize this with mutiple channels each with its own size
	eventChannel chan *pb.Event

	//timeout duration for producer to send an event.
	//if < 0, if buffer full, unblocks immediately and not send
	//if 0, if buffer full, will block and guarantee the event will be sent out
	//if > 0, if buffer full, blocks till timeout
	timeout time.Duration
}

//global eventProcessor singleton created by initializeEvents. Openchain producers
//send events simply over a reentrant static method
var gEventProcessor *eventProcessor

func (ep *eventProcessor) start() {
	logger.Info("Event processor started")
	for {
		//wait for event
		e := <-ep.eventChannel

		var hl handlerList
		eType := getMessageType(e)
		ep.Lock()
		if hl, _ = ep.eventConsumers[eType]; hl == nil {
			logger.Errorf("Event of type %s does not exist", eType)
			ep.Unlock()
			continue
		}
		//lock the handler map lock
		ep.Unlock()

		hl.foreach(e, func(h *handler) {
			if e.Event != nil {
				h.SendMessage(e)
			}
		})

	}
}

//initialize and start
func initializeEvents(bufferSize uint, tout time.Duration) {
	if gEventProcessor != nil {
		panic("should not be called twice")
	}

	gEventProcessor = &eventProcessor{eventConsumers: make(map[pb.EventType]handlerList), eventChannel: make(chan *pb.Event, bufferSize), timeout: tout}

	addInternalEventTypes()

	//start the event processor
	go gEventProcessor.start()
}

//AddEventType supported event
func AddEventType(eventType pb.EventType) error {
	gEventProcessor.Lock()
	logger.Debugf("Registering %s", pb.EventType_name[int32(eventType)])
	if _, ok := gEventProcessor.eventConsumers[eventType]; ok {
		gEventProcessor.Unlock()
		return fmt.Errorf("event type exists %s", pb.EventType_name[int32(eventType)])
	}

	switch eventType {
	case pb.EventType_BLOCK:
		gEventProcessor.eventConsumers[eventType] = &genericHandlerList{handlers: make(map[*handler]bool)}
	case pb.EventType_CHAINCODE:
		gEventProcessor.eventConsumers[eventType] = &chaincodeHandlerList{handlers: make(map[string]map[string]map[*handler]bool)}
	case pb.EventType_REJECTION:
		gEventProcessor.eventConsumers[eventType] = &genericHandlerList{handlers: make(map[*handler]bool)}
	}
	gEventProcessor.Unlock()

	return nil
}

func registerHandler(ie *pb.Interest, h *handler) error {
	logger.Debugf("registering event type: %s", ie.EventType)
	gEventProcessor.Lock()
	defer gEventProcessor.Unlock()
	if hl, ok := gEventProcessor.eventConsumers[ie.EventType]; !ok {
		return fmt.Errorf("event type %s does not exist", ie.EventType)
	} else if _, err := hl.add(ie, h); err != nil {
		return fmt.Errorf("error registering handler for  %s: %s", ie.EventType, err)
	}

	return nil
}

func deRegisterHandler(ie *pb.Interest, h *handler) error {
	logger.Debugf("deregistering event type: %s", ie.EventType)

	gEventProcessor.Lock()
	defer gEventProcessor.Unlock()
	if hl, ok := gEventProcessor.eventConsumers[ie.EventType]; !ok {
		return fmt.Errorf("event type %s does not exist", ie.EventType)
	} else if _, err := hl.del(ie, h); err != nil {
		return fmt.Errorf("error deregistering handler for %s: %s", ie.EventType, err)
	}

	return nil
}

//------------- producer API's -------------------------------

//Send sends the event to interested consumers
func Send(e *pb.Event) error {
	logger.Debugf("Entry")
	defer logger.Debugf("Exit")
	if e.Event == nil {
		logger.Error("event not set")
		return fmt.Errorf("event not set")
	}

	if gEventProcessor == nil {
		logger.Debugf("Event processor is nil")
		return nil
	}

	if gEventProcessor.timeout < 0 {
		logger.Debugf("Event processor timeout < 0")
		select {
		case gEventProcessor.eventChannel <- e:
		default:
			return fmt.Errorf("could not send the blocking event")
		}
	} else if gEventProcessor.timeout == 0 {
		logger.Debugf("Event processor timeout = 0")
		gEventProcessor.eventChannel <- e
	} else {
		logger.Debugf("Event processor timeout > 0")
		select {
		case gEventProcessor.eventChannel <- e:
		case <-time.After(gEventProcessor.timeout):
			return fmt.Errorf("could not send the blocking event")
		}
	}

	logger.Debugf("Event sent successfully")
	return nil
}
