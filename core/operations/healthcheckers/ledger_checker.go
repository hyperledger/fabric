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
	"github.com/hyperledger/fabric/gossip/common"
	"github.com/hyperledger/fabric/gossip/service"
)

// LedgerChecker verifies ledgers are initialized and readable. Lag checking is
// informational by default and doesn't fail readiness unless failOnLag is enabled.
type LedgerChecker struct {
	peer      *peer.Peer
	gossip    *service.GossipService
	maxLag    uint64
	failOnLag bool
	timeout   time.Duration
}

// NewLedgerChecker creates a new LedgerChecker. If failOnLag is true, also checks
// that ledgers aren't lagging behind gossip-advertised heights by more than maxLag.
func NewLedgerChecker(peer *peer.Peer, gossip *service.GossipService, maxLag uint64, failOnLag bool, timeout time.Duration) *LedgerChecker {
	return &LedgerChecker{
		peer:      peer,
		gossip:    gossip,
		maxLag:    maxLag,
		failOnLag: failOnLag,
		timeout:   timeout,
	}
}

func (l *LedgerChecker) ReadinessCheck(ctx context.Context) error {
	if l.peer == nil {
		return fmt.Errorf("peer not initialized")
	}

	if l.peer.LedgerMgr == nil {
		return fmt.Errorf("ledger manager not initialized")
	}

	channelIDs, err := l.peer.LedgerMgr.GetLedgerIDs()
	if err != nil {
		return fmt.Errorf("failed to get ledger IDs: %w", err)
	}

	for _, channelID := range channelIDs {
		ledger := l.peer.GetLedger(channelID)
		if ledger == nil {
			return fmt.Errorf("ledger for channel %s is nil", channelID)
		}

		blockchainInfo, err := ledger.GetBlockchainInfo()
		if err != nil {
			return fmt.Errorf("failed to read blockchain info for channel %s: %w", channelID, err)
		}

		if blockchainInfo == nil {
			return fmt.Errorf("blockchain info is nil for channel %s", channelID)
		}

		qe, err := ledger.NewQueryExecutor()
		if err != nil {
			return fmt.Errorf("failed to create query executor for channel %s: %w", channelID, err)
		}
		qe.Done()

		if l.failOnLag && l.gossip != nil {
			localHeight := blockchainInfo.Height
			maxHeight := l.getMaxAdvertisedHeight(channelID)
			if maxHeight > localHeight {
				lag := maxHeight - localHeight
				if lag > l.maxLag {
					return fmt.Errorf("channel %s is lagging: %d blocks behind (max allowed: %d)", 
						channelID, lag, l.maxLag)
				}
			}
		}
	}

	return nil
}

func (l *LedgerChecker) getMaxAdvertisedHeight(channelID string) uint64 {
	if l.gossip == nil {
		return 0
	}

	peers := l.gossip.PeersOfChannel(common.ChannelID(channelID))
	maxHeight := uint64(0)
	
	for _, peer := range peers {
		if peer.Properties != nil && peer.Properties.LedgerHeight > maxHeight {
			maxHeight = peer.Properties.LedgerHeight
		}
	}
	
	return maxHeight
}

func (l *LedgerChecker) GetStatus() healthz.ComponentStatus {
	if l.peer == nil {
		return healthz.ComponentStatus{
			Status:  healthz.StatusUnavailable,
			Message: "Peer not initialized",
		}
	}

	if l.peer.LedgerMgr == nil {
		return healthz.ComponentStatus{
			Status:  healthz.StatusUnavailable,
			Message: "Ledger manager not initialized",
		}
	}

	channelIDs, err := l.peer.LedgerMgr.GetLedgerIDs()
	if err != nil {
		return healthz.ComponentStatus{
			Status:  healthz.StatusUnavailable,
			Message: fmt.Sprintf("Failed to get ledger IDs: %v", err),
		}
	}

	channelDetails := make(map[string]interface{})
	allOK := true
	hasLag := false

	for _, channelID := range channelIDs {
		ledger := l.peer.GetLedger(channelID)
		if ledger == nil {
			channelDetails[channelID] = map[string]interface{}{
				"status": "unavailable",
				"error":  "ledger is nil",
			}
			allOK = false
			continue
		}

		blockchainInfo, err := ledger.GetBlockchainInfo()
		if err != nil {
			channelDetails[channelID] = map[string]interface{}{
				"status": "unavailable",
				"error":  err.Error(),
			}
			allOK = false
			continue
		}

		channelInfo := map[string]interface{}{
			"height": blockchainInfo.Height,
			"status": "ok",
		}

		if l.failOnLag && l.gossip != nil {
			localHeight := blockchainInfo.Height
			maxHeight := l.getMaxAdvertisedHeight(channelID)
			if maxHeight > localHeight {
				lag := maxHeight - localHeight
				channelInfo["lag"] = lag
				channelInfo["max_advertised_height"] = maxHeight
				if lag > l.maxLag {
					channelInfo["status"] = "lagging"
					hasLag = true
				}
			}
		}

		channelDetails[channelID] = channelInfo
	}

	status := healthz.StatusOK
	message := fmt.Sprintf("All %d channel(s) accessible", len(channelIDs))

	if !allOK {
		status = healthz.StatusUnavailable
		message = "One or more channels are unavailable"
	} else if hasLag {
		status = healthz.StatusDegraded
		message = "One or more channels are lagging"
	}

	details := map[string]interface{}{
		"channels": channelDetails,
		"total":    len(channelIDs),
	}

	if l.failOnLag {
		details["lag_checking_enabled"] = true
		details["max_lag"] = l.maxLag
	}

	return healthz.ComponentStatus{
		Status:  status,
		Message: message,
		Details: details,
	}
}

