/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package healthz

import (
	"context"
	"testing"
	"time"

	libhealthz "github.com/hyperledger/fabric-lib-go/healthz"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockChecker struct {
	err error
}

func (m *mockChecker) ReadinessCheck(ctx context.Context) error {
	return m.err
}

type mockDetailedChecker struct {
	err     error
	status  ComponentStatus
}

func (m *mockDetailedChecker) ReadinessCheck(ctx context.Context) error {
	return m.err
}

func (m *mockDetailedChecker) GetStatus() ComponentStatus {
	return m.status
}

func TestReadinessHandler_RegisterChecker(t *testing.T) {
	handler := NewReadinessHandler()

	checker := &mockChecker{}
	err := handler.RegisterChecker("test", checker)
	require.NoError(t, err)

	// Try to register again - should fail
	err = handler.RegisterChecker("test", checker)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already registered")
}

func TestReadinessHandler_DeregisterChecker(t *testing.T) {
	handler := NewReadinessHandler()

	checker := &mockChecker{}
	handler.RegisterChecker("test", checker)
	handler.DeregisterChecker("test")

	// Should be able to register again
	err := handler.RegisterChecker("test", checker)
	require.NoError(t, err)
}

func TestReadinessHandler_RunChecks(t *testing.T) {
	handler := NewReadinessHandler()

	// Add passing checker
	handler.RegisterChecker("pass", &mockChecker{err: nil})
	
	// Add failing checker
	handler.RegisterChecker("fail", &mockChecker{err: libhealthz.AlreadyRegisteredError("test error")})

	failedChecks := handler.RunChecks(context.Background())
	assert.Len(t, failedChecks, 1)
	assert.Equal(t, "fail", failedChecks[0].Component)
}

func TestReadinessHandler_GetDetailedStatus(t *testing.T) {
	handler := NewReadinessHandler()

	// Add detailed checker
	handler.RegisterChecker("detailed", &mockDetailedChecker{
		err: nil,
		status: ComponentStatus{
			Status:  StatusOK,
			Message: "All good",
		},
	})

	// Add simple checker
	handler.RegisterChecker("simple", &mockChecker{err: nil})

	status := handler.GetDetailedStatus(context.Background())
	assert.Equal(t, StatusOK, status.Status)
	assert.Len(t, status.Components, 2)
	assert.Equal(t, StatusOK, status.Components["detailed"].Status)
	assert.Equal(t, StatusOK, status.Components["simple"].Status)
}

func TestReadinessHandler_SetTimeout(t *testing.T) {
	handler := NewReadinessHandler()
	handler.SetTimeout(30 * time.Second)
	assert.Equal(t, 30*time.Second, handler.timeout)
}

