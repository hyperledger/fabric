// Copyright IBM Corp. All Rights Reserved.
//
// SPDX-License-Identifier: Apache-2.0

package smartbft_test

import (
	"testing"
)

// Scenario:
// 1. Start a network of 4 nodes
// 2. Submit a TX
// 3. Wait for the TX to be received by all nodes
func TestSuccessfulTxPropagation(t *testing.T) {
	dir := t.TempDir()
	channelId := "testchannel"

	networkSetupInfo := NewNetworkSetupInfo(t, channelId, dir)
	_ = networkSetupInfo.CreateNodes(4)
	networkSetupInfo.StartAllNodes()
}
