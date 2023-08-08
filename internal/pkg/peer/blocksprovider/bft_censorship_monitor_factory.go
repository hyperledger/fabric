/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package blocksprovider

import "github.com/hyperledger/fabric/internal/pkg/peer/orderers"

// BFTCensorshipMonitorFactory creates an instance of a BFTCensorshipMonitor. It is an implementation of the
// CensorshipDetectorFactory interface which abstracts the creation of a BFTCensorshipMonitor.
type BFTCensorshipMonitorFactory struct{}

func (f *BFTCensorshipMonitorFactory) Create(
	chainID string,
	verifier BlockVerifier,
	requester DeliverClientRequester,
	progressReporter BlockProgressReporter,
	fetchSources []*orderers.Endpoint,
	blockSourceIndex int,
	timeoutConf TimeoutConfig,
) CensorshipDetector {
	return NewBFTCensorshipMonitor(chainID, verifier, requester, progressReporter, fetchSources, blockSourceIndex, timeoutConf)
}
