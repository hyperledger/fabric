/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode

import (
	"sync"

	"github.com/hyperledger/fabric/core/ledger"
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
		handlers:                     map[string]*Handler{},
		launching:                    map[string]*LaunchState{},
		allowUnsolicitedRegistration: allowUnsolicitedRegistration,
	}
}

// Launching indicates that chaincode is being launched. The LaunchState that
// is returned provides mechanisms to determine when the operation has
// completed and whether or not it failed. The bool indicates whether or not
// the chaincode has already been started.
func (r *HandlerRegistry) Launching(ccid string) (*LaunchState, bool) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	// launch happened or already happening
	if launchState, ok := r.launching[ccid]; ok {
		return launchState, true
	}

	// handler registered without going through launch
	if _, ok := r.handlers[ccid]; ok {
		launchState := NewLaunchState()
		launchState.Notify(nil)
		return launchState, true
	}

	// first attempt to launch so the runtime needs to start
	launchState := NewLaunchState()
	r.launching[ccid] = launchState
	return launchState, false
}

// Ready indicates that the chaincode registration has completed and the
// READY response has been sent to the chaincode.
func (r *HandlerRegistry) Ready(ccid string) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	launchStatus := r.launching[ccid]
	if launchStatus != nil {
		launchStatus.Notify(nil)
	}
}

// Failed indicates that registration of a launched chaincode has failed.
func (r *HandlerRegistry) Failed(ccid string, err error) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	launchStatus := r.launching[ccid]
	if launchStatus != nil {
		launchStatus.Notify(err)
	}
}

// Handler retrieves the handler for a chaincode instance.
func (r *HandlerRegistry) Handler(ccid string) *Handler {
	r.mutex.Lock()
	h := r.handlers[ccid]
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

	if r.handlers[h.chaincodeID] != nil {
		chaincodeLogger.Debugf("duplicate registered handler(key:%s) return error", h.chaincodeID)
		return errors.Errorf("duplicate chaincodeID: %s", h.chaincodeID)
	}

	// This chaincode was not launched by the peer but is attempting
	// to register. Only allowed in development mode.
	if r.launching[h.chaincodeID] == nil && !r.allowUnsolicitedRegistration {
		return errors.Errorf("peer will not accept external chaincode connection %s (except in dev mode)", h.chaincodeID)
	}

	r.handlers[h.chaincodeID] = h

	chaincodeLogger.Debugf("registered handler complete for chaincode %s", h.chaincodeID)
	return nil
}

// Deregister clears references to state associated specified chaincode.
// As part of the cleanup, it closes the handler so it can cleanup any state.
// If the registry does not contain the provided handler, an error is returned.
func (r *HandlerRegistry) Deregister(ccid string) error {
	chaincodeLogger.Debugf("deregister handler: %s", ccid)

	r.mutex.Lock()
	handler := r.handlers[ccid]
	delete(r.handlers, ccid)
	delete(r.launching, ccid)
	r.mutex.Unlock()

	if handler == nil {
		return errors.Errorf("could not find handler: %s", ccid)
	}

	handler.Close()

	chaincodeLogger.Debugf("deregistered handler with key: %s", ccid)
	return nil
}

type TxQueryExecutorGetter struct {
	HandlerRegistry *HandlerRegistry
	CCID            string
}

func (g *TxQueryExecutorGetter) TxQueryExecutor(chainID, txID string) ledger.SimpleQueryExecutor {
	handler := g.HandlerRegistry.Handler(g.CCID)
	if handler == nil {
		return nil
	}
	txContext := handler.TXContexts.Get(chainID, txID)
	if txContext == nil {
		return nil
	}
	return txContext.TXSimulator
}
