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

package events

import (
	"time"

	"github.com/op/go-logging"
)

var logger *logging.Logger // package-level logger

func init() {
	logger = logging.MustGetLogger("consensus/util/events")
}

// Event is a type meant to clearly convey that the return type or parameter to a function will be supplied to/from an events.Manager
type Event interface{}

// Receiver is a consumer of events, ProcessEvent will be called serially
// as events arrive
type Receiver interface {
	// ProcessEvent delivers an event to the Receiver, if it returns non-nil, the return is the next processed event
	ProcessEvent(e Event) Event
}

// ------------------------------------------------------------
//
// Threaded object
//
// ------------------------------------------------------------

// threaded holds an exit channel to allow threads to break from a select
type threaded struct {
	exit chan struct{}
}

// halt tells the threaded object's thread to exit
func (t *threaded) Halt() {
	select {
	case <-t.exit:
		logger.Warning("Attempted to halt a threaded object twice")
	default:
		close(t.exit)
	}
}

// ------------------------------------------------------------
//
// Event Manager
//
// ------------------------------------------------------------

// Manager provides a serialized interface for submitting events to
// a Receiver on the other side of the queue
type Manager interface {
	Inject(Event)         // A temporary interface to allow the event manager thread to skip the queue
	Queue() chan<- Event  // Get a write-only reference to the queue, to submit events
	SetReceiver(Receiver) // Set the target to route events to
	Start()               // Starts the Manager thread TODO, these thread management things should probably go away
	Halt()                // Stops the Manager thread
}

// managerImpl is an implementation of Manger
type managerImpl struct {
	threaded
	receiver Receiver
	events   chan Event
}

// NewManagerImpl creates an instance of managerImpl
func NewManagerImpl() Manager {
	return &managerImpl{
		events:   make(chan Event),
		threaded: threaded{make(chan struct{})},
	}
}

// SetReceiver sets the destination for events
func (em *managerImpl) SetReceiver(receiver Receiver) {
	em.receiver = receiver
}

// Start creates the go routine necessary to deliver events
func (em *managerImpl) Start() {
	go em.eventLoop()
}

// queue returns a write only reference to the event queue
func (em *managerImpl) Queue() chan<- Event {
	return em.events
}

// SendEvent performs the event loop on a receiver to completion
func SendEvent(receiver Receiver, event Event) {
	next := event
	for {
		// If an event returns something non-nil, then process it as a new event
		next = receiver.ProcessEvent(next)
		if next == nil {
			break
		}
	}
}

// Inject can only safely be called by the managerImpl thread itself, it skips the queue
func (em *managerImpl) Inject(event Event) {
	if em.receiver != nil {
		SendEvent(em.receiver, event)
	}
}

// eventLoop is where the event thread loops, delivering events
func (em *managerImpl) eventLoop() {
	for {
		select {
		case next := <-em.events:
			em.Inject(next)
		case <-em.exit:
			logger.Debug("eventLoop told to exit")
			return
		}
	}
}

// ------------------------------------------------------------
//
// Event Timer
//
// ------------------------------------------------------------

// Timer is an interface for managing time driven events
// the special contract Timer gives which a traditional golang
// timer does not, is that if the event thread calls stop, or reset
// then even if the timer has already fired, the event will not be
// delivered to the event queue
type Timer interface {
	SoftReset(duration time.Duration, event Event) // start a new countdown, only if one is not already started
	Reset(duration time.Duration, event Event)     // start a new countdown, clear any pending events
	Stop()                                         // stop the countdown, clear any pending events
	Halt()                                         // Stops the Timer thread
}

// TimerFactory abstracts the creation of Timers, as they may
// need to be mocked for testing
type TimerFactory interface {
	CreateTimer() Timer // Creates an Timer which is stopped
}

// TimerFactoryImpl implements the TimerFactory
type timerFactoryImpl struct {
	manager Manager // The Manager to use in constructing the event timers
}

// NewTimerFactoryImpl creates a new TimerFactory for the given Manager
func NewTimerFactoryImpl(manager Manager) TimerFactory {
	return &timerFactoryImpl{manager}
}

// CreateTimer creates a new timer which deliver events to the Manager for this factory
func (etf *timerFactoryImpl) CreateTimer() Timer {
	return newTimerImpl(etf.manager)
}

// timerStart is used to deliver the start request to the eventTimer thread
type timerStart struct {
	hard     bool          // Whether to reset the timer if it is running
	event    Event         // What event to push onto the event queue
	duration time.Duration // How long to wait before sending the event
}

// timerImpl is an implementation of Timer
type timerImpl struct {
	threaded                   // Gives us the exit chan
	timerChan <-chan time.Time // When non-nil, counts down to preparing to do the event
	startChan chan *timerStart // Channel to deliver the timer start events to the service go routine
	stopChan  chan struct{}    // Channel to deliver the timer stop events to the service go routine
	manager   Manager          // The event manager to deliver the event to after timer expiration
}

// newTimer creates a new instance of timerImpl
func newTimerImpl(manager Manager) Timer {
	et := &timerImpl{
		startChan: make(chan *timerStart),
		stopChan:  make(chan struct{}),
		threaded:  threaded{make(chan struct{})},
		manager:   manager,
	}
	go et.loop()
	return et
}

// softReset tells the timer to start a new countdown, only if it is not currently counting down
// this will not clear any pending events
func (et *timerImpl) SoftReset(timeout time.Duration, event Event) {
	et.startChan <- &timerStart{
		duration: timeout,
		event:    event,
		hard:     false,
	}
}

// reset tells the timer to start counting down from a new timeout, this also clears any pending events
func (et *timerImpl) Reset(timeout time.Duration, event Event) {
	et.startChan <- &timerStart{
		duration: timeout,
		event:    event,
		hard:     true,
	}
}

// stop tells the timer to stop, and not to deliver any pending events
func (et *timerImpl) Stop() {
	et.stopChan <- struct{}{}
}

// loop is where the timer thread lives, looping
func (et *timerImpl) loop() {
	var eventDestChan chan<- Event
	var event Event

	for {
		// A little state machine, relying on the fact that nil channels will block on read/write indefinitely

		select {
		case start := <-et.startChan:
			if et.timerChan != nil {
				if start.hard {
					logger.Debug("Resetting a running timer")
				} else {
					continue
				}
			}
			logger.Debug("Starting timer")
			et.timerChan = time.After(start.duration)
			if eventDestChan != nil {
				logger.Debug("Timer cleared pending event")
			}
			event = start.event
			eventDestChan = nil
		case <-et.stopChan:
			if et.timerChan == nil && eventDestChan == nil {
				logger.Debug("Attempting to stop an unfired idle timer")
			}
			et.timerChan = nil
			logger.Debug("Stopping timer")
			if eventDestChan != nil {
				logger.Debug("Timer cleared pending event")
			}
			eventDestChan = nil
			event = nil
		case <-et.timerChan:
			logger.Debug("Event timer fired")
			et.timerChan = nil
			eventDestChan = et.manager.Queue()
		case eventDestChan <- event:
			logger.Debug("Timer event delivered")
			eventDestChan = nil
		case <-et.exit:
			logger.Debug("Halting timer")
			return
		}
	}
}
