/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package operations

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	libhealthz "github.com/hyperledger/fabric-lib-go/healthz"
	"github.com/hyperledger/fabric/core/operations/healthz"
)

// DetailedHealthHandler handles /healthz/detailed. Access should be restricted
// via TLS/client authentication.
type DetailedHealthHandler struct {
	readinessHandler *healthz.ReadinessHandler
	healthHandler    HealthHandler
	timeout          time.Duration
}

type HealthHandler interface {
	RunChecks(context.Context) []libhealthz.FailedCheck
}

func NewDetailedHealthHandler(readinessHandler *healthz.ReadinessHandler, healthHandler HealthHandler, timeout time.Duration) *DetailedHealthHandler {
	return &DetailedHealthHandler{
		readinessHandler: readinessHandler,
		healthHandler:    healthHandler,
		timeout:          timeout,
	}
}

func (h *DetailedHealthHandler) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	if req.Method != "GET" {
		rw.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	ctx, cancel := context.WithTimeout(req.Context(), h.timeout)
	defer cancel()

	detailedStatus := h.readinessHandler.GetDetailedStatus(ctx)

	livenessChecks := h.healthHandler.RunChecks(ctx)
	if len(livenessChecks) > 0 {
		for _, check := range livenessChecks {
			detailedStatus.FailedChecks = append(detailedStatus.FailedChecks, healthz.FailedCheck{
				Component: check.Component,
				Reason:    check.Reason,
			})
			if _, exists := detailedStatus.Components[check.Component]; !exists {
				detailedStatus.Components[check.Component] = healthz.ComponentStatus{
					Status:  healthz.StatusUnavailable,
					Message: check.Reason,
				}
			}
		}
		if detailedStatus.Status == healthz.StatusOK {
			detailedStatus.Status = healthz.StatusUnavailable
		}
	}

	rw.Header().Set("Content-Type", "application/json")
	
	var statusCode int
	switch detailedStatus.Status {
	case healthz.StatusOK:
		statusCode = http.StatusOK
	case healthz.StatusDegraded:
		statusCode = http.StatusOK
	default:
		statusCode = http.StatusServiceUnavailable
	}

	resp, err := json.Marshal(detailedStatus)
	if err != nil {
		rw.WriteHeader(http.StatusInternalServerError)
		return
	}

	rw.WriteHeader(statusCode)
	rw.Write(resp)
}

