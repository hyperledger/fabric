/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

// Package healthz provides readiness checking functionality for the operations service.
// This package extends the healthz package from fabric-lib-go to support readiness
// checks separate from liveness checks.
package healthz

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	libhealthz "github.com/hyperledger/fabric-lib-go/healthz"
)

// ReadinessChecker defines the interface components must implement to register
// readiness checks. Readiness checks verify that dependencies are available
// and the service is ready to accept traffic.
type ReadinessChecker interface {
	ReadinessCheck(context.Context) error
}

// DetailedReadinessChecker extends ReadinessChecker to provide detailed status information.
type DetailedReadinessChecker interface {
	ReadinessChecker
	// GetStatus returns detailed status information for the component.
	GetStatus() ComponentStatus
}

// ReadinessHandler is responsible for executing registered readiness checks.
// It provides an HTTP handler which returns readiness status for all registered
// components. Readiness checks are separate from liveness checks and are used
// by Kubernetes readiness probes.
type ReadinessHandler struct {
	mutex             sync.RWMutex
	readinessCheckers map[string]ReadinessChecker
	now               func() time.Time
	timeout           time.Duration
}

// NewReadinessHandler returns a new ReadinessHandler instance.
func NewReadinessHandler() *ReadinessHandler {
	return &ReadinessHandler{
		readinessCheckers: make(map[string]ReadinessChecker),
		now:                time.Now,
		timeout:            10 * time.Second,
	}
}

// RegisterChecker registers a ReadinessChecker for a named component.
func (h *ReadinessHandler) RegisterChecker(component string, checker ReadinessChecker) error {
	h.mutex.Lock()
	defer h.mutex.Unlock()

	if _, ok := h.readinessCheckers[component]; ok {
		return AlreadyRegisteredError(component)
	}
	h.readinessCheckers[component] = checker
	return nil
}

func (h *ReadinessHandler) DeregisterChecker(component string) {
	h.mutex.Lock()
	defer h.mutex.Unlock()
	delete(h.readinessCheckers, component)
}

// SetTimeout sets the timeout for handling HTTP requests. Defaults to 10 seconds.
func (h *ReadinessHandler) SetTimeout(timeout time.Duration) {
	h.timeout = timeout
}

// ServeHTTP handles readiness checks. Returns 200 if all components are OK or DEGRADED,
// 503 only if at least one component is UNAVAILABLE. DEGRADED components are informational
// and don't block readiness.
func (h *ReadinessHandler) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	if req.Method != "GET" {
		rw.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	checksCtx, cancel := context.WithTimeout(req.Context(), h.timeout)
	defer cancel()

	unavailableChecksCh := make(chan []libhealthz.FailedCheck)
	go func() {
		unavailableChecksCh <- h.RunChecksForReadiness(checksCtx)
	}()

	select {
	case unavailableChecks := <-unavailableChecksCh:
		hs := libhealthz.HealthStatus{
			Status: libhealthz.StatusOK,
			Time:   h.now(),
		}
		if len(unavailableChecks) > 0 {
			hs.Status = libhealthz.StatusUnavailable
			hs.FailedChecks = unavailableChecks
		}
		writeHTTPResponse(rw, hs)
	case <-checksCtx.Done():
		if checksCtx.Err() == context.DeadlineExceeded {
			rw.WriteHeader(http.StatusRequestTimeout)
		}
	}
}

// RunChecks runs all readiness checkers and returns any failures.
// Used for backward compatibility.
func (h *ReadinessHandler) RunChecks(ctx context.Context) []libhealthz.FailedCheck {
	h.mutex.RLock()
	defer h.mutex.RUnlock()

	var failedChecks []libhealthz.FailedCheck
	for component, checker := range h.readinessCheckers {
		if err := checker.ReadinessCheck(ctx); err != nil {
			failedCheck := libhealthz.FailedCheck{
				Component: component,
				Reason:    err.Error(),
			}
			failedChecks = append(failedChecks, failedCheck)
		}
	}
	return failedChecks
}

// RunChecksForReadiness returns only UNAVAILABLE failures. DEGRADED components
// don't cause readiness to fail, ensuring /readyz returns 503 only when truly unavailable.
func (h *ReadinessHandler) RunChecksForReadiness(ctx context.Context) []libhealthz.FailedCheck {
	h.mutex.RLock()
	defer h.mutex.RUnlock()

	var unavailableChecks []libhealthz.FailedCheck
	for component, checker := range h.readinessCheckers {
		err := checker.ReadinessCheck(ctx)
		
		var isUnavailable bool
		if detailedChecker, ok := checker.(DetailedReadinessChecker); ok {
			status := detailedChecker.GetStatus()
			isUnavailable = (status.Status == StatusUnavailable)
		} else {
			isUnavailable = (err != nil)
		}
		
		if isUnavailable {
			reason := "component unavailable"
			if err != nil {
				reason = err.Error()
			}
			unavailableChecks = append(unavailableChecks, libhealthz.FailedCheck{
				Component: component,
				Reason:    reason,
			})
		}
	}
	return unavailableChecks
}

// GetDetailedStatus returns detailed status for all components, including DEGRADED.
// DEGRADED is informational only and doesn't cause /readyz to fail.
func (h *ReadinessHandler) GetDetailedStatus(ctx context.Context) DetailedHealthStatus {
	h.mutex.RLock()
	defer h.mutex.RUnlock()

	components := make(map[string]ComponentStatus)
	var failedChecks []FailedCheck
	overallStatus := StatusOK

	for component, checker := range h.readinessCheckers {
		err := checker.ReadinessCheck(ctx)
		
		var status ComponentStatus
		if detailedChecker, ok := checker.(DetailedReadinessChecker); ok {
			status = detailedChecker.GetStatus()
			if err != nil {
				status.Status = StatusUnavailable
				status.Message = err.Error()
			}
		} else {
			if err != nil {
				status = ComponentStatus{
					Status:  StatusUnavailable,
					Message: err.Error(),
				}
				overallStatus = StatusUnavailable
				failedChecks = append(failedChecks, FailedCheck{
					Component: component,
					Reason:    err.Error(),
				})
			} else {
				status = ComponentStatus{
					Status:  StatusOK,
					Message: "Component is healthy",
				}
			}
		}
		
		if status.Status == StatusUnavailable {
			overallStatus = StatusUnavailable
		} else if status.Status == StatusDegraded && overallStatus == StatusOK {
			overallStatus = StatusDegraded
		}
		
		components[component] = status
	}

	return DetailedHealthStatus{
		Status:       overallStatus,
		Time:         h.now(),
		Components:   components,
		FailedChecks: failedChecks,
	}
}

func writeHTTPResponse(rw http.ResponseWriter, hs libhealthz.HealthStatus) {
	var resp []byte
	rc := http.StatusOK
	rw.Header().Set("Content-Type", "application/json")
	if len(hs.FailedChecks) > 0 {
		rc = http.StatusServiceUnavailable
	}
	resp, err := json.Marshal(hs)
	if err != nil {
		rc = http.StatusInternalServerError
	}
	rw.WriteHeader(rc)
	rw.Write(resp)
}

// AlreadyRegisteredError indicates that a component has already been registered.
type AlreadyRegisteredError string

func (are AlreadyRegisteredError) Error() string {
	return string(are) + " is already registered"
}

