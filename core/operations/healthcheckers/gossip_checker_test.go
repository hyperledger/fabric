/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package healthcheckers

import (
	"context"
	"testing"

	"github.com/hyperledger/fabric/core/operations/healthz"
	"github.com/hyperledger/fabric/gossip/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGossipChecker_ReadinessCheck(t *testing.T) {
		tests := []struct {
		name        string
		gossip      *service.GossipService
		minPeers    int
		expectError bool
		errorMsg    string
	}{
		{
			name:        "nil gossip service",
			gossip:      nil,
			minPeers:    0,
			expectError: true,
			errorMsg:    "gossip service not initialized",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checker := &GossipChecker{
				gossipService: tt.gossip,
				minPeers:      tt.minPeers,
				timeout:       5,
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

func TestGossipChecker_GetStatus(t *testing.T) {
		tests := []struct {
		name           string
		gossip         *service.GossipService
		minPeers       int
		expectedStatus string
	}{
		{
			name:           "nil gossip service",
			gossip:         nil,
			minPeers:       0,
			expectedStatus: healthz.StatusUnavailable,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checker := &GossipChecker{
				gossipService: tt.gossip,
				minPeers:      tt.minPeers,
				timeout:       5,
			}

			status := checker.GetStatus()
			assert.Equal(t, tt.expectedStatus, status.Status)
			assert.NotEmpty(t, status.Message)
		})
	}
}

