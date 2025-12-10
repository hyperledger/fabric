/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package healthcheckers

import (
	"context"
	"testing"

	"github.com/hyperledger/fabric/core/operations/healthz"
	"github.com/hyperledger/fabric/core/peer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLedgerChecker_ReadinessCheck(t *testing.T) {
	tests := []struct {
		name        string
		peer        *peer.Peer
		expectError bool
		errorMsg    string
	}{
		{
			name:        "nil peer",
			peer:        nil,
			expectError: true,
			errorMsg:    "peer not initialized",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checker := &LedgerChecker{
				peer:      tt.peer,
				gossip:    nil,
				maxLag:    10,
				failOnLag: false,
				timeout:   5,
			}

			err := checker.ReadinessCheck(context.Background())

			if tt.expectError {
				require.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestLedgerChecker_GetStatus(t *testing.T) {
	tests := []struct {
		name           string
		peer           *peer.Peer
		expectedStatus string
	}{
		{
			name:           "nil peer",
			peer:           nil,
			expectedStatus: healthz.StatusUnavailable,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checker := &LedgerChecker{
				peer:      tt.peer,
				gossip:    nil,
				maxLag:    10,
				failOnLag: false,
				timeout:   5,
			}

			status := checker.GetStatus()
			assert.Equal(t, tt.expectedStatus, status.Status)
			assert.NotEmpty(t, status.Message)
		})
	}
}

