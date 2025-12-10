/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package healthcheckers

import (
	"context"
	"fmt"
	"time"

	"github.com/hyperledger/fabric/core/operations/healthz"
	"github.com/hyperledger/fabric/gossip/service"
)

// GossipChecker verifies gossip service connectivity. When minPeers = 0 (default),
// only checks that gossip is initialized, avoiding false negatives in dev setups.
// When minPeers > 0, enforces minimum peer count. DEGRADED at minimum is informational.
type GossipChecker struct {
	gossipService *service.GossipService
	minPeers      int
	timeout       time.Duration
}

// NewGossipChecker creates a new GossipChecker. If minPeers is 0, only verifies
// gossip is initialized.
func NewGossipChecker(gossipService *service.GossipService, minPeers int, timeout time.Duration) *GossipChecker {
	return &GossipChecker{
		gossipService: gossipService,
		minPeers:      minPeers,
		timeout:       timeout,
	}
}

func (g *GossipChecker) ReadinessCheck(ctx context.Context) error {
	if g.gossipService == nil {
		return fmt.Errorf("gossip service not initialized")
	}

	peers := g.gossipService.Peers()
	connectedCount := len(peers)

	if g.minPeers > 0 && connectedCount < g.minPeers {
		return fmt.Errorf("insufficient peers connected: %d (minimum: %d)", connectedCount, g.minPeers)
	}

	return nil
}

func (g *GossipChecker) GetStatus() healthz.ComponentStatus {
	if g.gossipService == nil {
		return healthz.ComponentStatus{
			Status:  healthz.StatusUnavailable,
			Message: "Gossip service not initialized",
		}
	}

	peers := g.gossipService.Peers()
	connectedCount := len(peers)
	
	status := healthz.StatusOK
	message := fmt.Sprintf("Connected to %d peers", connectedCount)
	
	if g.minPeers > 0 && connectedCount < g.minPeers {
		status = healthz.StatusUnavailable
		message = fmt.Sprintf("Insufficient peers: %d (minimum: %d)", connectedCount, g.minPeers)
	} else if g.minPeers > 0 && connectedCount == g.minPeers {
		status = healthz.StatusDegraded
		message = fmt.Sprintf("Connected to minimum required peers: %d", connectedCount)
	}

	details := map[string]interface{}{
		"connected_peers": connectedCount,
		"min_peers":       g.minPeers,
	}

	return healthz.ComponentStatus{
		Status:  status,
		Message: message,
		Details: details,
	}
}

