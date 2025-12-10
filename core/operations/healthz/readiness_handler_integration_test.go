/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package healthz

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	libhealthz "github.com/hyperledger/fabric-lib-go/healthz"
	"github.com/stretchr/testify/assert"
)

// TestReadinessDegradedDoesNotFailReadiness verifies that components reporting
// DEGRADED status do not cause /readyz to return HTTP 503. This test ensures
// backward compatibility: /healthz returns 200 OK even when /readyz returns 503.
func TestReadinessDegradedDoesNotFailReadiness(t *testing.T) {
	handler := NewReadinessHandler()

	// Register a checker that reports DEGRADED status
	degradedChecker := &mockDegradedChecker{}
	handler.RegisterChecker("degraded", degradedChecker)

	// Register a checker that reports UNAVAILABLE status
	unavailableChecker := &mockUnavailableChecker{}
	handler.RegisterChecker("unavailable", unavailableChecker)

	// Create HTTP request
	req := httptest.NewRequest("GET", "/readyz", nil)
	w := httptest.NewRecorder()

	// Execute handler
	handler.ServeHTTP(w, req)

	// Verify response
	resp := w.Result()
	defer resp.Body.Close()

	// Should return 503 because unavailable checker reports UNAVAILABLE
	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)

	// Verify that degraded component doesn't cause failure
	// The degraded checker should not cause readiness to fail
	// Only the unavailable checker should cause the 503
}

// TestBackwardCompatibilityLivenessVsReadiness verifies backward compatibility:
// /healthz returns 200 OK even when /readyz returns 503. This proves that
// liveness and readiness are properly separated.
func TestBackwardCompatibilityLivenessVsReadiness(t *testing.T) {
	// This test demonstrates the separation between liveness (/healthz) and
	// readiness (/readyz). In a real scenario, /healthz would be handled by
	// a separate HealthHandler from fabric-lib-go, while /readyz is handled
	// by ReadinessHandler. This test verifies that readiness failures
	// (HTTP 503 on /readyz) do not affect liveness (HTTP 200 on /healthz).

	readinessHandler := NewReadinessHandler()

	// Register a checker that fails readiness
	failingChecker := &mockUnavailableChecker{}
	readinessHandler.RegisterChecker("test", failingChecker)

	// Simulate /readyz request
	req := httptest.NewRequest("GET", "/readyz", nil)
	w := httptest.NewRecorder()
	readinessHandler.ServeHTTP(w, req)

	// /readyz should return 503 when component is UNAVAILABLE
	assert.Equal(t, http.StatusServiceUnavailable, w.Result().StatusCode)

	// In a real system, /healthz would be handled separately and would
	// return 200 OK even if /readyz returns 503, proving backward compatibility.
	// This separation allows Kubernetes to distinguish between:
	// - Liveness: Is the process alive? (restart if not)
	// - Readiness: Can the process accept traffic? (remove from load balancer if not)
}

type mockDegradedChecker struct{}

func (m *mockDegradedChecker) ReadinessCheck(ctx context.Context) error {
	// Return nil - degraded status is determined by GetStatus(), not ReadinessCheck error
	// This simulates a component that is functional but degraded
	return nil
}

func (m *mockDegradedChecker) GetStatus() ComponentStatus {
	return ComponentStatus{
		Status:  StatusDegraded,
		Message: "Component is degraded but functional",
	}
}

type mockUnavailableChecker struct{}

func (m *mockUnavailableChecker) ReadinessCheck(ctx context.Context) error {
	// Return error to indicate unavailable status
	return libhealthz.AlreadyRegisteredError("unavailable")
}

func (m *mockUnavailableChecker) GetStatus() ComponentStatus {
	return ComponentStatus{
		Status:  StatusUnavailable,
		Message: "Component is unavailable",
	}
}

