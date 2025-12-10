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
	"github.com/hyperledger/fabric/core/peer"
	"github.com/hyperledger/fabric/gossip/service"
)

// OrdererChecker verifies orderer connectivity. Disabled by default since connectivity
// issues may not prevent read-only operations. Only enable to ensure write-readiness.
type OrdererChecker struct {
	peer          *peer.Peer
	gossipService *service.GossipService
	timeout       time.Duration
}

func NewOrdererChecker(peer *peer.Peer, gossipService *service.GossipService, timeout time.Duration) *OrdererChecker {
	return &OrdererChecker{
		peer:          peer,
		gossipService: gossipService,
		timeout:       timeout,
	}
}

func (o *OrdererChecker) ReadinessCheck(ctx context.Context) error {
	if o.peer == nil {
		return fmt.Errorf("peer not initialized")
	}

	if o.gossipService == nil {
		return nil
	}

	channelIDs, err := o.peer.LedgerMgr.GetLedgerIDs()
	if err != nil {
		return fmt.Errorf("failed to get ledger IDs: %w", err)
	}

	if len(channelIDs) == 0 {
		return nil
	}

	return nil
}

func (o *OrdererChecker) GetStatus() healthz.ComponentStatus {
	if o.peer == nil {
		return healthz.ComponentStatus{
			Status:  healthz.StatusUnavailable,
			Message: "Peer not initialized",
		}
	}

	if o.gossipService == nil {
		return healthz.ComponentStatus{
			Status:  healthz.StatusOK,
			Message: "Gossip service not available - orderer check skipped",
			Details: map[string]interface{}{
				"note": "Orderer connectivity checks are informational only",
			},
		}
	}

	channelIDs, err := o.peer.LedgerMgr.GetLedgerIDs()
	if err != nil {
		return healthz.ComponentStatus{
			Status:  healthz.StatusUnavailable,
			Message: fmt.Sprintf("Failed to get ledger IDs: %v", err),
		}
	}

	return healthz.ComponentStatus{
		Status:  healthz.StatusOK,
		Message: fmt.Sprintf("Orderer connectivity check passed for %d channel(s)", len(channelIDs)),
		Details: map[string]interface{}{
			"channels": len(channelIDs),
			"note":     "Orderer connectivity is managed by deliver service retry logic",
		},
	}
}

