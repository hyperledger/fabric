/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode

import (
	"sync"

	"github.com/hyperledger/fabric/core/container/ccintf"
	"github.com/pkg/errors"
)

// HandlerRegistry maintains chaincode Handler instances.
type HandlerRegistry struct {
	allowUnsolicitedRegistration bool // from cs.userRunsCC

	mutex     sync.Mutex                   // lock covering handlers and launching
	handlers  map[ccintf.CCID]*Handler     // chaincode cname to associated handler
	launching map[ccintf.CCID]*LaunchState // launching chaincodes to LaunchState
}

type LaunchState struct {
	mutex    sync.Mutex
	notified bool
	done     chan struct{}
	err      error
}

func NewLaunchState() *LaunchState {
	return &LaunchState{
		done: make(chan struct{}),
	}
}

func (l *LaunchState) Done() <-chan struct{} {
	return l.done
}

func (l *LaunchState) Err() error {
	l.mutex.Lock()
	err := l.err
	l.mutex.Unlock()
	return err
}

func (l *LaunchState) Notify(err error) {
	l.mutex.Lock()
	if !l.notified {
		l.notified = true
		l.err = err
		close(l.done)
	}
	l.mutex.Unlock()
}

// NewHandlerRegistry constructs a HandlerRegistry.
func NewHandlerRegistry(allowUnsolicitedRegistration bool) *HandlerRegistry {
	return &HandlerRegistry{
		handlers:                     map[ccintf.CCID]*Handler{},
		launching:                    map[ccintf.CCID]*LaunchState{},
		allowUnsolicitedRegistration: allowUnsolicitedRegistration,
	}
}

// Launching indicates that chaincode is being launched. The LaunchState that
// is returned provides mechanisms to determine when the operation has
// completed and whether or not it failed. The bool indicates whether or not
// the chaincode has already been started.
func (r *HandlerRegistry) Launching(packageID ccintf.CCID) (*LaunchState, bool) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	// launch happened or already happening
	if launchState, ok := r.launching[packageID]; ok {
		return launchState, true
	}

	// handler registered without going through launch
	if _, ok := r.handlers[packageID]; ok {
		launchState := NewLaunchState()
		launchState.Notify(nil)
		return launchState, true
	}

	// first attempt to launch so the runtime needs to start
	launchState := NewLaunchState()
	r.launching[packageID] = launchState
	return launchState, false
}

// Ready indicates that the chaincode registration has completed and the
// READY response has been sent to the chaincode.
func (r *HandlerRegistry) Ready(packageID ccintf.CCID) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	launchStatus := r.launching[packageID]
	if launchStatus != nil {
		launchStatus.Notify(nil)
	}
}

// Failed indicates that registration of a launched chaincode has failed.
func (r *HandlerRegistry) Failed(packageID ccintf.CCID, err error) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	launchStatus := r.launching[packageID]
	if launchStatus != nil {
		launchStatus.Notify(err)
	}
}

// Handler retrieves the handler for a chaincode instance.
func (r *HandlerRegistry) Handler(packageID ccintf.CCID) *Handler {
	r.mutex.Lock()
	h := r.handlers[packageID]
	r.mutex.Unlock()
	return h
}

// Register adds a chaincode handler to the registry.
// An error will be returned if a handler is already registered for the
// chaincode. An error will also be returned if the chaincode has not already
// been "launched", and unsolicited registration is not allowed.
func (r *HandlerRegistry) Register(h *Handler) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	// FIXME: the packageID must be its own field in the handler
	// This hack works because currently h.chaincodeID.Name is
	// actually set as the concatenation of chaincode name and
	// chaincode ID, which is set at build time. While it's ok
	// for the chaincode to communicate back its packageID, the
	// usage of the chaincodeID field is misleading (FAB-14630)
	packageID := ccintf.CCID(h.chaincodeID.Name)

	if r.handlers[packageID] != nil {
		chaincodeLogger.Debugf("duplicate registered handler(key:%s) return error", packageID)
		return errors.Errorf("duplicate chaincodeID: %s", h.chaincodeID.Name)
	}

	// This chaincode was not launched by the peer but is attempting
	// to register. Only allowed in development mode.
	if r.launching[packageID] == nil && !r.allowUnsolicitedRegistration {
		return errors.Errorf("peer will not accept external chaincode connection %v (except in dev mode)", h.chaincodeID.Name)
	}

	r.handlers[packageID] = h

	chaincodeLogger.Debugf("registered handler complete for chaincode %s", packageID)
	return nil
}

// Deregister clears references to state associated specified chaincode.
// As part of the cleanup, it closes the handler so it can cleanup any state.
// If the registry does not contain the provided handler, an error is returned.
func (r *HandlerRegistry) Deregister(packageID ccintf.CCID) error {
	chaincodeLogger.Debugf("deregister handler: %s", packageID)

	r.mutex.Lock()
	handler := r.handlers[packageID]
	delete(r.handlers, packageID)
	delete(r.launching, packageID)
	r.mutex.Unlock()

	if handler == nil {
		return errors.Errorf("could not find handler: %s", packageID)
	}

	handler.Close()

	chaincodeLogger.Debugf("deregistered handler with key: %s", packageID)
	return nil
}
