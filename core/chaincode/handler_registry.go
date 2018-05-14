/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode

import (
	"sync"

	"github.com/pkg/errors"
)

// HandlerRegistry maintains chaincode Handler instances.
type HandlerRegistry struct {
	allowUnsolicitedRegistration bool // from cs.userRunsCC

	mutex     sync.Mutex              // lock covering handlers and launching
	handlers  map[string]*Handler     // chaincode cname to associated handler
	launching map[string]*LaunchState // launching chaincodes to LaunchState
}

type LaunchState struct {
	done chan struct{}
	err  error
}

func NewLaunchState() *LaunchState {
	return &LaunchState{
		done: make(chan struct{}),
	}
}

func (l *LaunchState) Done() <-chan struct{} { return l.done }
func (l *LaunchState) Err() error            { return l.err }
func (l *LaunchState) Notify(err error) {
	l.err = err
	close(l.done)
}

// NewHandlerRegistry constructs a HandlerRegistry.
func NewHandlerRegistry(allowUnsolicitedRegistration bool) *HandlerRegistry {
	return &HandlerRegistry{
		handlers:                     map[string]*Handler{},
		launching:                    map[string]*LaunchState{},
		allowUnsolicitedRegistration: allowUnsolicitedRegistration,
	}
}

// HasLaunched returns true if the named chaincode is launching or running.
func (r *HandlerRegistry) HasLaunched(chaincode string) bool {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	return r.hasLaunched(chaincode)
}

func (r *HandlerRegistry) hasLaunched(chaincode string) bool {
	if _, ok := r.handlers[chaincode]; ok {
		return true
	}
	if _, ok := r.launching[chaincode]; ok {
		return true
	}
	return false
}

// Launching indicates that chaincode is being launched. The LaunchState that
// is returned provides mechanisms to determine when the operation has completed
// and whether or not it failed. An error will be returned if chaincode launch
// processing has already been initated.
func (r *HandlerRegistry) Launching(cname string) (*LaunchState, error) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	if r.hasLaunched(cname) {
		return nil, errors.Errorf("chaincode %s has already been launched", cname)
	}

	launchState := NewLaunchState()
	r.launching[cname] = launchState
	return launchState, nil
}

// Ready indicates that the chaincode registration has completed and the
// READY response has been sent to the chaincode.
func (r *HandlerRegistry) Ready(cname string) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	launchStatus := r.launching[cname]
	if launchStatus != nil {
		delete(r.launching, cname)
		launchStatus.Notify(nil)
	}
}

// Failed indicates that registration of a launched chaincode has failed.
func (r *HandlerRegistry) Failed(cname string, err error) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	launchStatus := r.launching[cname]
	if launchStatus != nil {
		launchStatus.Notify(err)
	}
}

// Handler retrieves the handler for a chaincode instance.
func (r *HandlerRegistry) Handler(cname string) *Handler {
	r.mutex.Lock()
	h := r.handlers[cname]
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
	key := h.chaincodeID.Name

	if r.handlers[key] != nil {
		chaincodeLogger.Debugf("duplicate registered handler(key:%s) return error", key)
		return errors.Errorf("duplicate chaincodeID: %s", h.chaincodeID.Name)
	}

	// This chaincode was not launched by the peer but is attempting
	// to register. Only allowed in development mode.
	if r.launching[key] == nil && !r.allowUnsolicitedRegistration {
		return errors.Errorf("peer will not accept external chaincode connection %v (except in dev mode)", h.chaincodeID.Name)
	}

	r.handlers[key] = h

	chaincodeLogger.Debugf("registered handler complete for chaincode %s", key)
	return nil
}

// Deregister clears references to state associated specified chaincode.
// As part of the cleanup, it closes the handler so it can cleanup any state.
// If the registry does not contain the provided handler, an error is returned.
func (r *HandlerRegistry) Deregister(cname string) error {
	chaincodeLogger.Debugf("deregister handler: %s", cname)

	r.mutex.Lock()
	handler := r.handlers[cname]
	delete(r.handlers, cname)
	delete(r.launching, cname)
	r.mutex.Unlock()

	if handler == nil {
		return errors.Errorf("could not find handler: %s", cname)
	}

	handler.Close()

	chaincodeLogger.Debugf("deregistered handler with key: %s", cname)
	return nil
}
